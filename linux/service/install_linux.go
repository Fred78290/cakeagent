package service

import (
	"fmt"
	"os"

	glog "github.com/sirupsen/logrus"

	"github.com/Fred78290/cakeagent/cmd/types"
	svc "github.com/kardianos/service"
)

type program struct{}

func (p *program) Start(s svc.Service) error {
	return nil
}

func (p *program) Stop(s svc.Service) error {
	return nil
}

func InstallService(cfg *types.Config) (err error) {
	var service svc.Service

	args := []string{
		fmt.Sprintf("--listen=%s", cfg.Address),
	}

	if cfg.CaCert != "" {
		args = append(args, fmt.Sprintf("--ca-cert=%s", cfg.CaCert))
	}

	if cfg.TlsCert != "" {
		args = append(args, fmt.Sprintf("--tls-cert=%s", cfg.TlsCert))
	}

	if cfg.TlsKey != "" {
		args = append(args, fmt.Sprintf("--tls-key=%s", cfg.TlsKey))
	}

	svcConfig := &svc.Config{
		Name:        "cakeagent",
		DisplayName: "CakeAgent",
		Description: "CakeAgent Service.",
		UserName:    "root",
		Executable:  os.Args[0],
		Arguments:   args,
		Dependencies: []string{
			"After=network.target",
		},
		EnvVars: map[string]string{
			"PATH": "/usr/local/bin:/usr/bin:/bin:/usr/sbin/:/sbin",
		},
	}

	prg := &program{}

	if service, err = svc.New(prg, svcConfig); err == nil {
		if err = service.Install(); err != nil {
			glog.Errorf("Failed to install service: %v", err)
		} else {
			glog.Info("Service installed successfully")

			if err = service.Start(); err != nil {
				glog.Errorf("Failed to start service: %v", err)
			} else {
				glog.Infof("Service started successfully")
			}
		}
	}

	return
}
