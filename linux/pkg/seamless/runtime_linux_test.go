//go:build linux

package seamless

import (
	"context"
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

	return Config{User: current.Username, Binary: binary, Layout: "fr", Port: 5111}, out
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

	for _, want := range []string{"--connect vsock:5111", "--socket caker-seamless", "layout=fr", "renderer=pixman"} {
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

	for _, want := range []string{"WAYLAND_DISPLAY=caker-seamless", "XDG_RUNTIME_DIR=", "GDK_BACKEND=wayland", "QT_QPA_PLATFORM=wayland", "MOZ_ENABLE_WAYLAND=1"} {
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

	if st := CurrentStatus(cfg); st.Running || !st.Installed {
		t.Fatalf("unexpected %+v", st)
	}
}
