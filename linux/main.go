package main

import (
	"fmt"
	"os"

	"github.com/Fred78290/cakeagent/cmd/types"
	"github.com/Fred78290/cakeagent/pkg"
	"github.com/Fred78290/cakeagent/pkg/mount"
	"github.com/Fred78290/cakeagent/version"

	// Remplacez par le chemin de votre module
	"github.com/Fred78290/cakeagent/service"
	glog "github.com/sirupsen/logrus"
)

func main() {
	cfg := types.NewConfig()

	if err := cfg.ParseFlags(os.Args[1:], version.VERSION); err != nil {
		glog.Fatalf("flag parsing error: %v", err)
	}

	ll, err := glog.ParseLevel(cfg.LogLevel)
	if err != nil {
		glog.Fatalf("failed to parse log level: %v", err)
	}

	glog.SetLevel(ll)

	if cfg.LogFormat == "json" {
		glog.SetFormatter(&glog.JSONFormatter{})
	}

	if cfg.DisplayVersion {
		fmt.Printf("The current version is: %s, build at: %s\n", version.VERSION, version.BUILD_DATE)
	} else {
		glog.Infof("config: %s", cfg)

		if len(cfg.Mounts) > 0 {
			err = mount.MountEndpoints(cfg.Mounts)
		}

		if err == nil {
			if cfg.InstallService {
				err = service.InstallService(cfg)
			} else if cfg.StopService {
				err = service.StopService(cfg)
			} else if cfg.StartService {
				err = service.StartService(cfg)
			} else {
				_, err = pkg.StartServer(cfg)
			}
		}
	}

	if err != nil {
		glog.Fatalf("%v", err)
	}
}
