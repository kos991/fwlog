package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	accessTrendTimeout    = 20 * time.Second
	accessTrendPointLimit = 50
)

type AccessTrendSnapshot struct {
	Summary  AccessTrendSummary
	All      []AccessTrendPoint
	Domestic []AccessTrendPoint
	Overseas []AccessTrendPoint
}

func (s appDashboardService) AccessTrend(r *http.Request) (AccessTrendResponse, error) {
	store := s.appStore()
	if store == nil {
		return AccessTrendResponse{Points: []AccessTrendPoint{}}, nil
	}
	from, until, err := accessTrendWindow(strings.TrimSpace(r.URL.Query().Get("range")), time.Now())
	if err != nil {
		return AccessTrendResponse{}, err
	}
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = "all"
	}
	if mode != "all" && mode != "domestic" && mode != "overseas" {
		return AccessTrendResponse{}, fmt.Errorf("unsupported access trend mode %q", mode)
	}

	ctx, cancel := context.WithTimeout(r.Context(), accessTrendTimeout)
	defer cancel()
	sourceID := strings.TrimSpace(r.URL.Query().Get("source_id"))
	key := accessTrendCacheKey(from, until, sourceID)
	snapshot, _, err := s.app.accessTrendCache.Get(ctx, key, func(loadCtx context.Context) (AccessTrendSnapshot, error) {
		rows, loadErr := store.AccessTrendRows(loadCtx, from, until, sourceID)
		if loadErr != nil {
			return AccessTrendSnapshot{}, loadErr
		}
		s.app.mu.RLock()
		engine := s.app.ipEngine
		s.app.mu.RUnlock()
		return buildAccessTrendSnapshot(rows, engine), nil
	})
	if err != nil {
		return AccessTrendResponse{}, err
	}

	points := snapshot.All
	switch mode {
	case "domestic":
		points = snapshot.Domestic
	case "overseas":
		points = snapshot.Overseas
	}
	return AccessTrendResponse{Points: points, Summary: snapshot.Summary}, nil
}

func buildAccessTrendSnapshot(rows []AccessTrendRow, engine *IPEngine) AccessTrendSnapshot {
	snapshot := AccessTrendSnapshot{
		All:      make([]AccessTrendPoint, 0, accessTrendPointLimit),
		Domestic: make([]AccessTrendPoint, 0, accessTrendPointLimit),
		Overseas: make([]AccessTrendPoint, 0, accessTrendPointLimit),
	}
	for _, row := range rows {
		targetIP := strings.TrimSpace(row.TargetIP)
		tag := engine.GetTag(targetIP)
		mode := accessLocationMode(targetIP, tag.Location)
		snapshot.Summary.Total += row.Count
		switch mode {
		case "domestic":
			snapshot.Summary.Domestic += row.Count
		case "overseas":
			snapshot.Summary.Overseas += row.Count
		default:
			snapshot.Summary.Unknown += row.Count
		}

		point := AccessTrendPoint{
			ID:           targetIP,
			TargetIP:     targetIP,
			TargetRegion: tag.Location,
			Count:        row.Count,
			Mode:         mode,
		}
		if tag.Latitude != 0 || tag.Longitude != 0 {
			point.Coord = []float64{tag.Longitude, tag.Latitude}
		}
		if mode != "unknown" && len(snapshot.All) < accessTrendPointLimit {
			snapshot.All = append(snapshot.All, point)
		}
		switch mode {
		case "domestic":
			if len(snapshot.Domestic) < accessTrendPointLimit {
				snapshot.Domestic = append(snapshot.Domestic, point)
			}
		case "overseas":
			if len(snapshot.Overseas) < accessTrendPointLimit {
				snapshot.Overseas = append(snapshot.Overseas, point)
			}
		}
	}
	return snapshot
}

func accessTrendCacheKey(from, until time.Time, sourceID string) string {
	return fmt.Sprintf("%s|%s|%s", from.Format("2006-01-02"), until.Format("2006-01-02"), sourceID)
}

func accessTrendWindow(value string, now time.Time) (time.Time, time.Time, error) {
	until := startOfDay(now).AddDate(0, 0, 1)
	switch strings.TrimSpace(value) {
	case "", "7d":
		return startOfDay(now.AddDate(0, 0, -6)), until, nil
	case "3d":
		return startOfDay(now.AddDate(0, 0, -2)), until, nil
	case "today":
		return startOfDay(now), until, nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unsupported access trend range %q", value)
	}
}

func isPrivateOrInvalid(value string) bool {
	ip := net.ParseIP(value)
	return ip == nil || ip.IsPrivate()
}

func accessLocationMode(ipValue, location string) string {
	if isPrivateOrInvalid(ipValue) {
		return "unknown"
	}
	if location == "未知公网" || location == "未知" || location == "" {
		return "unknown"
	}
	if location == "中国" || strings.HasPrefix(location, "中国 /") {
		return "domestic"
	}
	return "overseas"
}
