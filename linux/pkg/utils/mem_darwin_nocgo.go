//go:build darwin && !cgo
// +build darwin,!cgo

package utils

import "golang.org/x/sys/unix"

func getMemoryUsage() MemoryUsage {
	total, _ := unix.SysctlUint64("hw.memsize")
	freePages, okPages := unix.SysctlUint64("vm.page_free_count")
	pageSize, okPageSize := unix.SysctlUint64("hw.pagesize")

	free := uint64(0)
	if okPages == nil && okPageSize == nil {
		free = freePages * pageSize
	}

	used := uint64(0)
	if total >= free {
		used = total - free
	}

	// vm.swapusage requires xsw_usage from C headers; keep swap at 0 in no-cgo builds.
	return MemoryUsage{
		Total:     total,
		Free:      free,
		Used:      used,
		SwapTotal: 0,
		SwapFree:  0,
		SwapUsed:  0,
	}
}
