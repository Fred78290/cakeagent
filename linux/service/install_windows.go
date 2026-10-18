//go:build windows
// +build windows

package service

import (
	"fmt"
	"os"
	"strings"

	"github.com/Fred78290/cakeagent/cmd/types"
	glog "github.com/sirupsen/logrus"

	svc "github.com/kardianos/service"
)

type program struct{}

func (p *program) Start(s svc.Service) error {
	return nil
}

func (p *program) Stop(s svc.Service) error {
	return nil
}

func getService(cfg *types.Config) (svc.Service, error) {
	args := []string{
		"serve",
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
		Executable:  os.Args[0],
		Arguments:   args,
	}

	prg := &program{}
	return svc.New(prg, svcConfig)
}

func StopService(cfg *types.Config) (err error) {
	var service svc.Service

	if service, err = getService(cfg); err == nil {
		if err = service.Stop(); err != nil {
			glog.Errorf("Failed to stop service: %v", err)
		} else {
			glog.Info("Service stopped successfully")
		}
	}

	return
}

func StartService(cfg *types.Config) (err error) {
	var service svc.Service

	if service, err = getService(cfg); err == nil {
		if err = service.Start(); err != nil {
			glog.Errorf("Failed to start service: %v", err)
		} else {
			glog.Info("Service started successfully")
		}
	}

	return
}

func installService(service svc.Service) (err error) {
	service.Uninstall()

	if err = service.Install(); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "exists") {
			glog.Infof("Service already installed")
			err = nil
		} else {
			glog.Errorf("Failed to install service: %v", err)
		}
	} else {
		glog.Info("Service installed successfully")
	}

	return
}

func RemoveService(cfg *types.Config) (err error) {
	var service svc.Service

	if service, err = getService(cfg); err == nil {
		if err = service.Uninstall(); err != nil {
			glog.Errorf("Failed to remove service: %v", err)
		} else {
			glog.Info("Service removed successfully")
		}
	}

	return
}

func InstallService(cfg *types.Config) (err error) {
	var service svc.Service

	if service, err = getService(cfg); err == nil {
		if err = installService(service); err == nil {
			if status, _ := service.Status(); status != svc.StatusRunning {
				if startErr := service.Start(); startErr != nil {
					glog.Warnf("Service installed but could not be started now: %v", startErr)
				} else {
					glog.Infof("Service started successfully")
				}
			} else {
				glog.Infof("Service is already running")
			}
		}
	}

	return
}
