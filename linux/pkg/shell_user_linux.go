//go:build linux

package pkg

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"

	"github.com/Fred78290/cakeagent/pkg/seamless"
)

var (
	lookupUser     = user.Lookup
	getEUID        = os.Geteuid
	listEnviron    = os.Environ
	openPasswdFile = func() (*os.File, error) { return os.Open("/etc/passwd") }
	seamlessStatus = seamless.CurrentStatus
	seamlessEnv    = seamless.Env
)

// userIdentity is who a shell or command runs as when the request names a user other than the agent's own.
type userIdentity struct {
	credential *syscall.Credential
	home       string
	env        []string
}

// identityFor resolves the user of an execute request. An empty name, or the agent's own user, gives nil: the
// process stays the agent's. Only root can become another user.
//
// Caker sends the guest's seamless user here (the account the compositor and its windows belong to): the
// agent runs as root, whose runtime directory (/run/user/0) holds no compositor socket, so a root shell or command
// could never open a window.
func (s *server) identityFor(name string) (*userIdentity, error) {
	if name == "" {
		return nil, nil
	}

	account, err := lookupUser(name)

	if err != nil {
		return nil, fmt.Errorf("cannot run as user %q: %w", name, err)
	}

	uid, err := strconv.ParseUint(account.Uid, 10, 32)

	if err != nil {
		return nil, fmt.Errorf("cannot run as user %q: bad uid %q", name, account.Uid)
	}

	gid, err := strconv.ParseUint(account.Gid, 10, 32)

	if err != nil {
		return nil, fmt.Errorf("cannot run as user %q: bad gid %q", name, account.Gid)
	}

	if int(uid) == getEUID() {
		return nil, nil
	}

	if getEUID() != 0 {
		return nil, fmt.Errorf("cannot run as user %q: the agent is not root", name)
	}

	cred := &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}

	if ids, err := account.GroupIds(); err == nil {
		for _, id := range ids {
			if g, err := strconv.ParseUint(id, 10, 32); err == nil {
				cred.Groups = append(cred.Groups, uint32(g))
			}
		}
	}

	runtimeDir := fmt.Sprintf("/run/user/%d", uid)

	var display []string

	// With seamless windows the compositor's socket may be in its own directory when logind has none, and a
	// command (unlike a login shell, whose profile does it) needs the display variables: follow the running
	// compositor.
	if s.seamless {
		cfg := seamless.Config{User: name}

		if seamlessStatus(cfg).Running {
			if env, err := seamlessEnv(cfg); err == nil {
				display = env

				for _, kv := range env {
					if dir, ok := strings.CutPrefix(kv, "XDG_RUNTIME_DIR="); ok {
						runtimeDir = dir
					}
				}
			}
		}
	}

	shell := "/bin/sh"

	if file, err := openPasswdFile(); err == nil {
		for _, a := range seamless.ParsePasswd(file) {
			if a.UID == uint32(uid) && a.Shell != "" {
				shell = a.Shell

				break
			}
		}

		file.Close()
	}

	var env []string

	for _, kv := range listEnviron() {
		switch {
		case strings.HasPrefix(kv, "HOME="), strings.HasPrefix(kv, "USER="), strings.HasPrefix(kv, "LOGNAME="), strings.HasPrefix(kv, "SHELL="), strings.HasPrefix(kv, "XDG_RUNTIME_DIR="), strings.HasPrefix(kv, "MAIL="), strings.HasPrefix(kv, "CAKEAGENT_"):
		default:
			env = append(env, kv)
		}
	}

	env = append(env, "HOME="+account.HomeDir, "USER="+account.Username, "LOGNAME="+account.Username, "SHELL="+shell, "XDG_RUNTIME_DIR="+runtimeDir)
	env = append(env, display...)

	return &userIdentity{credential: cred, home: account.HomeDir, env: env}, nil
}
