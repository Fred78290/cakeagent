package seamless

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
	"unicode/utf8"

	glog "github.com/sirupsen/logrus"
)

// The hub lets every guest user have a compositor of their own while the host sees one connection.
//
// The host listens for a single compositor per VM: a new connection replaces the previous one. Several users means
// several compositors, so instead of changing the host (and the gRPC tunnel a remote host is reached through), the
// agent sits in between:
//
//	caker-seamlessd (alice) --\
//	caker-seamlessd (bob)   ---+--> hub (this file) --> one vsock connection --> the host
//
// Each compositor connects to the hub's unix socket, as the user it runs as (the kernel says which, SO_PEERCRED).
// The hub does the hello exchange with the host once, answers the compositors' own hellos, and tells their windows
// apart by putting the compositor's slot in the top byte of every window id (the compositors count from 1, so
// 24 bits are plenty). Input the host sends back is routed by that same byte; the keyboard goes to the compositor
// that owns the focused window. When a compositor goes away the hub destroys its windows on the host, so one
// user's windows can disappear without the others'.
//
// Nothing in the wire format changes: a host and a compositor that know nothing about the hub keep working.

const (
	slotShift   = 24
	localIDMask = 1<<slotShift - 1
	// maxSlot is the number of compositors the hub serves at once.
	maxSlot = 255

	// maxGuestFrame mirrors SeamlessProtocol.maxFrameLength.
	maxGuestFrame = 192 * 1024 * 1024
	// maxHostFrame mirrors SEAMLESS_MAX_HOST_FRAME: host messages are tiny.
	maxHostFrame = 64 * 1024
	// maxString mirrors SeamlessProtocol.maxStringLength.
	maxString = 4096

	hubHandshakeTimeout = 10 * time.Second
)

// Guest -> host message types.
const (
	gHello         = 0x01
	gWindowCreate  = 0x02
	gWindowTitle   = 0x03
	gWindowAppID   = 0x04
	gWindowHints   = 0x05
	gWindowFrame   = 0x06
	gWindowDestroy = 0x07
	gCursor        = 0x08
)

// Host -> guest message types.
const (
	hHelloAck      = 0x81
	hOutput        = 0x82
	hWindowResize  = 0x83
	hWindowClose   = 0x84
	hWindowFocus   = 0x85
	hPointerEnter  = 0x86
	hPointerLeave  = 0x87
	hPointerMotion = 0x88
	hPointerButton = 0x89
	hPointerAxis   = 0x8A
	hKey           = 0x8B
)

var errShortMessage = errors.New("short message")

// readFrame reads one `u32 length` + type + payload frame, prefix included, so that a message can be patched and
// forwarded without copying it. limit bounds the length field.
func readFrame(r io.Reader, limit uint32) ([]byte, error) {
	var header [4]byte

	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}

	n := binary.LittleEndian.Uint32(header[:])

	if n == 0 || n > limit {
		return nil, fmt.Errorf("frame of %d bytes", n)
	}

	frame := make([]byte, 4+int(n))

	copy(frame, header[:])

	if _, err := io.ReadFull(r, frame[4:]); err != nil {
		return nil, err
	}

	return frame, nil
}

// buildFrame makes a complete frame.
func buildFrame(typ byte, payload []byte) []byte {
	frame := make([]byte, 5+len(payload))

	binary.LittleEndian.PutUint32(frame, uint32(1+len(payload)))
	frame[4] = typ
	copy(frame[5:], payload)

	return frame
}

func globalID(slot, local uint32) uint32 {
	return slot<<slotShift | local
}

// parseHello decodes `u16 version` + string name.
func parseHello(frame []byte) (version uint16, name string, err error) {
	if len(frame) < 5 || frame[4] != gHello {
		return 0, "", errors.New("the first message must be hello")
	}

	p := frame[5:]

	if len(p) < 4 {
		return 0, "", errShortMessage
	}

	version = binary.LittleEndian.Uint16(p)
	n := int(binary.LittleEndian.Uint16(p[2:]))

	if 4+n != len(p) {
		return 0, "", errors.New("malformed hello")
	}

	return version, string(p[4:]), nil
}

func buildHello(version uint16, name string) []byte {
	if len(name) > maxString {
		name = name[:maxString]
	}

	payload := binary.LittleEndian.AppendUint16(nil, version)

	payload = binary.LittleEndian.AppendUint16(payload, uint16(len(name)))
	payload = append(payload, name...)

	return buildFrame(gHello, payload)
}

func buildHelloAck(version uint16, scale uint32) []byte {
	payload := binary.LittleEndian.AppendUint16(nil, version)

	return buildFrame(hHelloAck, binary.LittleEndian.AppendUint32(payload, scale))
}

