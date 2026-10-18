//go:build windows
// +build windows

package pkg

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// AF_VSOCK on Windows comes from the virtio-win "viosock" driver (virtio-vsock). Its Winsock provider is registered by
// the driver package, and the address family number is not fixed: the driver reports it through an ioctl on
// \\.\Viosock (ViosockGetAF in viosock/inc/vio_sockets.h). 40 is what Linux uses and what viosock has always reported.
const (
	viosockDevice      = `\\.\Viosock`
	viosockIoctlGetAF  = 0x0801300C
	viosockDefaultAF   = 40
	socketError        = ^uintptr(0)
	invalidSocket      = ^uintptr(0)
	wsaVersion         = 0x0202
	solSocket          = 0xffff
	soRcvTimeo         = 0x1006
	soSndTimeo         = 0x1005
	wsaETimedOut       = syscall.Errno(10060)
	wsaEInterrupted    = syscall.Errno(10004)
	wsaENotSocket      = syscall.Errno(10038)
	maxIO              = 1 << 30
	vsockListenBacklog = 128
)

var (
	ws2_32     = windows.NewLazySystemDLL("ws2_32.dll")
	procBind   = ws2_32.NewProc("bind")
	procAccept = ws2_32.NewProc("accept")

	wsaOnce sync.Once
	wsaErr  error
)

// sockaddrVM is struct sockaddr_vm of vio_sockets.h: it is 12 bytes, unlike Linux's 16.
type sockaddrVM struct {
	family    uint16
	reserved1 uint16
	port      uint32
	cid       uint32
}

type vsockAddr struct {
	cid  uint32
	port uint32
}

func (a vsockAddr) Network() string { return "vsock" }
func (a vsockAddr) String() string  { return fmt.Sprintf("%d:%d", a.cid, a.port) }

func startWinsock() error {
	wsaOnce.Do(func() {
		var data windows.WSAData

		wsaErr = windows.WSAStartup(wsaVersion, &data)
	})

	return wsaErr
}

// vsockAddressFamily asks the viosock driver for its address family and falls back to 40, which is what it reports.
func vsockAddressFamily() int32 {
	name, err := windows.UTF16PtrFromString(viosockDevice)

	if err != nil {
		return viosockDefaultAF
	}

	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)

	if err != nil {
		return viosockDefaultAF
	}

	defer windows.CloseHandle(handle)

	var af, returned uint32

	if err = windows.DeviceIoControl(handle, viosockIoctlGetAF, nil, 0, (*byte)(unsafe.Pointer(&af)), uint32(unsafe.Sizeof(af)), &returned, nil); err != nil || af == 0 {
		return viosockDefaultAF
	}

	return int32(af)
}

// listenVsock listens on a virtio-vsock port. cid is the context id to bind (vsockCIDAny for the guest's own).
func listenVsock(cid uint32, port uint32) (net.Listener, error) {
	if err := startWinsock(); err != nil {
		return nil, fmt.Errorf("WSAStartup failed: %w", err)
	}

	family := vsockAddressFamily()
	handle, err := windows.Socket(int(family), windows.SOCK_STREAM, 0)

	if err != nil {
		return nil, fmt.Errorf("cannot create an AF_VSOCK socket (is the viosock driver installed?): %w", err)
	}

	addr := sockaddrVM{family: uint16(family), port: port, cid: cid}

	if r, _, errno := procBind.Call(uintptr(handle), uintptr(unsafe.Pointer(&addr)), unsafe.Sizeof(addr)); r == socketError {
		windows.Closesocket(handle)

		return nil, fmt.Errorf("cannot bind vsock port %d: %w", port, errno)
	}

	if err = windows.Listen(handle, vsockListenBacklog); err != nil {
		windows.Closesocket(handle)

		return nil, fmt.Errorf("cannot listen on vsock port %d: %w", port, err)
	}

	return &vsockListener{handle: handle, addr: vsockAddr{cid: cid, port: port}}, nil
}

type vsockListener struct {
	handle windows.Handle
	addr   vsockAddr
	closed atomic.Bool
}

