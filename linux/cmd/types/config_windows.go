//go:build windows
// +build windows

package types

func defaultListenAddress() string {
	return "tcp://127.0.0.1:5000"
}
