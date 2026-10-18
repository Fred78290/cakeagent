//go:build windows
// +build windows

package console

import "context"

type Console struct {
	Address    string `json:"address"`
	SyslogFile string `json:"syslog_file,omitempty"`
}

func NewConsole() *Console {
	return nil
}

func (c *Console) Forward(_ context.Context) {}
