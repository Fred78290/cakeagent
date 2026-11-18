//go:build darwin

package pkg

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func setDarwinIdentityHooks() func() {
	oldAccount, oldGroups, oldEUID, oldEnviron := lookupAccount, lookupGroups, getEUID, listEnviron

	return func() {
		lookupAccount, lookupGroups, getEUID, listEnviron = oldAccount, oldGroups, oldEUID, oldEnviron
	}
}

func TestDarwinIdentityForNoUserKeepsTheAgentsOwn(t *testing.T) {
	identity, err := (&server{}).identityFor(context.Background(), "")

	if identity != nil || err != nil {
		t.Fatalf("an empty user must leave the process as it is: %v %v", identity, err)
	}
}

func TestDarwinIdentityForTheAgentsOwnUserKeepsTheAgentsOwn(t *testing.T) {
	current, err := user.Current()

	if err != nil {
		t.Fatal(err)
	}

	identity, err := (&server{}).identityFor(context.Background(), current.Username)

	if identity != nil || err != nil {
		t.Fatalf("the agent's own user needs no switch: %v %v", identity, err)
	}
}

func TestDarwinIdentityForUnknownUser(t *testing.T) {
	if _, err := (&server{}).identityFor(context.Background(), "no-such-user-caker"); err == nil {
		t.Fatal("expected an error for an unknown user")
	}
}

func TestDarwinIdentityForRejectsNamesThatAreNotUserNames(t *testing.T) {
	for _, name := range []string{"-a", "../root", "a/b", "a\nb"} {
		if _, err := (&server{}).identityFor(context.Background(), name); err == nil {
			t.Fatalf("%q must be refused", name)
		}
	}
}

func TestDarwinIdentityForNeedsRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root")
	}

	if _, err := (&server{}).identityFor(context.Background(), "root"); err == nil {
		t.Fatal("a non-root agent cannot switch user")
	}
}

func TestDarwinIdentityForSwitchesUserAndSanitizesEnvironment(t *testing.T) {
	defer setDarwinIdentityHooks()()

	lookupAccount = func(_ context.Context, name string) (account, error) {
		if name != "guest" {
			t.Fatalf("unexpected lookup: %q", name)
		}

		return account{uid: 502, gid: 20, home: "/Users/guest", shell: "/bin/zsh"}, nil
	}
	lookupGroups = func(context.Context, string, uint32) ([]uint32, error) { return []uint32{20, 12}, nil }
	getEUID = func() int { return 0 }
	listEnviron = func() []string {
		return []string{"PATH=/usr/bin", "TERM=xterm", "LANG=fr_FR.UTF-8", "LC_ALL=fr_FR.UTF-8", "CAKEAGENT_TOKEN=secret", "API_SECRET=hunter2", "SSH_AUTH_SOCK=/tmp/agent", "HOME=/var/root", "USER=root", "TMPDIR=/var/folders/zz/T/", "SHELL=/bin/sh"}
	}

	identity, err := (&server{}).identityFor(context.Background(), "guest")

	if err != nil || identity == nil {
		t.Fatalf("expected an identity: %v %v", identity, err)
	}

	if identity.credential.Uid != 502 || identity.credential.Gid != 20 || !reflect.DeepEqual(identity.credential.Groups, []uint32{20, 12}) {
		t.Fatalf("wrong credential: %+v", identity.credential)
	}

	if identity.home != "/Users/guest" {
		t.Fatalf("wrong home: %q", identity.home)
	}

	env := strings.Join(identity.env, "\n")

	for _, want := range []string{"PATH=/usr/bin", "TERM=xterm", "LANG=fr_FR.UTF-8", "LC_ALL=fr_FR.UTF-8", "HOME=/Users/guest", "USER=guest", "LOGNAME=guest", "SHELL=/bin/zsh"} {
		if !strings.Contains(env, want) {
			t.Errorf("environment lacks %q: %v", want, identity.env)
		}
	}

	for _, leaked := range []string{"CAKEAGENT_TOKEN", "API_SECRET", "SSH_AUTH_SOCK", "HOME=/var/root", "USER=root", "TMPDIR="} {
		if strings.Contains(env, leaked) {
			t.Errorf("environment leaks %q: %v", leaked, identity.env)
		}
	}
}

