package types

import (
	"fmt"
	"strconv"
	"time"

	"github.com/alecthomas/kingpin"
	glog "github.com/sirupsen/logrus"
)

const (
	DefaultMaxRequestTimeout time.Duration = 120 * time.Second
	DefaulktTickEvent        time.Duration = 1 * time.Second
)

type Config struct {
	InstallService  bool
	Mounts          []string
	Address         string
	CaCert          string
	TlsCert         string
	TlsKey          string
	LogFormat       string
	LogLevel        string
	DisplayVersion  bool
	ConsoleEndpoint string
	Timeout         time.Duration
	TickEvent       time.Duration
}

func NewConfig() *Config {
	return &Config{
		Address:        "vsock://any:5000",
		DisplayVersion: false,
		LogFormat:      "text",
		LogLevel:       glog.InfoLevel.String(),
		Timeout:        DefaultMaxRequestTimeout,
		TickEvent:      DefaulktTickEvent,
	}
}

// allLogLevelsAsStrings returns all logrus levels as a list of strings
func allLogLevelsAsStrings() []string {
	var levels []string
	for _, level := range glog.AllLevels {
		levels = append(levels, level.String())
	}
	return levels
}

func (cfg *Config) String() string {
	return fmt.Sprintf("Address: %s CaCert: %s TlsCert: %s TlsKey: %s LogFormat: %s LogLevel: %s, DisplayVersion: %s",
		cfg.Address, cfg.CaCert, cfg.TlsCert, cfg.TlsKey, cfg.LogFormat, cfg.LogLevel, strconv.FormatBool(cfg.DisplayVersion))
}

func (cfg *Config) ParseFlags(args []string, version string) error {
	app := kingpin.New("cakeagent", "Cake agent running in guest vm.\n\nNote that all flags may be replaced with env vars prefixed with CAKEAGENT_")

	//	app.Version(version)
	app.HelpFlag.Short('h')
	app.DefaultEnvars()

	app.Flag("install", "Install as service").Default("false").BoolVar(&cfg.InstallService)
	app.Flag("mount", "Mount endpoint").StringsVar(&cfg.Mounts)
	app.Flag("listen", "Listen on address").Default(cfg.Address).StringVar(&cfg.Address)
	app.Flag("console", "Console endpoint").Default(cfg.ConsoleEndpoint).StringVar(&cfg.ConsoleEndpoint)

	app.Flag("ca-cert", "CA TLS certificate").Default(cfg.CaCert).StringVar(&cfg.CaCert)
	app.Flag("tls-cert", "Server TLS certificate").Default(cfg.TlsCert).StringVar(&cfg.TlsCert)
	app.Flag("tls-key", "Server private key").Default(cfg.TlsKey).StringVar(&cfg.TlsKey)

	app.Flag("log-format", "The format in which log messages are printed (default: text, options: text, json)").Default(cfg.LogFormat).EnumVar(&cfg.LogFormat, "text", "json")
	app.Flag("log-level", "Set the level of logging. (default: info, options: panic, debug, info, warning, error, fatal").Default(cfg.LogLevel).EnumVar(&cfg.LogLevel, allLogLevelsAsStrings()...)

	app.Flag("timeout", "Request timeout for operation. 0s means no timeout").Default(DefaultMaxRequestTimeout.String()).DurationVar(&cfg.Timeout)
	app.Flag("tick", "Tick event idling").Default(DefaulktTickEvent.String()).DurationVar(&cfg.TickEvent)

	app.Flag("version", "Display version and exit").BoolVar(&cfg.DisplayVersion)

	_, err := app.Parse(args)
	if err != nil {
		return err
	}

	return nil
}
