//go:build linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Fred78290/cakeagent/pkg/seamless"
	glog "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func seamlessConfig() seamless.Config {
	return seamless.Config{
		User:       cfg.SeamlessUser,
		Port:       cfg.SeamlessPort,
		Binary:     cfg.SeamlessBinary,
		Layout:     cfg.SeamlessLayout,
		SocketName: "",
	}
}

// startSeamless runs the compositor next to the agent when the service was installed with --seamless.
//
// Catching SIGINT/SIGTERM here turns off Go's default (the process dies), and nothing else in the agent handles
// them: without the forwarding below "systemctl stop cakeagent" waited for systemd's SIGKILL. So once the
// compositor has been stopped the signal is delivered again with its default action.
func startSeamless() {
	if !cfg.Seamless {
		return
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)

		if err := seamless.Run(ctx, seamlessConfig()); err != nil {
			glog.Errorf("seamless: %v", err)
		}
	}()

	go func() {
		received := <-signals

		cancel()

		// The compositor gets SIGTERM and a few seconds to leave, Run returns once it has.
		select {
		case <-done:
		case <-time.After(8 * time.Second):
		}

		signal.Reset(syscall.SIGINT, syscall.SIGTERM)

		if sig, ok := received.(syscall.Signal); ok {
			_ = syscall.Kill(syscall.Getpid(), sig)
		}

		// The signal ends the process by its default action; this is only for it not arriving in time (and a
		// clean exit status, so that systemd does not mark the unit failed on a stop).
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}()
}

var seamlessCmd = &cobra.Command{
	Use:   "seamless",
	Short: "Show the guest's windows on the host (Caker seamless windows)",
	Long: `Seamless windows: a Wayland compositor in the guest (caker-seamlessd) streams every window to the host over
vsock, where each one is shown as a native window.

The agent ships the compositor and runs it as the guest's login user; applications are started with
"cakeagent seamless launch" so they find it.`,
}

var seamlessInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the compositor binary",
	Run: func(cmd *cobra.Command, args []string) {
		setupLogging()

		source, changed, err := seamless.Install(cfg.SeamlessBinary)

		if err != nil {
			glog.Fatalf("%v", err)
		}

		if changed {
			fmt.Printf("installed %s (from %s)\n", binaryPath(), source)
		} else {
			fmt.Printf("%s is up to date\n", binaryPath())
		}
	},
}

func binaryPath() string {
	if cfg.SeamlessBinary != "" {
		return cfg.SeamlessBinary
	}

	return seamless.DefaultBinary
}

var seamlessRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the compositor in the foreground, restarting it when it exits",
	Run: func(cmd *cobra.Command, args []string) {
		setupLogging()

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		if err := seamless.Run(ctx, seamlessConfig()); err != nil {
			glog.Fatalf("%v", err)
		}
	},
}

var seamlessStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Tell whether the compositor is installed and running",
	Run: func(cmd *cobra.Command, args []string) {
		st := seamless.CurrentStatus(seamlessConfig())

		if jsonOutput {
			out, _ := json.MarshalIndent(st, "", "  ")
			fmt.Println(string(out))
		} else {
			fmt.Printf("binary:   %s (installed: %t)\nuser:     %s\nruntime:  %s\ndisplay:  %s\nrunning:  %t\n", st.Binary, st.Installed, st.User, st.RuntimeDir, st.WaylandDisplay, st.Running)
		}

		if !st.Running {
			os.Exit(1)
		}
	},
}

var seamlessAppsCmd = &cobra.Command{
	Use:   "apps",
	Short: "List the guest's applications (.desktop files)",
	Run: func(cmd *cobra.Command, args []string) {
		home := ""

		if a, err := seamless.LookupAccount(cfg.SeamlessUser); err == nil {
			home = a.Home
		}

		apps := seamless.ListApps(seamless.ApplicationDirs(home))

		if jsonOutput {
			out, _ := json.MarshalIndent(apps, "", "  ")
			fmt.Println(string(out))

			return
		}

		for _, app := range apps {
			fmt.Printf("%-40s %s\n", app.ID, app.Name)
		}
	},
}

var seamlessEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Print the environment applications need to use the compositor",
	Run: func(cmd *cobra.Command, args []string) {
		env, err := seamless.Env(seamlessConfig())

		if err != nil {
			glog.Fatalf("%v", err)
		}

		fmt.Println(strings.Join(env, "\n"))
	},
}

var seamlessLaunchDesktop string

var seamlessLaunchCmd = &cobra.Command{
	Use:   "launch [--desktop id | -- command args...]",
	Short: "Start an application as the guest's user, shown through the compositor",
	Example: `  cakeagent seamless launch --desktop org.gnome.gedit
  cakeagent seamless launch -- foot`,
	Run: func(cmd *cobra.Command, args []string) {
		setupLogging()

		var pid int
		var err error

		if seamlessLaunchDesktop != "" {
			pid, err = seamless.LaunchDesktop(seamlessConfig(), seamlessLaunchDesktop)
		} else {
			pid, err = seamless.Launch(seamlessConfig(), args)
		}

		if err != nil {
			glog.Fatalf("%v", err)
		}

		fmt.Println(pid)
	},
}

func init() {
	for _, c := range []*cobra.Command{serveCmd, serviceInstallCmd} {
		c.Flags().BoolVar(&cfg.Seamless, "seamless", cfg.Seamless, "Run the seamless windows compositor next to the agent")
	}

	// Shared by the seamless subcommands, serve and service install.
	for _, c := range []*cobra.Command{serveCmd, serviceInstallCmd, seamlessCmd} {
		c.PersistentFlags().StringVar(&cfg.SeamlessUser, "seamless-user", cfg.SeamlessUser, "User the windows and applications belong to (default: the first regular user)")
		c.PersistentFlags().IntVar(&cfg.SeamlessPort, "seamless-port", cfg.SeamlessPort, "Host vsock port of the seamless windows channel (default 5100)")
		c.PersistentFlags().StringVar(&cfg.SeamlessLayout, "seamless-layout", cfg.SeamlessLayout, "XKB keyboard layout, must match the host's")
		c.PersistentFlags().StringVar(&cfg.SeamlessBinary, "seamless-binary", cfg.SeamlessBinary, "Compositor path (default "+seamless.DefaultBinary+")")
	}

	seamlessLaunchCmd.Flags().StringVar(&seamlessLaunchDesktop, "desktop", "", "Launch the application with this desktop file ID")
	seamlessStatusCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	seamlessAppsCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	seamlessCmd.AddCommand(seamlessInstallCmd, seamlessRunCmd, seamlessStatusCmd, seamlessAppsCmd, seamlessEnvCmd, seamlessLaunchCmd)
	rootCmd.AddCommand(seamlessCmd)
}
