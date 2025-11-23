package types

import (
	"fmt"
	"time"

	glog "github.com/sirupsen/logrus"
)

const (
	DefaultMaxRequestTimeout time.Duration = 120 * time.Second
	DefaulktTickEvent        time.Duration = 1 * time.Second
)

type Config struct {
	Mounts    []string
	Address   string
	CaCert    string
	TlsCert   string
	TlsKey    string
	LogFormat string
	LogLevel  string
	Timeout   time.Duration
	TickEvent time.Duration
}

func NewConfig() *Config {
	return &Config{
		Address:   "vsock://any:5000",
		LogFormat: "text",
		LogLevel:  glog.InfoLevel.String(),
		Timeout:   DefaultMaxRequestTimeout,
		TickEvent: DefaulktTickEvent,
	}
}

func (cfg *Config) String() string {
	return fmt.Sprintf("Address: %s CaCert: %s TlsCert: %s TlsKey: %s LogFormat: %s LogLevel: %s",
		cfg.Address, cfg.CaCert, cfg.TlsCert, cfg.TlsKey, cfg.LogFormat, cfg.LogLevel)
}
