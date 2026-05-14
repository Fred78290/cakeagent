//go:build linux
// +build linux

package utils

import "syscall"

func getMemoryUsage() MemoryUsage {
	in := &syscall.Sysinfo_t{}
	err := syscall.Sysinfo(in)

	if err != nil {
		return MemoryUsage{}
	}

	unit := uint64(in.Unit)
	if unit == 0 {
		unit = 1
	}

	return MemoryUsage{
		Total:     uint64(in.Totalram) * unit,
		Free:      uint64(in.Freeram) * unit,
		Used:      uint64(in.Totalram-in.Freeram) * unit,
		SwapTotal: uint64(in.Totalswap) * unit,
		SwapFree:  uint64(in.Freeswap) * unit,
		SwapUsed:  uint64(in.Totalswap-in.Freeswap) * unit,
	}
}