func TestDarwinIdentityForCapsSupplementaryGroups(t *testing.T) {
	defer setDarwinIdentityHooks()()

	lookupAccount = func(context.Context, string) (account, error) { return account{uid: 502, gid: 20}, nil }
	lookupGroups = func(context.Context, string, uint32) ([]uint32, error) {
		groups := make([]uint32, 20)

		for i := range groups {
			groups[i] = uint32(i + 1)
		}

		return groups, nil
	}
	getEUID = func() int { return 0 }

	identity, err := (&server{}).identityFor(context.Background(), "guest")

	if err != nil || identity == nil {
		t.Fatalf("expected an identity: %v %v", identity, err)
	}

	if len(identity.credential.Groups) != maxSupplementaryGroups {
		t.Fatalf("groups must be capped at %d: %d", maxSupplementaryGroups, len(identity.credential.Groups))
	}

	if identity.home != "/var/empty" || !strings.Contains(strings.Join(identity.env, "\n"), "SHELL=/bin/sh") {
		t.Fatalf("a record without a home or shell needs defaults: %+v", identity)
	}
}

func TestDarwinIdentityForRejectsAChangedAccount(t *testing.T) {
	defer setDarwinIdentityHooks()()

	lookupAccount = func(context.Context, string) (account, error) { return account{uid: 502, gid: 20}, nil }
	lookupGroups = func(_ context.Context, _ string, uid uint32) ([]uint32, error) {
		if uid != 502 {
			t.Fatalf("groups must be asked for the resolved uid: %d", uid)
		}

		return nil, errAccountChanged
	}
	getEUID = func() int { return 0 }

	if identity, err := (&server{}).identityFor(context.Background(), "guest"); err == nil || identity != nil {
		t.Fatalf("a changed account must be rejected: %v %v", identity, err)
	}
}

func TestDarwinIdentityForRejectsAnUnusableGroupLookup(t *testing.T) {
	cases := map[string]func(context.Context, string, uint32) ([]uint32, error){
		"failure":        func(context.Context, string, uint32) ([]uint32, error) { return nil, errors.New("id failed") },
		"empty output":   func(context.Context, string, uint32) ([]uint32, error) { return nil, nil },
		"no primary gid": func(context.Context, string, uint32) ([]uint32, error) { return []uint32{12, 80}, nil },
	}

	for name, lookup := range cases {
		t.Run(name, func(t *testing.T) {
			defer setDarwinIdentityHooks()()

			lookupAccount = func(context.Context, string) (account, error) { return account{uid: 502, gid: 20}, nil }
			lookupGroups = lookup
			getEUID = func() int { return 0 }

			if identity, err := (&server{}).identityFor(context.Background(), "guest"); err == nil || identity != nil {
				t.Fatalf("an unusable group lookup must be a setup error: %+v %v", identity, err)
			}
		})
	}
}

func TestParseDsclAccount(t *testing.T) {
	acc, err := parseDsclAccount("NFSHomeDirectory:\n /Users/with space\nPrimaryGroupID: 20\nUniqueID: 501\nUserShell: /bin/zsh\n")

	if err != nil {
		t.Fatal(err)
	}

	want := account{uid: 501, gid: 20, home: "/Users/with space", shell: "/bin/zsh"}

	if acc != want {
		t.Fatalf("got %+v, want %+v", acc, want)
	}

	if _, err := parseDsclAccount("UniqueID: 501\n"); err == nil {
		t.Fatal("a record without a primary group is incomplete")
	}
}

func TestParseGroupIDs(t *testing.T) {
	if got := parseGroupIDs("20 101 x 12\n"); !reflect.DeepEqual(got, []uint32{20, 101, 12}) {
		t.Fatalf("got %v", got)
	}
}

// The real directory service answers for the user running the test.
func TestDsclAccountForTheCurrentUser(t *testing.T) {
	current, err := user.Current()

	if err != nil {
		t.Fatal(err)
	}

	acc, err := dsclAccount(context.Background(), current.Username)

	if err != nil {
		t.Fatal(err)
	}

	if strconv.Itoa(int(acc.uid)) != current.Uid || acc.home == "" {
		t.Fatalf("unexpected record %+v for %s", acc, fmt.Sprint(current.Uid))
	}

	groups, err := idGroups(context.Background(), current.Username, acc.uid)

	if err != nil || len(groups) == 0 {
		t.Fatalf("no groups: %v %v", groups, err)
	}
}

// A cancelled request stops the directory lookups instead of waiting for them.
func TestDarwinLookupsStopWhenTheRequestIsCancelled(t *testing.T) {
	current, err := user.Current()

	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := dsclAccount(ctx, current.Username); err == nil {
		t.Fatal("dscl must not run for a cancelled request")
	}

	if _, err := idGroups(ctx, current.Username, 0); err == nil {
		t.Fatal("id must not run for a cancelled request")
	}
}
