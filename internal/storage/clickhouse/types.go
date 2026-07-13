package clickhouse

import (
	"fwlog/internal/config"
	"fwlog/internal/model"
	"fwlog/internal/query"
)

type Config = config.Config
type IngestStatus = model.IngestStatus
type DateIngestState = model.DateIngestState
type FileIngestState = model.FileIngestState
type QueryCursor = query.QueryCursor
type QueryPageOptions = query.QueryPageOptions
type DashboardMetrics = model.DashboardMetrics
type DistributionItem = model.DistributionItem
type LogTrendPoint = model.LogTrendPoint
type SystemHealth = model.SystemHealth
type CPUHealth = model.CPUHealth
type MemoryHealth = model.MemoryHealth
type DatabaseHealth = model.DatabaseHealth

const (
	StatusIdle      = model.StatusIdle
	StatusPending   = model.StatusPending
	StatusScanning  = model.StatusScanning
	StatusImporting = model.StatusImporting
	StatusReady     = model.StatusReady
	StatusFailed    = model.StatusFailed
	StatusSucceeded = model.StatusSucceeded
)
