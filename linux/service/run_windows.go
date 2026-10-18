//go:build windows
// +build windows

package service

import (
	svc "github.com/kardianos/service"

	"github.com/Fred78290/cakeagent/cmd/types"
)

// runner lets the Service Control Manager start and stop the server: Start must return quickly (the SCM gives a
// service 30 seconds to report it is running), so the blocking server goes on a goroutine.
type runner struct {
	start func()
}

func (r *runner) Start(s svc.Service) error {
	go r.start()

	return nil
}

func (r *runner) Stop(s svc.Service) error {
	return nil
}

// RunAsService hands the process to the Service Control Manager when it was started by it, and reports whether it
// did. Started from a console it does nothing and the caller runs the server itself. Without this, a service whose
// executable never answers the SCM is killed with error 1053 ("did not respond to the start or control request").
func RunAsService(cfg *types.Config, start func()) (bool, error) {
	if svc.Interactive() {
		return false, nil
	}

	s, err := svc.New(&runner{start: start}, &svc.Config{Name: "cakeagent"})

	if err != nil {
		return true, err
	}

	return true, s.Run()
}