func parseHelloAck(frame []byte) (version uint16, scale uint32, err error) {
	if len(frame) != 4+1+6 || frame[4] != hHelloAck {
		return 0, 0, errors.New("expected the host's hello acknowledgement")
	}

	return binary.LittleEndian.Uint16(frame[5:]), binary.LittleEndian.Uint32(frame[7:]), nil
}

// withTitlePrefix returns the frame with prefix put before the title string that starts at payload offset off. A
// title that would not fit (the protocol allows maxString bytes) is cut on a character boundary.
func withTitlePrefix(frame []byte, off int, prefix string) ([]byte, error) {
	p := frame[5:]

	if off+2 > len(p) {
		return nil, errShortMessage
	}

	n := int(binary.LittleEndian.Uint16(p[off:]))
	end := off + 2 + n

	if end > len(p) {
		return nil, errShortMessage
	}

	title := p[off+2 : end]

	if room := maxString - len(prefix); len(title) > room {
		for room > 0 && !utf8.RuneStart(title[room]) {
			room--
		}

		title = title[:room]
	}

	out := make([]byte, 5, len(frame)+len(prefix))

	copy(out, frame[:5])

	out = append(out, p[:off]...)
	out = binary.LittleEndian.AppendUint16(out, uint16(len(prefix)+len(title)))
	out = append(out, prefix...)
	out = append(out, title...)
	out = append(out, p[end:]...)

	binary.LittleEndian.PutUint32(out, uint32(len(out)-4))

	return out, nil
}

// Hub multiplexes the compositors of several guest users onto one connection to the host.
type Hub struct {
	// Dial opens the connection to the host.
	Dial func() (net.Conn, error)
	// Authenticate says which guest account is behind a compositor connection.
	Authenticate func(net.Conn) (Account, error)

	// linkMu serializes what decides whether the host connection exists: making it, and dropping it when the last
	// compositor is gone.
	linkMu sync.Mutex
	mu     sync.Mutex
	peers  [maxSlot + 1]*hubPeer
	link   *hubLink
	// primary is the account whose window titles stay as they are; the others get their user name in front.
	primary string
	// focus is the slot of the compositor that owns the focused window, where keys go.
	focus uint32
}

// hubLink is the connection to the host.
type hubLink struct {
	conn    net.Conn
	reader  *bufio.Reader
	wmu     sync.Mutex
	version uint16
	mu      sync.Mutex
	scale   uint32
}

func (l *hubLink) write(frame []byte) error {
	l.wmu.Lock()
	defer l.wmu.Unlock()

	_, err := l.conn.Write(frame)

	return err
}

func (l *hubLink) currentScale() uint32 {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.scale
}

func (l *hubLink) setScale(scale uint32) {
	l.mu.Lock()
	l.scale = scale
	l.mu.Unlock()
}

// hubPeer is one compositor.
type hubPeer struct {
	slot    uint32
	account Account
	conn    net.Conn
	link    *hubLink
	wmu     sync.Mutex
	// windows are the compositor's own window ids still alive. Only the peer's reader goroutine touches it.
	windows map[uint32]struct{}
}

func (p *hubPeer) write(frame []byte) {
	p.wmu.Lock()
	_, err := p.conn.Write(frame)
	p.wmu.Unlock()

	if err != nil {
		// The reader notices and cleans up.
		p.conn.Close()
	}
}

// SetPrimary names the account whose window titles are left alone.
func (h *Hub) SetPrimary(name string) {
	h.mu.Lock()
	h.primary = name
	h.mu.Unlock()
}

func (h *Hub) titlePrefix(a Account) string {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.primary == "" || a.Name == h.primary {
		return ""
	}

	return a.Name + ": "
}

// Serve accepts compositors on l until ctx is done.
func (h *Hub) Serve(ctx context.Context, l net.Listener) error {
	go func() {
		<-ctx.Done()
		l.Close()
	}()

	defer h.closeAll()

	for {
		conn, err := l.Accept()

		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			return err
		}

		go h.handle(conn)
	}
}

func (h *Hub) closeAll() {
	h.mu.Lock()

	link := h.link
	h.link = nil

	var peers []*hubPeer

	for _, p := range h.peers {
		if p != nil {
			peers = append(peers, p)
		}
	}

	h.mu.Unlock()

	if link != nil {
		link.conn.Close()
	}

	for _, p := range peers {
		p.conn.Close()
	}
}

// handle serves one compositor.
func (h *Hub) handle(conn net.Conn) {
	defer conn.Close()

	account, err := h.Authenticate(conn)

	if err != nil {
		glog.Warnf("seamless hub: refusing a connection: %v", err)

		return
	}

	reader := bufio.NewReaderSize(conn, 64*1024)

	_ = conn.SetReadDeadline(time.Now().Add(hubHandshakeTimeout))

	first, err := readFrame(reader, maxString+16)

	if err == nil {
		var version uint16
		var name string

		if version, name, err = parseHello(first); err == nil {
			_ = conn.SetReadDeadline(time.Time{})

			err = h.serve(conn, reader, account, version, name)
		}
	}

	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
		glog.Warnf("seamless hub: compositor of %s: %v", account.Name, err)
	}
}

