//go:build linux

package pkg

import (
	"os"
	"os/user"
	"strings"
	"testing"

	"github.com/Fred78290/cakeagent/pkg/seamless"
)

func TestIdentityForNoUserKeepsTheAgentsOwn(t *testing.T) {
	identity, err := (&server{}).identityFor("")

	if identity != nil || err != nil {
		t.Fatalf("an empty user must leave the process as it is: %v %v", identity, err)
	}
}

func TestIdentityForTheAgentsOwnUserKeepsTheAgentsOwn(t *testing.T) {
	current, err := user.Current()

	if err != nil {
		t.Fatal(err)
	}

	identity, err := (&server{}).identityFor(current.Username)

	if identity != nil || err != nil {
		t.Fatalf("the agent's own user needs no switch: %v %v", identity, err)
	}
}

func TestIdentityForUnknownUser(t *testing.T) {
	if _, err := (&server{}).identityFor("no-such-user-caker"); err == nil {
		t.Fatal("expected an error for an unknown user")
	}
}

func TestIdentityForNeedsRoot(t *testing.T) {
	if getEUID() == 0 {
		t.Skip("running as root")
	}

	other := "root"

	if current, err := user.Current(); err == nil && current.Username == other {
		t.Skip("running as root")
	}

	if _, err := (&server{}).identityFor(other); err == nil {
		t.Fatal("a non-root agent cannot switch user")
	}
}

func TestIdentityForSwitchesUserAndSanitizesEnvironment(t *testing.T) {
	restore := setIdentityForTestHooks()

	defer restore()

	lookupUser = func(name string) (*user.User, error) {
		if name != "guest" {
			t.Fatalf("unexpected lookup: %q", name)
		}

		return &user.User{
			Uid:      "1001",
			Gid:      "1002",
			Username: "guest",
			HomeDir:  "/home/guest",
		}, nil
	}
	getEUID = func() int { return 0 }
	openPasswdFile = func() (*os.File, error) { return nil, os.ErrNotExist }
	listEnviron = func() []string {
		return []string{
			"PATH=/usr/bin",
			"CAKEAGENT_TOKEN=secret",
			"HOME=/root",
			"LOGNAME=root",
		}
	}
	seamlessStatus = func(cfg seamless.Config) seamless.Status {
		if cfg.User != "guest" {
			t.Fatalf("unexpected seamless user: %q", cfg.User)
		}

		return seamless.Status{Running: true}
	}
	seamlessEnv = func(cfg seamless.Config) ([]string, error) {
		if cfg.User != "guest" {
			t.Fatalf("unexpected seamless env user: %q", cfg.User)
		}

		return []string{"WAYLAND_DISPLAY=wayland-1", "XDG_RUNTIME_DIR=/run/caker-seamless/guest"}, nil
	}

	identity, err := (&server{seamless: true}).identityFor("guest")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if identity == nil {
		t.Fatal("expected identity")
	}

	if identity.home != "/home/guest" {
		t.Fatalf("unexpected home: %q", identity.home)
	}

	if identity.credential == nil {
		t.Fatal("expected credentials")
	}

	if identity.credential.Uid != uint32(1001) || identity.credential.Gid != uint32(1002) {
		t.Fatalf("unexpected credentials: %+v", identity.credential)
	}

	for _, kv := range identity.env {
		if strings.HasPrefix(kv, "CAKEAGENT_") {
			t.Fatalf("CAKEAGENT variables must be filtered: %q", kv)
		}
	}

	assertHasEnv(t, identity.env, "HOME=/home/guest")
	assertHasEnv(t, identity.env, "USER=guest")
	assertHasEnv(t, identity.env, "LOGNAME=guest")
	assertHasEnv(t, identity.env, "SHELL=/bin/sh")
	assertHasEnv(t, identity.env, "XDG_RUNTIME_DIR=/run/caker-seamless/guest")
	assertHasEnv(t, identity.env, "WAYLAND_DISPLAY=wayland-1")
	assertHasEnv(t, identity.env, "PATH=/usr/bin")
}

func setIdentityForTestHooks() func() {
	lookupUserOld := lookupUser
	getEUIDOld := getEUID
	listEnvironOld := listEnviron
	openPasswdFileOld := openPasswdFile
	seamlessStatusOld := seamlessStatus
	seamlessEnvOld := seamlessEnv

	return func() {
		lookupUser = lookupUserOld
		getEUID = getEUIDOld
		listEnviron = listEnvironOld
		openPasswdFile = openPasswdFileOld
		seamlessStatus = seamlessStatusOld
		seamlessEnv = seamlessEnvOld
	}
}

func assertHasEnv(t *testing.T, env []string, want string) {
	t.Helper()

	for _, kv := range env {
		if kv == want {
			return
		}
	}

	t.Fatalf("missing env %q in %v", want, env)
}
