//go:build darwin

package pkg

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// macOS accepts at most NGROUPS_MAX (16) supplementary groups in setgroups; more makes the exec fail with EINVAL.
const maxSupplementaryGroups = 16

// lookupTimeout bounds the whole resolution of a user (the dscl and id calls together): a directory service that
// does not answer must not hold an execute request forever.
const lookupTimeout = 10 * time.Second

var (
	getEUID     = os.Geteuid
	listEnviron = os.Environ
	// lookupAccount and lookupGroups ask the directory service (search policy included). os/user is not used: a build without cgo (the
	// agent is cross-compiled) only reads /etc/passwd, which on macOS holds the system accounts alone.
	lookupAccount = dsclAccount
	lookupGroups  = idGroups
)

// userIdentity is who a shell or command runs as when the request names a user other than the agent's own.
type userIdentity struct {
	credential *syscall.Credential
	home       string
	env        []string
}

// account is what the directory service knows of a user.
type account struct {
	uid   uint32
	gid   uint32
	home  string
	shell string
}

// identityFor resolves the user of an execute request. An empty name, or the agent's own user, gives nil: the
// process stays the agent's. Only root can become another user.
func (s *server) identityFor(ctx context.Context, name string) (*userIdentity, error) {
	if name == "" {
		return nil, nil
	}

	// The lookups stop when the request is cancelled or after lookupTimeout, whichever comes first.
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()

	acc, err := lookupAccount(ctx, name)

	if err != nil {
		return nil, fmt.Errorf("cannot run as user %q: %w", name, err)
	}

	if euid := getEUID(); euid >= 0 && acc.uid == uint32(euid) {
		return nil, nil
	}

	if getEUID() != 0 {
		return nil, fmt.Errorf("cannot run as user %q: the agent is not root", name)
	}

	cred := &syscall.Credential{Uid: acc.uid, Gid: acc.gid}

	groups, err := lookupGroups(ctx, name, acc.uid)

	if err != nil {
		return nil, fmt.Errorf("cannot run as user %q: %w", name, err)
	}

	// Running with none of the user's groups, or with another account's, would misapply its permissions: accept
	// only a result that holds the primary group.
	if !slices.Contains(groups, acc.gid) {
		return nil, fmt.Errorf("cannot run as user %q: the group lookup returned no usable groups", name)
	}

	if len(groups) > maxSupplementaryGroups {
		groups = groups[:maxSupplementaryGroups]
	}

	cred.Groups = groups

	shell := acc.shell

	if shell == "" {
		shell = "/bin/sh"
	}

	home := acc.home

	if home == "" {
		home = "/var/empty"
	}

	var env []string

	// The agent's environment is not the user's: copy only what a session needs (an allowlist, so a secret in the
	// agent's environment cannot leak) and add the identity below. TMPDIR is left out on purpose: it is root's
	// private directory under /var/folders and the user could not write to it.
	for _, kv := range listEnviron() {
		if inheritedByUserEnv(kv) {
			env = append(env, kv)
		}
	}

	env = append(env, "HOME="+home, "USER="+name, "LOGNAME="+name, "SHELL="+shell)

	return &userIdentity{credential: cred, home: home, env: env}, nil
}

// inheritedByUserEnv tells whether an "KEY=value" entry of the agent's environment is passed to a shell or command
// run as another user: the search path, the terminal type, the time zone and the locale.
func inheritedByUserEnv(kv string) bool {
	key, _, _ := strings.Cut(kv, "=")

	switch key {
	case "PATH", "TERM", "COLORTERM", "TZ", "LANG":
		return true
	}

	return strings.HasPrefix(key, "LC_")
}

// validAccountName keeps a name from reaching the directory path or the command line as anything but a user name.
func validAccountName(name string) bool {
	if name == "" || strings.HasPrefix(name, "-") {
		return false
	}

	return !strings.ContainsAny(name, "/\x00\n\r")
}

func dsclAccount(ctx context.Context, name string) (account, error) {
	if !validAccountName(name) {
		return account{}, fmt.Errorf("invalid user name")
	}

	// /Search follows the directory search policy (local, LDAP, Active Directory...); "." would only see local accounts.
	out, err := exec.CommandContext(ctx, "/usr/bin/dscl", "/Search", "-read", "/Users/"+name, "UniqueID", "PrimaryGroupID", "NFSHomeDirectory", "UserShell").Output()

	if err != nil {
		return account{}, fmt.Errorf("unknown user")
	}

	return parseDsclAccount(string(out))
}

// parseDsclAccount reads the "Key: value" lines of `dscl -read`. A value that does not fit on the line is on the
// next one, indented.
func parseDsclAccount(out string) (account, error) {
	var acc account
	var haveUID, haveGID bool

	values := map[string]string{}
	key := ""

	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if key != "" && values[key] == "" {
				values[key] = strings.TrimSpace(line)
			}

			continue
		}

		k, v, found := strings.Cut(line, ":")

		if !found {
			key = ""

			continue
		}

		key = strings.TrimSpace(k)
		values[key] = strings.TrimSpace(v)
	}

	if uid, err := strconv.ParseUint(values["UniqueID"], 10, 32); err == nil {
		acc.uid, haveUID = uint32(uid), true
	}

	if gid, err := strconv.ParseUint(values["PrimaryGroupID"], 10, 32); err == nil {
		acc.gid, haveGID = uint32(gid), true
	}

	if !haveUID || !haveGID {
		return account{}, fmt.Errorf("incomplete account record")
	}

	acc.home = values["NFSHomeDirectory"]
	acc.shell = values["UserShell"]

	return acc, nil
}

// errAccountChanged means the name no longer designates the account the groups were asked for.
var errAccountChanged = errors.New("the account changed during the lookup")

// idGroups lists the group ids of a user, the primary group first (`id -G`). The groups are looked up by name, so
// the name must still designate uid, the account dsclAccount resolved, before and after the groups are read:
// otherwise the groups of another account would be granted to uid.
func idGroups(ctx context.Context, name string, uid uint32) ([]uint32, error) {
	if !validAccountName(name) {
		return nil, fmt.Errorf("invalid user name")
	}

	if err := checkAccountUID(ctx, name, uid); err != nil {
		return nil, err
	}

	out, err := exec.CommandContext(ctx, "/usr/bin/id", "-G", name).Output()

	if err != nil {
		return nil, err
	}

	if err := checkAccountUID(ctx, name, uid); err != nil {
		return nil, err
	}

	return parseGroupIDs(string(out)), nil
}

func checkAccountUID(ctx context.Context, name string, uid uint32) error {
	out, err := exec.CommandContext(ctx, "/usr/bin/id", "-u", name).Output()

	if err != nil {
		return err
	}

	if got, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 32); err != nil || uint32(got) != uid {
		return errAccountChanged
	}

	return nil
}

func parseGroupIDs(out string) []uint32 {
	var groups []uint32

	for _, field := range strings.Fields(out) {
		if gid, err := strconv.ParseUint(field, 10, 32); err == nil {
			groups = append(groups, uint32(gid))
		}
	}

	return groups
}
