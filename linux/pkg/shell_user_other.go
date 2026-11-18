//go:build !windows && !linux && !darwin

package pkg

import "syscall"

type userIdentity struct {
	credential *syscall.Credential
	home       string
	env        []string
}

func (s *server) identityFor(name string) (*userIdentity, error) { return nil, nil }
