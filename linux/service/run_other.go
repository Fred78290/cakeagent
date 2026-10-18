//go:build !windows
// +build !windows

package service

import "github.com/Fred78290/cakeagent/cmd/types"

// RunAsService is only needed on Windows, where the Service Control Manager has to be answered.
func RunAsService(cfg *types.Config, start func()) (bool, error) {
	return false, nil
}
