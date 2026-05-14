package utils

type MemoryUsage struct {
	Total     uint64
	Free      uint64
	Used      uint64
	SwapTotal uint64
	SwapFree  uint64
	SwapUsed  uint64
}

func GetMemoryUsage() MemoryUsage {
	return getMemoryUsage()
}
