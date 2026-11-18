//go:build !windows && !linux && !darwin

package pkg

import (
	"context"
	"syscall"
)

type userIdentity struct {
	credential *syscall.Credential
	home       string
	env        []string
}

func (s *server) identityFor(_ context.Context, name string) (*userIdentity, error) { return nil, nil }
