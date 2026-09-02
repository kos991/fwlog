package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"fwlog/internal/threatintel"
)

type threatIntelligenceService interface {
	Providers(context.Context) ([]threatintel.ProviderStatus, error)
	UpdateProvider(context.Context, threatintel.Provider, threatintel.ProviderConfigUpdate) (threatintel.ProviderStatus, error)
	TestProvider(context.Context, threatintel.Provider) (threatintel.ProviderTestStatus, error)
	Result(context.Context, threatintel.Provider, string) (*threatintel.Result, error)
	Analyze(context.Context, threatintel.Provider, string) (threatintel.AnalyzeOutcome, error)
}

type appThreatIntelligenceResultStore struct {
	app *App
}

var _ threatintel.ResultStore = (*appThreatIntelligenceResultStore)(nil)

func (s *appThreatIntelligenceResultStore) LatestResult(ctx context.Context, provider threatintel.Provider, ip string) (threatintel.Result, bool, error) {
	store := s.appStore()
	if store == nil {
		return threatintel.Result{}, false, errors.New("clickhouse connection is not initialized")
	}
	return store.LatestResult(ctx, provider, ip)
}

func (s *appThreatIntelligenceResultStore) SaveResult(ctx context.Context, result threatintel.Result) error {
	store := s.appStore()
	if store == nil {
		return errors.New("clickhouse connection is not initialized")
	}
	return store.SaveResult(ctx, result)
}

func (s *appThreatIntelligenceResultStore) appStore() *ClickHouseStore {
	if s == nil || s.app == nil {
		return nil
	}
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	return s.app.store
}

func newThreatIntelligenceService(app *App) *threatintel.Service {
	return threatintel.NewService(
		newAppThreatIntelligenceConfigStore(app),
		&appThreatIntelligenceResultStore{app: app},
		threatintel.DefaultAdapters(http.DefaultClient),
		15*time.Second,
	)
}
