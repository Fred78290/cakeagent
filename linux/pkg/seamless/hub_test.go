package seamless

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"maps"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
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
	version   uint16
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

	return newHubRigVersion(t, 2)
}

// newHubRigVersion makes a rig whose host and compositors speak the given protocol version.
func newHubRigVersion(t *testing.T, version uint16) *hubRig {
	t.Helper()

	rig := &hubRig{t: t, version: version, hosts: make(chan *fakeHost, 4)}

	// The compositor's account is told by a one byte "who am I" the test's compositors send before anything else:
	// there is no SO_PEERCRED on a pipe.
	rig.hub = &Hub{
		Dial: func() (net.Conn, error) {
			hubSide, hostSide := net.Pipe()

			closeOnCleanup(t, hubSide, hostSide)

			host := &fakeHost{t: t, conn: hostSide, reader: bufio.NewReader(hostSide), version: version, scale: 240, received: make(chan []byte, 64)}

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

// closeOnCleanup closes the connections when the test ends, whether it passes or stops through Fatal. Closing a
// net.Pipe end twice is harmless, so tests may still close them earlier on purpose.
func closeOnCleanup(t *testing.T, conns ...net.Conn) {
	t.Helper()

	for _, c := range conns {
		t.Cleanup(func() { _ = c.Close() })
	}
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

	closeOnCleanup(r.t, hubSide, compSide)

	r.mu.Lock()
	r.accounts[hubSide] = account
	r.mu.Unlock()

	go r.hub.handle(hubSide)

	if _, err := compSide.Write(buildHello(r.version, "caker-seamlessd 0.1")); err != nil {
		r.t.Fatal(err)
	}

	reader := bufio.NewReader(compSide)

	_ = compSide.SetReadDeadline(time.Now().Add(5 * time.Second))

	ack, err := readFrame(reader, maxHostFrame)

	if err != nil {
		r.t.Fatalf("no hello acknowledgement: %v", err)
	}

	version, scale, err := parseHelloAck(ack)

	if err != nil || version != r.version || scale != 240 {
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

// expectClosed waits for the connection to be closed by the other end. Only an end of stream or a closed connection
// counts: a deadline that expires, or any other error, means the connection was not closed as expected.
func expectClosed(t *testing.T, conn net.Conn, reader io.Reader, what string) {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	frame, err := readFrame(reader, maxHostFrame)

	switch {
	case err == nil:
		t.Fatalf("%s: read a frame %x instead of a closed connection", what, frame)
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrClosedPipe), errors.Is(err, net.ErrClosed):
	default:
		t.Fatalf("%s: expected the connection to be closed, got: %v", what, err)
	}
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

	// Two connections have no relative order: bob writes once alice's window has reached the host.
	first := expect(t, host.received)

	if _, err := bobConn.Write(createWindow(1, 0, "Terminal")); err != nil {
		t.Fatal(err)
	}

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

	// The compositors write on separate connections, so the frames arrive in no fixed order: the window ids are
	// derived from the slot each peer was given, not from the order of the frames.
	slotOf := func(uid uint32) uint32 {
		rig.hub.mu.Lock()
		defer rig.hub.mu.Unlock()

		for _, p := range rig.hub.peers {
			if p != nil && p.account.UID == uid {
				return p.slot
			}
		}

		t.Fatalf("no compositor attached for uid %d", uid)

		return 0
	}

	aliceSlot, bobSlot := slotOf(alice.UID), slotOf(bob.UID)

	if aliceSlot == bobSlot {
		t.Fatalf("both compositors share the slot %d", aliceSlot)
	}

	aliceIDs := map[uint32]bool{globalID(aliceSlot, 1): true, globalID(aliceSlot, 2): true}
	bobID := globalID(bobSlot, 1)

	created := map[uint32]bool{}

	for i := 0; i < 3; i++ {
		frame := expect(t, host.received)

		if frame[4] != gWindowCreate {
			t.Fatalf("expected a window creation, got 0x%x", frame[4])
		}

		created[binary.LittleEndian.Uint32(frame[5:])] = true
	}

	wantCreated := map[uint32]bool{bobID: true}

	for id := range aliceIDs {
		wantCreated[id] = true
	}

	if !maps.Equal(created, wantCreated) {
		t.Fatalf("unexpected window ids reaching the host: got %v, want %v", created, wantCreated)
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

	if !maps.Equal(destroyed, aliceIDs) {
		t.Fatalf("exactly alice's two windows go: destroyed %v, alice %v (bob %d)", destroyed, aliceIDs, bobID)
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

	expectClosed(t, first, firstReader, "the previous connection of a user must be closed")

	_ = second
}

func TestHubRejectsMalformedCompositorMessages(t *testing.T) {
	rig := newHubRig(t)

	conn, reader := rig.compositor(alice)
	<-rig.hosts

	// A window id with slot bits already set would let a compositor speak for another one.
	_, _ = conn.Write(createWindow(1<<slotShift|1, 0, "evil"))

	expectClosed(t, conn, reader, "the connection must be closed")
}

func TestRewriteRejectsMalformedTitles(t *testing.T) {
	id := binary.LittleEndian.AppendUint32(nil, 1)
	str := func(n int, s string) []byte {
		return append(binary.LittleEndian.AppendUint16(slices.Clone(id), uint16(n)), s...)
	}

	cases := map[string][]byte{
		"no length field": id,
		"truncated":       str(5, "abc"),
		"trailing bytes":  str(2, "abcd"),
		"invalid UTF-8":   str(2, "\xff\xfe"),
		"too long":        str(maxString+1, strings.Repeat("a", maxString+1)),
	}

	for name, payload := range cases {
		for _, prefix := range []string{"", "bob: "} {
			p := &hubPeer{slot: 2, windows: map[uint32]struct{}{}}

			if _, err := p.rewrite(buildFrame(gWindowTitle, payload), prefix); err == nil {
				t.Errorf("%s (prefix %q): the title must be refused", name, prefix)
			}
		}
	}

	p := &hubPeer{slot: 2, windows: map[uint32]struct{}{}}

	out, err := p.rewrite(buildFrame(gWindowTitle, str(2, "ok")), "")

	if err != nil || binary.LittleEndian.Uint32(out[5:]) != globalID(2, 1) {
		t.Fatalf("a valid title must pass with its id patched: %v", err)
	}
}

func TestHubDropsCompositorsWhenTheHostIsGone(t *testing.T) {
	rig := newHubRig(t)

	conn, reader := rig.compositor(alice)
	host := <-rig.hosts

	host.conn.Close()

	expectClosed(t, conn, reader, "compositors are dropped with the host connection, so that they dial again")

	// A compositor that comes back makes a new host connection.
	rig.compositor(alice)

	select {
	case <-rig.hosts:
	case <-time.After(5 * time.Second):
		t.Fatal("no new host connection")
	}
}

// The shipped compositor (seamlessd/, transport_close) does not exit when its connection ends: it
// retries every second and announces all its windows again after the next acknowledgement. So when the host
// connection is lost the hub only has to close every compositor, and each one comes back on its own, windows included.
func TestHubHostLossLetsEveryCompositorReconnectAndAnnounceAgain(t *testing.T) {
	rig := newHubRig(t)

	aliceConn, aliceReader := rig.compositor(alice)
	host := <-rig.hosts

	bobConn, bobReader := rig.compositor(bob)

	_, _ = aliceConn.Write(createWindow(1, 0, "a"))
	_, _ = bobConn.Write(createWindow(1, 0, "b"))

	expect(t, host.received)
	expect(t, host.received)

	host.conn.Close()

	for _, c := range []struct {
		conn   net.Conn
		reader *bufio.Reader
	}{{aliceConn, aliceReader}, {bobConn, bobReader}} {
		expectClosed(t, c.conn, c.reader, "every compositor must be disconnected so that it reconnects")
	}

	// Both reconnect, as the compositor does, and announce their windows again to the new host.
	aliceConn, _ = rig.compositor(alice)
	newHost := <-rig.hosts
	bobConn, _ = rig.compositor(bob)

	_, _ = aliceConn.Write(createWindow(1, 0, "a"))
	_, _ = bobConn.Write(createWindow(1, 0, "b"))

	seen := map[uint32]bool{}

	for range 2 {
		frame := expect(t, newHost.received)

		if frame[4] != gWindowCreate {
			t.Fatalf("expected a window creation, got 0x%x", frame[4])
		}

		seen[binary.LittleEndian.Uint32(frame[5:])] = true
	}

	if len(seen) != 2 {
		t.Fatalf("both compositors' windows must reach the new host with distinct ids, got %v", seen)
	}
}

func TestHubLabelsEveryoneUntilThePrimaryIsKnown(t *testing.T) {
	rig := newHubRig(t)

	aliceConn, _ := rig.compositor(alice)
	host := <-rig.hosts

	if _, err := aliceConn.Write(createWindow(1, 0, "Terminal")); err != nil {
		t.Fatal(err)
	}

	if first := expect(t, host.received); !strings.Contains(string(first), "alice: Terminal") {
		t.Fatalf("with no primary known the title says whose it is: %q", first)
	}

	rig.hub.SetPrimary("alice")

	if _, err := aliceConn.Write(createWindow(2, 0, "Editor")); err != nil {
		t.Fatal(err)
	}

	if second := expect(t, host.received); !strings.Contains(string(second), "Editor") || strings.Contains(string(second), "alice: ") {
		t.Fatalf("once the primary is known its titles are left alone: %q", second)
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
	if !utf8.Valid(out[5+17+2 : 5+17+2+n]) {
		t.Fatal("a character was cut in half")
	}

	if !strings.HasSuffix(string(out), "app") {
		t.Fatal("trailer lost")
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

func TestServeShutdownCancelsPendingHostDial(t *testing.T) {
	rig := newHubRig(t)

	release := make(chan struct{})
	dialing := make(chan struct{})
	late := make(chan net.Conn, 1)

	rig.hub.Dial = func() (net.Conn, error) {
		close(dialing)
		<-release

		hubSide, hostSide := net.Pipe()

		closeOnCleanup(t, hubSide, hostSide)

		late <- hostSide

		return hubSide, nil
	}

	rig.hub.Authenticate = func(net.Conn) (Account, error) { return Account{Name: "alice", UID: 1000}, nil }

	l, err := net.Listen("unix", t.TempDir()+"/hub.sock")

	if err != nil {
		t.Skip("no unix sockets here")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- rig.hub.Serve(ctx, l) }()

	comp, err := net.Dial("unix", l.Addr().String())

	if err != nil {
		t.Fatal(err)
	}

	defer comp.Close()

	if _, err = comp.Write(buildHello(2, "caker-seamlessd")); err != nil {
		t.Fatal(err)
	}

	select {
	case <-dialing:
	case <-time.After(5 * time.Second):
		t.Fatal("the hub never dialled the host")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve must not wait for a host dial that hangs")
	}

	// The dial finishes after the shutdown: its connection must be dropped, not published.
	close(release)

	host := <-late

	_ = host.SetReadDeadline(time.Now().Add(5 * time.Second))

	if _, err = host.Read(make([]byte, 1)); !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) && !errors.Is(err, net.ErrClosed) {
		t.Fatalf("the late host connection must be closed, got: %v", err)
	}

	rig.hub.mu.Lock()
	published := rig.hub.link
	rig.hub.mu.Unlock()

	if published != nil {
		t.Fatal("a host link was published after shutdown")
	}
}

func TestMemBudgetWaitsForRelease(t *testing.T) {
	b := newMemBudget(100)

	if err := b.acquire(80, nil); err != nil {
		t.Fatal(err)
	}

	got := make(chan error, 1)

	go func() { got <- b.acquire(50, nil) }()

	select {
	case <-got:
		t.Fatal("reserved beyond the budget")
	case <-time.After(50 * time.Millisecond):
	}

	b.release(80)

	if err := <-got; err != nil {
		t.Fatal(err)
	}

	if err := b.acquire(101, nil); err == nil {
		t.Fatal("a reservation that can never fit was accepted")
	}
}

func TestMemBudgetCancelledWaitReleasesNothing(t *testing.T) {
	b := newMemBudget(100)

	if err := b.acquire(100, nil); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	got := make(chan error, 1)

	go func() { got <- b.acquire(10, done) }()

	time.Sleep(20 * time.Millisecond)
	close(done)

	if err := <-got; err == nil {
		t.Fatal("a cancelled wait succeeded")
	}

	b.release(100)

	if err := b.acquire(100, nil); err != nil {
		t.Fatalf("the budget leaked: %v", err)
	}
}

func TestReadBudgetedFrameReleasesOnError(t *testing.T) {
	b := newMemBudget(1024)

	// The header promises 100 bytes, only 3 come.
	truncated := append(binary.LittleEndian.AppendUint32(nil, 100), 1, 2, 3)

	if _, _, err := readBudgetedFrame(bytes.NewReader(truncated), 1024, b, nil); err == nil {
		t.Fatal("expected a read error")
	}

	frame, reserved, err := readBudgetedFrame(bytes.NewReader(buildFrame(gWindowDestroy, make([]byte, 4))), 1024, b, nil)

	if err != nil || len(frame) != 9 || reserved != 9 {
		t.Fatalf("frame %d, reserved %d, err %v", len(frame), reserved, err)
	}

	b.release(reserved)

	if err := b.acquire(1024, nil); err != nil {
		t.Fatalf("the budget leaked: %v", err)
	}
}

func TestReadBudgetedFrameLargeFitsWholeBudget(t *testing.T) {
	b := newMemBudget(frameBudget)
	payload := make([]byte, 1<<20)
	frame, reserved, err := readBudgetedFrame(bytes.NewReader(buildFrame(gWindowFrame, payload)), maxGuestFrame, b, nil)

	if err != nil || len(frame) != 5+len(payload) {
		t.Fatalf("len %d, err %v", len(frame), err)
	}

	b.release(reserved)
}

func TestHubStalledCompositorDoesNotHoldUpTheOthers(t *testing.T) {
	rig := newHubRig(t)

	aliceConn, _ := rig.compositor(alice)
	host := <-rig.hosts
	bobConn, bobReader := rig.compositor(bob)

	if _, err := aliceConn.Write(createWindow(1, 0, "Terminal")); err != nil {
		t.Fatal(err)
	}

	// Two connections have no relative order: bob writes once alice's window has reached the host.
	aliceWindow := binary.LittleEndian.Uint32(expect(t, host.received)[5:])

	if _, err := bobConn.Write(createWindow(1, 0, "Terminal")); err != nil {
		t.Fatal(err)
	}

	bobWindow := binary.LittleEndian.Uint32(expect(t, host.received)[5:])

	// Alice never reads what the host sends her: more than her queue holds. The host connection must keep being read.
	done := make(chan struct{})

	go func() {
		defer close(done)

		for i := 0; i < peerQueueLen+16; i++ {
			host.send(buildFrame(hPointerLeave, u32(aliceWindow)))
		}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("a compositor that does not read held up the host connection")
	}

	host.send(buildFrame(hPointerLeave, u32(bobWindow)))

	if back := readOne(t, bobConn, bobReader); back[4] != hPointerLeave {
		t.Fatalf("bob got %x", back)
	}
}

// The protocol has no acknowledgement of a window's destruction, so the host may still send events for a window that
// is gone. They must not reach a window that only reuses the id.
func TestHubDropsHostEventsForWindowsTheCompositorDoesNotOwn(t *testing.T) {
	rig := newHubRig(t)

	aliceConn, aliceReader := rig.compositor(alice)
	host := <-rig.hosts

	_, _ = aliceConn.Write(createWindow(1, 0, "a"))

	created := expect(t, host.received)
	id := binary.LittleEndian.Uint32(created[5:])

	// A window that was never created, then one destroyed by the compositor: both are stale to the compositor.
	host.send(buildFrame(hPointerLeave, u32(id&^localIDMask|7)))

	_, _ = aliceConn.Write(createWindow(2, 0, "b"))

	second := expect(t, host.received)

	_, _ = aliceConn.Write(buildFrame(gWindowDestroy, u32(2)))
	expect(t, host.received)

	host.send(buildFrame(hWindowResize, append(u32(binary.LittleEndian.Uint32(second[5:])), make([]byte, 8)...)))

	// The first event the compositor sees is the one for its live window: the earlier ones were dropped, in order.
	host.send(buildFrame(hPointerLeave, u32(id)))

	if back := readOne(t, aliceConn, aliceReader); back[4] != hPointerLeave || binary.LittleEndian.Uint32(back[5:]) != 1 {
		t.Fatalf("only the event for the live window may arrive, got %x", back)
	}
}

// A slot is not handed out again at once: the host may still have events for the windows of the compositor that left.
func TestHubDoesNotReuseASlotRightAway(t *testing.T) {
	rig := newHubRig(t)

	aliceConn, _ := rig.compositor(alice)
	host := <-rig.hosts
	bobConn, _ := rig.compositor(bob)

	_, _ = aliceConn.Write(createWindow(1, 0, "a"))

	oldID := binary.LittleEndian.Uint32(expect(t, host.received)[5:])

	aliceConn.Close()

	if frame := expect(t, host.received); frame[4] != gWindowDestroy {
		t.Fatalf("expected a destroy, got 0x%x", frame[4])
	}

	carolConn, carolReader := rig.compositor(Account{Name: "carol", UID: 1002, GID: 1002})

	_, _ = carolConn.Write(createWindow(1, 0, "c"))

	newID := binary.LittleEndian.Uint32(expect(t, host.received)[5:])

	if newID == oldID {
		t.Fatalf("a new compositor took over the global id %d of a window that just went", oldID)
	}

	// An event in flight for the old window reaches nobody; the next one for the new window arrives first.
	host.send(buildFrame(hPointerLeave, u32(oldID)))
	host.send(buildFrame(hPointerLeave, u32(newID)))

	if back := readOne(t, carolConn, carolReader); binary.LittleEndian.Uint32(back[5:]) != 1 {
		t.Fatalf("carol got %x", back)
	}

	_ = bobConn
}

func TestHubAttached(t *testing.T) {
	h := &Hub{}

	if h.Attached(1000) {
		t.Fatal("no compositor is attached yet")
	}

	peer := &hubPeer{account: Account{UID: 1000}, done: make(chan struct{})}
	h.peers[1] = peer

	if !h.Attached(1000) {
		t.Fatal("the compositor of 1000 is attached")
	}

	if h.Attached(1001) {
		t.Fatal("nothing of 1001 is attached")
	}

	close(peer.done)

	if h.Attached(1000) {
		t.Fatal("a closed connection is not an attached compositor")
	}
}

func clipboardFrame(typ byte, text string) []byte {
	payload := append([]byte{0}, u32(uint32(len(text)))...)

	return buildFrame(typ, append(payload, text...))
}

func TestHubGivesTheHostClipboardToTheFocusedUserOnly(t *testing.T) {
	rig := newHubRigVersion(t, clipboardVersion)

	aliceConn, aliceReader := rig.compositor(alice)
	host := <-rig.hosts
	bobConn, bobReader := rig.compositor(bob)

	if _, err := aliceConn.Write(createWindow(1, 0, "Terminal")); err != nil {
		t.Fatal(err)
	}

	aliceWindow := binary.LittleEndian.Uint32(expect(t, host.received)[5:])

	if _, err := bobConn.Write(createWindow(1, 0, "Terminal")); err != nil {
		t.Fatal(err)
	}

	expect(t, host.received)

	host.send(buildFrame(hWindowFocus, append(u32(aliceWindow), 1)))
	readOne(t, aliceConn, aliceReader)

	host.send(clipboardFrame(hClipboard, "from the Mac"))

	got := readOne(t, aliceConn, aliceReader)

	if got[4] != hClipboard || string(got[10:]) != "from the Mac" {
		t.Fatalf("alice must get the text: %x", got)
	}

	// Bob's next message is the output scale, not the clipboard.
	host.send(buildFrame(hOutput, u32(360)))

	if out := readOne(t, bobConn, bobReader); out[4] != hOutput {
		t.Fatalf("bob must not see the host's clipboard: %x", out)
	}
}

func TestHubTakesTheClipboardOfTheFocusedUserOnly(t *testing.T) {
	rig := newHubRigVersion(t, clipboardVersion)

	aliceConn, aliceReader := rig.compositor(alice)
	host := <-rig.hosts
	bobConn, _ := rig.compositor(bob)

	if _, err := aliceConn.Write(createWindow(1, 0, "Terminal")); err != nil {
		t.Fatal(err)
	}

	aliceWindow := binary.LittleEndian.Uint32(expect(t, host.received)[5:])

	host.send(buildFrame(hWindowFocus, append(u32(aliceWindow), 1)))
	readOne(t, aliceConn, aliceReader)

	// Bob is in the background: dropped. A connection's frames are handled in order, so a window creation sent after
	// his clipboard and seen by the host proves the clipboard was routed (and dropped) before Alice's is sent.
	_, _ = bobConn.Write(clipboardFrame(gClipboard, "bob's secret"))
	_, _ = bobConn.Write(createWindow(1, 0, "marker"))

	if marker := expect(t, host.received); marker[4] != gWindowCreate {
		t.Fatalf("bob's clipboard must not reach the host, got %x", marker)
	}

	// Alice's goes through, and it is the next thing the host receives.
	_, _ = aliceConn.Write(clipboardFrame(gClipboard, "alice's text"))

	got := expect(t, host.received)

	if got[4] != gClipboard || string(got[10:]) != "alice's text" {
		t.Fatalf("only the focused user's clipboard reaches the host: %x", got)
	}
}

func TestHubRejectsAMalformedClipboard(t *testing.T) {
	for name, payload := range map[string][]byte{
		"length does not match": append([]byte{0}, append(u32(10), 'a', 'b')...),
		"unknown format":        append([]byte{7}, append(u32(1), 'a')...),
		"too short":             {0, 1},
		"over the limit":        append([]byte{0}, u32(maxClipboard+1)...),
		"not UTF-8":             append([]byte{0}, append(u32(2), 0xff, 0xfe)...),
	} {
		t.Run(name, func(t *testing.T) {
			rig := newHubRigVersion(t, clipboardVersion)

			conn, reader := rig.compositor(alice)
			<-rig.hosts

			_, _ = conn.Write(buildFrame(gClipboard, payload))

			expectClosed(t, conn, reader, "the connection must be closed")
		})
	}
}

func TestCheckClipboardAcceptsAnEmptyAndAFullText(t *testing.T) {
	if err := checkClipboard(append([]byte{0}, u32(0)...)); err != nil {
		t.Fatal(err)
	}

	full := append([]byte{0}, u32(maxClipboard)...)

	if err := checkClipboard(append(full, make([]byte, maxClipboard)...)); err != nil {
		t.Fatal(err)
	}
}

func TestHubKeepsClipboardOffProtocolVersion2(t *testing.T) {
	rig := newHubRig(t)

	aliceConn, aliceReader := rig.compositor(alice)
	host := <-rig.hosts
	bobConn, bobReader := rig.compositor(bob)

	if _, err := aliceConn.Write(createWindow(1, 0, "Terminal")); err != nil {
		t.Fatal(err)
	}

	aliceWindow := binary.LittleEndian.Uint32(expect(t, host.received)[5:])

	host.send(buildFrame(hWindowFocus, append(u32(aliceWindow), 1)))
	readOne(t, aliceConn, aliceReader)

	// A clipboard from the host is not delivered: the next thing alice gets is the output scale.
	host.send(clipboardFrame(hClipboard, "from the Mac"))
	host.send(buildFrame(hOutput, u32(360)))

	if out := readOne(t, aliceConn, aliceReader); out[4] != hOutput {
		t.Fatalf("a protocol 2 compositor must not get the clipboard: %x", out)
	}

	if out := readOne(t, bobConn, bobReader); out[4] != hOutput {
		t.Fatalf("bob must not get the clipboard either: %x", out)
	}

	// A clipboard from a compositor never reaches the host, and ends that compositor only.
	_, _ = aliceConn.Write(clipboardFrame(gClipboard, "unknown to the host"))

	expectClosed(t, aliceConn, aliceReader, "the compositor must be disconnected")

	// The host sees alice's window go, not her clipboard, and the shared connection lives on for bob.
	if got := expect(t, host.received); got[4] != gWindowDestroy {
		t.Fatalf("the host must see no clipboard: %x", got)
	}

	if _, err := bobConn.Write(createWindow(1, 0, "Terminal")); err != nil {
		t.Fatal(err)
	}

	if got := expect(t, host.received); got[4] != gWindowCreate {
		t.Fatalf("the shared host connection must live on: %x", got)
	}
}
