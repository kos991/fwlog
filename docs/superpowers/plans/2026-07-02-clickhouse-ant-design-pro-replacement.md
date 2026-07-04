# ClickHouse Ant Design Pro Replacement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用 ClickHouse 后端和 Ant Design Pro 前端完全替换旧 DuckDB + Vue 单页生产路径。

**Architecture:** Go 服务拆成配置、认证、ClickHouse 存储、导入状态机、查询可见性、HTTP API 和静态前端嵌入几个边界清晰的模块。ClickHouse 使用 native TCP 驱动和 `PrepareBatch` 写入，状态表使用 `ReplacingMergeTree(updated_at)`，前端使用 Ant Design Pro / React 并由 Go 二进制嵌入发布。

**Tech Stack:** Go 1.21、Gin、`github.com/ClickHouse/clickhouse-go/v2`、ClickHouse MergeTree、Ant Design Pro / React / Vite、Go `embed`。

## Global Constraints

- 永远使用简体中文回复、注释和用户可见文案。
- 新版本完全替换 DuckDB；DuckDB 查询库、索引重建、`tmp_import.csv`、旧 Vue 单页都不进入新生产路径。
- 日志只读取归档文件：`.log-YYYYMMDD` 和 `.log-YYYYMMDD.gz`，不读取当天在线写入的 `.log`。
- Go 端使用 `clickhouse-go/v2` native TCP 和 `PrepareBatch`；不走 HTTP 写入，不把 `database/sql` 作为生产导入主路径。
- ClickHouse 表不使用 `Nullable`，缺失值由 Go 层写默认值。
- `nat_logs` 第一版 `PARTITION BY log_date`，`ORDER BY (log_date, source_id, src_ip, timestamp)`。
- 查询跨未入库日期时，只查询已入库部分并返回 `visibility` 提示。
- Web 主导航固定为：监控大屏、日志检索、增量进度、系统维护。
- 单一管理员密码登录，不引入用户账号和角色权限。
- Go 版本保持 `go 1.21`。添加或整理依赖后必须检查 `go.mod`，不能因为 `go get` 或 `go mod tidy` 自动抬高 Go 基线。
- 不提交 `web/node_modules/`、`web/dist/`、`dist/`、`*.tsbuildinfo`、Vite 日志和本地构建产物；提交命令必须列出源文件和配置文件。
- 每个任务按 TDD 执行：先写失败测试，再实现，再验证通过。

---

## 文件结构

后端目标文件：

- `main.go`：缩减为启动入口、依赖装配、路由注册、静态文件服务。
- `config.go`：运行时配置、环境变量、默认值、导出目录。
- `auth.go`：单一管理员密码、会话 cookie、密码修改。
- `ip_engine.go`：保留现有 IP 标注引擎，修正/复用自定义映射和 GeoIP 加载。
- `clickhouse_store.go`：ClickHouse 连接、`EnsureTables()`、DDL 常量、健康检查。
- `settings_store.go`：`app_settings`、`log_sources` 读写。
- `ingest_state_store.go`：`ingest_dates`、`ingest_files` 状态读写、`argMax` 列表查询。
- `log_scanner.go`：归档日志扫描、日期解析、静置判断。
- `log_parser.go`：NAT 行解析、默认值填充、协议/动作归一化。
- `importer.go`：流式解压、批量写入 ClickHouse、进度更新、失败重试。
- `scheduler.go`：自动增量调度、手动同步、重试队列。
- `query_service.go`：查询参数、可见性计算、ClickHouse SQL 构造。
- `dashboard_service.go`：数据健康、导入健康、IP 分布、国家地区分布。
- `handlers.go`：Gin API handler。
- `static.go`：嵌入 `web/dist` 并服务前端。
- `types.go`：API 请求响应、领域模型、状态枚举。

后端测试文件：

- `clickhouse_store_test.go`
- `settings_store_test.go`
- `ingest_state_store_test.go`
- `log_scanner_test.go`
- `log_parser_test.go`
- `query_service_test.go`
- `dashboard_service_test.go`
- `handlers_test.go`
- `auth_test.go`

前端目标文件：

- `web/package.json`：保留 React / Ant Design / Pro Components / Vite 依赖。
- `web/src/main.tsx`：入口和路由。
- `web/src/api.ts`：API 客户端和类型。
- `web/src/layout/AppLayout.tsx`：Ant Design Pro 布局。
- `web/src/pages/LoginPage.tsx`
- `web/src/pages/HealthDashboard.tsx`
- `web/src/pages/LogSearchPage.tsx`
- `web/src/pages/IncrementalProgressPage.tsx`
- `web/src/pages/SystemMaintenancePage.tsx`
- `web/src/styles.css`

前端测试/验证：

- `npm run build`
- Playwright 或浏览器验证登录、导航、查询、增量进度、系统维护表单。

---

### Task 1: 清理旧 DuckDB 入口并建立 ClickHouse 配置模型

**Files:**
- Modify: `go.mod`
- Modify: `main.go`
- Create: `types.go`
- Create: `config.go`
- Test: `config_test.go`

**Interfaces:**
- Produces: `type Config struct`
- Produces: `func LoadConfig() Config`
- Produces: `func (c Config) ExportDir() string`
- Produces: `type App struct`

- [ ] **Step 1: 写失败测试**

在 `config_test.go` 中写：

