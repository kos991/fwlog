package dashboard

import (
	"time"

	"fwlog/internal/importer"
	"fwlog/internal/ip"
)

func NewIPEngine() *IPEngine {
	return ip.NewIPEngine()
}

func BuildAutoScanPlan(settings map[string]string, now time.Time) importer.AutoScanPlan {
	return importer.BuildAutoScanPlan(settings, now)
}
