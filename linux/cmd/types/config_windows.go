//go:build windows
// +build windows

package types

func defaultListenAddress() string {
	return "tcp://0.0.0.0:5000"
}
