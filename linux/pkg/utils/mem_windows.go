//go:build windows
// +build windows

package utils

import "github.com/shirou/gopsutil/v4/mem"

func getMemoryUsage() MemoryUsage {
	usage := MemoryUsage{}

	if vm, err := mem.VirtualMemory(); err == nil {
		usage.Total = vm.Total
		usage.Free = vm.Available
		usage.Used = vm.Used
	}

	if swap, err := mem.SwapMemory(); err == nil {
		usage.SwapTotal = swap.Total
		usage.SwapUsed = swap.Used
		if swap.Total >= swap.Used {
			usage.SwapFree = swap.Total - swap.Used
		}
	}

	return usage
}
