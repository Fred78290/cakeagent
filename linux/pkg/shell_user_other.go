//go:build !windows && !linux

package pkg

import "syscall"

type shellIdentity struct {
	credential *syscall.Credential
	home       string
	env        []string
}

func (s *server) seamlessShellIdentity() *shellIdentity { return nil }
