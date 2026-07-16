package query

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

const (
	partialVisibilityMessage = "所选时间包含未完成入库日期，已自动只查询已入库部分。"
	missingStateReason       = "当天没有入库状态，已跳过"
	notReadyReason           = "当天未完成入库，已跳过"
	notQueryableReason       = "当天状态不可查询，已跳过"
	noVisibleImportReason    = "当天尚无可见入库数据，已跳过"
	noVisibleRangeError      = "没有可查询的已入库时间范围"
)

const (
	queryTimeout           = 10 * time.Second
	maxLegacyQueryPage     = 20
	maxUnfilteredQuerySpan = 24 * time.Hour
	maxFilteredQuerySpan   = 31 * 24 * time.Hour
)

const (
	QueryTimeout           = queryTimeout
	MaxUnfilteredQuerySpan = maxUnfilteredQuerySpan
)

type QueryRequest struct {
	Start    time.Time
	End      time.Time
	SourceID string
	IP       string
	SrcIP    string
	DstIP    string
	NATIP    string
	Port     uint16
	SrcPort  uint16
	DstPort  uint16
	NATPort  uint16
	Protocol string
	Action   string
	LogTag   string
	Cursor   *QueryCursor
}

type QueryCursor struct {
	Timestamp    time.Time `json:"timestamp"`
	SourceID     string    `json:"source_id"`
	SourceFile   string    `json:"source_file"`
	SourceOffset uint64    `json:"source_offset"`
}

type QueryPageOptions struct {
	Page     int
	PageSize int
	Cursor   *QueryCursor
}

