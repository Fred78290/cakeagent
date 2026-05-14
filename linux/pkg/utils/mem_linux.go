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

	total := uint64(in.Totalram)
	free := uint64(in.Freeram)
	used := uint64(0)
	if total >= free {
		used = total - free
	}

	swapTotal := uint64(in.Totalswap)
	swapFree := uint64(in.Freeswap)
	swapUsed := uint64(0)
	if swapTotal >= swapFree {
		swapUsed = swapTotal - swapFree
	}

	return MemoryUsage{
		Total:     total * unit,
		Free:      free * unit,
		Used:      used * unit,
		SwapTotal: swapTotal * unit,
		SwapFree:  swapFree * unit,
		SwapUsed:  swapUsed * unit,
	}
}
