package main

import (
	"os"

	"github.com/Fred78290/cakeagent/cmd/types"
	"github.com/Fred78290/cakeagent/pkg"
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

	glog.Infof("config: %s", cfg)

	if cfg.DisplayVersion {
		glog.Infof("The current version is: %s, build at: %s", version.VERSION, version.BUILD_DATE)
	} else if cfg.InstallService {
		err = service.InstallService(cfg)
	} else {
		_, err = pkg.StartServer(cfg)
	}

	if err != nil {
		glog.Fatalf("%v", err)
	}
}
