package clickhouse

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"fwlog/internal/threatintel"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

func TestThreatIntelligenceDDLStoresLatestSuccessfulResult(t *testing.T) {
	ddl := strings.Join(ClickHouseDDL(), "\n")
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS threat_intelligence_results",
		"confidence_score Nullable(Float64)",
		"confidence_level LowCardinality(String)",
		"ENGINE = ReplacingMergeTree(analyzed_at)",
		"ORDER BY (provider, ip)",
	} {
		if !strings.Contains(ddl, want) {
			t.Fatalf("DDL missing %q", want)
		}
	}
	if strings.Contains(ddl[strings.Index(ddl, "threat_intelligence_results"):], "TTL ") {
		t.Fatal("result table must be permanent")
	}
}

func TestThreatIntelligenceLatestQueryUsesFinal(t *testing.T) {
	sql := ThreatIntelligenceLatestSQL()
	for _, want := range []string{"FROM threat_intelligence_results FINAL", "WHERE provider = ? AND ip = ?", "LIMIT 1"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("query missing %q", want)
		}
	}
}

func TestThreatIntelligenceSaveResultExecutesParameterizedInsert(t *testing.T) {
	score := 96.5
	firstSeen := time.Date(2026, 8, 29, 9, 10, 0, 0, time.FixedZone("CST", 8*60*60))
	lastSeen := time.Date(2026, 8, 30, 10, 20, 0, 0, time.FixedZone("CST", 8*60*60))
	sourceUpdatedAt := time.Date(2026, 8, 30, 11, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	analyzedAt := time.Date(2026, 8, 31, 12, 40, 0, 0, time.FixedZone("CST", 8*60*60))
	conn := &fakeThreatIntelConn{}
	store := &ClickHouseStore{conn: conn}

	err := store.SaveResult(context.Background(), threatintel.Result{
		Provider:        threatintel.ProviderThreatBook,
		IP:              "8.8.8.8",
		Verdict:         "malicious",
		RiskLevel:       "critical",
		ConfidenceScore: &score,
		ConfidenceLevel: "high",
		Tags:            []string{"botnet", "scanner"},
		FirstSeen:       &firstSeen,
		LastSeen:        &lastSeen,
		SourceUpdatedAt: &sourceUpdatedAt,
		AnalyzedAt:      analyzedAt,
		Summary:         "命中高危情报",
		RawResponse:     json.RawMessage(`{"source":"fixture"}`),
	})
	if err != nil {
		t.Fatalf("SaveResult returned error: %v", err)
	}

	if !strings.Contains(conn.execQuery, "INSERT INTO threat_intelligence_results") {
		t.Fatalf("SaveResult should insert result table, got %s", conn.execQuery)
	}
	if strings.Contains(conn.execQuery, "malicious") || strings.Contains(conn.execQuery, "8.8.8.8") {
		t.Fatalf("SaveResult must use query parameters, got %s", conn.execQuery)
	}
	if len(conn.execArgs) != 13 {
		t.Fatalf("SaveResult args len = %d, want 13: %#v", len(conn.execArgs), conn.execArgs)
	}
	if conn.execArgs[0] != string(threatintel.ProviderThreatBook) || conn.execArgs[1] != "8.8.8.8" {
		t.Fatalf("provider/ip args = %#v", conn.execArgs[:2])
	}
	gotScore, ok := conn.execArgs[4].(float64)
	if !ok || gotScore != score {
		t.Fatalf("confidence score arg = %#v, want %v", conn.execArgs[4], score)
	}
	for _, idx := range []int{7, 8, 9, 10} {
		got, ok := conn.execArgs[idx].(time.Time)
		if !ok || got.Location() != time.UTC {
			t.Fatalf("time arg %d = %#v, want UTC time.Time", idx, conn.execArgs[idx])
		}
	}
	if got := conn.execArgs[12]; got != `{"source":"fixture"}` {
		t.Fatalf("raw response arg = %#v", got)
	}
}

func TestThreatIntelligenceLatestResultScansNullableValuesAndNormalizesRawResponse(t *testing.T) {
	firstSeen := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	analyzedAt := time.Date(2026, 8, 31, 4, 5, 6, 0, time.UTC)
	conn := &fakeThreatIntelConn{
		row: fakeThreatIntelRow{values: []any{
			"threatbook",
			"8.8.8.8",
			"malicious",
			"critical",
			sql.NullFloat64{Float64: 88.5, Valid: true},
			"high",
			[]string{"botnet"},
			sql.NullTime{Time: firstSeen, Valid: true},
			sql.NullTime{},
			sql.NullTime{},
			analyzedAt,
			"命中情报",
			"",
		}},
	}
	store := &ClickHouseStore{conn: conn}

	result, ok, err := store.LatestResult(context.Background(), threatintel.ProviderThreatBook, "8.8.8.8")
	if err != nil {
		t.Fatalf("LatestResult returned error: %v", err)
	}
	if !ok {
		t.Fatalf("LatestResult found = false, want true")
	}

	if conn.queryRowSQL != ThreatIntelligenceLatestSQL() {
		t.Fatalf("LatestResult SQL = %q, want helper SQL", conn.queryRowSQL)
	}
	if !reflect.DeepEqual(conn.queryRowArgs, []any{string(threatintel.ProviderThreatBook), "8.8.8.8"}) {
		t.Fatalf("LatestResult args = %#v", conn.queryRowArgs)
	}
	if result.Provider != threatintel.ProviderThreatBook || result.IP != "8.8.8.8" || result.Verdict != "malicious" {
		t.Fatalf("result identity = %#v", result)
	}
	if result.ConfidenceScore == nil || *result.ConfidenceScore != 88.5 {
		t.Fatalf("confidence score = %#v", result.ConfidenceScore)
	}
	if result.FirstSeen == nil || !result.FirstSeen.Equal(firstSeen) || result.LastSeen != nil || result.SourceUpdatedAt != nil {
		t.Fatalf("nullable times = first:%#v last:%#v source:%#v", result.FirstSeen, result.LastSeen, result.SourceUpdatedAt)
	}
	if string(result.RawResponse) != `{}` {
		t.Fatalf("empty raw response should normalize to {}, got %q", string(result.RawResponse))
	}
}

func TestThreatIntelligenceLatestResultReturnsEmptyWhenMissing(t *testing.T) {
	store := &ClickHouseStore{conn: &fakeThreatIntelConn{row: fakeThreatIntelRow{err: sql.ErrNoRows}}}

	result, ok, err := store.LatestResult(context.Background(), threatintel.ProviderThreatBook, "8.8.8.8")
	if err != nil {
		t.Fatalf("LatestResult returned error: %v", err)
	}
	if ok || !reflect.DeepEqual(result, threatintel.Result{}) {
		t.Fatalf("LatestResult = %#v, %v, want empty false", result, ok)
	}
}

func TestThreatIntelligenceLatestResultPropagatesDatabaseError(t *testing.T) {
	dbErr := errors.New("boom")
	store := &ClickHouseStore{conn: &fakeThreatIntelConn{row: fakeThreatIntelRow{err: dbErr}}}

	_, _, err := store.LatestResult(context.Background(), threatintel.ProviderThreatBook, "8.8.8.8")
	if !errors.Is(err, dbErr) {
		t.Fatalf("LatestResult error = %v, want %v", err, dbErr)
	}
}

type fakeThreatIntelConn struct {
	execQuery    string
	execArgs     []any
	queryRowSQL  string
	queryRowArgs []any
	row          fakeThreatIntelRow
}

func (c *fakeThreatIntelConn) Contributors() []string { return nil }
func (c *fakeThreatIntelConn) ServerVersion() (*driver.ServerVersion, error) {
	return nil, nil
}
func (c *fakeThreatIntelConn) Select(context.Context, any, string, ...any) error { return nil }
func (c *fakeThreatIntelConn) Query(context.Context, string, ...any) (driver.Rows, error) {
	return nil, nil
}
func (c *fakeThreatIntelConn) QueryRow(_ context.Context, query string, args ...any) driver.Row {
	c.queryRowSQL = query
	c.queryRowArgs = append([]any{}, args...)
	return c.row
}
func (c *fakeThreatIntelConn) PrepareBatch(context.Context, string, ...driver.PrepareBatchOption) (driver.Batch, error) {
	return nil, nil
}
func (c *fakeThreatIntelConn) Exec(_ context.Context, query string, args ...any) error {
	c.execQuery = query
	c.execArgs = append([]any{}, args...)
	return nil
}
func (c *fakeThreatIntelConn) AsyncInsert(context.Context, string, bool, ...any) error { return nil }
func (c *fakeThreatIntelConn) Ping(context.Context) error                              { return nil }
func (c *fakeThreatIntelConn) Stats() driver.Stats                                     { return driver.Stats{} }
func (c *fakeThreatIntelConn) Close() error                                            { return nil }

type fakeThreatIntelRow struct {
	values []any
	err    error
}

func (r fakeThreatIntelRow) Err() error { return r.err }
func (r fakeThreatIntelRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return errors.New("unexpected scan destination count")
	}
	for i := range dest {
		target := reflect.ValueOf(dest[i])
		if target.Kind() != reflect.Pointer || target.IsNil() {
			return errors.New("scan destination must be pointer")
		}
		value := reflect.ValueOf(r.values[i])
		if !value.Type().AssignableTo(target.Elem().Type()) {
			return errors.New("scan value is not assignable")
		}
		target.Elem().Set(value)
	}
	return nil
}
func (r fakeThreatIntelRow) ScanStruct(any) error { return r.err }