func (h *Hub) serve(conn net.Conn, reader *bufio.Reader, account Account, version uint16, name string) error {
	peer, err := h.attach(conn, account, version, name)

	if err != nil {
		return err
	}

	defer h.drop(peer)

	glog.Infof("seamless hub: compositor of %s connected (slot %d)", account.Name, peer.slot)

	prefix := h.titlePrefix(account)

	for {
		frame, err := readFrame(reader, maxGuestFrame)

		if err != nil {
			return err
		}

		if frame, err = peer.rewrite(frame, prefix); err != nil {
			return err
		}

		if err = peer.link.write(frame); err != nil {
			return fmt.Errorf("host connection: %w", err)
		}
	}
}

// attach gives the compositor a slot, opening the connection to the host if it is the first one.
func (h *Hub) attach(conn net.Conn, account Account, version uint16, name string) (*hubPeer, error) {
	h.linkMu.Lock()
	defer h.linkMu.Unlock()

	link, err := h.ensureLink(version, name)

	if err != nil {
		return nil, err
	}

	if link.version != version {
		return nil, fmt.Errorf("protocol version %d, the host connection speaks %d", version, link.version)
	}

	h.mu.Lock()

	// A compositor that restarted may be seen before its previous connection is noticed to be gone: it takes over.
	for _, p := range h.peers {
		if p != nil && p.account.UID == account.UID {
			p.conn.Close()
		}
	}

	var peer *hubPeer

	for slot := uint32(1); slot <= maxSlot; slot++ {
		if h.peers[slot] == nil {
			peer = &hubPeer{slot: slot, account: account, conn: conn, link: link, windows: map[uint32]struct{}{}}
			h.peers[slot] = peer

			break
		}
	}

	h.mu.Unlock()

	if peer == nil {
		return nil, errors.New("too many compositors")
	}

	peer.write(buildHelloAck(link.version, link.currentScale()))

	return peer, nil
}

// ensureLink returns the host connection, making it (hello and acknowledgement included) when there is none. It
// is called with linkMu held.
func (h *Hub) ensureLink(version uint16, name string) (*hubLink, error) {
	h.mu.Lock()
	link := h.link
	h.mu.Unlock()

	if link != nil {
		return link, nil
	}

	conn, err := h.Dial()

	if err != nil {
		return nil, fmt.Errorf("cannot reach the host: %w", err)
	}

	_ = conn.SetDeadline(time.Now().Add(hubHandshakeTimeout))

	link = &hubLink{conn: conn, reader: bufio.NewReaderSize(conn, 64*1024)}

	if _, err = conn.Write(buildHello(version, name)); err != nil {
		conn.Close()

		return nil, err
	}

	ack, err := readFrame(link.reader, maxHostFrame)

	if err != nil {
		conn.Close()

		return nil, fmt.Errorf("host hello: %w", err)
	}

	if link.version, link.scale, err = parseHelloAck(ack); err != nil {
		conn.Close()

		return nil, err
	}

	_ = conn.SetDeadline(time.Time{})

	h.mu.Lock()
	h.link = link
	h.mu.Unlock()

	go h.readLink(link)

	glog.Infof("seamless hub: connected to the host (protocol %d)", link.version)

	return link, nil
}

// drop forgets a compositor: its windows go from the host, and the connection to the host with the last one.
func (h *Hub) drop(peer *hubPeer) {
	h.linkMu.Lock()
	defer h.linkMu.Unlock()

	h.mu.Lock()

	if h.peers[peer.slot] == peer {
		h.peers[peer.slot] = nil
	}

	if h.focus == peer.slot {
		h.focus = 0
	}

	link := h.link
	remaining := 0

	for _, p := range h.peers {
		if p != nil {
			remaining++
		}
	}

	h.mu.Unlock()

	peer.conn.Close()

	// A window id of a previous connection means nothing to a new host session (the slot may even be in use again).
	if link == nil || link != peer.link {
		return
	}

	for id := range peer.windows {
		_ = link.write(buildFrame(gWindowDestroy, binary.LittleEndian.AppendUint32(nil, globalID(peer.slot, id))))
	}

	glog.Infof("seamless hub: compositor of %s gone", peer.account.Name)

	if remaining == 0 {
		h.mu.Lock()

		if h.link == link {
			h.link = nil
		}

		h.mu.Unlock()

		link.conn.Close()
	}
}

