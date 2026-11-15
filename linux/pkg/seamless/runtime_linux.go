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

// runtimeDirCandidates lists where the compositor's socket may be: the logind directory first, then our own.
func runtimeDirCandidates(a Account) []string {
	return []string{fmt.Sprintf("/run/user/%d", a.UID), filepath.Join("/run/caker-seamless", strconv.Itoa(int(a.UID)))}
}

// liveRuntimeDir returns the directory holding the socket of a running compositor, if any. The directory RuntimeDir
// would pick can change while the compositor runs (/run/user/<uid> appears when the user logs in), so what
// applications are given must follow the compositor, not the choice RuntimeDir makes now.
func liveRuntimeDir(cfg Config, a Account) (string, bool) {
	for _, dir := range runtimeDirCandidates(a) {
		if conn, err := net.DialTimeout("unix", filepath.Join(dir, cfg.SocketName), time.Second); err == nil {
			conn.Close()

			return dir, true
		}
	}

	return "", false
}

// appRuntimeDir is the runtime directory applications must use: the running compositor's, else RuntimeDir's.
func appRuntimeDir(cfg Config, a Account) (string, error) {
	if dir, ok := liveRuntimeDir(cfg, a); ok {
		return dir, nil
	}

	return ensureRuntimeDir(a)
}

