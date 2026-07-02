package main

import (
	"errors"
	"fmt"
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

type QueryRequest struct {
	Start    time.Time
	End      time.Time
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
	QueryTimeMS int64            `json:"query_time_ms"`
	Visibility  QueryVisibility  `json:"visibility"`
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
			appendSkippedDate(&visibility, day, "", missingStateReason)
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
		case StatusIdle, StatusScanning, StatusSucceeded, "":
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

	if req.IP != "" {
		sql.WriteString(" AND (src_ip = ? OR dst_ip = ? OR nat_ip = ?)")
		args = append(args, req.IP, req.IP, req.IP)
	}
	if req.SrcIP != "" {
		sql.WriteString(" AND src_ip = ?")
		args = append(args, req.SrcIP)
	}
	if req.DstIP != "" {
		sql.WriteString(" AND dst_ip = ?")
		args = append(args, req.DstIP)
	}
	if req.NATIP != "" {
		sql.WriteString(" AND nat_ip = ?")
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
		sql.WriteString(" AND protocol = ?")
		args = append(args, req.Protocol)
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
