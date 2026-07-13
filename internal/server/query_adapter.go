package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fwlog/internal/query"
	"fwlog/internal/storage/clickhouse"
)

type appQueryService struct {
	app *App
}

func (s appQueryService) Query(r *http.Request) (QueryResponse, error) {
	store := s.appStore()
	if store == nil {
		return emptyQueryResponse(), nil
	}

	req, page, pageSize, err := parseQueryRequest(r)
	if err != nil {
		return QueryResponse{}, err
	}
	pageOptions := QueryPageOptions{Page: page, PageSize: pageSize, Cursor: req.Cursor}
	if err := query.ValidateQueryProtection(req, pageOptions); err != nil {
		return QueryResponse{}, err
	}

	release, err := s.acquireQuerySlot(r.Context())
	if err != nil {
		return QueryResponse{}, err
	}
	defer release()

	ctx, cancel := context.WithTimeout(r.Context(), query.QueryTimeout)
	defer cancel()

	states, err := store.ListDateStates(ctx, startOfDay(req.Start))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return QueryResponse{}, queryTimeoutError()
		}
		return QueryResponse{}, err
	}

	visibility := query.BuildVisibleRanges(req.Start, req.End, states)
	querySQL, args, err := query.BuildQuerySQL(req, visibility)
	if err != nil {
		return QueryResponse{
			Records:     []map[string]any{},
			Total:       0,
			Page:        page,
			PageSize:    pageSize,
			QueryTimeMS: 0,
			Visibility:  visibility,
		}, nil
	}

	startedAt := time.Now()
	sortMode := clickhouse.QuerySortTimeAsc
	total, err := store.CountNATLogs(ctx, querySQL, args)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return QueryResponse{}, queryTimeoutError()
		}
		return QueryResponse{}, err
	}
	records, hasMore, err := store.QueryNATLogs(ctx, querySQL, args, pageOptions, sortMode)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return QueryResponse{}, queryTimeoutError()
		}
		return QueryResponse{}, err
	}
	s.enrichRecords(records)
	nextCursor := ""
	if hasMore {
		nextCursor, err = nextCursorFromRecords(records)
		if err != nil {
			return QueryResponse{}, err
		}
	}

	return QueryResponse{
		Records:     records,
		Total:       total,
		Page:        page,
		PageSize:    pageSize,
		NextCursor:  nextCursor,
		HasMore:     hasMore,
		QueryTimeMS: time.Since(startedAt).Milliseconds(),
		Visibility:  visibility,
	}, nil
}

func (s appQueryService) acquireQuerySlot(ctx context.Context) (func(), error) {
	if s.app == nil || s.app.querySem == nil {
		return func() {}, nil
	}
	select {
	case s.app.querySem <- struct{}{}:
		return func() { <-s.app.querySem }, nil
	case <-ctx.Done():
		return nil, queryTimeoutError()
	default:
		return nil, &QueryError{
			Code:    "query_busy",
			Message: "查询并发过高，请稍后重试",
			Status:  http.StatusTooManyRequests,
		}
	}
}

func queryTimeoutError() error {
	return &QueryError{
		Code:    "query_timeout",
		Message: "查询超时，请缩小时间范围或增加筛选条件",
		Status:  http.StatusGatewayTimeout,
	}
}

func (s appQueryService) appStore() *ClickHouseStore {
	if s.app == nil {
		return nil
	}
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	return s.app.store
}

func (s appQueryService) enrichRecords(records []map[string]any) {
	if s.app == nil {
		return
	}
	s.app.mu.RLock()
	engine := s.app.ipEngine
	s.app.mu.RUnlock()
	enrichQueryRecords(records, engine)
}

func enrichQueryRecords(records []map[string]any, engine *IPEngine) {
	if engine == nil {
		return
	}
	for _, record := range records {
		if protocol, ok := record["protocol"].(string); ok {
			record["protocol"] = query.NormalizeProtocol(protocol)
		}
		if srcIP, ok := record["src_ip"].(string); ok && srcIP != "" && srcIP != "0.0.0.0" {
			record["src_ip_label"] = engine.GetTag(srcIP).Label
		}
		if dstIP, ok := record["dst_ip"].(string); ok && dstIP != "" && dstIP != "0.0.0.0" {
			record["dst_geo"] = engine.GetTag(dstIP).Location
		}
	}
}

