//go:build linux

package seamless

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	glog "github.com/sirupsen/logrus"
)

// LookupAccount finds name in /etc/passwd, or the first regular user when name is empty.
func LookupAccount(name string) (Account, error) {
	file, err := os.Open("/etc/passwd")

	if err != nil {
		return Account{}, err
	}

	defer file.Close()

	return PickAccount(ParsePasswd(file), name)
}

// ensureRuntimeDir is RuntimeDir, replaceable by tests.
var ensureRuntimeDir = RuntimeDir

// RuntimeDir returns the account's XDG runtime directory, creating it when nobody is logged in: /run/user/<uid> if
// logind can provide it (it also holds the user's D-Bus and audio sockets), else a private directory of ours.
func RuntimeDir(a Account) (string, error) {
	dir := fmt.Sprintf("/run/user/%d", a.UID)

	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return dir, nil
	}

	if os.Getuid() == 0 {
		// Lingering makes systemd keep the user's manager, and its runtime directory, without a login.
		if path, err := exec.LookPath("loginctl"); err == nil {
			_ = exec.Command(path, "enable-linger", a.Name).Run()

			for i := 0; i < 50; i++ {
				if info, err := os.Stat(dir); err == nil && info.IsDir() {
					return dir, nil
				}

				time.Sleep(100 * time.Millisecond)
			}
		}
	}

	dir = filepath.Join("/run/caker-seamless", strconv.Itoa(int(a.UID)))

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}

	if os.Getuid() == 0 {
		if err := os.Chown(dir, int(a.UID), int(a.GID)); err != nil {
			return "", err
		}
	}

	return dir, os.Chmod(dir, 0o700)
}

func credential(a Account) *syscall.Credential {
	if uint32(os.Getuid()) == a.UID {
		return nil
	}

	var groups []uint32

	if u, err := user.LookupId(strconv.Itoa(int(a.UID))); err == nil {
		if ids, err := u.GroupIds(); err == nil {
			for _, id := range ids {
				if gid, err := strconv.ParseUint(id, 10, 32); err == nil {
					groups = append(groups, uint32(gid))
				}
			}
		}
	}

	return &syscall.Credential{Uid: a.UID, Gid: a.GID, Groups: groups}
}

// systemLang is the guest's default locale, as set by /etc/default/locale or /etc/locale.conf.
func systemLang() string {
	for _, path := range []string{"/etc/default/locale", "/etc/locale.conf"} {
		if data, err := os.ReadFile(path); err == nil {
			if lang := ParseLang(string(data)); lang != "" {
				return lang
			}
		}
	}

	return "C.UTF-8"
}

func baseEnv(a Account, runtimeDir string) []string {
	home := a.Home

	if home == "" {
		home = "/"
	}

	return []string{
		"LANG=" + systemLang(),
		"HOME=" + home,
		"USER=" + a.Name,
		"LOGNAME=" + a.Name,
		"SHELL=" + a.Shell,
		"PATH=/usr/local/bin:/usr/bin:/bin:/usr/local/sbin:/usr/sbin:/sbin",
		"XDG_RUNTIME_DIR=" + runtimeDir,
	}
}

// AppEnv is the environment applications need to find the compositor.
func AppEnv(cfg Config, a Account, runtimeDir string) []string {
	cfg = cfg.WithDefaults()

	return append(baseEnv(a, runtimeDir),
		"WAYLAND_DISPLAY="+cfg.SocketName,
		"XDG_SESSION_TYPE=wayland",
		"GDK_BACKEND=wayland",
		"QT_QPA_PLATFORM=wayland",
		"SDL_VIDEODRIVER=wayland",
		"MOZ_ENABLE_WAYLAND=1",
		"ELECTRON_OZONE_PLATFORM_HINT=wayland",
		"_JAVA_AWT_WM_NONREPARENTING=1",
	)
}

// logLines forwards a process' output, line by line, to the agent's log.
func logLines(prefix string, r io.Reader) {
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		glog.Infof("%s: %s", prefix, scanner.Text())
	}
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// runOnce starts the compositor and waits for it to exit.
func runOnce(ctx context.Context, cfg Config, a Account, runtimeDir string) error {
	args := []string{"--connect", cfg.Connect(), "--socket", cfg.SocketName}
	cmd := exec.CommandContext(ctx, cfg.Binary, args...)

	cmd.Env = append(baseEnv(a, runtimeDir), "WLR_RENDERER=pixman")

	if cfg.Layout != "" {
		cmd.Env = append(cmd.Env, "XKB_DEFAULT_LAYOUT="+cfg.Layout)
	}

	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: credential(a), Setpgid: true, Pdeathsig: syscall.SIGTERM}
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 5 * time.Second

	stdout, err := cmd.StdoutPipe()

	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()

	if err != nil {
		return err
	}

	if err = cmd.Start(); err != nil {
		return err
	}

	glog.Infof("seamless: compositor started as %s (pid %d), host on %s", a.Name, cmd.Process.Pid, cfg.Connect())

	go logLines("caker-seamlessd", stdout)
	go logLines("caker-seamlessd", stderr)

	return cmd.Wait()
}