func (l *vsockListener) Accept() (net.Conn, error) {
	for {
		var peer sockaddrVM

		size := int32(unsafe.Sizeof(peer))
		r, _, errno := procAccept.Call(uintptr(l.handle), uintptr(unsafe.Pointer(&peer)), uintptr(unsafe.Pointer(&size)))

		if r == invalidSocket {
			if l.closed.Load() {
				return nil, net.ErrClosed
			}

			if errno == wsaEInterrupted {
				continue
			}

			return nil, &net.OpError{Op: "accept", Net: "vsock", Addr: l.addr, Err: errno}
		}

		return &vsockConn{handle: windows.Handle(r), local: l.addr, remote: vsockAddr{cid: peer.cid, port: peer.port}}, nil
	}
}

func (l *vsockListener) Close() error {
	if !l.closed.CompareAndSwap(false, true) {
		return nil
	}

	return windows.Closesocket(l.handle)
}

func (l *vsockListener) Addr() net.Addr { return l.addr }

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// vsockConn is a blocking Winsock stream. Deadlines are applied as SO_RCVTIMEO/SO_SNDTIMEO before each call,
// which is how the driver's provider is told to give up (grpc only sets them around the TLS handshake).
type vsockConn struct {
	handle        windows.Handle
	local, remote vsockAddr
	closed        atomic.Bool

	mu            sync.Mutex
	readDeadline  time.Time
	writeDeadline time.Time
}

func (c *vsockConn) timeout(deadline time.Time, option int32) error {
	var ms int32

	if !deadline.IsZero() {
		remaining := time.Until(deadline)

		if remaining <= 0 {
			return os.ErrDeadlineExceeded
		}

		ms = int32(remaining / time.Millisecond)

		if ms == 0 {
			ms = 1
		}
	}

	// A provider that doesn't support the option just never times out: not an error
	_ = windows.SetsockoptInt(c.handle, solSocket, int(option), int(ms))

	return nil
}

func (c *vsockConn) deadlines() (read, write time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.readDeadline, c.writeDeadline
}

func (c *vsockConn) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}

	if c.closed.Load() {
		return 0, net.ErrClosed
	}

	read, _ := c.deadlines()

	if err := c.timeout(read, soRcvTimeo); err != nil {
		return 0, err
	}

	if len(b) > maxIO {
		b = b[:maxIO]
	}

	var received, flags uint32

	buf := windows.WSABuf{Len: uint32(len(b)), Buf: &b[0]}

	if err := windows.WSARecv(c.handle, &buf, 1, &received, &flags, nil, nil); err != nil {
		switch {
		case c.closed.Load():
			return 0, net.ErrClosed
		case err == wsaETimedOut:
			return 0, timeoutError{}
		default:
			return 0, &net.OpError{Op: "read", Net: "vsock", Source: c.local, Addr: c.remote, Err: err}
		}
	}

	if received == 0 {
		return 0, io.EOF
	}

	return int(received), nil
}

func (c *vsockConn) Write(b []byte) (int, error) {
	written := 0

	for written < len(b) {
		if c.closed.Load() {
			return written, net.ErrClosed
		}

		_, write := c.deadlines()

		if err := c.timeout(write, soSndTimeo); err != nil {
			return written, err
		}

		chunk := b[written:]

		if len(chunk) > maxIO {
			chunk = chunk[:maxIO]
		}

		var sent uint32

		buf := windows.WSABuf{Len: uint32(len(chunk)), Buf: &chunk[0]}

		if err := windows.WSASend(c.handle, &buf, 1, &sent, 0, nil, nil); err != nil {
			switch {
			case c.closed.Load():
				return written, net.ErrClosed
			case err == wsaETimedOut:
				return written, timeoutError{}
			default:
				return written, &net.OpError{Op: "write", Net: "vsock", Source: c.local, Addr: c.remote, Err: err}
			}
		}

		written += int(sent)
	}

	return written, nil
}

func (c *vsockConn) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}

	return windows.Closesocket(c.handle)
}

func (c *vsockConn) LocalAddr() net.Addr  { return c.local }
func (c *vsockConn) RemoteAddr() net.Addr { return c.remote }

func (c *vsockConn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.readDeadline, c.writeDeadline = t, t

	return nil
}

func (c *vsockConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.readDeadline = t

	return nil
}

func (c *vsockConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.writeDeadline = t

	return nil
}
