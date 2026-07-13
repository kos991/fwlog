package storage

import (
	"context"
	"time"

	"fwlog/internal/importer"
)

type IngestStateStore interface {
	ListDateStates(context.Context, time.Time) ([]importer.DateIngestState, error)
	WriteDateState(context.Context, importer.DateIngestState) error
	WriteFileState(context.Context, importer.FileIngestState) error
	LatestDateState(context.Context, string, time.Time) (importer.DateIngestState, bool, error)
	LatestFileState(context.Context, string) (importer.FileIngestState, bool, error)
}
