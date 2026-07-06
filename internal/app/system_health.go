package app

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

func collectSystemHealth(database DatabaseHealth) SystemHealth {
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

	loadAvg, ok := readLoadAverage()
	if !ok || cores <= 0 {
		return health
	}

	loadPercent := (loadAvg / float64(cores)) * 100
	health.LoadAverage = loadAvg
	health.LoadPercent = round1(loadPercent)
	health.Status = statusByPercent(loadPercent, 70, 90)
	health.Description = "CPU 负载正常"
	if health.Status == "warning" {
		health.Description = "CPU 负载偏高"
	}
	if health.Status == "critical" {
		health.Description = "CPU 负载过高"
	}
	return health
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
