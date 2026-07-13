package dashboard

import (
	"fwlog/internal/ip"
	"fwlog/internal/model"
)

type IngestStatus = model.IngestStatus
type DateIngestState = model.DateIngestState
type DashboardMetrics = model.DashboardMetrics
type DistributionItem = model.DistributionItem
type SystemHealth = model.SystemHealth
type CPUHealth = model.CPUHealth
type MemoryHealth = model.MemoryHealth
type DatabaseHealth = model.DatabaseHealth
type IPEngine = ip.IPEngine

const (
	StatusIdle      = model.StatusIdle
	StatusPending   = model.StatusPending
	StatusScanning  = model.StatusScanning
	StatusImporting = model.StatusImporting
	StatusReady     = model.StatusReady
	StatusFailed    = model.StatusFailed
	StatusSucceeded = model.StatusSucceeded
)
