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
	// maxClipboard mirrors SEAMLESS_MAX_CLIPBOARD: the text of a clipboard message, in bytes.
	maxClipboard = 1024 * 1024
	// maxHostFrame mirrors SEAMLESS_MAX_HOST_FRAME: host messages are tiny, but for the clipboard.
	maxHostFrame = maxClipboard + 64*1024
	// maxString mirrors SeamlessProtocol.maxStringLength.
	maxString = 4096

	// frameBudget is how much memory the frames being read from the compositors may take all together. It must hold
	// a frame of maxGuestFrame, so that a complete large frame is still accepted when nothing else is in flight.
	frameBudget = 256 * 1024 * 1024

	hubHandshakeTimeout = 10 * time.Second

	// attachAttempts is how many times a compositor's attach retries when the host connection ends under it.
	attachAttempts = 3

	// A write to the host may not stall forever: it gets hostWriteBase plus the time a slow link (hostWriteMinRate
	// bytes per second) would need for the frame. Past that the connection is closed.
	hostWriteBase    = 30 * time.Second
	hostWriteMinRate = 1024 * 1024

	// What the host sends to a compositor is queued (peerQueueLen messages) and written by a goroutine of its own,
	// so a compositor that stops reading cannot hold up the host connection's reader and with it the other
	// compositors. One that lets the queue fill, or does not take a message within peerWriteTimeout, is disconnected.
	peerQueueLen     = 1024
	peerWriteTimeout = 10 * time.Second
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
	gClipboard     = 0x09
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
	hClipboard     = 0x8C
)

var errShortMessage = errors.New("short message")

// memBudget is a memory allowance shared by concurrent readers. Reservations are served in arrival order, so a large
// frame is not starved by a stream of small ones.
type memBudget struct {
	mu      sync.Mutex
	total   int64
	avail   int64
	waiters []*budgetWaiter
}

type budgetWaiter struct {
	n       int64
	granted chan struct{}
}

func newMemBudget(total int64) *memBudget {
	return &memBudget{total: total, avail: total}
}

// acquire reserves n bytes, waiting for them to be free until done is closed. A reservation that cannot ever fit is
// refused at once. The caller releases what it got.
func (b *memBudget) acquire(n int64, done <-chan struct{}) error {
	if n > b.total {
		return fmt.Errorf("frame of %d bytes exceeds the memory budget of %d", n, b.total)
	}

	b.mu.Lock()

	if len(b.waiters) == 0 && b.avail >= n {
		b.avail -= n
		b.mu.Unlock()

		return nil
	}

	w := &budgetWaiter{n: n, granted: make(chan struct{})}

	b.waiters = append(b.waiters, w)
	b.mu.Unlock()

	select {
	case <-w.granted:
		return nil
	case <-done:
	}

	b.mu.Lock()

	select {
	case <-w.granted:
		// Granted just as we gave up: hand it back.
		b.avail += n
	default:
		for i, o := range b.waiters {
			if o == w {
				b.waiters = append(b.waiters[:i], b.waiters[i+1:]...)

				break
			}
		}
	}

	b.grant()
	b.mu.Unlock()

	return net.ErrClosed
}

func (b *memBudget) release(n int64) {
	b.mu.Lock()
	b.avail += n
	b.grant()
	b.mu.Unlock()
}

// grant wakes the waiters at the head of the queue that now fit. It is called with mu held.
func (b *memBudget) grant() {
	for len(b.waiters) > 0 && b.waiters[0].n <= b.avail {
		w := b.waiters[0]

		b.waiters = b.waiters[1:]
		b.avail -= w.n

		close(w.granted)
	}
}

// readFrame reads one `u32 length` + type + payload frame, prefix included, so that a message can be patched and
// forwarded without copying it. limit bounds the length field.
func readFrame(r io.Reader, limit uint32) ([]byte, error) {
	frame, _, err := readBudgetedFrame(r, limit, nil, nil)

	return frame, err
}

