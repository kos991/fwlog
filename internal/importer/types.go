package importer

import (
	"fwlog/internal/model"
	"fwlog/internal/storage/clickhouse"
)

type IngestStatus = model.IngestStatus
type LogSource = model.LogSource
type DateIngestState = model.DateIngestState
type FileIngestState = model.FileIngestState
type ClickHouseStore = clickhouse.ClickHouseStore

const (
	StatusIdle      = model.StatusIdle
	StatusPending   = model.StatusPending
	StatusScanning  = model.StatusScanning
	StatusImporting = model.StatusImporting
	StatusReady     = model.StatusReady
	StatusFailed    = model.StatusFailed
	StatusSucceeded = model.StatusSucceeded
)
