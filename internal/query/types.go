package query

import "fwlog/internal/model"

type IngestStatus = model.IngestStatus
type DateIngestState = model.DateIngestState

const (
	StatusIdle      = model.StatusIdle
	StatusPending   = model.StatusPending
	StatusScanning  = model.StatusScanning
	StatusImporting = model.StatusImporting
	StatusReady     = model.StatusReady
	StatusFailed    = model.StatusFailed
	StatusSucceeded = model.StatusSucceeded
)
