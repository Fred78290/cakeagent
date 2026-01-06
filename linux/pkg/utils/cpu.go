package utils

import (
	"fmt"
	"math"
	"runtime"
	"time"

	"github.com/Fred78290/cakeagent/pkg/cakeagent"
	"github.com/shirou/gopsutil/v4/cpu"
	glog "github.com/sirupsen/logrus"
)

type CPUUsage struct {
	LastTotalCpuTimes   []cpu.TimesStat
	LastPerCoreCpuTimes []cpu.TimesStat
	TotalCpuUsage       []float64
	PerCoreCpuUsage     []float64
}

func getAllBusy(t cpu.TimesStat) (float64, float64) {
	tot := t.Total()
	if runtime.GOOS == "linux" {
		tot -= t.Guest     // Linux 2.6.24+
		tot -= t.GuestNice // Linux 3.2.0+
	}

	busy := tot - t.Idle - t.Iowait

	return tot, busy
}

func calculateBusy(t1, t2 cpu.TimesStat) float64 {
	t1All, t1Busy := getAllBusy(t1)
	t2All, t2Busy := getAllBusy(t2)

	if t2Busy <= t1Busy {
		return 0
	}

	if t2All <= t1All {
		return 100
	}

	return math.Min(100, math.Max(0, (t2Busy-t1Busy)/(t2All-t1All)*100))
}

func calculateAllBusy(t1, t2 []cpu.TimesStat) ([]float64, error) {
	// Make sure the CPU measurements have the same length.
	if len(t1) != len(t2) {
		return nil, fmt.Errorf(
			"received two CPU counts: %d != %d",
			len(t1), len(t2),
		)
	}

	ret := make([]float64, len(t1))

	for i, t := range t2 {
		ret[i] = calculateBusy(t1[i], t)
	}

	return ret, nil
}

func NewCPUUsage(interval time.Duration) (usage *CPUUsage, err error) {
	usage = &CPUUsage{}

	if err = usage.Setup(interval); err != nil {
		return nil, err
	}

	return usage, nil
}

func (c *CPUUsage) Setup(interval time.Duration) (err error) {
	// Get detailed CPU times
	if c.LastTotalCpuTimes, err = cpu.Times(false); err != nil {
		glog.Errorf("Failed to get CPU times: %v", err)
		return
	}

	// Get detailed CPU stats per core
	if c.LastPerCoreCpuTimes, err = cpu.Times(true); err != nil {
		glog.Errorf("Error getting CPU stats: %v", err)
		return
	}

	// Get CPU usage percentage (total)
	if c.TotalCpuUsage, err = cpu.Percent(interval, false); err != nil {
		glog.Errorf("Error getting total CPU usage: %v", err)
		return
	}

	// Get CPU usage percentage per core
	if c.PerCoreCpuUsage, err = cpu.Percent(interval, true); err != nil {
		glog.Errorf("Error getting per-core CPU usage: %v", err)
		return
	}

	return
}

func (c *CPUUsage) Infos() (cpuInfo *cakeagent.CakeAgent_InfoReply_CpuInfo) {
	if len(c.LastPerCoreCpuTimes) == 0 || len(c.TotalCpuUsage) == 0 {
		return
	}

	// Use times from the first core (average of all cores)
	times := c.LastTotalCpuTimes[0]
	cpuInfo = &cakeagent.CakeAgent_InfoReply_CpuInfo{
		TotalUsagePercent: c.TotalCpuUsage[0],
		User:              times.User,
		System:            times.System,
		Idle:              times.Idle,
		Iowait:            times.Iowait,
		Irq:               times.Irq,
		Softirq:           times.Softirq,
		Steal:             times.Steal,
		Guest:             times.Guest,
		GuestNice:         times.GuestNice,
		Nice:              times.Nice,
	}

	// Use cpuStats for per-core data, fallback to cpuTimes if empty
	coreStatsToUse := c.LastPerCoreCpuTimes
	if len(coreStatsToUse) == 0 {
		coreStatsToUse = c.LastTotalCpuTimes
	}

	// Some platforms include total as first element
	startIndex := 0
	if len(coreStatsToUse) > runtime.NumCPU() {
		// Probably includes total as first element
		startIndex = 1
	}

	// Process up to NumCPU cores
	for i := startIndex; i < len(coreStatsToUse) && (i-startIndex) < runtime.NumCPU(); i++ {
		coreTime := coreStatsToUse[i]
		coreIndex := i - startIndex // Actual core index starting at 0

		usagePercent := float64(0)
		if coreIndex < len(c.PerCoreCpuUsage) {
			usagePercent = c.PerCoreCpuUsage[coreIndex]
		}

		coreInfo := &cakeagent.CakeAgent_InfoReply_CpuCoreInfo{
			CoreId:       int32(coreIndex),
			UsagePercent: usagePercent,
			User:         coreTime.User,
			System:       coreTime.System,
			Idle:         coreTime.Idle,
			Iowait:       coreTime.Iowait,
			Irq:          coreTime.Irq,
			Softirq:      coreTime.Softirq,
			Steal:        coreTime.Steal,
			Guest:        coreTime.Guest,
			GuestNice:    coreTime.GuestNice,
		}

		cpuInfo.Cores = append(cpuInfo.Cores, coreInfo)
	}

	return
}

func (c *CPUUsage) Collect() (cpuInfo *cakeagent.CakeAgent_InfoReply_CpuInfo, err error) {
	var totalCpuTimes []cpu.TimesStat
	var perCoreCpuTimes []cpu.TimesStat
	var totalCpuUsage []float64
	var perCoreCpuUsage []float64

	// Get detailed CPU times
	if totalCpuTimes, err = cpu.Times(false); err != nil {
		glog.Errorf("Failed to get CPU times: %v", err)
		return
	}

	// Get detailed CPU stats per core
	if perCoreCpuTimes, err = cpu.Times(true); err != nil {
		glog.Errorf("Error getting CPU stats: %v", err)
		return
	}

	if totalCpuUsage, err = calculateAllBusy(c.LastTotalCpuTimes, totalCpuTimes); err != nil {
		glog.Errorf("Error calculating total CPU usage: %v", err)
		return
	}

	if perCoreCpuUsage, err = calculateAllBusy(c.LastPerCoreCpuTimes, perCoreCpuTimes); err != nil {
		glog.Errorf("Error calculating total CPU usage: %v", err)
		return
	}

	c.LastTotalCpuTimes = totalCpuTimes
	c.LastPerCoreCpuTimes = perCoreCpuTimes
	c.TotalCpuUsage = totalCpuUsage
	c.PerCoreCpuUsage = perCoreCpuUsage

	return c.Infos(), nil
}
