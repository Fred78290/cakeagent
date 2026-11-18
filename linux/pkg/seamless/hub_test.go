package seamless

import (
	"bufio"
	"context"
	"encoding/binary"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// A fake host: the other end of the hub's connection to the host.
type fakeHost struct {
	t        *testing.T
	conn     net.Conn
	reader   *bufio.Reader
	version  uint16
	scale    uint32
	received chan []byte
}

type hubRig struct {
	t         *testing.T
	hub       *Hub
	dir       string
	hosts     chan *fakeHost
	accounts  map[net.Conn]Account
	nextUID   uint32
	mu        sync.Mutex
	cancel    context.CancelFunc
	compSocks []string
}

func newHubRig(t *testing.T) *hubRig {
	t.Helper()

	rig := &hubRig{t: t, hosts: make(chan *fakeHost, 4)}

	// The compositor's account is told by a one byte "who am I" the test's compositors send before anything else:
	// there is no SO_PEERCRED on a pipe.
	rig.hub = &Hub{
		Dial: func() (net.Conn, error) {
			hubSide, hostSide := net.Pipe()
			host := &fakeHost{t: t, conn: hostSide, reader: bufio.NewReader(hostSide), version: 2, scale: 240, received: make(chan []byte, 64)}

			go host.serve()

			rig.hosts <- host

			return hubSide, nil
		},
		Authenticate: func(conn net.Conn) (Account, error) {
			rig.mu.Lock()
			defer rig.mu.Unlock()

			a, ok := rig.accounts[conn]

			if !ok {
				return Account{}, net.ErrClosed
			}

			return a, nil
		},
	}
	rig.accounts = map[net.Conn]Account{}

	return rig
}

func (h *fakeHost) serve() {
	// hello, then the acknowledgement
	frame, err := readFrame(h.reader, maxString+16)

	if err != nil {
		return
	}

	if _, _, err = parseHello(frame); err != nil {
		h.t.Errorf("host got a bad hello: %v", err)

		return
	}

	_, _ = h.conn.Write(buildHelloAck(h.version, h.scale))

	for {
		frame, err = readFrame(h.reader, maxGuestFrame)

		if err != nil {
			close(h.received)

			return
		}

		h.received <- frame
	}
}

func (h *fakeHost) send(frame []byte) {
	_, _ = h.conn.Write(frame)
}

// compositor connects as account and returns the compositor's end of the connection, after the hello exchange.
func (r *hubRig) compositor(account Account) (net.Conn, *bufio.Reader) {
	r.t.Helper()

	hubSide, compSide := net.Pipe()

	r.mu.Lock()
	r.accounts[hubSide] = account
	r.mu.Unlock()

	go r.hub.handle(hubSide)

	if _, err := compSide.Write(buildHello(2, "caker-seamlessd 0.1")); err != nil {
		r.t.Fatal(err)
	}

	reader := bufio.NewReader(compSide)

	_ = compSide.SetReadDeadline(time.Now().Add(5 * time.Second))

	ack, err := readFrame(reader, maxHostFrame)

	if err != nil {
		r.t.Fatalf("no hello acknowledgement: %v", err)
	}

	version, scale, err := parseHelloAck(ack)

	if err != nil || version != 2 || scale != 240 {
		r.t.Fatalf("bad acknowledgement: %d %d %v", version, scale, err)
	}

	_ = compSide.SetReadDeadline(time.Time{})

	return compSide, reader
}

func u32(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }

func str(s string) []byte {
	return append(binary.LittleEndian.AppendUint16(nil, uint16(len(s))), s...)
}

func createWindow(id, parent uint32, title string) []byte {
	payload := append(u32(id), u32(parent)...)

	payload = append(payload, 0)
	payload = append(payload, u32(0)...)
	payload = append(payload, u32(0)...)
	payload = append(payload, str(title)...)
	payload = append(payload, str("app")...)

	return buildFrame(gWindowCreate, payload)
}

func expect(t *testing.T, ch <-chan []byte) []byte {
	t.Helper()

	select {
	case frame, ok := <-ch:
		if !ok {
			t.Fatal("connection closed")
		}

		return frame
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a message")
	}

	return nil
}

func readOne(t *testing.T, conn net.Conn, reader *bufio.Reader) []byte {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	frame, err := readFrame(reader, maxHostFrame)

	if err != nil {
		t.Fatalf("read: %v", err)
	}

	return frame
}

var (
	alice = Account{Name: "alice", UID: 1000, GID: 1000}
	bob   = Account{Name: "bob", UID: 1001, GID: 1001}
)

func TestHubTellsUsersWindowsApartAndRoutesInput(t *testing.T) {
	rig := newHubRig(t)

	rig.hub.SetPrimary("alice")

	aliceConn, aliceReader := rig.compositor(alice)
	host := <-rig.hosts
	bobConn, bobReader := rig.compositor(bob)

	// Both compositors count their windows from 1.
	if _, err := aliceConn.Write(createWindow(1, 0, "Terminal")); err != nil {
		t.Fatal(err)
	}

	if _, err := bobConn.Write(createWindow(1, 0, "Terminal")); err != nil {
		t.Fatal(err)
	}

	first := expect(t, host.received)
	second := expect(t, host.received)

	idOf := func(frame []byte) uint32 { return binary.LittleEndian.Uint32(frame[5:]) }

	if idOf(first) == idOf(second) {
		t.Fatalf("two users' windows share the host id %d", idOf(first))
	}

	if !strings.Contains(string(first), "Terminal") || strings.Contains(string(first), "alice: ") {
		t.Fatalf("the primary user's titles are left alone: %q", first)
	}

	if !strings.Contains(string(second), "bob: Terminal") {
		t.Fatalf("another user's titles say whose they are: %q", second)
	}

	if int(binary.LittleEndian.Uint32(second)) != len(second)-4 {
		t.Fatalf("length prefix of a rewritten frame is wrong")
	}

	// A popup's parent is in the same user's space.
	if _, err := bobConn.Write(createWindow(2, 1, "menu")); err != nil {
		t.Fatal(err)
	}

	popup := expect(t, host.received)

	if binary.LittleEndian.Uint32(popup[9:]) != idOf(second) {
		t.Fatalf("the parent id was not translated")
	}

	// Input goes to the compositor that owns the window, with its own id back.
	host.send(buildFrame(hPointerLeave, u32(idOf(second))))

	back := readOne(t, bobConn, bobReader)

	if back[4] != hPointerLeave || binary.LittleEndian.Uint32(back[5:]) != 1 {
		t.Fatalf("bob got %x", back)
	}

	host.send(buildFrame(hWindowFocus, append(u32(idOf(first)), 1)))

	focus := readOne(t, aliceConn, aliceReader)

	if focus[4] != hWindowFocus || binary.LittleEndian.Uint32(focus[5:]) != 1 {
		t.Fatalf("alice got %x", focus)
	}

	// Keys follow the focus.
	host.send(buildFrame(hKey, append(u32(30), 1)))

	key := readOne(t, aliceConn, aliceReader)

	if key[4] != hKey {
		t.Fatalf("the key must reach the focused compositor: %x", key)
	}

	// The output scale reaches everyone.
	host.send(buildFrame(hOutput, u32(360)))

	for _, c := range []struct {
		conn   net.Conn
		reader *bufio.Reader
	}{{aliceConn, aliceReader}, {bobConn, bobReader}} {
		if out := readOne(t, c.conn, c.reader); out[4] != hOutput || binary.LittleEndian.Uint32(out[5:]) != 360 {
			t.Fatalf("output: %x", out)
		}
	}
}

func TestHubDestroysTheWindowsOfACompositorThatLeaves(t *testing.T) {
	rig := newHubRig(t)

	aliceConn, _ := rig.compositor(alice)
	host := <-rig.hosts
	bobConn, _ := rig.compositor(bob)

	_, _ = aliceConn.Write(createWindow(1, 0, "a"))
	_, _ = aliceConn.Write(createWindow(2, 0, "b"))
	_, _ = bobConn.Write(createWindow(1, 0, "c"))

	var bobID uint32

	for i := 0; i < 3; i++ {
		frame := expect(t, host.received)

		if strings.Contains(string(frame), "c") && frame[4] == gWindowCreate && i == 2 {
			bobID = binary.LittleEndian.Uint32(frame[5:])
		}
	}

	aliceConn.Close()

	destroyed := map[uint32]bool{}

	for i := 0; i < 2; i++ {
		frame := expect(t, host.received)

		if frame[4] != gWindowDestroy {
			t.Fatalf("expected a destroy, got 0x%x", frame[4])
		}

		destroyed[binary.LittleEndian.Uint32(frame[5:])] = true
	}

	if len(destroyed) != 2 || destroyed[bobID] {
		t.Fatalf("only alice's two windows go: %v (bob %d)", destroyed, bobID)
	}

	// The host connection stays while bob is there, and goes with the last compositor.
	select {
	case frame, ok := <-host.received:
		if !ok {
			t.Fatalf("the host connection closed with a compositor left")
		}

		t.Fatalf("unexpected message %x", frame)
	case <-time.After(200 * time.Millisecond):
	}

	bobConn.Close()

	for {
		select {
		case _, ok := <-host.received:
			if !ok {
				return
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the host connection must close with the last compositor")
		}
	}
}

func TestHubReplacesAUsersPreviousCompositor(t *testing.T) {
	rig := newHubRig(t)

	first, firstReader := rig.compositor(alice)
	<-rig.hosts

	second, _ := rig.compositor(alice)

	_ = first.SetReadDeadline(time.Now().Add(5 * time.Second))

	if _, err := readFrame(firstReader, maxHostFrame); err == nil {
		t.Fatal("the previous connection of a user must be closed")
	}

	_ = second
}

func TestHubRejectsMalformedCompositorMessages(t *testing.T) {
	rig := newHubRig(t)

	conn, reader := rig.compositor(alice)
	<-rig.hosts

	// A window id with slot bits already set would let a compositor speak for another one.
	_, _ = conn.Write(createWindow(1<<slotShift|1, 0, "evil"))

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	if _, err := readFrame(reader, maxHostFrame); err == nil {
		t.Fatal("the connection must be closed")
	}
}

func TestHubDropsCompositorsWhenTheHostIsGone(t *testing.T) {
	rig := newHubRig(t)

	conn, reader := rig.compositor(alice)
	host := <-rig.hosts

	host.conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	if _, err := readFrame(reader, maxHostFrame); err == nil {
		t.Fatal("compositors are dropped with the host connection, so that they dial again")
	}

	// A compositor that comes back makes a new host connection.
	rig.compositor(alice)

	select {
	case <-rig.hosts:
	case <-time.After(5 * time.Second):
		t.Fatal("no new host connection")
	}
}

func TestWithTitlePrefixKeepsTitlesWithinTheLimit(t *testing.T) {
	title := strings.Repeat("é", maxString/2)
	frame := createWindow(1, 0, title)

	out, err := withTitlePrefix(frame, 17, "bob: ")

	if err != nil {
		t.Fatal(err)
	}

	n := int(binary.LittleEndian.Uint16(out[5+17:]))

	if n > maxString || !strings.HasPrefix(string(out[5+17+2:]), "bob: ") {
		t.Fatalf("title of %d bytes", n)
	}

	if int(binary.LittleEndian.Uint32(out)) != len(out)-4 {
		t.Fatal("bad length prefix")
	}

	// The app id after the title is kept, and the cut is on a character boundary.
	if !strings.HasSuffix(string(out), "app") || strings.Contains(string(out[5+17+2:5+17+2+n]), "�") {
		t.Fatal("trailer lost or a character cut in half")
	}
}

func TestServeStopsWithItsContext(t *testing.T) {
	rig := newHubRig(t)

	ctx, cancel := context.WithCancel(context.Background())

	l, err := net.Listen("unix", t.TempDir()+"/hub.sock")

	if err != nil {
		t.Skip("no unix sockets here")
	}

	done := make(chan error, 1)

	go func() { done <- rig.hub.Serve(ctx, l) }()

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve must return once the context is done")
	}
}
