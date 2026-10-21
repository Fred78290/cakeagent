//go:build windows

package display

import (
	"os"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	glog "github.com/sirupsen/logrus"
)

const (
	// The installer sets FollowDisplaySize (DWORD) to 1 under this key on guests whose GPU driver copes with the guest
	// changing its mode. With the stock virtio-win driver Virtualization.framework shows a corrupted picture after a
	// resolution change, so following the host's size stays off unless the installer asked for it.
	settingsKey   = `SOFTWARE\Caker`
	settingsValue = "FollowDisplaySize"

	superviseInterval = 2 * time.Second
	noSession         = 0xFFFFFFFF
)

// Enabled tells whether this guest follows the size the host asks for.
func Enabled() bool {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, settingsKey, registry.QUERY_VALUE)

	if err != nil {
		return false
	}

	defer key.Close()

	value, _, err := key.GetIntegerValue(settingsValue)

	return err == nil && value != 0
}

// helper is a `cakeagent display-watch` process running in a user's session.
type helper struct {
	process windows.Handle
	session uint32
}

func (h *helper) exited() bool {
	event, err := windows.WaitForSingleObject(h.process, 0)

	return err != nil || event != uint32(windows.WAIT_TIMEOUT)
}

func (h *helper) kill() {
	if !h.exited() {
		windows.TerminateProcess(h.process, 0)
	}

	windows.CloseHandle(h.process)
}

// startHelper runs `<this executable> display-watch` in the session, as its user. It fails with ERROR_NO_TOKEN while
// nobody is logged on there (the logon screen), which is expected.
func startHelper(session uint32) (*helper, error) {
	var token windows.Token

	// Only a service running as SYSTEM may ask for another session's user token
	if err := windows.WTSQueryUserToken(session, &token); err != nil {
		return nil, err
	}

	defer token.Close()

	var primary windows.Token

	if err := windows.DuplicateTokenEx(token, windows.MAXIMUM_ALLOWED, nil, windows.SecurityIdentification, windows.TokenPrimary, &primary); err != nil {
		return nil, err
	}

	defer primary.Close()

	var environment *uint16

	if err := windows.CreateEnvironmentBlock(&environment, primary, false); err != nil {
		return nil, err
	}

	defer windows.DestroyEnvironmentBlock(environment)

	executable, err := os.Executable()

	if err != nil {
		return nil, err
	}

	commandLine, err := windows.UTF16PtrFromString(syscall.EscapeArg(executable) + " display-watch")

	if err != nil {
		return nil, err
	}

	desktop, err := windows.UTF16PtrFromString(`winsta0\default`)

	if err != nil {
		return nil, err
	}

	startup := windows.StartupInfo{Desktop: desktop}

	startup.Cb = uint32(unsafe.Sizeof(startup))

	var info windows.ProcessInformation

	if err = windows.CreateProcessAsUser(primary, nil, commandLine, nil, nil, false, windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_NO_WINDOW, environment, nil, &startup, &info); err != nil {
		return nil, err
	}

	windows.CloseHandle(info.Thread)

	return &helper{process: info.Process, session: session}, nil
}

// Supervise keeps a display-watch helper running in the active console session while Enabled, until stop is closed:
// it starts one when someone logs on, replaces it when the console session changes, and restarts it if it dies.
// It is meant for the service, which can't follow the display itself.
func Supervise(stop <-chan struct{}) {
	var (
		current *helper
		failed  uint32 = noSession
	)

	ticker := time.NewTicker(superviseInterval)

	defer ticker.Stop()
	defer func() {
		if current != nil {
			current.kill()
		}
	}()

	for {
		enabled := Enabled()
		session := windows.WTSGetActiveConsoleSessionId()

		if current != nil && (!enabled || session != current.session || current.exited()) {
			current.kill()

			current = nil
		}

		if current == nil && enabled && session != noSession {
			if h, err := startHelper(session); err == nil {
				glog.Infof("display: following the host's size in session %d", session)

				current = h
				failed = noSession
			} else if failed != session && err != windows.ERROR_NO_TOKEN {
				// Nobody logged on yet is normal; anything else is worth saying once per session
				glog.Warnf("display: can't start the helper in session %d: %v", session, err)

				failed = session
			}
		}

		select {
		case <-stop:
			return
		case <-ticker.C:
		}
	}
}
