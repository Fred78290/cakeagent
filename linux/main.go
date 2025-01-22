package main

import (
	"os"

	"github.com/Fred78290/cakeagent/cmd/types" // Remplacez par le chemin de votre module
	"github.com/Fred78290/cakeagent/pkg"       // Remplacez par le chemin de votre module
	"github.com/Fred78290/cakeagent/service"
	glog "github.com/sirupsen/logrus"
)

var phVersion = "v0.0.0-unset"
var phBuildDate = ""

func main() {
	cfg := types.NewConfig()

	if err := cfg.ParseFlags(os.Args[1:], phVersion); err != nil {
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
		glog.Infof("The current version is: %s, build at: %s", phVersion, phBuildDate)
	} else if cfg.InstallService {
		err = service.InstallService(cfg.Address)
	} else {
		_, err = pkg.StartServer(cfg)
	}

	if err != nil {
		glog.Fatalf("%v", err)
	}
}