```go
package main

import "testing"

func TestLoadConfigUsesClickHouseDefaults(t *testing.T) {
	t.Setenv("LOG_DIR", "")
	t.Setenv("CLICKHOUSE_ADDR", "")
	t.Setenv("CLICKHOUSE_DATABASE", "")

	cfg := LoadConfig()

	if cfg.LogDir != "/data/sangfor_fw_log" {
		t.Fatalf("LogDir = %q", cfg.LogDir)
	}
	if cfg.ClickHouseAddr != "127.0.0.1:9000" {
		t.Fatalf("ClickHouseAddr = %q", cfg.ClickHouseAddr)
	}
	if cfg.ClickHouseDatabase != "default" {
		t.Fatalf("ClickHouseDatabase = %q", cfg.ClickHouseDatabase)
	}
	if cfg.LogTag != "深信服 NAT" {
		t.Fatalf("LogTag = %q", cfg.LogTag)
	}
	if cfg.ExportDir() != "/data/export" {
		t.Fatalf("ExportDir = %q", cfg.ExportDir())
	}
}

func TestLoadConfigReadsRuntimeSettingsFromEnvironment(t *testing.T) {
	t.Setenv("LOG_DIR", "/logs/fw")
	t.Setenv("LOG_TAG", "出口防火墙")
	t.Setenv("CLICKHOUSE_ADDR", "10.0.0.8:9000")
	t.Setenv("CLICKHOUSE_DATABASE", "nat")
	t.Setenv("CUSTOM_IP_MAP", "/opt/nat/custom.csv")
	t.Setenv("GEOIP_DB", "/opt/nat/GeoLite2-City.mmdb")
	t.Setenv("PORT", "18080")

	cfg := LoadConfig()

	if cfg.LogDir != "/logs/fw" || cfg.LogTag != "出口防火墙" {
		t.Fatalf("unexpected log source config: %#v", cfg)
	}
	if cfg.ClickHouseAddr != "10.0.0.8:9000" || cfg.ClickHouseDatabase != "nat" {
		t.Fatalf("unexpected ClickHouse config: %#v", cfg)
	}
	if cfg.CustomIPMapPath != "/opt/nat/custom.csv" || cfg.GeoIPDBPath != "/opt/nat/GeoLite2-City.mmdb" {
		t.Fatalf("unexpected IP data config: %#v", cfg)
	}
	if cfg.Port != 18080 {
		t.Fatalf("Port = %d", cfg.Port)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./...`

Expected: FAIL，提示 `LoadConfig`、`ClickHouseAddr` 等符号不存在或仍是 DuckDB 配置。

- [ ] **Step 3: 实现配置模型**

在 `types.go` 中定义核心状态枚举和配置：

```go
package main

type IngestStatus string

const (
	StatusIdle      IngestStatus = "idle"
	StatusPending   IngestStatus = "pending"
	StatusScanning  IngestStatus = "scanning"
	StatusImporting IngestStatus = "importing"
	StatusReady     IngestStatus = "ready"
	StatusFailed    IngestStatus = "failed"
	StatusSucceeded IngestStatus = "succeeded"
)

type Config struct {
	LogDir              string
	LogTag              string
	Port                int
	Workers             int
	ClickHouseAddr      string
	ClickHouseDatabase  string
	ClickHouseUser      string
	ClickHousePassword  string
	CustomIPMapPath     string
	GeoIPDBPath         string
	IPMapEnabled        bool
	GeoIPEnabled        bool
	AutoScanEnabled     bool
	AutoScanMode        string
	AutoScanTimes       string
	AutoScanIntervalSec int
	AutoScanTimezone    string
	AutoScanJitterSec   int
	ExportRoot          string
}
```

在 `config.go` 中实现：

```go
package main

import (
	"os"
	"strconv"
	"strings"
)

const (
	defaultLogDir      = "/data/sangfor_fw_log"
	defaultLogTag      = "深信服 NAT"
	defaultExportDir   = "/data/export"
	defaultPort        = 8080
	defaultWorkers     = 4
	defaultAutoScanSec = 3600
)

func LoadConfig() Config {
	return Config{
		LogDir:              getEnv("LOG_DIR", defaultLogDir),
		LogTag:              normalizeLogTag(getEnv("LOG_TAG", defaultLogTag)),
		Port:                getEnvInt("PORT", defaultPort),
		Workers:             getEnvInt("WORKERS", defaultWorkers),
		ClickHouseAddr:      getEnv("CLICKHOUSE_ADDR", "127.0.0.1:9000"),
		ClickHouseDatabase:  getEnv("CLICKHOUSE_DATABASE", "default"),
		ClickHouseUser:      getEnv("CLICKHOUSE_USER", "default"),
		ClickHousePassword:  os.Getenv("CLICKHOUSE_PASSWORD"),
		CustomIPMapPath:     getEnv("CUSTOM_IP_MAP", "/opt/nat-query/custom_ip_map.csv"),
		GeoIPDBPath:         getEnv("GEOIP_DB", "/data/index/GeoLite2-City.mmdb"),
		IPMapEnabled:        getEnvBool("IP_MAP_ENABLED", true),
		GeoIPEnabled:        getEnvBool("GEOIP_ENABLED", true),
		AutoScanEnabled:     getEnvBool("AUTO_SCAN_ENABLED", false),
		AutoScanMode:        getEnv("AUTO_SCAN_MODE", "hourly"),
		AutoScanTimes:       getEnv("AUTO_SCAN_TIMES", "01:00"),
		AutoScanIntervalSec: getEnvInt("AUTO_SCAN_INTERVAL_SEC", defaultAutoScanSec),
		AutoScanTimezone:    getEnv("AUTO_SCAN_TIMEZONE", "Asia/Shanghai"),
		AutoScanJitterSec:   getEnvInt("AUTO_SCAN_JITTER_SEC", 60),
		ExportRoot:          getEnv("EXPORT_DIR", defaultExportDir),
	}
}

func (c Config) ExportDir() string {
	if strings.TrimSpace(c.ExportRoot) == "" {
		return defaultExportDir
	}
	return c.ExportRoot
}

func normalizeLogTag(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultLogTag
	}
	return value
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}
```

在 `go.mod` 中移除 `github.com/marcboeker/go-duckdb`，暂不添加 ClickHouse 依赖，下一任务添加。

把 `main.go` 临时缩到可编译入口：

```go
package main

func main() {
	cfg := LoadConfig()
	app := NewApp(cfg)
	if err := app.Run(); err != nil {
		panic(err)
	}
}
```

并创建最小 `app.go`：

```go
package main

type App struct {
	cfg Config
}

func NewApp(cfg Config) *App {
	return &App{cfg: cfg}
}

func (a *App) Run() error {
	return nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./...`

Expected: PASS。若旧 DuckDB 测试引用已删除函数，应同步删除旧测试或在后续任务重建等价 ClickHouse 测试。

- [ ] **Step 5: 提交**

```bash
git add go.mod go.sum main.go app.go types.go config.go config_test.go
git commit -m "refactor: replace duckdb bootstrap with clickhouse config"
```

---

### Task 2: ClickHouse 连接与建表

