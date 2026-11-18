//go:build darwin

package pkg

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// macOS accepts at most NGROUPS_MAX (16) supplementary groups in setgroups; more makes the exec fail with EINVAL.
const maxSupplementaryGroups = 16

var (
	getEUID     = os.Geteuid
	listEnviron = os.Environ
	// lookupAccount and lookupGroups ask the directory service. os/user is not used: a build without cgo (the
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
func (s *server) identityFor(name string) (*userIdentity, error) {
	if name == "" {
		return nil, nil
	}

	acc, err := lookupAccount(name)

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

	if groups, err := lookupGroups(name); err == nil {
		if len(groups) > maxSupplementaryGroups {
			groups = groups[:maxSupplementaryGroups]
		}

		cred.Groups = groups
	}

	shell := acc.shell

	if shell == "" {
		shell = "/bin/sh"
	}

	home := acc.home

	if home == "" {
		home = "/var/empty"
	}

	var env []string

	for _, kv := range listEnviron() {
		switch {
		// TMPDIR is root's private directory under /var/folders: the user could not write to it.
		case strings.HasPrefix(kv, "HOME="), strings.HasPrefix(kv, "USER="), strings.HasPrefix(kv, "LOGNAME="), strings.HasPrefix(kv, "SHELL="), strings.HasPrefix(kv, "TMPDIR="), strings.HasPrefix(kv, "MAIL="), strings.HasPrefix(kv, "CAKEAGENT_"):
		default:
			env = append(env, kv)
		}
	}

	env = append(env, "HOME="+home, "USER="+name, "LOGNAME="+name, "SHELL="+shell)

	return &userIdentity{credential: cred, home: home, env: env}, nil
}

// validAccountName keeps a name from reaching the directory path or the command line as anything but a user name.
func validAccountName(name string) bool {
	if name == "" || strings.HasPrefix(name, "-") {
		return false
	}

	return !strings.ContainsAny(name, "/\x00\n\r")
}

func dsclAccount(name string) (account, error) {
	if !validAccountName(name) {
		return account{}, fmt.Errorf("invalid user name")
	}

	out, err := exec.Command("/usr/bin/dscl", ".", "-read", "/Users/"+name, "UniqueID", "PrimaryGroupID", "NFSHomeDirectory", "UserShell").Output()

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

// idGroups lists the group ids of a user, the primary group first (`id -G`).
func idGroups(name string) ([]uint32, error) {
	if !validAccountName(name) {
		return nil, fmt.Errorf("invalid user name")
	}

	out, err := exec.Command("/usr/bin/id", "-G", name).Output()

	if err != nil {
		return nil, err
	}

	return parseGroupIDs(string(out)), nil
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
