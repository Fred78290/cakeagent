//go:build linux

package pkg

import (
	"os"
	"testing"
)

func TestSeamlessShellIdentityIsOffWithoutSeamlessWindows(t *testing.T) {
	if (&server{}).seamlessShellIdentity() != nil {
		t.Fatal("a shell must stay the agent's own when seamless windows are off")
	}
}

func TestSeamlessShellIdentityNeedsRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root")
	}

	// Only root can become another user: otherwise the shell stays as it is.
	if (&server{seamless: true}).seamlessShellIdentity() != nil {
		t.Fatal("a non-root agent cannot switch user")
	}
}