**Files:**
- Modify: `go.mod`
- Create: `clickhouse_store.go`
- Test: `clickhouse_store_test.go`

**Interfaces:**
- Consumes: `Config`
- Produces: `type ClickHouseStore struct`
- Produces: `func OpenClickHouse(ctx context.Context, cfg Config) (*ClickHouseStore, error)`
- Produces: `func (s *ClickHouseStore) EnsureTables(ctx context.Context) error`
- Produces: `func ClickHouseDDL() []string`

- [ ] **Step 1: 写失败测试**

在 `clickhouse_store_test.go` 中写不依赖真实 ClickHouse 的 DDL 测试：

```go
package main

import (
	"strings"
	"testing"
)

func TestClickHouseDDLContainsCoreTables(t *testing.T) {
	sql := strings.Join(ClickHouseDDL(), "\n")
	for _, table := range []string{"app_settings", "log_sources", "ingest_dates", "ingest_files", "nat_logs"} {
		if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Fatalf("DDL missing table %s:\n%s", table, sql)
		}
	}
}

func TestNatLogsDDLUsesExpectedPartitionAndOrderKey(t *testing.T) {
	sql := strings.Join(ClickHouseDDL(), "\n")
	if !strings.Contains(sql, "PARTITION BY log_date") {
		t.Fatalf("nat_logs must partition by log_date:\n%s", sql)
	}
	if !strings.Contains(sql, "ORDER BY (log_date, source_id, src_ip, timestamp)") {
		t.Fatalf("nat_logs must use expected sorting key:\n%s", sql)
	}
	if strings.Contains(sql, "Nullable(") {
		t.Fatalf("DDL must not use Nullable:\n%s", sql)
	}
}

func TestStateTablesUseReplacingMergeTree(t *testing.T) {
	sql := strings.Join(ClickHouseDDL(), "\n")
	for _, snippet := range []string{
		"ENGINE = ReplacingMergeTree(updated_at)",
		"PRIMARY KEY (source_id, log_date)",
		"PRIMARY KEY path",
	} {
		if !strings.Contains(sql, snippet) {
			t.Fatalf("DDL missing %q:\n%s", snippet, sql)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./...`

Expected: FAIL，提示 `ClickHouseDDL` 不存在。

- [ ] **Step 3: 添加 ClickHouse 依赖并实现**

Run: `go get github.com/ClickHouse/clickhouse-go/v2`

创建 `clickhouse_store.go`：

```go
package main

import (
	"context"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

type ClickHouseStore struct {
	conn clickhouse.Conn
}

func OpenClickHouse(ctx context.Context, cfg Config) (*ClickHouseStore, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.ClickHouseAddr},
		Auth: clickhouse.Auth{
			Database: cfg.ClickHouseDatabase,
			Username: cfg.ClickHouseUser,
			Password: cfg.ClickHousePassword,
		},
		DialTimeout:     10 * time.Second,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
	})
	if err != nil {
		return nil, err
	}
	if err := conn.Ping(ctx); err != nil {
		return nil, err
	}
	return &ClickHouseStore{conn: conn}, nil
}

func (s *ClickHouseStore) EnsureTables(ctx context.Context) error {
	for _, statement := range ClickHouseDDL() {
		if err := s.conn.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func ClickHouseDDL() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS app_settings
(
    key String,
    value String,
    updated_at DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(updated_at)
PRIMARY KEY key
ORDER BY key`,
		`CREATE TABLE IF NOT EXISTS log_sources
(
    source_id String,
    log_dir String,
    log_tag LowCardinality(String),
    enabled UInt8 DEFAULT 1,
    updated_at DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(updated_at)
PRIMARY KEY source_id
ORDER BY source_id`,
		`CREATE TABLE IF NOT EXISTS ingest_dates
(
    source_id String,
    log_tag LowCardinality(String),
    log_date Date,
    status LowCardinality(String) DEFAULT 'pending',
    files_total UInt64 DEFAULT 0,
    files_done UInt64 DEFAULT 0,
    rows_imported UInt64 DEFAULT 0,
    bytes_total UInt64 DEFAULT 0,
    bytes_done UInt64 DEFAULT 0,
    current_file String DEFAULT '',
    progress_pct Float64 DEFAULT 0,
    max_visible_timestamp DateTime DEFAULT toDateTime(0),
    retry_count UInt8 DEFAULT 0,
    next_retry_at DateTime DEFAULT toDateTime(0),
    started_at DateTime DEFAULT toDateTime(0),
    finished_at DateTime DEFAULT toDateTime(0),
    error String DEFAULT '',
    updated_at DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(updated_at)
PRIMARY KEY (source_id, log_date)
ORDER BY (source_id, log_date)`,
		`CREATE TABLE IF NOT EXISTS ingest_files
(
    path String,
    source_id String,
    log_tag LowCardinality(String),
    log_date Date,
    size_bytes UInt64 DEFAULT 0,
    mtime DateTime DEFAULT toDateTime(0),
    status LowCardinality(String) DEFAULT 'pending',
    rows_imported UInt64 DEFAULT 0,
    bytes_total UInt64 DEFAULT 0,
    bytes_done UInt64 DEFAULT 0,
    progress_pct Float64 DEFAULT 0,
    stable_seen_count UInt8 DEFAULT 0,
    first_seen_at DateTime DEFAULT toDateTime(0),
    retry_count UInt8 DEFAULT 0,
    next_retry_at DateTime DEFAULT toDateTime(0),
    started_at DateTime DEFAULT toDateTime(0),
    finished_at DateTime DEFAULT toDateTime(0),
    error String DEFAULT '',
    updated_at DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(updated_at)
PRIMARY KEY path
ORDER BY path`,
		`CREATE TABLE IF NOT EXISTS nat_logs
(
    source_id String CODEC(ZSTD(3)),
    log_tag LowCardinality(String) CODEC(ZSTD(3)),
    log_date Date CODEC(DoubleDelta, ZSTD(3)),
    timestamp DateTime CODEC(DoubleDelta, ZSTD(3)),
    src_ip IPv4 CODEC(ZSTD(3)),
    src_port UInt16 CODEC(T64, ZSTD(3)),
    dst_ip IPv4 CODEC(ZSTD(3)),
    dst_port UInt16 CODEC(T64, ZSTD(3)),
    nat_ip IPv4 CODEC(ZSTD(3)),
    nat_port UInt16 CODEC(T64, ZSTD(3)),
    protocol LowCardinality(String) CODEC(ZSTD(3)),
    action LowCardinality(String) DEFAULT 'ALLOW' CODEC(ZSTD(3)),
    source_file LowCardinality(String) CODEC(ZSTD(3)),
    source_offset UInt64 CODEC(Delta, ZSTD(3)),
    batch_id String DEFAULT '' CODEC(ZSTD(3)),
    ingested_at DateTime DEFAULT now() CODEC(DoubleDelta, ZSTD(3))
)
ENGINE = MergeTree
PARTITION BY log_date
ORDER BY (log_date, source_id, src_ip, timestamp)
SETTINGS index_granularity = 8192`,
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./...`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add go.mod go.sum clickhouse_store.go clickhouse_store_test.go
git commit -m "feat: add clickhouse store and ddl"
```

