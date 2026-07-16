package health

import "testing"

func TestParseCPUTimesReadsAggregateCounters(t *testing.T) {
	got, ok := parseCPUTimes("cpu  100 20 30 400 50 6 7 8 9 10\ncpu0 1 2 3 4")
	if !ok {
		t.Fatal("aggregate CPU counters should parse")
	}
	if got.Total != 621 || got.Idle != 450 {
		t.Fatalf("CPU counters = %#v, want total=621 idle=450", got)
	}
}

func TestCPUUsagePercentUsesCounterDelta(t *testing.T) {
	previous := cpuTimes{Total: 1000, Idle: 400}
	current := cpuTimes{Total: 1200, Idle: 450}

	got, ok := cpuUsagePercent(previous, current)
	if !ok {
		t.Fatal("increasing CPU counters should produce utilization")
	}
	if got != 75 {
		t.Fatalf("CPU usage = %v, want 75", got)
	}
}

func TestCPUUsagePercentRejectsInvalidCounterMovement(t *testing.T) {
	tests := []struct {
		name     string
		previous cpuTimes
		current  cpuTimes
	}{
		{name: "unchanged", previous: cpuTimes{Total: 100, Idle: 40}, current: cpuTimes{Total: 100, Idle: 40}},
		{name: "total moved backwards", previous: cpuTimes{Total: 100, Idle: 40}, current: cpuTimes{Total: 90, Idle: 45}},
		{name: "idle moved backwards", previous: cpuTimes{Total: 100, Idle: 40}, current: cpuTimes{Total: 110, Idle: 30}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := cpuUsagePercent(tt.previous, tt.current); ok {
				t.Fatal("invalid counters should not produce utilization")
			}
		})
	}
}
