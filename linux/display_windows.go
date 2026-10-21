//go:build windows

package main

import (
	"os"
	"os/signal"
	"path/filepath"

	"github.com/Fred78290/cakeagent/pkg/display"

	glog "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// displayWatchCmd is what the service runs in the user's session (see display.Supervise): the display can only be
// queried and changed from there.
var displayWatchCmd = &cobra.Command{
	Use:    "display-watch",
	Short:  "Make the display follow the size the host asks for",
	Long:   "Make the display follow the size the host asks for. Run by the service in the logged-on user's session.",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		setupLogging()

		// A process started by the service has no console: log to a file
		if programData := os.Getenv("ProgramData"); programData != "" {
			dir := filepath.Join(programData, "caker")

			if err := os.MkdirAll(dir, 0o755); err == nil {
				if file, err := os.OpenFile(filepath.Join(dir, "display.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
					defer file.Close()

					glog.SetOutput(file)
				}
			}
		}

		stop := make(chan struct{})
		interrupt := make(chan os.Signal, 1)

		signal.Notify(interrupt, os.Interrupt)

		go func() {
			<-interrupt
			close(stop)
		}()

		display.Watch(stop)
	},
}

func init() {
	rootCmd.AddCommand(displayWatchCmd)
}