// readBudgetedFrame is readFrame for a peer that cannot be trusted with memory: before allocating what the length
// field declares, it reserves that many bytes from budget, waiting until they are free or done is closed. The
// reservation (reserved bytes, 0 without a budget) is the caller's to release once it is finished with the frame;
// it is already released when an error is returned.
func readBudgetedFrame(r io.Reader, limit uint32, budget *memBudget, done <-chan struct{}) (frame []byte, reserved int64, err error) {
	var header [4]byte

	if _, err = io.ReadFull(r, header[:]); err != nil {
		return nil, 0, err
	}

	n := binary.LittleEndian.Uint32(header[:])

	if n == 0 || n > limit {
		return nil, 0, fmt.Errorf("frame of %d bytes", n)
	}

	if budget != nil {
		reserved = 4 + int64(n)

		if err = budget.acquire(reserved, done); err != nil {
			return nil, 0, err
		}
	}

	frame = make([]byte, 4+int(n))

	copy(frame, header[:])

	if _, err = io.ReadFull(r, frame[4:]); err != nil {
		if budget != nil {
			budget.release(reserved)
		}

		return nil, 0, err
	}

	return frame, reserved, nil
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

// checkString verifies that p holds, from offset off to its end, exactly one protocol string (`u16 length` + UTF-8
// bytes, at most maxString long): what the host's decoder accepts, so that nothing it would refuse is forwarded.
func checkString(p []byte, off int) error {
	if off+2 > len(p) {
		return errShortMessage
	}

	n := int(binary.LittleEndian.Uint16(p[off:]))

	if n > maxString {
		return fmt.Errorf("string of %d bytes", n)
	}

	if off+2+n != len(p) {
		return errors.New("malformed string")
	}

	if !utf8.Valid(p[off+2:]) {
		return errors.New("string is not UTF-8")
	}

	return nil
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
	// lastSlot is the slot given last. Slots are handed out round-robin rather than lowest-free: the protocol has no
	// acknowledgement of a window's destruction, so the host may still have events for a window of a compositor that
	// left in flight, and a slot reused at once (with the new compositor counting its windows from 1 again) would
	// deliver them to the wrong user. Cycling through the other slots first leaves those events time to drain.
	lastSlot uint32
	link     *hubLink
	// primary is the account whose window titles stay as they are; the others get their user name in front. While it
	// is unknown (SetPrimary not called yet) every account gets its name in front, so nobody is passed off as the primary.
	primary string
	// focus is the compositor that owns the focused window, where keys go. It is the peer itself, not its slot: slots
	// are reused, and a replacement must not inherit the keyboard of the compositor that left.
	focus *hubPeer
	// budget bounds the memory of the compositors' frames being read; made on first use.
	budget *memBudget

	// closing is set when Serve shuts down: from then on no compositor is attached and no host connection published.
	// stop is closed with it, to cut short a dial or a handshake with the host in progress. conns are the accepted
	// connections whose handlers run, and wg counts them (and the host readers), so shutdown can wait for them.
	closing bool
	stop    chan struct{}
	conns   map[net.Conn]struct{}
	wg      sync.WaitGroup
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

// write sends one frame to the host within a deadline. A write that fails or is cut short leaves the stream in the
// middle of a frame, so nothing more may follow it: the connection is closed, which ends readLink and retires the
// compositors' sessions.
func (l *hubLink) write(frame []byte) error {
	l.wmu.Lock()
	defer l.wmu.Unlock()

	timeout := hostWriteBase + time.Duration(len(frame))*time.Second/hostWriteMinRate

	_ = l.conn.SetWriteDeadline(time.Now().Add(timeout))

	n, err := l.conn.Write(frame)

	if err == nil && n < len(frame) {
		err = io.ErrShortWrite
	}

	if err != nil {
		l.conn.Close()
	}

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
	// out holds the messages for the compositor, in order; writeLoop sends them.
	out chan []byte
	// windows are the compositor's own window ids still alive, guarded by winMu: the reader goroutine maintains it and
	// the host reader checks incoming events against it.
	winMu   sync.Mutex
	windows map[uint32]struct{}
	// done is closed with the connection, to stop a read waiting for memory.
	done      chan struct{}
	closeOnce sync.Once
}

func (p *hubPeer) addWindow(id uint32) {
	p.winMu.Lock()
	p.windows[id] = struct{}{}
	p.winMu.Unlock()
}

func (p *hubPeer) removeWindow(id uint32) {
	p.winMu.Lock()
	delete(p.windows, id)
	p.winMu.Unlock()
}

func (p *hubPeer) hasWindow(id uint32) bool {
	p.winMu.Lock()
	defer p.winMu.Unlock()

	_, ok := p.windows[id]

	return ok
}

func (p *hubPeer) liveWindows() []uint32 {
	p.winMu.Lock()
	defer p.winMu.Unlock()

	ids := make([]uint32, 0, len(p.windows))

	for id := range p.windows {
		ids = append(ids, id)
	}

	return ids
}

// close ends the connection and whatever waits on it.
func (p *hubPeer) close() {
	p.closeOnce.Do(func() { close(p.done) })
	p.conn.Close()
}

func (h *Hub) frameBudget() *memBudget {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.budget == nil {
		h.budget = newMemBudget(frameBudget)
	}

	return h.budget
}

// write queues a message for the compositor without ever blocking. A compositor that does not keep up is closed.
func (p *hubPeer) write(frame []byte) {
	select {
	case p.out <- frame:
	default:
		glog.Warnf("seamless hub: compositor of %s does not read its input, disconnecting it", p.account.Name)

		// The reader notices and cleans up.
		p.close()
	}
}

// writeLoop sends the queued messages until the compositor is gone.
func (p *hubPeer) writeLoop() {
	for {
		select {
		case <-p.done:
			return
		case frame := <-p.out:
			_ = p.conn.SetWriteDeadline(time.Now().Add(peerWriteTimeout))

			if _, err := p.conn.Write(frame); err != nil {
				p.close()

				return
			}
		}
	}
}

// SetPrimary names the account whose window titles are left alone.
func (h *Hub) SetPrimary(name string) {
	h.mu.Lock()
	h.primary = name
	h.mu.Unlock()
}

// Attached tells whether a compositor of the account with this uid is connected to the hub and acknowledged.
func (h *Hub) Attached(uid uint32) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, p := range h.peers {
		if p == nil || p.account.UID != uid {
			continue
		}

		select {
		case <-p.done:
		default:
			return true
		}
	}

	return false
}

// titlePrefix is what goes in front of the window titles of an account. It is asked again for every title, so
// compositors that connected before the primary account was known are labelled properly once it is.
func (h *Hub) titlePrefix(a Account) string {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.primary != "" && a.Name == h.primary {
		return ""
	}

	return a.Name + ": "
}

// stopChan returns the channel closed at shutdown. It is called with mu held.
func (h *Hub) stopChan() chan struct{} {
	if h.stop == nil {
		h.stop = make(chan struct{})
	}

	return h.stop
}

// register records an accepted connection and counts its handler, unless the hub is shutting down.
func (h *Hub) register(conn net.Conn) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closing {
		return false
	}

	if h.conns == nil {
		h.conns = map[net.Conn]struct{}{}
	}

	h.conns[conn] = struct{}{}
	h.wg.Add(1)

	return true
}

