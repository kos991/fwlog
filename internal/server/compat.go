package server

import (
	"context"

	"fwlog/internal/config"
	"fwlog/internal/importer"
	"fwlog/internal/ip"
	"fwlog/internal/storage/clickhouse"
)

func OpenClickHouse(ctx context.Context, cfg Config) (*ClickHouseStore, error) {
	return clickhouse.OpenClickHouse(ctx, cfg)
}

func LoadConfig() Config {
	return config.LoadConfig()
}

func NewIPEngine() *IPEngine {
	return ip.NewIPEngine()
}

func NewImportCoordinator(maxSources, maxWrites int) *ImportCoordinator {
	return importer.NewImportCoordinator(maxSources, maxWrites)
}

type batchWriteGate interface {
	WithWriteSlot(context.Context, func() error) error
}