---

### Task 3: 配置、日志源和状态存储

**Files:**
- Create: `settings_store.go`
- Create: `ingest_state_store.go`
- Test: `settings_store_test.go`
- Test: `ingest_state_store_test.go`

**Interfaces:**
- Produces: `type LogSource`
- Produces: `type AppSettings`
- Produces: `func SettingsUpsertSQL() string`
- Produces: `func EnabledLogSourcesSQL() string`
- Produces: `type DateIngestState`
- Produces: `func DateStatePointQuery() string`
- Produces: `func DateStateListQuery() string`

- [ ] **Step 1: 写 SQL 契约失败测试**

`settings_store_test.go`：

```go
package main

import (
	"strings"
	"testing"
)

func TestSettingsQueriesUseFinalOnlyForSmallTables(t *testing.T) {
	if !strings.Contains(EnabledLogSourcesSQL(), "log_sources FINAL") {
		t.Fatalf("log_sources query should use FINAL: %s", EnabledLogSourcesSQL())
	}
	if !strings.Contains(AppSettingsSQL(), "app_settings FINAL") {
		t.Fatalf("app_settings query should use FINAL: %s", AppSettingsSQL())
	}
}
```

`ingest_state_store_test.go`：

```go
package main

import (
	"strings"
	"testing"
)

func TestDateStatePointQueryOrdersByUpdatedAt(t *testing.T) {
	sql := DateStatePointQuery()
	if !strings.Contains(sql, "ORDER BY updated_at DESC") || !strings.Contains(sql, "LIMIT 1") {
		t.Fatalf("point query must read newest state: %s", sql)
	}
	if strings.Contains(sql, "FINAL") {
		t.Fatalf("point query must not use FINAL: %s", sql)
	}
}

func TestDateStateListQueryUsesArgMax(t *testing.T) {
	sql := DateStateListQuery()
	for _, want := range []string{"argMax(status, updated_at)", "GROUP BY source_id, log_date"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("list query missing %q: %s", want, sql)
		}
	}
	if strings.Contains(sql, "FINAL") {
		t.Fatalf("list query must not use FINAL: %s", sql)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./...`

Expected: FAIL，提示 SQL helper 不存在。

- [ ] **Step 3: 实现状态存储类型和 SQL**

`settings_store.go`：

```go
package main

import "time"

type LogSource struct {
	SourceID  string    `json:"source_id"`
	LogDir    string    `json:"log_dir"`
	LogTag    string    `json:"log_tag"`
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AppSettings map[string]string

func AppSettingsSQL() string {
	return `SELECT key, value FROM app_settings FINAL`
}

func EnabledLogSourcesSQL() string {
	return `SELECT source_id, log_dir, log_tag, enabled FROM log_sources FINAL WHERE enabled = 1`
}

func SettingsUpsertSQL() string {
	return `INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, now())`
}
```

`ingest_state_store.go`：

```go
package main

import "time"

type DateIngestState struct {
	SourceID            string       `json:"source_id"`
	LogTag              string       `json:"log_tag"`
	LogDate             time.Time    `json:"log_date"`
	Status              IngestStatus `json:"status"`
	FilesTotal          uint64       `json:"files_total"`
	FilesDone           uint64       `json:"files_done"`
	RowsImported        uint64       `json:"rows_imported"`
	BytesTotal          uint64       `json:"bytes_total"`
	BytesDone           uint64       `json:"bytes_done"`
	CurrentFile         string       `json:"current_file"`
	ProgressPct         float64      `json:"progress_pct"`
	MaxVisibleTimestamp time.Time    `json:"max_visible_timestamp"`
	Error               string       `json:"error"`
	UpdatedAt           time.Time    `json:"updated_at"`
}

func DateStatePointQuery() string {
	return `SELECT
    status, files_total, files_done, rows_imported, bytes_total, bytes_done,
    current_file, progress_pct, max_visible_timestamp, error, updated_at
FROM ingest_dates
WHERE source_id = ? AND log_date = ?
ORDER BY updated_at DESC
LIMIT 1`
}

func DateStateListQuery() string {
	return `SELECT
    source_id,
    argMax(log_tag, updated_at) AS log_tag,
    log_date,
    argMax(status, updated_at) AS status,
    argMax(files_total, updated_at) AS files_total,
    argMax(files_done, updated_at) AS files_done,
    argMax(rows_imported, updated_at) AS rows_imported,
    argMax(bytes_total, updated_at) AS bytes_total,
    argMax(bytes_done, updated_at) AS bytes_done,
    argMax(current_file, updated_at) AS current_file,
    argMax(progress_pct, updated_at) AS progress_pct,
    argMax(max_visible_timestamp, updated_at) AS max_visible_timestamp,
    argMax(error, updated_at) AS error,
    max(updated_at) AS updated_at