func (h *Hub) unregister(conn net.Conn) {
	h.mu.Lock()
	delete(h.conns, conn)
	h.mu.Unlock()

	h.wg.Done()
}

// Serve accepts compositors on l until ctx is done, and returns once their handlers are gone.
func (h *Hub) Serve(ctx context.Context, l net.Listener) error {
	h.mu.Lock()

	// A hub that served before starts afresh.
	if h.closing {
		h.closing = false
		h.stop = nil
	}

	h.mu.Unlock()

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

		// Registered before the handler starts, so shutdown cannot miss it.
		if !h.register(conn) {
			conn.Close()

			return net.ErrClosed
		}

		go func() {
			defer h.unregister(conn)

			h.handle(conn)
		}()
	}
}

// closeAll shuts the hub down: nothing new is attached or published from here on, pending host work is cancelled,
// every connection is closed, and the handlers are waited for.
func (h *Hub) closeAll() {
	h.mu.Lock()

	h.closing = true

	if stop := h.stopChan(); !isClosed(stop) {
		close(stop)
	}

	link := h.link
	h.link = nil

	var peers []*hubPeer

	for _, p := range h.peers {
		if p != nil {
			peers = append(peers, p)
		}
	}

	conns := make([]net.Conn, 0, len(h.conns))

	for c := range h.conns {
		conns = append(conns, c)
	}

	h.mu.Unlock()

	if link != nil {
		link.conn.Close()
	}

	for _, p := range peers {
		p.close()
	}

	for _, c := range conns {
		c.Close()
	}

	h.wg.Wait()
}

func isClosed(c <-chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
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

	budget := h.frameBudget()

	for {
		frame, reserved, err := readBudgetedFrame(reader, maxGuestFrame, budget, peer.done)

		if err != nil {
			return err
		}

		// Only titles carry the prefix, and the primary account may have become known since the last one.
		prefix := ""

		if t := frame[4]; t == gWindowCreate || t == gWindowTitle {
			prefix = h.titlePrefix(account)
		}

		if frame[4] == gClipboard && !h.mayShareClipboard(peer) {
			// Not the user the host is looking at: a background session does not get to overwrite the host's clipboard.
			// Still checked, so that a malformed message ends this compositor as it would anywhere else.
			if err = checkClipboard(frame[5:]); err == nil {
				budget.release(reserved)

				continue
			}
		} else {
			err = peer.forward(frame, prefix)
		}

		// The frame is out of the way, however it went.
		budget.release(reserved)

		if err != nil {
			return err
		}
	}
}