type QueryError struct {
	Code    string `json:"error"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (e *QueryError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (r QueryRequest) HasFilters() bool {
	return r.IP != "" ||
		r.SourceID != "" ||
		r.SrcIP != "" ||
		r.DstIP != "" ||
		r.NATIP != "" ||
		r.Port != 0 ||
		r.SrcPort != 0 ||
		r.DstPort != 0 ||
		r.NATPort != 0 ||
		r.Protocol != "" ||
		r.Action != "" ||
		r.LogTag != ""
}

type VisibleRange struct {
	LogDate   time.Time    `json:"log_date"`
	StartTime time.Time    `json:"start_time"`
	EndTime   time.Time    `json:"end_time"`
	Status    IngestStatus `json:"status"`
}

type SkippedLogDate struct {
	LogDate time.Time    `json:"log_date"`
	Status  IngestStatus `json:"status"`
	Reason  string       `json:"reason"`
}

type QueryVisibility struct {
	Partial       bool             `json:"partial"`
	Message       string           `json:"message"`
	QueriedRanges []VisibleRange   `json:"queried_ranges"`
	SkippedDates  []SkippedLogDate `json:"skipped_dates"`
}

type QueryResponse struct {
	Records     []map[string]any `json:"records"`
	Total       int              `json:"total"`
	Page        int              `json:"page"`
	PageSize    int              `json:"page_size"`
	NextCursor  string           `json:"next_cursor,omitempty"`
	HasMore     bool             `json:"has_more"`
	QueryTimeMS int64            `json:"query_time_ms"`
	Visibility  QueryVisibility  `json:"visibility"`
}

func encodeQueryCursor(cursor QueryCursor) (string, error) {
	payload, err := json.Marshal(struct {
		Timestamp    string `json:"timestamp"`
		SourceID     string `json:"source_id"`
		SourceFile   string `json:"source_file"`
		SourceOffset uint64 `json:"source_offset"`
	}{
		Timestamp:    formatDateTime(cursor.Timestamp),
		SourceID:     cursor.SourceID,
		SourceFile:   cursor.SourceFile,
		SourceOffset: cursor.SourceOffset,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeQueryCursor(value string) (*QueryCursor, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor")
	}
	var wire struct {
		Timestamp    string `json:"timestamp"`
		SourceID     string `json:"source_id"`
		SourceFile   string `json:"source_file"`
		SourceOffset uint64 `json:"source_offset"`
	}
	if err := json.Unmarshal(payload, &wire); err != nil {
		return nil, fmt.Errorf("invalid cursor")
	}
	timestamp, err := parseTimeQuery(wire.Timestamp, time.Time{})
	if err != nil || timestamp.IsZero() {
		return nil, fmt.Errorf("invalid cursor")
	}
	return &QueryCursor{
		Timestamp:    timestamp,
		SourceID:     wire.SourceID,
		SourceFile:   wire.SourceFile,
		SourceOffset: wire.SourceOffset,
	}, nil
}

func EncodeQueryCursor(cursor QueryCursor) (string, error) {
	return encodeQueryCursor(cursor)
}

func DecodeQueryCursor(value string) (*QueryCursor, error) {
	return decodeQueryCursor(value)
}

func ValidateQueryProtection(req QueryRequest, options QueryPageOptions) error {
	return validateQueryProtection(req, options)
}

func validateQueryProtection(req QueryRequest, options QueryPageOptions) error {
	if options.Page > maxLegacyQueryPage && options.Cursor == nil {
		return &QueryError{
			Code:    "query_too_broad",
			Message: "分页过深，请使用下一页游标继续查询，或缩小查询范围",
			Status:  http.StatusBadRequest,
		}
	}

	span := req.End.Sub(req.Start)
	limit := maxUnfilteredQuerySpan
	hasFilters := req.HasFilters()
	if hasFilters {
		limit = maxFilteredQuerySpan
	}
	if span > limit {
		return &QueryError{
			Code:    "query_too_broad",
			Message: queryProtectionMessage(hasFilters),
			Status:  http.StatusBadRequest,
		}
	}
	return nil
}

func queryProtectionMessage(hasFilters bool) string {
	if hasFilters {
		return fmt.Sprintf("查询时间范围过大。带筛选查询最多支持 %s，请缩小时间范围后重试。", humanDuration(maxFilteredQuerySpan))
	}
	return fmt.Sprintf("查询时间范围过大。无筛选查询最多支持 %s；填写 IP、端口、协议、结果或日志名称后，带筛选查询最多支持 %s。", humanDuration(maxUnfilteredQuerySpan), humanDuration(maxFilteredQuerySpan))
}

func humanDuration(value time.Duration) string {
	if value%(24*time.Hour) == 0 {
		return fmt.Sprintf("%d 天", int(value/(24*time.Hour)))
	}
	return value.String()
}

func BuildVisibleRanges(start, end time.Time, states []DateIngestState) QueryVisibility {
	visibility := QueryVisibility{
		QueriedRanges: make([]VisibleRange, 0),
		SkippedDates:  make([]SkippedLogDate, 0),
	}
	if end.Before(start) {
		return visibility
	}

	stateByDate := make(map[string]DateIngestState, len(states))
	for _, state := range states {
		dayStart := startOfDay(state.LogDate)
		state.LogDate = dayStart
		stateByDate[dateKey(dayStart)] = state
	}

	for day := startOfDay(start); !day.After(end); day = day.AddDate(0, 0, 1) {
		rangeStart := maxTime(start, day)
		rangeEnd := minTime(end, endOfDayTime(day))
		if rangeEnd.Before(rangeStart) {
			continue
		}

		state, ok := stateByDate[dateKey(day)]
		if !ok {
			visibility.SkippedDates = append(visibility.SkippedDates, SkippedLogDate{
				LogDate: day,
				Reason:  missingStateReason,
			})
			continue
		}

		switch state.Status {
		case StatusReady:
			visibility.QueriedRanges = append(visibility.QueriedRanges, VisibleRange{
				LogDate:   day,
				StartTime: rangeStart,
				EndTime:   rangeEnd,
				Status:    state.Status,
			})
		case StatusImporting:
			if state.MaxVisibleTimestamp.IsZero() || state.MaxVisibleTimestamp.Before(rangeStart) {
				appendSkippedDate(&visibility, day, state.Status, noVisibleImportReason)
				continue
			}

			visibleEnd := minTime(rangeEnd, state.MaxVisibleTimestamp)
			visibility.QueriedRanges = append(visibility.QueriedRanges, VisibleRange{
				LogDate:   day,
				StartTime: rangeStart,
				EndTime:   visibleEnd,
				Status:    state.Status,
			})
			if visibleEnd.Before(rangeEnd) {
				visibility.Partial = true
			}
		case StatusPending, StatusFailed:
			appendSkippedDate(&visibility, day, state.Status, notReadyReason)
		case StatusIdle, StatusScanning, StatusSucceeded, StatusNoData, "":
			appendSkippedDate(&visibility, day, state.Status, notQueryableReason)
		default:
			appendSkippedDate(&visibility, day, state.Status, notQueryableReason)
		}
	}

	if visibility.Partial {
		visibility.Message = partialVisibilityMessage
	}
	return visibility
}

func BuildQuerySQL(req QueryRequest, visibility QueryVisibility) (string, []any, error) {
	if err := normalizeIPFilters(&req); err != nil {
		return "", nil, err
	}
	if len(visibility.QueriedRanges) == 0 {
		return "", nil, errors.New(noVisibleRangeError)
	}

	var sql strings.Builder
	sql.WriteString("SELECT * FROM nat_logs WHERE (")

	args := make([]any, 0, len(visibility.QueriedRanges)*3+8)
	for i, visibleRange := range visibility.QueriedRanges {
		if i > 0 {
			sql.WriteString(" OR ")
		}
		sql.WriteString("(log_date = ? AND timestamp >= ? AND timestamp <= ?)")
		args = append(args, visibleRange.LogDate, visibleRange.StartTime, visibleRange.EndTime)
	}
	sql.WriteString(")")

	if req.SourceID != "" {
		sql.WriteString(" AND source_id = ?")
		args = append(args, req.SourceID)
	}
	if req.IP != "" {
		sql.WriteString(" AND (src_ip = toIPv6(?) OR dst_ip = toIPv6(?) OR nat_ip = toIPv6(?))")
		args = append(args, req.IP, req.IP, req.IP)
	}
	if req.SrcIP != "" {
		sql.WriteString(" AND src_ip = toIPv6(?)")
		args = append(args, req.SrcIP)
	}
	if req.DstIP != "" {
		sql.WriteString(" AND dst_ip = toIPv6(?)")
		args = append(args, req.DstIP)
	}
	if req.NATIP != "" {
		sql.WriteString(" AND nat_ip = toIPv6(?)")
		args = append(args, req.NATIP)
	}
	if req.Port != 0 {
		sql.WriteString(" AND (src_port = ? OR dst_port = ? OR nat_port = ?)")
		args = append(args, req.Port, req.Port, req.Port)
	}
	if req.SrcPort != 0 {
		sql.WriteString(" AND src_port = ?")
		args = append(args, req.SrcPort)
	}
	if req.DstPort != 0 {
		sql.WriteString(" AND dst_port = ?")
		args = append(args, req.DstPort)
	}
	if req.NATPort != 0 {
		sql.WriteString(" AND nat_port = ?")
		args = append(args, req.NATPort)
	}
	if req.Protocol != "" {
		sql.WriteString(" AND protocol IN (?, ?)")
		protocol := normalizeProtocol(req.Protocol)
		args = append(args, protocol, protocolNumber(protocol))
	}
	if req.Action != "" {
		sql.WriteString(" AND action = ?")
		args = append(args, req.Action)
	}
	if req.LogTag != "" {
		sql.WriteString(" AND log_tag = ?")
		args = append(args, req.LogTag)
	}

	return sql.String(), args, nil
}

func normalizeIPFilters(req *QueryRequest) error {
	filters := []*string{&req.IP, &req.SrcIP, &req.DstIP, &req.NATIP}
	for _, filter := range filters {
		value := strings.TrimSpace(*filter)
		if value == "" {
			continue
		}
		addr, err := netip.ParseAddr(value)
		if err != nil || addr.Zone() != "" {
			return &QueryError{
				Code:    "invalid_ip",
				Message: fmt.Sprintf("IP 地址格式无效：%s", value),
				Status:  http.StatusBadRequest,
			}
		}
		*filter = addr.Unmap().String()
	}
	return nil
}

func protocolNumber(protocol string) string {
	switch normalizeProtocol(protocol) {
	case "TCP":
		return "6"
	case "UDP":
		return "17"
	case "ICMP":
		return "1"
	default:
		return protocol
	}
}

func appendSkippedDate(visibility *QueryVisibility, logDate time.Time, status IngestStatus, reason string) {
	visibility.Partial = true
	visibility.SkippedDates = append(visibility.SkippedDates, SkippedLogDate{
		LogDate: logDate,
		Status:  status,
		Reason:  reason,
	})
}

func startOfDay(ts time.Time) time.Time {
	year, month, day := ts.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, ts.Location())
}

func dateKey(ts time.Time) string {
	year, month, day := ts.Date()
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day)
}

func endOfDayTime(ts time.Time) time.Time {
	return startOfDay(ts).Add(24*time.Hour - time.Second)
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
