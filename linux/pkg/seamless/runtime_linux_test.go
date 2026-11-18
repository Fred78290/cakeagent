//go:build linux

package seamless

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testConfig(t *testing.T, script string) (Config, string) {
	t.Helper()

	current, err := user.Current()

	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	binary := filepath.Join(dir, "compositor")

	if err = os.WriteFile(binary, []byte("#!/bin/sh\n"+strings.ReplaceAll(script, "$OUT", out)+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	runtime := filepath.Join(dir, "run")

	if err = os.Mkdir(runtime, 0o700); err != nil {
		t.Fatal(err)
	}

	previous := ensureRuntimeDir

	ensureRuntimeDir = func(Account) (string, error) { return runtime, nil }

	t.Cleanup(func() { ensureRuntimeDir = previous })

	// The hub socket lives in the test's directory: the default one is under /run, which only root can create.
	return Config{User: current.Username, Binary: binary, Layout: "fr", Port: 5111, ProfilePath: filepath.Join(dir, "profile.sh"), HubSocket: filepath.Join(dir, "hub", "hub.sock")}, out
}

func waitFor(t *testing.T, what string, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if condition() {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s", what)
}

func TestSupervisorShutdownRejectsNewSpawn(t *testing.T) {
	sup := &supervisor{ctx: context.Background(), users: map[string]bool{}}
	sup.shutdown()

	if sup.spawn("alice") {
		t.Fatal("spawn succeeded after supervisor shutdown")
	}

	sup.wg.Wait()
}

func TestRunStartsTheCompositorWithItsArgumentsAndRestartsIt(t *testing.T) {
	cfg, out := testConfig(t, `echo "$@ layout=$XKB_DEFAULT_LAYOUT runtime=$XDG_RUNTIME_DIR renderer=$WLR_RENDERER" >> $OUT; sleep 0.1`)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- Run(ctx, cfg) }()

	// It exits at once each time, so it is started again after the (1 second) backoff.
	waitFor(t, "two starts", 6*time.Second, func() bool {
		data, _ := os.ReadFile(out)
		return strings.Count(string(data), "\n") >= 2
	})

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}

	data, _ := os.ReadFile(out)
	line := strings.SplitN(string(data), "\n", 2)[0]

	// The compositor reaches the host through the hub, not by dialing vsock itself.
	for _, want := range []string{"--connect " + cfg.HubConnect(), "--socket wayland-1", "layout=fr", "renderer=pixman"} {
		if !strings.Contains(line, want) {
			t.Errorf("%q is missing from %q", want, line)
		}
	}
}

func TestRunStopsTheCompositorWithTheContext(t *testing.T) {
	cfg, out := testConfig(t, `echo $$ > $OUT; exec sleep 60`)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- Run(ctx, cfg) }()

	waitFor(t, "the compositor to start", 5*time.Second, func() bool {
		data, _ := os.ReadFile(out)
		return len(data) > 0
	})

	data, _ := os.ReadFile(out)
	pid := strings.TrimSpace(string(data))

	cancel()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return")
	}

	if _, err := os.Stat("/proc/" + pid); err == nil {
		// Zombies still have a /proc entry until reaped: it must at least not be running.
		status, _ := os.ReadFile("/proc/" + pid + "/stat")

		if !strings.Contains(string(status), ") Z ") {
			t.Fatalf("the compositor (pid %s) is still running", pid)
		}
	}
}

func TestLaunchGivesTheApplicationTheCompositorEnvironment(t *testing.T) {
	cfg, out := testConfig(t, "")

	pid, err := Launch(cfg, []string{"/bin/sh", "-c", "env > " + out})

	if err != nil || pid <= 0 {
		t.Fatalf("Launch: %d %v", pid, err)
	}

	waitFor(t, "the application to write its environment", 5*time.Second, func() bool {
		data, _ := os.ReadFile(out)
		return strings.Contains(string(data), "WAYLAND_DISPLAY")
	})

	data, _ := os.ReadFile(out)

	for _, want := range []string{"WAYLAND_DISPLAY=wayland-1", "XDG_RUNTIME_DIR=", "GDK_BACKEND=wayland", "QT_QPA_PLATFORM=wayland", "MOZ_ENABLE_WAYLAND=1"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("%q is missing from the environment", want)
		}
	}

	if _, err := Launch(cfg, nil); err == nil {
		t.Fatal("expected an error without a command")
	}
}

