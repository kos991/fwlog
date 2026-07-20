package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"fwlog/internal/dashboard"
	"fwlog/internal/health"
	"fwlog/internal/importer"
)

const dashboardTimeout = 20 * time.Second

type appDashboardService struct {
	app *App
}

func (s appDashboardService) HealthDashboard(r *http.Request) (HealthDashboardResponse, error) {
	summary, err := s.DashboardSummary(r)
	if err != nil {
		return HealthDashboardResponse{}, err
	}
	rankings, err := s.DashboardRankings(r)
	if err != nil {
		return HealthDashboardResponse{}, err
	}
	summary.IPDistribution = rankings.IPDistribution
	summary.GeoDistribution = rankings.GeoDistribution
	summary.Cache = rankings.Cache
	return summary, nil
}

func (s appDashboardService) DashboardSummary(r *http.Request) (HealthDashboardResponse, error) {
	store := s.appStore()
	if store == nil {
		return dashboard.BuildHealthDashboard(nil, DashboardMetrics{}), nil
	}
	ctx, cancel := context.WithTimeout(r.Context(), dashboardTimeout)
	defer cancel()
	states, err := store.ListDateStates(ctx, dashboardSince(r))
	if err != nil {
		return HealthDashboardResponse{}, err
	}
	metrics, err := store.DashboardSummaryMetrics(ctx)
	if err != nil {
		return HealthDashboardResponse{}, err
	}
	metrics = s.withAutoScanPlan(metrics)
	metrics.SystemHealth = health.CollectSystemHealth(metrics.SystemHealth.Database)
	return dashboard.BuildHealthDashboard(states, metrics), nil
}

func (s appDashboardService) DashboardRankings(r *http.Request) (HealthDashboardResponse, error) {
	store := s.appStore()
	if store == nil {
		return dashboard.BuildHealthDashboard(nil, DashboardMetrics{}), nil
	}
	ctx, cancel := context.WithTimeout(r.Context(), dashboardTimeout)
	defer cancel()
	since := dashboardMetricsSince(r)
	sourceID := strings.TrimSpace(r.URL.Query().Get("source_id"))
	key := fmt.Sprintf("%s|%s", r.URL.Query().Get("metrics_range"), sourceID)
	if key == "|" {
		key = fmt.Sprintf("%s|%s", since.Format("2006-01-02"), sourceID)
	}
	cache := s.app.dashboardCache
	metrics, cacheState, err := cache.Get(ctx, key, func(loadCtx context.Context) (DashboardMetrics, error) {
		return store.DashboardRankingMetrics(loadCtx, since, sourceID)
	})
	if err != nil {
		return HealthDashboardResponse{}, err
	}
	metrics.GeoIPLoaded = s.geoIPLoaded()
	metrics.GeoIPStatus = s.geoIPStatus()
	metrics = s.withGeoDistributions(metrics)
	response := dashboard.BuildHealthDashboard(nil, metrics)
	response.Cache = &dashboard.DashboardCache{Stale: cacheState.Stale, LoadedAt: cacheState.LoadedAt}
	return response, nil
}

func (s appDashboardService) IngestProgress(r *http.Request) (IngestProgressResponse, error) {
	store := s.appStore()
	includeReady := parseBoolQuery(r, "include_ready", false)
	if store == nil {
		return dashboard.BuildIngestProgress(nil, includeReady), nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), dashboardTimeout)
	defer cancel()

	states, err := store.ListDateStates(ctx, ingestProgressSince(r))
	if err != nil {
		return IngestProgressResponse{}, err
	}
	return dashboard.BuildIngestProgress(states, includeReady, s.withAutoScanPlan(DashboardMetrics{})), nil
}

func (s appDashboardService) appStore() *ClickHouseStore {
	if s.app == nil {
		return nil
	}
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	return s.app.store
}

func (s appDashboardService) geoIPLoaded() bool {
	if s.app == nil {
		return false
	}
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	return s.app.ipStatus.Loaded && s.app.ipStatus.GeoIPEnabled
}

func (s appDashboardService) geoIPStatus() string {
	if s.app == nil {
		return ""
	}
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	return s.app.ipStatus.Error
}

func (s appDashboardService) withGeoDistributions(metrics DashboardMetrics) DashboardMetrics {
	if s.app == nil {
		return metrics
	}
	s.app.mu.RLock()
	engine := s.app.ipEngine
	s.app.mu.RUnlock()
	return dashboard.EnrichGeoDistributionMetrics(metrics, engine)
}

func (s appDashboardService) withAutoScanPlan(metrics DashboardMetrics) DashboardMetrics {
	if s.app == nil {
		return metrics
	}
	s.app.mu.RLock()
	settings := make(map[string]string, len(s.app.settings))
	for key, value := range s.app.settings {
		settings[key] = value
	}
	s.app.mu.RUnlock()

	plan := importer.BuildAutoScanPlan(settings, time.Now())
	metrics.LastAutoScanAt = importer.ParseAutoScanDateTime(settings["last_auto_scan_at"], importer.AutoScanLocation(settings))
	metrics.NextAutoScanAt = plan.NextAt
	metrics.AutoScanPolicy = plan.Policy
	metrics.AutoScanEnabled = plan.Enabled
	metrics.AutoScanMode = plan.Mode
	return metrics
}

func dashboardSince(r *http.Request) time.Time {
	return dashboardRangeSince(r.URL.Query().Get("range"))
}

func dashboardMetricsSince(r *http.Request) time.Time {
	value := strings.TrimSpace(r.URL.Query().Get("metrics_range"))
	if value != "" {
		return dashboardRangeSince(value)
	}
	return dashboardSince(r)
}

func dashboardRangeSince(value string) time.Time {
	switch strings.TrimSpace(value) {
	case "today":
		return startOfDay(time.Now())
	case "yesterday":
		return startOfDay(time.Now().AddDate(0, 0, -1))
	case "30d":
		return time.Now().AddDate(0, 0, -30)
	case "all":
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.Local)
	default:
		return time.Now().AddDate(0, 0, -7)
	}
}

func ingestProgressSince(r *http.Request) time.Time {
	if strings.TrimSpace(r.URL.Query().Get("range")) != "" {
		return dashboardSince(r)
	}
	return time.Date(1970, 1, 1, 0, 0, 0, 0, time.Local)
}
