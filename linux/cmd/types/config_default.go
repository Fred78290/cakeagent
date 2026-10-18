//go:build !windows
// +build !windows

package types

func defaultListenAddress() string {
	return "vsock://any:5000"
}