// checkClipboard validates the payload of a clipboard message: a known format and a length that is exactly what follows.
func checkClipboard(payload []byte) error {
	if len(payload) < 5 {
		return errShortMessage
	}

	if payload[0] != 0 {
		return fmt.Errorf("clipboard format %d", payload[0])
	}

	n := binary.LittleEndian.Uint32(payload[1:])

	if n > maxClipboard || int(n) != len(payload)-5 {
		return fmt.Errorf("clipboard of %d bytes in a message of %d", n, len(payload)-5)
	}

	return nil
}

// mayShareClipboard says whether the compositor's clipboard goes to the host: the host's keyboard focus is in its
// windows, or nowhere yet.
func (h *Hub) mayShareClipboard(p *hubPeer) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.focus == nil || h.focus == p
}

// forward rewrites one compositor message and sends it to the host.
func (p *hubPeer) forward(frame []byte, prefix string) error {
	frame, err := p.rewrite(frame, prefix)

	if err != nil {
		return err
	}

	if err = p.link.write(frame); err != nil {
		return fmt.Errorf("host connection: %w", err)
	}

	return nil
}

// attach gives the compositor a slot, opening the connection to the host if it is the first one.
func (h *Hub) attach(conn net.Conn, account Account, version uint16, name string) (*hubPeer, error) {
	h.linkMu.Lock()
	defer h.linkMu.Unlock()

	// readLink retires a host connection without taking linkMu, so the link ensureLink returned may be gone by the time
	// the peer is registered. Registering on it would acknowledge a compositor whose connection is already dead (and
	// that readLink, having listed its peers earlier, would never close). Retry with a current link a few times.
	for attempt := 0; attempt < attachAttempts; attempt++ {
		link, err := h.ensureLink(version, name)

		if err != nil {
			return nil, err
		}

		if link.version != version {
			return nil, fmt.Errorf("protocol version %d, the host connection speaks %d", version, link.version)
		}

		peer, err := h.claimSlot(conn, account, link)

		if errors.Is(err, errStaleLink) {
			continue
		}

		return peer, err
	}

	return nil, errStaleLink
}

var errStaleLink = errors.New("the host connection ended while the compositor was attaching")

// claimSlot puts the compositor in a slot of link, provided link is still the host connection.
func (h *Hub) claimSlot(conn net.Conn, account Account, link *hubLink) (*hubPeer, error) {
	h.mu.Lock()

	if h.closing {
		h.mu.Unlock()

		return nil, net.ErrClosed
	}

	if h.link != link {
		h.mu.Unlock()

		return nil, errStaleLink
	}

	// A compositor that restarted may be seen before its previous connection is noticed to be gone: it takes over.
	for _, p := range h.peers {
		if p != nil && p.account.UID == account.UID {
			p.close()
		}
	}

	var peer *hubPeer

	for i := uint32(0); i < maxSlot; i++ {
		slot := (h.lastSlot+i)%maxSlot + 1

		if h.peers[slot] == nil {
			peer = &hubPeer{slot: slot, account: account, conn: conn, link: link, windows: map[uint32]struct{}{}, done: make(chan struct{}), out: make(chan []byte, peerQueueLen)}

			// First in the queue, before the host can route anything to the compositor. The scale is read under mu, as
			// routeHost stores it and lists the receivers under mu: a concurrent output change either comes after this
			// registration (and is queued behind the acknowledgement) or is already part of the scale read here.
			peer.out <- buildHelloAck(link.version, link.currentScale())
			h.peers[slot] = peer
			h.lastSlot = slot

			break
		}
	}

	h.mu.Unlock()

	if peer == nil {
		return nil, errors.New("too many compositors")
	}

	go peer.writeLoop()

	return peer, nil
}