FROM ingest_dates
WHERE log_date >= ?
GROUP BY source_id, log_date
ORDER BY log_date DESC, source_id ASC`
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./...`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add settings_store.go ingest_state_store.go settings_store_test.go ingest_state_store_test.go
git commit -m "feat: add settings and ingest state stores"
```

---

### Task 4: 归档日志扫描和 NAT 行解析

**Files:**
- Create: `log_scanner.go`
- Create: `log_parser.go`
- Test: `log_scanner_test.go`
- Test: `log_parser_test.go`

**Interfaces:**
- Produces: `func IsArchivedLogFile(name string) bool`
- Produces: `func ExtractLogDate(name string) (time.Time, bool)`
- Produces: `func ScanArchivedLogFiles(root string, now time.Time) ([]LogFileSnapshot, error)`
- Produces: `func ParseNATLine(line string, meta ParseMeta) (NATLogRow, bool)`

- [ ] **Step 1: 写失败测试**

`log_scanner_test.go`：

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsArchivedLogFileSkipsOnlineLog(t *testing.T) {
	cases := map[string]bool{
		"sangfor.log":              false,
		"sangfor.log-20260701":     true,
		"sangfor.log-20260701.gz":  true,
		"sangfor.log-20260701.tmp": false,
	}
	for name, want := range cases {
		if got := IsArchivedLogFile(name); got != want {
			t.Fatalf("IsArchivedLogFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestScanArchivedLogFilesReturnsOnlyStableArchives(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-10 * time.Minute)
	for _, name := range []string{"sangfor.log", "sangfor.log-20260701", "sangfor.log-20260702.gz"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}

	files, err := ScanArchivedLogFiles(dir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 archive files, got %#v", files)
	}
}
```

`log_parser_test.go`：

```go
package main

import (
	"net/netip"
	"testing"
	"time"
)

func TestParseNATLineWritesDefaultsAndMetadata(t *testing.T) {
	line := "2026 Jun 28 01:17:18 源IP:192.168.1.10 源端口:12345 目的IP:222.186.177.145 目的端口:443 协议:6 转换后的IP:10.0.0.1 转换后的端口:50000"
	row, ok := ParseNATLine(line, ParseMeta{
		SourceID:     "src_1",
		LogTag:       "出口防火墙",
		LogDate:      time.Date(2026, 6, 28, 0, 0, 0, 0, time.Local),
		SourceFile:   "/logs/sangfor.log-20260628",
		SourceOffset: 128,
		BatchID:      "batch_1",
	})
	if !ok {
		t.Fatal("line should parse")
	}
	if row.SourceID != "src_1" || row.LogTag != "出口防火墙" {
		t.Fatalf("metadata missing: %#v", row)
	}
	if row.SrcIP != netip.MustParseAddr("192.168.1.10") || row.DstPort != 443 {
		t.Fatalf("unexpected parsed row: %#v", row)
	}
	if row.Protocol == "" || row.Action == "" {
		t.Fatalf("protocol/action should have defaults: %#v", row)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./...`

Expected: FAIL，提示扫描和解析函数不存在。

- [ ] **Step 3: 实现扫描和解析**

实现 `LogFileSnapshot`、`ParseMeta`、`NATLogRow`。扫描只接受 `.log-YYYYMMDD` 和 `.log-YYYYMMDD.gz`，mtime 距当前时间不足 5 分钟的文件不返回。

解析层第一版直接使用手写字段扫描，不在热路径使用循环内正则。实现时使用 `strings.Index` 定位固定字段，字段缺失时写默认值；只有确认日志存在多种格式且手写分支不可维护时，才允许引入初始化阶段预编译的 `regexp.MustCompile`。

`ParseNATLine` 的实现结构按下面边界写：

```go
func ParseNATLine(line string, meta ParseMeta) (NATLogRow, bool) {
	row := NATLogRow{
		SourceID:     meta.SourceID,
		LogTag:       meta.LogTag,
		LogDate:      meta.LogDate,
		SourceFile:   meta.SourceFile,
		SourceOffset: meta.SourceOffset,
		BatchID:      meta.BatchID,
		SrcIP:        netip.MustParseAddr("0.0.0.0"),
		DstIP:        netip.MustParseAddr("0.0.0.0"),
		NatIP:        netip.MustParseAddr("0.0.0.0"),
		Protocol:     "UNKNOWN",
		Action:       "ALLOW",
	}
	ts, ok := parseSangforTimestamp(line, meta.LogDate.Location())
	if !ok {
		return row, false
	}
	row.Timestamp = ts
	row.SrcIP = parseIPv4Field(line, "源IP:", row.SrcIP)
	row.SrcPort = parsePortField(line, "源端口:")
	row.DstIP = parseIPv4Field(line, "目的IP:", row.DstIP)
	row.DstPort = parsePortField(line, "目的端口:")
	row.NatIP = parseIPv4Field(line, "转换后的IP:", row.NatIP)
	row.NatPort = parsePortField(line, "转换后的端口:")
	row.Protocol = normalizeProtocol(parseStringField(line, "协议:", row.Protocol))
	return row, true
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./...`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add log_scanner.go log_parser.go log_scanner_test.go log_parser_test.go
git commit -m "feat: scan archives and parse nat rows"
```

---

### Task 5: ClickHouse 批量写入和增量状态机

**Files:**
- Create: `importer.go`
- Create: `scheduler.go`
- Test: `importer_test.go`
- Test: `scheduler_test.go`

**Interfaces:**
- Produces: `type Importer`
- Produces: `func (i *Importer) ImportDate(ctx context.Context, source LogSource, date time.Time) error`
- Produces: `func (i *Importer) AppendBatch(ctx context.Context, rows []NATLogRow) error`
- Produces: `func ShouldRetryDate(state DateIngestState, now time.Time) bool`
- Produces: `func NextRetryAt(retryCount uint8, now time.Time) time.Time`

- [ ] **Step 1: 写失败测试**

`scheduler_test.go`：

```go
package main

import (
	"testing"
	"time"
)

func TestNextRetryAtUsesExpectedBackoff(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.Local)
	if got := NextRetryAt(0, now); got.Sub(now) != time.Minute {
		t.Fatalf("retry 0 backoff = %s", got.Sub(now))
	}
	if got := NextRetryAt(1, now); got.Sub(now) != 5*time.Minute {
		t.Fatalf("retry 1 backoff = %s", got.Sub(now))
	}
	if got := NextRetryAt(2, now); got.Sub(now) != 15*time.Minute {
		t.Fatalf("retry 2 backoff = %s", got.Sub(now))
	}
}