func TestLaunchDesktopIsUnknownWhenTheApplicationDoesNotExist(t *testing.T) {
	cfg, _ := testConfig(t, "")

	if _, err := LaunchDesktop(cfg, "no.such.app"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestStatusWithoutACompositor(t *testing.T) {
	cfg, _ := testConfig(t, "")

	// Not the default name: the machine running the tests may have a real compositor on it.
	cfg.SocketName = "caker-seamless-test-nothing-here"

	if st := CurrentStatus(cfg); st.Running || !st.Installed {
		t.Fatalf("unexpected %+v", st)
	}
}

func TestLogLinesDrainsOversizedLines(t *testing.T) {
	input := "short\n" + strings.Repeat("x", 3*maxLogLine) + "\nlast\n"
	reader := bytes.NewReader([]byte(input))

	logLines("test", reader)

	if reader.Len() != 0 {
		t.Fatalf("%d bytes were left unread", reader.Len())
	}
}

func TestXKBConfigRootOnlyServesGuestsWithoutXKBData(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "libexec", BinaryName)
	bundled := XKBDir(binary)
	system := filepath.Join(dir, "system")

	fill := func(root string) {
		t.Helper()

		if err := os.MkdirAll(filepath.Join(root, "rules"), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(root, "rules", "evdev"), []byte("rules"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	legacy := filepath.Join(dir, "legacy")

	previous := systemXKBDirs
	systemXKBDirs = []string{system, legacy}

	t.Cleanup(func() { systemXKBDirs = previous })

	if got := xkbConfigRoot(binary); got != "" {
		t.Fatalf("nothing bundled, nothing in the guest: %q", got)
	}

	fill(bundled)

	if got := xkbConfigRoot(binary); got != bundled {
		t.Fatalf("a guest without XKB data must get the bundled one: %q", got)
	}

	// The versioned root exists but holds no data: the compositor does not fall back to the legacy one.
	if err := os.MkdirAll(system, 0o755); err != nil {
		t.Fatal(err)
	}

	fill(legacy)

	if got := xkbConfigRoot(binary); got != bundled {
		t.Fatalf("data in a directory the compositor does not search must not hide the bundled one: %q", got)
	}

	if err := os.RemoveAll(system); err != nil {
		t.Fatal(err)
	}

	if got := xkbConfigRoot(binary); got != "" {
		t.Fatalf("the legacy root serves when the versioned one does not exist: %q", got)
	}

	fill(system)

	if got := xkbConfigRoot(binary); got != "" {
		t.Fatalf("the guest's own data comes first: %q", got)
	}
}

func TestActivationCommands(t *testing.T) {
	vars := activationVars(Config{})

	if len(vars) == 0 || vars[0] != "WAYLAND_DISPLAY=wayland-1" {
		t.Fatalf("activation vars should start with the default display, got %v", vars)
	}

	cmds := activationCommands(vars)

	if len(cmds) != 2 {
		t.Fatalf("expected a primary command and a fallback, got %v", cmds)
	}

	if cmds[0][0] != "dbus-update-activation-environment" || cmds[0][1] != "--systemd" || cmds[0][2] != vars[0] {
		t.Fatalf("primary command should pass NAME=VALUE pairs to dbus-update-activation-environment --systemd: %v", cmds[0])
	}

	// systemctl import-environment takes names only: the values come from its environment.
	if cmds[1][0] != "systemctl" || cmds[1][3] != "WAYLAND_DISPLAY" {
		t.Fatalf("fallback should import names through systemctl --user: %v", cmds[1])
	}

	for _, a := range cmds[1][3:] {
		if strings.Contains(a, "=") {
			t.Fatalf("import-environment must not get values on its command line: %q", a)
		}
	}
}

// brokenListener fails every Accept, like a listener that ran out of file descriptors.
type brokenListener struct{}

func (brokenListener) Accept() (net.Conn, error) { return nil, errors.New("accept failed") }
func (brokenListener) Close() error              { return nil }
func (brokenListener) Addr() net.Addr            { return &net.UnixAddr{Net: "unix"} }

func TestServeHubListensAgainWhenAcceptFails(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "hub.sock")
	hub := &Hub{Authenticate: func(net.Conn) (Account, error) { return Account{}, errors.New("refused") }}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	relistened := make(chan struct{}, 1)

	go func() {
		done <- serveHub(ctx, hub, brokenListener{}, func() (net.Listener, error) {
			l, err := net.Listen("unix", socket)

			if err == nil {
				relistened <- struct{}{}
			}

			return l, err
		})
	}()

	select {
	case <-relistened:
	case <-time.After(5 * time.Second):
		t.Fatal("the hub did not listen again")
	}

	// The new listener really accepts compositors.
	waitFor(t, "the new listener to accept", 5*time.Second, func() bool {
		conn, err := net.Dial("unix", socket)

		if err == nil {
			conn.Close()
		}

		return err == nil
	})

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("a cancelled context is not an error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveHub did not return after the context was cancelled")
	}
}

func TestServeHubGivesUpWhenTheListenerCannotRecover(t *testing.T) {
	hub := &Hub{}
	attempts := 0

	err := serveHub(context.Background(), hub, brokenListener{}, func() (net.Listener, error) {
		attempts++

		return nil, errors.New("cannot listen")
	})

	if err == nil {
		t.Fatal("serveHub must report that it gave up")
	}

	if attempts == 0 {
		t.Fatal("the listener was never opened again")
	}
}

func TestEnsureInstalledIsSafeForConcurrentWorkers(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")

	if err := os.WriteFile(source, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv(EnvBinary, source)

	dest := filepath.Join(dir, "bin", BinaryName)
	errs := make(chan error, 8)

	for i := 0; i < cap(errs); i++ {
		go func() { errs <- ensureInstalled(dest) }()
	}

	for i := 0; i < cap(errs); i++ {
		if err := <-errs; err != nil {
			t.Fatalf("ensureInstalled: %v", err)
		}
	}

	// Once every caller returned, the installation is complete.
	if installNeeded(dest) {
		t.Fatal("the compositor is still missing or its XKB data incomplete")
	}
}
