package server

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestAccessTrendDefaultsToSevenNaturalDays(t *testing.T) {
	now := time.Date(2026, 9, 11, 15, 30, 0, 0, time.Local)
	from, until, err := accessTrendWindow("", now)
	if err != nil || !from.Equal(time.Date(2026, 9, 5, 0, 0, 0, 0, time.Local)) || !until.Equal(time.Date(2026, 9, 12, 0, 0, 0, 0, time.Local)) {
		t.Fatalf("default access trend window = %v..%v, err = %v", from, until, err)
	}
}

func TestAccessTrendRejectsUnsupportedRange(t *testing.T) {
	if _, _, err := accessTrendWindow("30d", time.Now()); err == nil {
		t.Fatal("unsupported range should fail")
	}
}

func TestBuildAccessTrendSnapshotClassifiesTargetPoints(t *testing.T) {
	rows := []AccessTrendRow{
		{TargetIP: "1.2.3.0", Count: 100},
		{TargetIP: "8.8.8.0", Count: 90},
		{TargetIP: "10.0.0.0", Count: 80},
	}
	engine := NewIPEngine()
	engine.AddOverride("1.2.3.0", "public", "中国 / 北京")
	engine.AddOverride("8.8.8.0", "public", "美国 / 山景城")

	snapshot := buildAccessTrendSnapshot(rows, engine)
	if snapshot.Summary.Total != 270 || snapshot.Summary.Domestic != 100 || snapshot.Summary.Overseas != 90 || snapshot.Summary.Unknown != 80 {
		t.Fatalf("summary = %#v", snapshot.Summary)
	}
	if len(snapshot.Domestic) != 1 || snapshot.Domestic[0].TargetIP != "1.2.3.0" {
		t.Fatalf("domestic points = %#v", snapshot.Domestic)
	}
	if len(snapshot.Overseas) != 1 || snapshot.Overseas[0].TargetIP != "8.8.8.0" {
		t.Fatalf("overseas points = %#v", snapshot.Overseas)
	}
	if len(snapshot.All) != 2 {
		t.Fatalf("all mapped points = %#v", snapshot.All)
	}
}

func TestAccessTrendModeUsesPreprocessedSnapshot(t *testing.T) {
	snapshot := AccessTrendSnapshot{
		Summary: AccessTrendSummary{Total: 190, Domestic: 100, Overseas: 90},
		All: []AccessTrendPoint{
			{ID: "1.2.3.0", TargetIP: "1.2.3.0", Mode: "domestic", Count: 100},
			{ID: "8.8.8.0", TargetIP: "8.8.8.0", Mode: "overseas", Count: 90},
		},
		Domestic: []AccessTrendPoint{{ID: "1.2.3.0", TargetIP: "1.2.3.0", Mode: "domestic", Count: 100}},
		Overseas: []AccessTrendPoint{{ID: "8.8.8.0", TargetIP: "8.8.8.0", Mode: "overseas", Count: 90}},
	}

	app := NewApp(Config{})
	app.store = &ClickHouseStore{}
	app.accessTrendCache = newAccessTrendCache(time.Hour)
	seedAccessTrendSnapshot(t, app.accessTrendCache, snapshot)

	req, err := http.NewRequest(http.MethodGet, "/api/access-trend?mode=overseas", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := (appDashboardService{app: app}).AccessTrend(req)
	if err != nil {
		t.Fatalf("AccessTrend returned error: %v", err)
	}
	if response.Summary != snapshot.Summary {
		t.Fatalf("summary = %#v, want %#v", response.Summary, snapshot.Summary)
	}
	if len(response.Points) != 1 || response.Points[0].TargetIP != "8.8.8.0" {
		t.Fatalf("points = %#v", response.Points)
	}
}

func seedAccessTrendSnapshot(t *testing.T, cache *accessTrendCache, snapshot AccessTrendSnapshot) {
	t.Helper()
	for _, dayOffset := range []int{-1, 0, 1} {
		from, until, err := accessTrendWindow("", time.Now().AddDate(0, 0, dayOffset))
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = cache.Get(context.Background(), accessTrendCacheKey(from, until, ""), func(context.Context) (AccessTrendSnapshot, error) {
			return snapshot, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestAccessLocationModeUsesPublicDestinationGeo(t *testing.T) {
	tests := []struct{ ip, location, want string }{
		{"10.0.0.1", "内网", "unknown"},
		{"8.8.8.8", "美国 / 山景城", "overseas"},
		{"1.2.3.4", "中国 / 北京", "domestic"},
		{"203.0.113.1", "未知公网", "unknown"},
	}
	for _, tt := range tests {
		if got := accessLocationMode(tt.ip, tt.location); got != tt.want {
			t.Fatalf("accessLocationMode(%q, %q) = %q, want %q", tt.ip, tt.location, got, tt.want)
		}
	}
}
