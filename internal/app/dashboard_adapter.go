package app

import (
	"net/http"
	"strings"
	"time"
)

type appDashboardService struct {
	app *App
}

func (s appDashboardService) HealthDashboard(r *http.Request) (HealthDashboardResponse, error) {
	store := s.appStore()
	if store == nil {
		return BuildHealthDashboard(nil, DashboardMetrics{}), nil
	}

	states, err := store.ListDateStates(r.Context(), dashboardSince(r))
	if err != nil {
		return HealthDashboardResponse{}, err
	}

	metrics, err := store.DashboardMetrics(r.Context(), dashboardMetricsSince(r), parseBoolQuery(r, "include_distributions", true))
	if err != nil {
		return HealthDashboardResponse{}, err
	}
	metrics.GeoIPLoaded = s.geoIPLoaded()
	metrics.GeoIPStatus = s.geoIPStatus()
	metrics = s.withGeoDistributions(metrics)
	metrics = s.withAutoScanPlan(metrics)
	metrics.SystemHealth = collectSystemHealth(metrics.SystemHealth.Database)

	return BuildHealthDashboard(states, metrics), nil
}

func (s appDashboardService) IngestProgress(r *http.Request) (IngestProgressResponse, error) {
	store := s.appStore()
	includeReady := parseBoolQuery(r, "include_ready", false)
	if store == nil {
		return BuildIngestProgress(nil, includeReady), nil
	}

	states, err := store.ListDateStates(r.Context(), ingestProgressSince(r))
	if err != nil {
		return IngestProgressResponse{}, err
	}
	return BuildIngestProgress(states, includeReady, s.withAutoScanPlan(DashboardMetrics{})), nil
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
	return enrichGeoDistributionMetrics(metrics, engine)
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

	plan := BuildAutoScanPlan(settings, time.Now())
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
