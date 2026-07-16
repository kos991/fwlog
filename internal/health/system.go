package health

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const initialCPUSampleInterval = 100 * time.Millisecond

type cpuTimes struct {
	Total uint64
	Idle  uint64
}

var cpuSampler struct {
	sync.Mutex
	previous cpuTimes
	ready    bool
}

func CollectSystemHealth(database DatabaseHealth) SystemHealth {
	return SystemHealth{
		CPU:      collectCPUHealth(),
		Memory:   collectMemoryHealth(),
		Database: database,
	}
}

func collectCPUHealth() CPUHealth {
	cores := runtime.NumCPU()
	health := CPUHealth{
		Status:      "unknown",
		Cores:       cores,
		Description: "当前平台暂不支持 CPU 采集",
	}

	if loadAvg, ok := readLoadAverage(); ok {
		health.LoadAverage = round1(loadAvg)
	}

	current, ok := readCPUTimes()
	if !ok {
		return health
	}
	usagePercent, ok := sampleCPUUsage(current)
	if !ok {
		time.Sleep(initialCPUSampleInterval)
		current, ok = readCPUTimes()
		if !ok {
			return health
		}
		usagePercent, ok = sampleCPUUsage(current)
		if !ok {
			return health
		}
	}

	health.LoadPercent = round1(usagePercent)
	health.Status = statusByPercent(usagePercent, 70, 90)
	health.Description = "CPU 使用率正常"
	if health.Status == "warning" {
		health.Description = "CPU 使用率偏高"
	}
	if health.Status == "critical" {
		health.Description = "CPU 使用率过高"
	}
	return health
}

func readCPUTimes() (cpuTimes, bool) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuTimes{}, false
	}
	return parseCPUTimes(string(data))
}

func parseCPUTimes(data string) (cpuTimes, bool) {
	line, _, _ := strings.Cut(data, "\n")
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuTimes{}, false
	}

	values := make([]uint64, 0, 8)
	for _, field := range fields[1:] {
		if len(values) == 8 {
			break
		}
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return cpuTimes{}, false
		}
		values = append(values, value)
	}
	if len(values) < 4 {
		return cpuTimes{}, false
	}

	var total uint64
	for _, value := range values {
		total += value
	}
	idle := values[3]
	if len(values) > 4 {
		idle += values[4]
	}
	return cpuTimes{Total: total, Idle: idle}, total > 0
}

func sampleCPUUsage(current cpuTimes) (float64, bool) {
	cpuSampler.Lock()
	defer cpuSampler.Unlock()

	previous := cpuSampler.previous
	ready := cpuSampler.ready
	cpuSampler.previous = current
	cpuSampler.ready = true
	if !ready {
		return 0, false
	}
	return cpuUsagePercent(previous, current)
}

func cpuUsagePercent(previous, current cpuTimes) (float64, bool) {
	if current.Total <= previous.Total || current.Idle < previous.Idle {
		return 0, false
	}
	totalDelta := current.Total - previous.Total
	idleDelta := current.Idle - previous.Idle
	if idleDelta > totalDelta {
		return 0, false
	}
	return float64(totalDelta-idleDelta) / float64(totalDelta) * 100, true
}

func collectMemoryHealth() MemoryHealth {
	health := MemoryHealth{
		Status:      "unknown",
		Description: "当前平台暂不支持内存采集",
	}

	total, available, ok := readMemInfo()
	if !ok || total == 0 {
		return health
	}

	usedPercent := (float64(total-available) / float64(total)) * 100
	health.TotalBytes = total
	health.AvailableBytes = available
	health.UsedPercent = round1(usedPercent)
	health.Status = statusByPercent(usedPercent, 75, 90)
	health.Description = "内存余量正常"
	if health.Status == "warning" {
		health.Description = "内存使用偏高"
	}
	if health.Status == "critical" {
		health.Description = "内存余量不足"
	}
	return health
}

func readLoadAverage() (float64, bool) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, false
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	return value, err == nil
}

func readMemInfo() (uint64, uint64, bool) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, false
	}

	values := map[string]uint64{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		values[key] = value * 1024
	}

	total := values["MemTotal"]
	available := values["MemAvailable"]
	if available == 0 {
		available = values["MemFree"] + values["Buffers"] + values["Cached"]
	}
	return total, available, total > 0
}

func statusByPercent(value float64, warning float64, critical float64) string {
	switch {
	case value >= critical:
		return "critical"
	case value >= warning:
		return "warning"
	default:
		return "ok"
	}
}

func round1(value float64) float64 {
	return float64(int(value*10+0.5)) / 10
}
