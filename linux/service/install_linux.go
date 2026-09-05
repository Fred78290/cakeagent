package service

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	glog "github.com/sirupsen/logrus"

	"github.com/Fred78290/cakeagent/cmd/types"
	"github.com/Fred78290/cakeagent/pkg"
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

	args = append(args, "$EXTRA_FLAGS")

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
		Option: map[string]interface{}{
			"LogDirectory": "/var/log",
			"LogOutput":    true,
		},
		EnvVars: map[string]string{
			"PATH": "/usr/local/bin:/usr/bin:/bin:/usr/sbin/:/sbin",
		},
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

// isSELinuxEnabled reports whether SELinux is enabled on the host, regardless of
// enforcing/permissive mode: selinuxfs exposes an "enforce" file when mounted.
func isSELinuxEnabled() bool {
	if _, err := os.Stat("/sys/fs/selinux/enforce"); err == nil {
		return true
	}
	_, err := os.Stat("/selinux/enforce")
	return err == nil
}

// restoreSELinuxContext labels path as bin_t so SELinux-enforcing distributions
// (RHEL/Fedora/CentOS and derivatives) allow the service manager to execute it.
// A missing semanage/restorecon or a context that is already set is not fatal.
func restoreSELinuxContext(path string) {
	if !isSELinuxEnabled() {
		return
	}

	escapedSpec := regexp.QuoteMeta(path) + "$"
	if out, err := exec.Command("semanage", "fcontext", "-a", "-t", "bin_t", escapedSpec).CombinedOutput(); err != nil {
		if strings.Contains(string(out), "already defined") {
			glog.Infof("SELinux fcontext already set for %s", path)
		} else {
			glog.Warnf("Failed to set SELinux fcontext for %s: %v (%s)", path, err, strings.TrimSpace(string(out)))
		}
	}

	if out, err := exec.Command("restorecon", "-v", path).CombinedOutput(); err != nil {
		glog.Warnf("Failed to restore SELinux context for %s: %v (%s)", path, err, strings.TrimSpace(string(out)))
	}
}

func installService(service svc.Service) (err error) {
	// Uninstall any existing service to ensure a clean installation
	service.Uninstall()

	// Install the service
	if err = service.Install(); err != nil {
		// Check if the error is due to the service already being installed, must not be a fatal error in this case
		if strings.Contains(err.Error(), "Init already exists") {
			glog.Infof("Service already installed")
			err = nil
		} else {
			glog.Errorf("Failed to install service: %v", err)
		}
	} else {
		glog.Info("Service installed successfully")
	}

	if err == nil {
		if execPath, pathErr := os.Executable(); pathErr == nil {
			restoreSELinuxContext(execPath)
		}
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
		// Install the service and start it if installation was successful or if the service is already installed
		if err = installService(service); err == nil {
			// Configure mounts if specified in the configuration
			if len(cfg.Mounts) > 0 {
				var mounts []pkg.MountVirtioFSRequest

				glog.Infof("Configure mounts")

				if mounts, err = pkg.ParseMountStrings(cfg.Mounts, true); err != nil {
					glog.Errorf("Failed to parse mounts: %v, %v", cfg.Mounts, err)
				} else if _, err = pkg.MountService(mounts); err != nil {
					glog.Errorf("Failed to mount service: %v", err)
				}
			}

			// Start the service if it's not already running. A failure to start here is
			// not fatal for the install: the service is already registered (rc-update/systemctl
			// enable succeeded) and will come up on the next real boot even if it can't be
			// started right now, e.g. when installing inside a chroot or VM image that hasn't
			// been booted through its init system yet.
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
