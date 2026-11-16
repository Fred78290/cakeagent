//go:build linux

package pkg

import (
	"os"
	"os/user"
	"testing"
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
	if os.Geteuid() == 0 {
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