func nextCursorFromRecords(records []map[string]any) (string, error) {
	if len(records) == 0 {
		return "", nil
	}
	last := records[len(records)-1]
	timestampText, ok := last["timestamp"].(string)
	if !ok || strings.TrimSpace(timestampText) == "" {
		return "", fmt.Errorf("cannot build cursor: missing timestamp")
	}
	timestamp, err := parseTimeQuery(timestampText, time.Time{})
	if err != nil {
		return "", fmt.Errorf("cannot build cursor: %w", err)
	}
	sourceID, _ := last["source_id"].(string)
	sourceFile, _ := last["source_file"].(string)
	sourceOffset, err := uint64FromAny(last["source_offset"])
	if err != nil {
		return "", fmt.Errorf("cannot build cursor: %w", err)
	}
	return query.EncodeQueryCursor(QueryCursor{
		Timestamp:    timestamp,
		SourceID:     sourceID,
		SourceFile:   sourceFile,
		SourceOffset: sourceOffset,
	})
}

func uint64FromAny(value any) (uint64, error) {
	switch typed := value.(type) {
	case uint64:
		return typed, nil
	case uint:
		return uint64(typed), nil
	case int:
		if typed < 0 {
			return 0, fmt.Errorf("invalid source_offset")
		}
		return uint64(typed), nil
	case int64:
		if typed < 0 {
			return 0, fmt.Errorf("invalid source_offset")
		}
		return uint64(typed), nil
	case float64:
		if typed < 0 || typed != float64(uint64(typed)) {
			return 0, fmt.Errorf("invalid source_offset")
		}
		return uint64(typed), nil
	default:
		return 0, fmt.Errorf("missing source_offset")
	}
}

func emptyQueryResponse() QueryResponse {
	return QueryResponse{
		Records:  []map[string]any{},
		Total:    0,
		Page:     1,
		PageSize: 50,
		Visibility: QueryVisibility{
			QueriedRanges: []VisibleRange{},
			SkippedDates:  []SkippedLogDate{},
		},
	}
}

func parseQueryRequest(r *http.Request) (QueryRequest, int, int, error) {
	values := r.URL.Query()
	now := time.Now()

	start, err := parseTimeQuery(values.Get("start"), now.Add(-query.MaxUnfilteredQuerySpan))
	if err != nil {
		return QueryRequest{}, 0, 0, err
	}
	end, err := parseTimeQuery(values.Get("end"), now)
	if err != nil {
		return QueryRequest{}, 0, 0, err
	}
	if end.Before(start) {
		return QueryRequest{}, 0, 0, fmt.Errorf("end must be after start")
	}

	req := QueryRequest{
		Start:    start,
		End:      end,
		IP:       strings.TrimSpace(values.Get("ip")),
		SrcIP:    strings.TrimSpace(values.Get("src_ip")),
		DstIP:    strings.TrimSpace(values.Get("dst_ip")),
		NATIP:    strings.TrimSpace(values.Get("nat_ip")),
		Protocol: strings.TrimSpace(values.Get("protocol")),
		Action:   strings.TrimSpace(values.Get("action")),
		LogTag:   strings.TrimSpace(values.Get("log_tag")),
	}
	req.Cursor, err = query.DecodeQueryCursor(values.Get("cursor"))
	if err != nil {
		return QueryRequest{}, 0, 0, err
	}

	var errPort error
	req.Port, errPort = parseUint16Query(values.Get("port"))
	if errPort != nil {
		return QueryRequest{}, 0, 0, errPort
	}
	req.SrcPort, errPort = parseUint16Query(values.Get("src_port"))
	if errPort != nil {
		return QueryRequest{}, 0, 0, errPort
	}
	req.DstPort, errPort = parseUint16Query(values.Get("dst_port"))
	if errPort != nil {
		return QueryRequest{}, 0, 0, errPort
	}
	req.NATPort, errPort = parseUint16Query(values.Get("nat_port"))
	if errPort != nil {
		return QueryRequest{}, 0, 0, errPort
	}

	page := parsePositiveInt(values.Get("page"), 1)
	pageSize := parseQueryPageSize(values.Get("page_size"), 50)

	return req, page, pageSize, nil
}

func parseTimeQuery(value string, fallback time.Time) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}

	for _, layout := range []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		"2006-01-02",
	} {
		parsed, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time %q", value)
}

func parseUint16Query(value string) (uint16, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q", value)
	}
	return uint16(parsed), nil
}

func parsePositiveInt(value string, fallback int) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseQueryPageSize(value string, fallback int) int {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "all") || value == "0" {
		return fallback
	}
	pageSize := parsePositiveInt(value, fallback)
	if pageSize > 500 {
		return 500
	}
	return pageSize
}

func parseBoolQuery(r *http.Request, key string, fallback bool) bool {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
