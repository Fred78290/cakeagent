//go:build darwin
// +build darwin

package utils

/*
#include <stdlib.h>
#include <stdint.h>
#include <sys/types.h>
#include <sys/sysctl.h>
*/
import "C"

import "unsafe"

func sysctlUint64(name string) (uint64, bool) {
	cname := C.CString(name)
	if cname == nil {
		return 0, false
	}
	defer C.free(unsafe.Pointer(cname))

	var out64 C.uint64_t
	size64 := C.size_t(C.sizeof_uint64_t)
	if C.sysctlbyname(cname, unsafe.Pointer(&out64), &size64, nil, 0) == 0 {
		return uint64(out64), true
	}

	var out32 C.uint32_t
	size32 := C.size_t(C.sizeof_uint32_t)
	if C.sysctlbyname(cname, unsafe.Pointer(&out32), &size32, nil, 0) == 0 {
		return uint64(out32), true
	}

	return 0, false
}

func sysctlSwapUsage() (uint64, uint64, bool) {
	cname := C.CString("vm.swapusage")
	if cname == nil {
		return 0, 0, false
	}
	defer C.free(unsafe.Pointer(cname))

	var swap C.struct_xsw_usage
	size := C.size_t(C.sizeof_struct_xsw_usage)
	if C.sysctlbyname(cname, unsafe.Pointer(&swap), &size, nil, 0) != 0 {
		return 0, 0, false
	}

	return uint64(swap.xsu_total), uint64(swap.xsu_used), true
}

func getMemoryUsage() MemoryUsage {
	total, _ := sysctlUint64("hw.memsize")
	freePages, okPages := sysctlUint64("vm.page_free_count")
	pageSize, okPageSize := sysctlUint64("hw.pagesize")

	free := uint64(0)
	if okPages && okPageSize {
		free = freePages * pageSize
	}

	used := uint64(0)
	if total >= free {
		used = total - free
	}

	swapTotal := uint64(0)
	swapUsed := uint64(0)
	if t, u, ok := sysctlSwapUsage(); ok {
		swapTotal = t
		swapUsed = u
	}

	swapFree := uint64(0)
	if swapTotal >= swapUsed {
		swapFree = swapTotal - swapUsed
	}

	return MemoryUsage{
		Total:     total,
		Free:      free,
		Used:      used,
		SwapTotal: swapTotal,
		SwapFree:  swapFree,
		SwapUsed:  swapUsed,
	}
}