// Run keeps the compositor running for the guest's user until ctx is cancelled: it waits for the user to exist,
// restarts the compositor when it exits (slowing down while it keeps failing) and stops it with ctx.
func Run(ctx context.Context, cfg Config) error {
	cfg = cfg.WithDefaults()
	backoff := time.Second

	for ctx.Err() == nil {
		a, err := LookupAccount(cfg.User)

		if err != nil {
			glog.Debugf("seamless: waiting for a user: %v", err)
			sleep(ctx, 5*time.Second)
			continue
		}

		if _, err = os.Stat(cfg.Binary); err != nil {
			if _, _, err = Install(cfg.Binary); err != nil {
				glog.Errorf("seamless: %v", err)
				sleep(ctx, 30*time.Second)
				continue
			}
		}

		runtimeDir, err := ensureRuntimeDir(a)

		if err != nil {
			glog.Errorf("seamless: no runtime directory for %s: %v", a.Name, err)
			sleep(ctx, 10*time.Second)
			continue
		}

		started := time.Now()
		err = runOnce(ctx, cfg, a, runtimeDir)

		if ctx.Err() != nil {
			return nil
		}

		wait := backoff

		if time.Since(started) > time.Minute {
			backoff, wait = time.Second, time.Second
		} else if backoff < 30*time.Second {
			backoff *= 2
		}

		glog.Warnf("seamless: compositor exited (%v), restarting in %s", err, wait)
		sleep(ctx, wait)
	}

	return nil
}

// Launch starts a command as the guest's user, detached, with the compositor's environment. It returns the pid.
func Launch(cfg Config, command []string) (int, error) {
	cfg = cfg.WithDefaults()

	if len(command) == 0 {
		return 0, fmt.Errorf("nothing to launch")
	}

	a, err := LookupAccount(cfg.User)

	if err != nil {
		return 0, err
	}

	runtimeDir, err := ensureRuntimeDir(a)

	if err != nil {
		return 0, err
	}

	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)

	if err != nil {
		return 0, err
	}

	defer devNull.Close()

	cmd := exec.Command(command[0], command[1:]...)

	cmd.Env = AppEnv(cfg, a, runtimeDir)
	cmd.Dir = a.Home

	if cmd.Dir == "" {
		cmd.Dir = "/"
	}

	cmd.Stdin, cmd.Stdout, cmd.Stderr = devNull, devNull, devNull
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: credential(a), Setsid: true}

	if err = cmd.Start(); err != nil {
		return 0, err
	}

	pid := cmd.Process.Pid

	// Reap it when it exits, the launcher is not waiting.
	go func() { _ = cmd.Wait() }()

	return pid, nil
}

// LaunchDesktop launches an application by desktop file ID.
func LaunchDesktop(cfg Config, id string) (int, error) {
	a, err := LookupAccount(cfg.User)

	if err != nil {
		return 0, err
	}

	app, ok := FindApp(ListApps(ApplicationDirs(a.Home)), id)

	if !ok {
		return 0, fmt.Errorf("application %q not found", id)
	}

	// The Exec line has quoting rules of its own: let the shell apply them.
	return Launch(cfg, []string{"/bin/sh", "-c", "exec " + app.Exec})
}

// Status describes what is installed and running.
type Status struct {
	Binary         string `json:"binary"`
	Installed      bool   `json:"installed"`
	User           string `json:"user,omitempty"`
	RuntimeDir     string `json:"runtimeDir,omitempty"`
	WaylandDisplay string `json:"waylandDisplay"`
	Running        bool   `json:"running"`
}

// CurrentStatus checks the binary and whether the compositor's Wayland socket answers.
func CurrentStatus(cfg Config) Status {
	cfg = cfg.WithDefaults()
	st := Status{Binary: cfg.Binary, WaylandDisplay: cfg.SocketName}

	if _, err := os.Stat(cfg.Binary); err == nil {
		st.Installed = true
	}

	a, err := LookupAccount(cfg.User)

	if err != nil {
		return st
	}

	st.User = a.Name

	for _, dir := range []string{fmt.Sprintf("/run/user/%d", a.UID), filepath.Join("/run/caker-seamless", strconv.Itoa(int(a.UID)))} {
		path := filepath.Join(dir, cfg.SocketName)

		if conn, err := net.DialTimeout("unix", path, time.Second); err == nil {
			conn.Close()
			st.RuntimeDir = dir
			st.Running = true

			return st
		}

		if _, err := os.Stat(dir); err == nil && st.RuntimeDir == "" {
			st.RuntimeDir = dir
		}
	}

	return st
}

// Shell quoting for display of an environment line.
func Env(cfg Config) ([]string, error) {
	cfg = cfg.WithDefaults()

	a, err := LookupAccount(cfg.User)

	if err != nil {
		return nil, err
	}

	runtimeDir, err := ensureRuntimeDir(a)

	if err != nil {
		return nil, err
	}

	var out []string

	for _, kv := range AppEnv(cfg, a, runtimeDir) {
		if strings.HasPrefix(kv, "XDG_RUNTIME_DIR=") || strings.HasPrefix(kv, "WAYLAND_DISPLAY=") || strings.HasPrefix(kv, "XDG_SESSION_TYPE=") || strings.Contains(kv, "WAYLAND=") || strings.HasPrefix(kv, "GDK_") || strings.HasPrefix(kv, "QT_") || strings.HasPrefix(kv, "MOZ_") || strings.HasPrefix(kv, "ELECTRON_") || strings.HasPrefix(kv, "SDL_") {
			out = append(out, kv)
		}
	}

	return out, nil
}
