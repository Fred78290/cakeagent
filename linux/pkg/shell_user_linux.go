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

// shellIdentity is who an interactive shell runs as when it is not the agent's own user.
type shellIdentity struct {
	credential *syscall.Credential
	home       string
	env        []string
}

// seamlessShellIdentity tells who "caked shell" must be when seamless windows are on: the compositor's socket
// belongs to the guest's login user, so a shell of the agent's own user (root) can neither find it (its runtime
// directory is /run/user/0) nor should it run desktop applications. It returns nil when seamless windows are off,
// the agent is not root or the user can't be resolved: the shell then stays the agent's.
func (s *server) seamlessShellIdentity() *shellIdentity {
	if !s.seamless || os.Geteuid() != 0 {
		return nil
	}

	account, err := seamless.LookupAccount(s.seamlessUser)

	if err != nil {
		return nil
	}

	cred := &syscall.Credential{Uid: account.UID, Gid: account.GID}

	if u, err := user.LookupId(strconv.FormatUint(uint64(account.UID), 10)); err == nil {
		if ids, err := u.GroupIds(); err == nil {
			for _, id := range ids {
				if gid, err := strconv.ParseUint(id, 10, 32); err == nil {
					cred.Groups = append(cred.Groups, uint32(gid))
				}
			}
		}
	}

	// The directory the compositor's socket really is in, not a guess: it moves when the user logs in.
	runtimeDir := fmt.Sprintf("/run/user/%d", account.UID)

	if env, err := seamless.Env(seamless.Config{User: account.Name}); err == nil {
		for _, kv := range env {
			if dir, ok := strings.CutPrefix(kv, "XDG_RUNTIME_DIR="); ok {
				runtimeDir = dir
			}
		}
	}

	shell := account.Shell

	if shell == "" {
		shell = "/bin/sh"
	}

	var env []string

	for _, kv := range os.Environ() {
		switch {
		case strings.HasPrefix(kv, "HOME="), strings.HasPrefix(kv, "USER="), strings.HasPrefix(kv, "LOGNAME="), strings.HasPrefix(kv, "SHELL="), strings.HasPrefix(kv, "XDG_RUNTIME_DIR="), strings.HasPrefix(kv, "MAIL="):
		default:
			env = append(env, kv)
		}
	}

	env = append(env, "HOME="+account.Home, "USER="+account.Name, "LOGNAME="+account.Name, "SHELL="+shell, "XDG_RUNTIME_DIR="+runtimeDir)

	return &shellIdentity{credential: cred, home: account.Home, env: env}
}
