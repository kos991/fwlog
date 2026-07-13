package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"fwlog/internal/ip"
)

func TestAcquireQuerySlotRejectsWhenBusy(t *testing.T) {
	app := &App{querySem: make(chan struct{}, 1)}
	app.querySem <- struct{}{}
	service := appQueryService{app: app}

	_, err := service.acquireQuerySlot(context.Background())
	if err == nil {
		t.Fatal("acquireQuerySlot should reject when the query semaphore is full")
	}
	var queryErr *QueryError
	if !errors.As(err, &queryErr) {
		t.Fatalf("error = %T %v, want QueryError", err, err)
	}
	if queryErr.Code != "query_busy" || queryErr.Status != http.StatusTooManyRequests {
		t.Fatalf("query error = %#v", queryErr)
	}
}

func TestEnrichQueryRecordsAddsLabelsAndNormalizesProtocol(t *testing.T) {
	engine := ip.NewIPEngine()
	if err := engine.AddSegment("2.55.80.0/24", "办公网络"); err != nil {
		t.Fatalf("add segment: %v", err)
	}
	records := []map[string]any{
		{
			"src_ip":   "2.55.80.9",
			"dst_ip":   "8.8.8.8",
			"protocol": "6,",
		},
	}

	enrichQueryRecords(records, engine)

	if records[0]["src_ip_label"] != "办公网络" {
		t.Fatalf("source label not enriched: %#v", records[0])
	}
	if records[0]["protocol"] != "TCP" {
		t.Fatalf("protocol not normalized: %#v", records[0])
	}
	if records[0]["dst_geo"] == "" {
		t.Fatalf("destination geo should be filled: %#v", records[0])
	}
}

func TestParseQueryRequestFallsBackWhenPageSizeAll(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/query?start=2026-06-10&end=2026-06-10&page_size=all", nil)

	_, _, pageSize, err := parseQueryRequest(r)
	if err != nil {
		t.Fatalf("parseQueryRequest returned error: %v", err)
	}
	if pageSize != 50 {
		t.Fatalf("pageSize = %d, want fallback 50", pageSize)
	}
}
