package clickhouse

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"fwlog/internal/threatintel"
)

func (s *ClickHouseStore) SaveResult(ctx context.Context, result threatintel.Result) error {
	if s == nil || s.conn == nil {
		return fmt.Errorf("clickhouse connection is not initialized")
	}
	return s.conn.Exec(ctx, ThreatIntelligenceInsertSQL(),
		string(result.Provider),
		result.IP,
		result.Verdict,
		result.RiskLevel,
		nullableFloatArg(result.ConfidenceScore),
		result.ConfidenceLevel,
		result.Tags,
		nullableTimeArg(result.FirstSeen),
		nullableTimeArg(result.LastSeen),
		nullableTimeArg(result.SourceUpdatedAt),
		result.AnalyzedAt.UTC(),
		result.Summary,
		normalizeRawResponseString(result.RawResponse),
	)
}

func (s *ClickHouseStore) LatestResult(ctx context.Context, provider threatintel.Provider, ip string) (threatintel.Result, bool, error) {
	if s == nil || s.conn == nil {
		return threatintel.Result{}, false, fmt.Errorf("clickhouse connection is not initialized")
	}

	var (
		result          threatintel.Result
		rawProvider     string
		confidenceScore sql.NullFloat64
		firstSeen       sql.NullTime
		lastSeen        sql.NullTime
		sourceUpdatedAt sql.NullTime
		rawResponse     string
	)
	err := s.conn.QueryRow(ctx, ThreatIntelligenceLatestSQL(), string(provider), ip).Scan(
		&rawProvider,
		&result.IP,
		&result.Verdict,
		&result.RiskLevel,
		&confidenceScore,
		&result.ConfidenceLevel,
		&result.Tags,
		&firstSeen,
		&lastSeen,
		&sourceUpdatedAt,
		&result.AnalyzedAt,
		&result.Summary,
		&rawResponse,
	)
	if err != nil {
		if isNoRowsError(err) {
			return threatintel.Result{}, false, nil
		}
		return threatintel.Result{}, false, err
	}

	result.Provider = threatintel.Provider(rawProvider)
	result.ConfidenceScore = floatPtrFromNull(confidenceScore)
	result.FirstSeen = timePtrFromNull(firstSeen)
	result.LastSeen = timePtrFromNull(lastSeen)
	result.SourceUpdatedAt = timePtrFromNull(sourceUpdatedAt)
	result.AnalyzedAt = result.AnalyzedAt.UTC()
	result.RawResponse = normalizeRawResponse(rawResponse)
	return result, true, nil
}

func ThreatIntelligenceInsertSQL() string {
	return `INSERT INTO threat_intelligence_results (
    provider, ip, verdict, risk_level, confidence_score, confidence_level,
    tags, first_seen, last_seen, source_updated_at, analyzed_at, summary, raw_response
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
}

func ThreatIntelligenceLatestSQL() string {
	return `SELECT provider, ip, verdict, risk_level, confidence_score, confidence_level,
tags, first_seen, last_seen, source_updated_at, analyzed_at, summary, raw_response
FROM threat_intelligence_results FINAL
WHERE provider = ? AND ip = ?
LIMIT 1`
}

func nullableFloatArg(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableTimeArg(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

func floatPtrFromNull(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}

func timePtrFromNull(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	utc := value.Time.UTC()
	return &utc
}

func normalizeRawResponse(raw string) json.RawMessage {
	if strings.TrimSpace(raw) == "" {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(raw)
}

func normalizeRawResponseString(raw json.RawMessage) string {
	if strings.TrimSpace(string(raw)) == "" {
		return `{}`
	}
	return string(raw)
}
