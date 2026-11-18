//go:build linux

package seamless

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/mdlayher/vsock"
	"golang.org/x/sys/unix"
)

// peerUID is the uid of the process on the other end of a unix connection, as the kernel recorded it.
func peerUID(conn net.Conn) (uint32, error) {
	unixConn, ok := conn.(*net.UnixConn)

	if !ok {
		return 0, fmt.Errorf("not a unix connection")
	}

	raw, err := unixConn.SyscallConn()

	if err != nil {
		return 0, err
	}

	var cred *unix.Ucred
	var credErr error

	if err = raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return 0, err
	}

	if credErr != nil {
		return 0, credErr
	}

	return cred.Uid, nil
}

// authenticateCompositor names the account a compositor connection comes from. Root is refused: the compositors
// run as the guest's users (the agent itself never has one), as everything else about seamless windows.
func authenticateCompositor(conn net.Conn) (Account, error) {
	uid, err := peerUID(conn)

	if err != nil {
		return Account{}, err
	}

	if uid == 0 {
		return Account{}, fmt.Errorf("a root compositor is refused")
	}

	file, err := os.Open("/etc/passwd")

	if err != nil {
		return Account{}, err
	}

	defer file.Close()

	for _, a := range ParsePasswd(file) {
		if a.UID == uid {
			return a, nil
		}
	}

	return Account{}, fmt.Errorf("uid %d is not in /etc/passwd", uid)
}

// newGuestHub makes the hub the compositors of this guest connect to, and the listener it serves. The socket is
// open to every local user, like the host's vsock port the compositors used to dial directly: what counts is
// who the kernel says is on the other end.
func newGuestHub(cfg Config) (*Hub, net.Listener, error) {
	cfg = cfg.WithDefaults()

	dir := filepath.Dir(cfg.HubSocket)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, err
	}

	// RuntimeDir makes the same directory for guests without a logind: it must be traversable by everyone.
	if info, err := os.Stat(dir); err == nil && info.Mode().Perm()&0o001 == 0 {
		if err = os.Chmod(dir, 0o755); err != nil {
			return nil, nil, err
		}
	}

	_ = os.Remove(cfg.HubSocket)

	listener, err := net.Listen("unix", cfg.HubSocket)

	if err != nil {
		return nil, nil, fmt.Errorf("seamless hub: %w", err)
	}

	if err = os.Chmod(cfg.HubSocket, 0o666); err != nil {
		listener.Close()

		return nil, nil, err
	}

	hub := &Hub{
		Dial: func() (net.Conn, error) {
			return vsock.Dial(vsock.Host, uint32(cfg.Port), nil)
		},
		Authenticate: authenticateCompositor,
	}

	return hub, listener, nil
}