// rewrite turns a compositor's message into the host's: the window ids get the slot in them. The frame is patched
// in place unless the title has to grow.
func (p *hubPeer) rewrite(frame []byte, prefix string) ([]byte, error) {
	payload := frame[5:]

	local := func(offset int) (uint32, error) {
		if len(payload) < offset+4 {
			return 0, errShortMessage
		}

		id := binary.LittleEndian.Uint32(payload[offset:])

		if id == 0 || id > localIDMask {
			return 0, fmt.Errorf("window id %d", id)
		}

		return id, nil
	}

	switch frame[4] {
	case gWindowCreate:
		// id, parent, kind, x, y, then the title and the app id.
		if len(payload) < 17 {
			return nil, errShortMessage
		}

		id, err := local(0)

		if err != nil {
			return nil, err
		}

		parent := binary.LittleEndian.Uint32(payload[4:])

		if parent > localIDMask {
			return nil, fmt.Errorf("parent window id %d", parent)
		}

		p.windows[id] = struct{}{}

		binary.LittleEndian.PutUint32(payload, globalID(p.slot, id))

		if parent != 0 {
			binary.LittleEndian.PutUint32(payload[4:], globalID(p.slot, parent))
		}

		if prefix != "" {
			return withTitlePrefix(frame, 17, prefix)
		}

	case gWindowTitle:
		id, err := local(0)

		if err != nil {
			return nil, err
		}

		binary.LittleEndian.PutUint32(payload, globalID(p.slot, id))

		if prefix != "" {
			return withTitlePrefix(frame, 4, prefix)
		}

	case gWindowAppID, gWindowHints, gWindowFrame, gCursor:
		id, err := local(0)

		if err != nil {
			return nil, err
		}

		binary.LittleEndian.PutUint32(payload, globalID(p.slot, id))

	case gWindowDestroy:
		id, err := local(0)

		if err != nil {
			return nil, err
		}

		delete(p.windows, id)
		binary.LittleEndian.PutUint32(payload, globalID(p.slot, id))

	case gHello:
		return nil, errors.New("duplicate hello")

	default:
		return nil, fmt.Errorf("unknown message type 0x%x", frame[4])
	}

	return frame, nil
}

// readLink reads what the host sends and hands it to the compositor it is for.
func (h *Hub) readLink(link *hubLink) {
	err := func() error {
		for {
			frame, err := readFrame(link.reader, maxHostFrame)

			if err != nil {
				return err
			}

			if err = h.routeHost(link, frame); err != nil {
				return err
			}
		}
	}()

	if err != nil && !errors.Is(err, net.ErrClosed) {
		glog.Warnf("seamless hub: host connection ended: %v", err)
	}

	// The compositors dial again (they retry on their own), and the next one makes a new host connection.
	h.mu.Lock()

	if h.link == link {
		h.link = nil
	}

	var peers []*hubPeer

	for _, p := range h.peers {
		if p != nil && p.link == link {
			peers = append(peers, p)
		}
	}

	h.mu.Unlock()

	link.conn.Close()

	for _, p := range peers {
		p.conn.Close()
	}
}

func (h *Hub) peer(slot uint32) *hubPeer {
	if slot == 0 || slot > maxSlot {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	return h.peers[slot]
}

// routeHost delivers one host message.
func (h *Hub) routeHost(link *hubLink, frame []byte) error {
	payload := frame[5:]

	switch frame[4] {
	case hOutput:
		if len(payload) != 4 {
			return errShortMessage
		}

		link.setScale(binary.LittleEndian.Uint32(payload))

		h.mu.Lock()

		var peers []*hubPeer

		for _, p := range h.peers {
			if p != nil && p.link == link {
				peers = append(peers, p)
			}
		}

		h.mu.Unlock()

		for _, p := range peers {
			p.write(frame)
		}

	case hWindowResize, hWindowClose, hWindowFocus, hPointerEnter, hPointerLeave, hPointerMotion, hPointerButton, hPointerAxis:
		if len(payload) < 4 {
			return errShortMessage
		}

		id := binary.LittleEndian.Uint32(payload)
		peer := h.peer(id >> slotShift)

		if peer == nil || peer.link != link {
			return nil
		}

		if frame[4] == hWindowFocus && len(payload) >= 5 && payload[4] != 0 {
			h.mu.Lock()
			h.focus = peer.slot
			h.mu.Unlock()
		}

		binary.LittleEndian.PutUint32(payload, id&localIDMask)
		peer.write(frame)

	case hKey:
		h.mu.Lock()
		slot := h.focus
		h.mu.Unlock()

		if peer := h.peer(slot); peer != nil && peer.link == link {
			peer.write(frame)
		}

	case hHelloAck:
		return errors.New("unexpected hello acknowledgement")

	default:
		return fmt.Errorf("unknown host message type 0x%x", frame[4])
	}

	return nil
}
