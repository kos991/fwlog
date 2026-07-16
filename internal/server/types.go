package server

import (
	"fwlog/internal/config"
	"fwlog/internal/dashboard"
	"fwlog/internal/importer"
	"fwlog/internal/ip"
	"fwlog/internal/model"
	"fwlog/internal/query"
	"fwlog/internal/storage/clickhouse"
)

type Config = config.Config
type ClickHouseStore = clickhouse.ClickHouseStore
type ImportCoordinator = importer.ImportCoordinator
type ImportStartResult = importer.ImportStartResult
type IPEngine = ip.IPEngine
type LogSource = importer.LogSource
type DateIngestState = importer.DateIngestState
type FileIngestState = importer.FileIngestState
type IngestStatus = model.IngestStatus
type IPDataStatus = ip.IPDataStatus
type DashboardMetrics = dashboard.DashboardMetrics
type DistributionItem = dashboard.DistributionItem
type DataHealth = dashboard.DataHealth
type IngestHealth = dashboard.IngestHealth
type IPDistribution = dashboard.IPDistribution
type GeoDistribution = dashboard.GeoDistribution
type QueryRequest = query.QueryRequest
type QueryResponse = query.QueryResponse
type QueryError = query.QueryError
type QueryCursor = query.QueryCursor
type QueryPageOptions = query.QueryPageOptions
type QueryVisibility = query.QueryVisibility
type VisibleRange = query.VisibleRange
type SkippedLogDate = query.SkippedLogDate
type CIDRAliasSetting = config.CIDRAliasSetting
type HealthDashboardResponse = dashboard.HealthDashboardResponse
type IngestProgressResponse = dashboard.IngestProgressResponse

const (
	StatusIdle      = model.StatusIdle
	StatusPending   = model.StatusPending
	StatusScanning  = model.StatusScanning
	StatusImporting = model.StatusImporting
	StatusReady     = model.StatusReady
	StatusNoData    = model.StatusNoData
	StatusFailed    = model.StatusFailed
	StatusSucceeded = model.StatusSucceeded
)