func TestShouldRetryDateStopsAfterThreeFailures(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.Local)
	state := DateIngestState{Status: StatusFailed, UpdatedAt: now.Add(-time.Hour)}
	if !ShouldRetryDate(state, now) {
		t.Fatal("failed date should retry when retry_count is below limit")
	}
	state.RetryCount = 3
	if ShouldRetryDate(state, now) {
		t.Fatal("failed date should stop retrying after 3 attempts")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./...`

Expected: FAIL。

- [ ] **Step 3: 实现导入器骨架**

在 `DateIngestState` 中补 `RetryCount uint8` 和 `NextRetryAt time.Time` 字段。实现 `NextRetryAt`、`ShouldRetryDate`。`Importer.AppendBatch` 使用 `PrepareBatch`：

```go
batch, err := i.store.conn.PrepareBatch(ctx, `INSERT INTO nat_logs`)
if err != nil {
	return err
}
for _, row := range rows {
	if err := batch.Append(
		row.SourceID,
		row.LogTag,
		row.LogDate,
		row.Timestamp,
		row.SrcIP,
		row.SrcPort,
		row.DstIP,
		row.DstPort,
		row.NatIP,
		row.NatPort,
		row.Protocol,
		row.Action,
		row.SourceFile,
		row.SourceOffset,
		row.BatchID,
	); err != nil {
		return err
	}
}
return batch.Send()
```

导入失败时更新 `ingest_files=failed`、`ingest_dates=failed`，不推进 ready。日期重试前执行：

```sql
ALTER TABLE nat_logs DROP PARTITION ?
```

执行成功后等待 1 秒再写入同日数据。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./...`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add importer.go scheduler.go importer_test.go scheduler_test.go ingest_state_store.go
git commit -m "feat: add incremental importer state machine"
```

---

### Task 6: 查询可见性和日志检索 API

**Files:**
- Create: `query_service.go`
- Create: `handlers.go`
- Test: `query_service_test.go`
- Test: `handlers_test.go`

**Interfaces:**
- Produces: `type QueryRequest`
- Produces: `type QueryVisibility`
- Produces: `func BuildVisibleRanges(start, end time.Time, states []DateIngestState) QueryVisibility`
- Produces: `func BuildQuerySQL(req QueryRequest, visibility QueryVisibility) (string, []any, error)`

- [ ] **Step 1: 写失败测试**

`query_service_test.go`：

```go
package main

import (
	"testing"
	"time"
)

func TestBuildVisibleRangesQueriesReadyAndPartialImportingDates(t *testing.T) {
	start := time.Date(2026, 6, 28, 1, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 1, 12, 0, 0, 0, time.Local)
	states := []DateIngestState{
		{LogDate: dateOnly(2026, 6, 28), Status: StatusReady, MaxVisibleTimestamp: endOfDay(2026, 6, 28)},
		{LogDate: dateOnly(2026, 6, 29), Status: StatusReady, MaxVisibleTimestamp: endOfDay(2026, 6, 29)},
		{LogDate: dateOnly(2026, 6, 30), Status: StatusImporting, MaxVisibleTimestamp: time.Date(2026, 6, 30, 10, 0, 0, 0, time.Local)},
		{LogDate: dateOnly(2026, 7, 1), Status: StatusPending},
	}

	visibility := BuildVisibleRanges(start, end, states)

	if !visibility.Partial {
		t.Fatal("visibility should be partial")
	}
	if len(visibility.QueriedRanges) != 3 {
		t.Fatalf("queried ranges = %#v", visibility.QueriedRanges)
	}
	if len(visibility.SkippedDates) != 1 {
		t.Fatalf("skipped dates = %#v", visibility.SkippedDates)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./...`

Expected: FAIL。

- [ ] **Step 3: 实现可见性和 SQL 构造**

`BuildVisibleRanges` 对 ready 日期取用户范围与整日交集，对 importing 日期取用户范围与 `max_visible_timestamp` 交集，对 pending/failed 记录 skipped。`BuildQuerySQL` 必须追加：

```sql
(
    (log_date = ? AND timestamp >= ? AND timestamp <= ?)
    OR (log_date = ? AND timestamp >= ? AND timestamp <= ?)
)
```

实际代码不要拼接上面的固定两段示例，而是按 `visibility.QueriedRanges` 循环生成同构条件。每个可见范围追加 3 个参数：`log_date`、`start_time`、`end_time`。如果没有任何可见范围，直接返回业务错误 `所选时间段内的日志尚未入库或不可查询`，不执行 ClickHouse 查询。

响应结构：

```go
type QueryVisibility struct {
	Partial       bool             `json:"partial"`
	Message       string           `json:"message"`
	QueriedRanges []VisibleRange   `json:"queried_ranges"`
	SkippedDates  []SkippedLogDate `json:"skipped_dates"`
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./...`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add query_service.go handlers.go query_service_test.go handlers_test.go
git commit -m "feat: add visible query ranges"
```

---

### Task 7: 健康大屏和增量进度 API

**Files:**
- Create: `dashboard_service.go`
- Modify: `handlers.go`
- Test: `dashboard_service_test.go`

**Interfaces:**
- Produces: `type HealthDashboardResponse`
- Produces: `type IngestProgressResponse`
- Produces: `func BuildHealthDashboard(states []DateIngestState, stats DashboardStats, ip IPDistribution, geo GeoDistribution) HealthDashboardResponse`
- Produces: `func BuildIngestProgress(states []DateIngestState, includeReady bool) IngestProgressResponse`

- [ ] **Step 1: 写失败测试**

`dashboard_service_test.go`：

```go
package main

import "testing"

func TestBuildIngestProgressShowsProblemDatesByDefault(t *testing.T) {
	states := []DateIngestState{
		{Status: StatusReady},
		{Status: StatusImporting},
		{Status: StatusPending},
		{Status: StatusFailed},
	}

	progress := BuildIngestProgress(states, false)

	if len(progress.Dates) != 3 {
		t.Fatalf("default list should hide ready dates, got %#v", progress.Dates)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./...`

Expected: FAIL。

- [ ] **Step 3: 实现响应聚合**

`BuildHealthDashboard` 分四块返回：

```go
type HealthDashboardResponse struct {
	DataHealth      DataHealth      `json:"data_health"`
	IngestHealth    IngestHealth    `json:"ingest_health"`
	IPDistribution  IPDistribution  `json:"ip_distribution"`
	GeoDistribution GeoDistribution `json:"geo_distribution"`
}
```

`BuildIngestProgress` 默认隐藏 ready 日期，传 `includeReady=true` 时展示全部。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./...`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add dashboard_service.go handlers.go dashboard_service_test.go
git commit -m "feat: add health dashboard and ingest progress api"
```

---

### Task 8: 认证、设置和 IP 库热加载 API

**Files:**
- Create: `auth.go`
- Create: `ip_data.go`
- Modify: `handlers.go`
- Test: `auth_test.go`
- Test: `ip_data_test.go`

**Interfaces:**
- Produces: `func HashPassword(password string) (string, error)`
- Produces: `func VerifyPassword(encoded, password string) bool`
- Produces: `func ReloadIPEngine(cfg Config, old *IPEngine) (*IPEngine, IPDataStatus)`

- [ ] **Step 1: 写失败测试**

`ip_data_test.go`：

```go
package main

import "testing"

func TestReloadIPEngineKeepsOldEngineWhenPathFails(t *testing.T) {
	old := NewIPEngine()
	next, status := ReloadIPEngine(Config{
		IPMapEnabled:    true,
		CustomIPMapPath: "Z:/missing/custom.csv",
		GeoIPEnabled:    false,
	}, old)

	if next != old {
		t.Fatal("failed reload should keep old engine")
	}
	if status.Error == "" {
		t.Fatal("failed reload should return error status")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./...`

Expected: FAIL。

- [ ] **Step 3: 实现认证和 IP 库热加载**

保留单一管理员密码，密码 hash 存入 `app_settings`。`ReloadIPEngine` 创建新引擎，全部启用库加载成功后替换；失败返回旧引擎和错误状态。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./...`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add auth.go ip_data.go handlers.go auth_test.go ip_data_test.go
git commit -m "feat: add auth settings and ip data reload"
```

---

### Task 9: Ant Design Pro 前端替换

**Files:**
- Modify: `web/package.json`
- Modify: `web/src/main.tsx`
- Modify: `web/src/styles.css`
- Create: `web/src/api.ts`
- Create: `web/src/layout/AppLayout.tsx`
- Create: `web/src/pages/LoginPage.tsx`
- Create: `web/src/pages/HealthDashboard.tsx`
- Create: `web/src/pages/LogSearchPage.tsx`
- Create: `web/src/pages/IncrementalProgressPage.tsx`
- Create: `web/src/pages/SystemMaintenancePage.tsx`

**Interfaces:**
- Consumes: `/api/session`、`/api/login`、`/api/health-dashboard`、`/api/query`、`/api/ingest-progress`、`/api/settings`
- Produces: `web/dist`

- [ ] **Step 1: 改造前端 API 类型**

`web/src/api.ts` 必须导出：

```ts
export async function apiGet<T>(path: string): Promise<T>;
export async function apiPost<T>(path: string, body?: unknown): Promise<T>;
export type QueryVisibility = {
  partial: boolean;
  message: string;
  queried_ranges: Array<{ log_date: string; start_time: string; end_time: string; status: string }>;
  skipped_dates: Array<{ log_date: string; status: string; reason: string }>;
};
```

- [ ] **Step 2: 实现主导航**

`AppLayout` 使用 `ProLayout`，菜单固定为：

```text
监控大屏
日志检索
增量进度
系统维护
```

- [ ] **Step 3: 实现登录页**

只输入管理员密码。登录成功后调用 `/api/session` 刷新状态。

- [ ] **Step 4: 实现监控大屏**

展示数据健康、导入健康、IP 分布、国家地区分布。默认范围最近 7 天，支持今天、昨天、最近 7 天、最近 30 天、全部切换。

- [ ] **Step 5: 实现日志检索**

使用 `RangePicker showTime`，结果表用 `ProTable`。当响应 `visibility.partial` 为 true 时显示黄色提示条。

- [ ] **Step 6: 实现增量进度**

独立页面，3 秒轮询运行中状态，空闲时 30 秒轮询。默认隐藏 ready 日期，提供“显示已入库日期”开关。

- [ ] **Step 7: 实现系统维护**

Tabs：日志源、IP 库、自动增量、维护操作、登录安全。

- [ ] **Step 8: 构建验证**

Run:

```bash
cd web
npm run build
```

Expected: `web/dist/index.html` 和静态资源生成成功。

- [ ] **Step 9: 提交**

```bash
git add web/.gitignore web/package.json web/package-lock.json web/index.html web/tsconfig.json web/tsconfig.node.json web/vite.config.ts
git add web/src/api.ts web/src/main.tsx web/src/styles.css
git add web/src/layout/AppLayout.tsx
git add web/src/pages/LoginPage.tsx web/src/pages/HealthDashboard.tsx web/src/pages/LogSearchPage.tsx web/src/pages/IncrementalProgressPage.tsx web/src/pages/SystemMaintenancePage.tsx
git commit -m "feat: replace frontend with ant design pro console"
```

---

### Task 10: Go 嵌入前端、端到端路由和旧路径清理

**Files:**
- Create: `static.go`
- Modify: `main.go`
- Modify: `app.go`
- Delete or stop using: `assets/index.html`
- Test: `routes_test.go`

**Interfaces:**
- Consumes: `web/dist`
- Produces: Go 服务 `/` 返回 Ant Design Pro 前端

- [ ] **Step 1: 写失败测试**

`routes_test.go`：

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouterRegistersClickHouseAPIs(t *testing.T) {
	app := NewApp(LoadConfig())
	router := app.Router()

	for _, path := range []string{"/api/health-dashboard", "/api/ingest-progress", "/api/settings"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code == http.StatusNotFound {
			t.Fatalf("%s should be registered", path)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./...`

Expected: FAIL。

- [ ] **Step 3: 实现路由**

`app.go` 提供：

```go
func (a *App) Router() *gin.Engine
func (a *App) Run() error
```

`static.go` 使用：

```go
//go:embed web/dist
var webDist embed.FS
```

所有非 `/api/` 路径回退到 `web/dist/index.html`。

- [ ] **Step 4: 删除 DuckDB 旧代码和旧测试**

移除所有 `go-duckdb`、`database/sql` DuckDB 生产路径、`buildIndex`、`tmp_import.csv`、旧 `/api/stats`、旧 `/api/dashboard`。

- [ ] **Step 5: 运行完整验证**

Run:

```bash
go test ./...
cd web && npm run build
cd ..
go build -o dist/nat-query-service-linux-amd64 .
```

Expected: 全部通过并生成二进制。

- [ ] **Step 6: 提交**

```bash
git add main.go app.go static.go handlers.go routes_test.go go.mod go.sum
git add web/.gitignore web/package.json web/package-lock.json web/index.html web/tsconfig.json web/tsconfig.node.json web/vite.config.ts
git add web/src
git commit -m "feat: serve ant design pro console from go"
```

---

### Task 11: 仓库 CI 切换到 ClickHouse 版验证

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `.github/workflows/release-build.yml`

**Interfaces:**
- Produces: CI 中运行 Go 测试、前端构建、Linux amd64 二进制构建。
- Removes: DuckDB shared library 下载、`duckdb_use_lib` build tag、CGO DuckDB 链接变量。

- [ ] **Step 1: 写 CI 内容检查测试**

新增或更新 `service_test.go`：

```go
func TestCIWorkflowDoesNotUseDuckDB(t *testing.T) {
	content, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("read ci workflow: %v", err)
	}
	text := string(content)
	for _, forbidden := range []string{"duckdb", "duckdb_use_lib", "libduckdb"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("ci workflow must not use %s after ClickHouse replacement", forbidden)
		}
	}
	for _, required := range []string{"go test ./...", "npm run build", "go build"} {
		if !strings.Contains(text, required) {
			t.Fatalf("ci workflow missing %q", required)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./...`

Expected: FAIL，因为当前 CI 仍包含 DuckDB。

- [ ] **Step 3: 更新 CI**

`.github/workflows/ci.yml` 至少包含：

```yaml
name: CI

on:
  push:
    branches: [main, v3-dev]
  pull_request:
    branches: [main, v3-dev]
  workflow_dispatch:

jobs:
  test-and-build:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: npm
          cache-dependency-path: web/package-lock.json
      - run: go test ./...
      - working-directory: web
        run: npm ci
      - working-directory: web
        run: npm run build
      - run: go build -trimpath -ldflags "-s -w" -o dist/nat-query-service-linux-amd64 .
      - uses: actions/upload-artifact@v4
        with:
          name: linux-amd64
          path: dist/nat-query-service-linux-amd64
```

同步更新 `release-build.yml`，不再下载 DuckDB，不再拷贝 `libduckdb.so`。

- [ ] **Step 4: 运行本地验证**

Run:

```bash
go test ./...
cd web && npm run build
cd ..
go build -o dist/nat-query-service-linux-amd64 .
```

Expected: 全部通过。

- [ ] **Step 5: 提交**

```bash
git add .github/workflows/ci.yml .github/workflows/release-build.yml service_test.go
git commit -m "ci: validate clickhouse replacement build"
```

---

### Task 12: 远端部署与真实数据验收

**Files:**
- Modify: `nat-query-service.service`
- Modify: `release-notes/` 或新增发布说明

**Interfaces:**
- Produces: `dist/nat-query-service-linux-amd64`
- Produces: systemd 服务运行 ClickHouse 版

- [ ] **Step 1: 更新 systemd 环境变量**

`nat-query-service.service` 包含：

```ini
Environment=LOG_DIR=/data/sangfor_fw_log
Environment=LOG_TAG=深信服 NAT
Environment=CLICKHOUSE_ADDR=127.0.0.1:9000
Environment=CLICKHOUSE_DATABASE=default
Environment=CUSTOM_IP_MAP=/opt/nat-query/custom_ip_map.csv
Environment=GEOIP_DB=/data/index/GeoLite2-City.mmdb
Environment=PORT=8080
```

- [ ] **Step 2: 构建 Linux 二进制**

如果本机没有 Go，在 142 服务器构建：

```bash
go test ./...
CGO_ENABLED=0 go build -o /tmp/nat-query-service .
```

拉回：

```powershell
scp root@192.168.0.142:/tmp/nat-query-service D:\项目工程\fwlog-v2\fwlog\dist\nat-query-service-linux-amd64
```

- [ ] **Step 3: 部署并重启**

```bash
install -m 0755 /tmp/nat-query-service /opt/nat-query/nat-query-service
systemctl daemon-reload
systemctl restart nat-query-service
systemctl is-active nat-query-service
```

- [ ] **Step 4: API 验收**

登录后验证：

```bash
curl -s http://127.0.0.1:8080/api/session
curl -s http://127.0.0.1:8080/api/settings
curl -s http://127.0.0.1:8080/api/ingest-progress
curl -s http://127.0.0.1:8080/api/health-dashboard
```

- [ ] **Step 5: Web 验收**

浏览器访问：

```text
http://192.168.0.142:8080
```

验收：

- 登录页只要管理员密码。
- 主导航只有监控大屏、日志检索、增量进度、系统维护。
- 增量进度显示当前源、日期、文件、进度和错误。
- 系统维护能看到日志源、IP 库、自动增量、维护操作、登录安全。
- 日志检索使用日期时间范围。
- 跨未入库日期查询显示部分查询提示。

- [ ] **Step 6: 提交发布说明**

```bash
git add nat-query-service.service release-notes
git commit -m "chore: package clickhouse replacement release"
```

`dist/nat-query-service-linux-amd64` 是本地构建产物或 CI artifact，不提交到源码仓库。发布时通过 GitHub Actions artifact、scp 或运维制品目录分发。

---

## 自审结果

- 规格覆盖：计划覆盖 ClickHouse DDL、状态存储、流式导入、查询 visibility、健康大屏、增量进度、系统维护、Ant Design Pro 前端、旧 DuckDB 清理、CI 验证和远端部署。
- 范围风险：这是一次完全替换，建议按任务顺序逐个提交，不要跨任务混改。
- 已知前置：当前工作区存在之前中断留下的 `main.go`、`query_filter_test.go` 半截 DuckDB 改动。执行 Task 1 时应以 ClickHouse 替换方向重写入口，旧半截改动不保留为生产逻辑。
- 测试策略：每个任务都有独立的失败测试和通过标准；真实 ClickHouse 连接测试可在部署阶段执行，单元测试优先验证 SQL、状态机和 API 契约。
