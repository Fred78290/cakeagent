//go:build windows
// +build windows

package serialport

import (
	"fmt"
	"os"
)

func openPort(name string) (p *Port, err error) {
	return nil, fmt.Errorf("serial port %q is not supported on windows", name)
}

type Port struct {
	f *os.File
}

func (p *Port) Read(_ []byte) (n int, err error) {
	return 0, fmt.Errorf("serial read is not supported on windows")
}

func (p *Port) Write(_ []byte) (n int, err error) {
	return 0, fmt.Errorf("serial write is not supported on windows")
}

func (p *Port) Close() (err error) {
	return nil
}