// ensureLink returns the host connection, making it (hello and acknowledgement included) when there is none. It
// is called with linkMu held.
func (h *Hub) ensureLink(version uint16, name string) (*hubLink, error) {
	h.mu.Lock()

	if h.closing {
		h.mu.Unlock()

		return nil, net.ErrClosed
	}

	link := h.link
	stop := h.stopChan()

	h.mu.Unlock()

	if link != nil {
		return link, nil
	}

	// Dial cannot be interrupted, so it runs on the side: at shutdown its late connection is closed unused.
	type dialed struct {
		conn net.Conn
		err  error
	}

	result := make(chan dialed, 1)

	go func() {
		c, e := h.Dial()

		result <- dialed{c, e}
	}()

	var conn net.Conn

	select {
	case d := <-result:
		if d.err != nil {
			return nil, fmt.Errorf("cannot reach the host: %w", d.err)
		}

		conn = d.conn
	case <-stop:
		go func() {
			if d := <-result; d.conn != nil {
				d.conn.Close()
			}
		}()

		return nil, net.ErrClosed
	}

	// Shutdown during the handshake closes the connection, which fails the exchange below.
	handshaking := make(chan struct{})

	defer close(handshaking)

	go func() {
		select {
		case <-stop:
			conn.Close()
		case <-handshaking:
		}
	}()

	_ = conn.SetDeadline(time.Now().Add(hubHandshakeTimeout))

	link = &hubLink{conn: conn, reader: bufio.NewReaderSize(conn, 64*1024)}

	var err error

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

	// A host that answers with another version is of no use to the compositor that asked: the connection is dropped
	// here, before it is published, rather than kept as a host session with nobody attached to it.
	if link.version != version {
		conn.Close()

		return nil, fmt.Errorf("protocol version %d, the host speaks %d", version, link.version)
	}

	_ = conn.SetDeadline(time.Time{})

	h.mu.Lock()

	// closeAll sets closing under mu, so a link published here is one it will see; one that comes too late is dropped.
	if h.closing {
		h.mu.Unlock()
		conn.Close()

		return nil, net.ErrClosed
	}

	h.link = link
	h.wg.Add(1)
	h.mu.Unlock()

	go func() {
		defer h.wg.Done()

		h.readLink(link)
	}()

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

	if h.focus == peer {
		h.focus = nil
	}

	link := h.link
	remaining := 0

	for _, p := range h.peers {
		if p != nil {
			remaining++
		}
	}

	h.mu.Unlock()

	peer.close()

	// A window id of a previous connection means nothing to a new host session (the slot may even be in use again).
	if link == nil || link != peer.link {
		return
	}

	for _, id := range peer.liveWindows() {
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

		p.addWindow(id)

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

		// The host closes the whole session on a title it cannot decode, and the primary account's titles are forwarded
		// without going through withTitlePrefix: check the frame here, before anything is patched, so that a malformed
		// one ends this compositor only.
		if err = checkString(payload, 4); err != nil {
			return nil, fmt.Errorf("window title: %w", err)
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

	case gClipboard:
		// u8 format (0: UTF-8 text) + u32 length + bytes. There is no window id: whose it is, is the hub's business.
		if err := checkClipboard(payload); err != nil {
			return nil, err
		}

	case gWindowDestroy:
		id, err := local(0)

		if err != nil {
			return nil, err
		}

		p.removeWindow(id)
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
		p.close()
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

		// The scale is stored and the receivers listed under the same mu hold that claimSlot uses to snapshot the scale
		// for a new compositor's acknowledgement and to register it. Either the new compositor is already registered
		// (so it is listed here and gets this output after its acknowledgement, which is first in its queue), or it
		// snapshots the new scale. A compositor can never register with a stale scale and miss this update.
		h.mu.Lock()

		link.setScale(binary.LittleEndian.Uint32(payload))

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

		// The host may still send events for a window that is already gone (destroyed by its compositor, or left with it),
		// and the protocol has no acknowledgement to wait for. Only a window the compositor currently owns takes input,
		// so a late event cannot reach a window that merely reuses the id.
		if !peer.hasWindow(id & localIDMask) {
			return nil
		}

		if frame[4] == hWindowFocus && len(payload) >= 5 && payload[4] != 0 {
			// The peer may have been dropped since it was looked up: only a peer still registered takes the focus.
			h.mu.Lock()

			if h.peers[peer.slot] == peer {
				h.focus = peer
			}

			h.mu.Unlock()
		}

		binary.LittleEndian.PutUint32(payload, id&localIDMask)
		peer.write(frame)

	case hKey:
		h.mu.Lock()
		peer := h.focus
		h.mu.Unlock()

		if peer != nil && peer.link == link {
			peer.write(frame)
		}

	case hClipboard:
		if err := checkClipboard(payload); err != nil {
			return err
		}

		// The user whose window has the focus: the clipboard of the host is not given to every account of the guest.
		h.mu.Lock()
		peer := h.focus
		h.mu.Unlock()

		if peer != nil && peer.link == link {
			peer.write(frame)
		}

	case hHelloAck:
		return errors.New("unexpected hello acknowledgement")

	default:
		return fmt.Errorf("unknown host message type 0x%x", frame[4])
	}

	return nil
}