// RuntimeDir returns the account's XDG runtime directory, creating it when nobody is logged in: /run/user/<uid> if
// logind can provide it (it also holds the user's D-Bus and audio sockets), else a private directory of ours.
func RuntimeDir(a Account) (string, error) {
	dir := runtimeDirCandidates(a)[0]

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

	dir = runtimeDirCandidates(a)[1]

	// The parent is created apart from the user's directory: MkdirAll would give it 0700 too, and, owned by root, it
	// could not be traversed once the compositor drops privileges.
	parent := filepath.Dir(dir)

	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}

	// Only repair the parent when it cannot be traversed: an unprivileged caller may not chmod a parent owned by root,
	// and has no need to when it is already usable.
	info, err := os.Stat(parent)

	if err != nil {
		return "", err
	}

	if info.Mode().Perm()&0o001 == 0 {
		if err := os.Chmod(parent, 0o755); err != nil {
			return "", err
		}
	}

	if err := os.Mkdir(dir, 0o700); err != nil && !os.IsExist(err) {
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

	env := baseEnv(a, runtimeDir)

	// Applications started outside a login session find no bus address and would try to launch their own with X11
	// (Firefox's portal and accessibility lookups, GTK settings): point them at the user's bus.
	if bus := filepath.Join(runtimeDir, "bus"); fileExists(bus) {
		env = append(env, "DBUS_SESSION_BUS_ADDRESS=unix:path="+bus)
	}

	return append(env,
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

// maxLogLine bounds the part of a line that is logged; the rest of a longer line is discarded.
const maxLogLine = 64 * 1024

// logLines forwards a process' output, line by line, to the agent's log. It always keeps draining the pipe, so the
// process never blocks on a full pipe: a line longer than maxLogLine is truncated, not fatal, and a read error is
// reported, then whatever remains is discarded.
func logLines(prefix string, r io.Reader) {
	reader := bufio.NewReaderSize(r, maxLogLine)

	var line []byte

	truncated := false

	for {
		chunk, isPrefix, err := reader.ReadLine()

		if len(line) < maxLogLine {
			line = append(line, chunk[:min(len(chunk), maxLogLine-len(line))]...)
		}

		if isPrefix {
			truncated = truncated || len(line) >= maxLogLine
		}

		if err == nil && isPrefix {
			continue
		}

		if len(line) > 0 || err == nil {
			if truncated {
				glog.Infof("%s: %s [line truncated]", prefix, line)
			} else {
				glog.Infof("%s: %s", prefix, line)
			}
		}

		line, truncated = line[:0], false

		if err != nil {
			if err != io.EOF {
				glog.Warnf("%s: reading output: %v", prefix, err)

				// Keep the pipe drained even after an error.
				_, _ = io.Copy(io.Discard, r)
			}

			return
		}
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

// systemXKBDirs are where a guest's xkb-data / xkeyboard-config package puts the XKB data, in the order libxkbcommon
// (the compositor's, 1.11 and later) tries them without XKB_CONFIG_ROOT: the versioned root, then the legacy one, which
// it only falls back to when the versioned directory does not exist; replaceable by tests.
var systemXKBDirs = []string{"/usr/share/xkeyboard-config-2", "/usr/share/X11/xkb"}

// hasXKBData tells whether dir holds the XKB data a keymap is compiled from.
func hasXKBData(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "rules", "evdev"))

	return err == nil && info.Mode().IsRegular()
}

// systemXKBServes tells whether the compositor, left to its defaults, finds XKB data in the guest: only the first
// existing directory of systemXKBDirs is its root, so data in a later one is ignored while an earlier one exists.
func systemXKBServes() bool {
	for _, dir := range systemXKBDirs {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return hasXKBData(dir)
		}
	}

	return false
}

// xkbConfigRoot returns the XKB data directory the compositor must be told about (XKB_CONFIG_ROOT), or "" to leave
// the guest's own data in charge: the guest's data comes first, so its layouts and customisations keep applying; the
// copy installed beside the compositor only serves guests where the compositor would find none.
func xkbConfigRoot(binary string) string {
	if systemXKBServes() {
		return ""
	}

	if dir := XKBDir(binary); hasXKBData(dir) {
		return dir
	}

	return ""
}

// runOnce starts the compositor and waits for it to exit.
func runOnce(ctx context.Context, cfg Config, a Account, runtimeDir string) error {
	args := []string{"--connect", cfg.Connect(), "--socket", cfg.SocketName}
	cmd := exec.CommandContext(ctx, cfg.Binary, args...)

	cmd.Env = append(baseEnv(a, runtimeDir), "WLR_RENDERER=pixman")

	if cfg.Layout != "" {
		cmd.Env = append(cmd.Env, "XKB_DEFAULT_LAYOUT="+cfg.Layout)
	}

	if root := xkbConfigRoot(cfg.Binary); root != "" {
		cmd.Env = append(cmd.Env, "XKB_CONFIG_ROOT="+root)
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

	actCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go exportActivationEnv(actCtx, cfg, a, runtimeDir)

	return cmd.Wait()
}

// activationVars is what a service that D-Bus or systemd starts on the user's behalf (gnome-terminal-server, the
// portals, Nautilus' and Files' services...) needs to reach the compositor. An application started by "launch" gets it
// from its own environment, but those services are children of the user's systemd and D-Bus, which have none.
func activationVars(cfg Config) []string {
	cfg = cfg.WithDefaults()

	return []string{
		"WAYLAND_DISPLAY=" + cfg.SocketName,
		"XDG_SESSION_TYPE=wayland",
		"GDK_BACKEND=wayland",
		"QT_QPA_PLATFORM=wayland",
		"SDL_VIDEODRIVER=wayland",
		"MOZ_ENABLE_WAYLAND=1",
		"ELECTRON_OZONE_PLATFORM_HINT=wayland",
	}
}

// activationCommands returns the commands that put vars into the user's D-Bus activation environment and systemd user
// manager, the first one that works is enough: dbus-update-activation-environment --systemd does both, the second
// is the systemd half alone.
func activationCommands(vars []string) [][]string {
	names := make([]string, 0, len(vars))

	for _, kv := range vars {
		if name, _, ok := strings.Cut(kv, "="); ok {
			names = append(names, name)
		}
	}

	return [][]string{
		append([]string{"dbus-update-activation-environment", "--systemd"}, vars...),
		append([]string{"systemctl", "--user", "import-environment"}, names...),
	}
}

// exportActivationEnv publishes the compositor's address to the user's session bus and systemd, so that services
// started by activation open their windows on it. The user manager can start after the agent (the account is lingering
// at boot), hence the retries.
func exportActivationEnv(ctx context.Context, cfg Config, a Account, runtimeDir string) {
	cfg = cfg.WithDefaults()

	vars := activationVars(cfg)
	bus := filepath.Join(runtimeDir, "bus")
	socket := filepath.Join(runtimeDir, cfg.SocketName)

	for attempt := 0; attempt < 30 && ctx.Err() == nil; attempt++ {
		// The services only matter once the compositor listens and the user's bus exists.
		if _, err := os.Stat(socket); err == nil {
			if _, err = os.Stat(bus); err == nil && runActivationCommands(ctx, a, runtimeDir, bus, vars) {
				glog.Infof("seamless: Wayland display exported to %s's session services", a.Name)
				return
			}
		}

		sleep(ctx, 2*time.Second)
	}

	if ctx.Err() == nil {
		glog.Warnf("seamless: could not export the Wayland display to %s's session services, applications that D-Bus starts (gnome-terminal, Files...) won't find the compositor", a.Name)
	}
}

func runActivationCommands(ctx context.Context, a Account, runtimeDir, bus string, vars []string) bool {
	for _, args := range activationCommands(vars) {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)

		cmd.Env = append(baseEnv(a, runtimeDir), "DBUS_SESSION_BUS_ADDRESS=unix:path="+bus)

		// systemctl --user import-environment reads the values from its own environment.
		cmd.Env = append(cmd.Env, vars...)
		cmd.Dir = "/"
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: credential(a), Setpgid: true}

		if out, err := cmd.CombinedOutput(); err != nil {
			glog.Debugf("seamless: %s: %v: %s", args[0], err, strings.TrimSpace(string(out)))
			continue
		}

		return true
	}

	return false
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

		// An agent updated in place finds the binary of the previous one: its XKB data is still to come, or its copy was interrupted.
		if _, err = os.Stat(cfg.Binary); err != nil || (embeddedXKB() != nil && !XKBInstalled(cfg.Binary)) {
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

	runtimeDir, err := appRuntimeDir(cfg, a)

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

	// The Exec line has quoting rules of its own: decode them, no shell must interpret the line.
	command, err := SplitExec(app.Exec)

	if err != nil {
		return 0, err
	}

	return Launch(cfg, command)
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

	for _, dir := range runtimeDirCandidates(a) {
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

	runtimeDir, err := appRuntimeDir(cfg, a)

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

func fileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}
