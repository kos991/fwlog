# 数据概览预聚合实施计划

> **供自动化执行者使用：** 必须使用 `executing-plans` 技能逐项实施；每个步骤使用复选框跟踪。

**目标：** 将数据概览的统计和排行从 30 天原始日志扫描迁移到按日预聚合表，并以缓存、请求合并和失败降级控制 CPU 峰值。

**架构：** ClickHouse 为 `nat_logs` 增加两个 `SummingMergeTree` 聚合表和四个物化视图，应用只从聚合表读取概览。服务端拆分 summary/rankings，rankings 使用 5 分钟缓存、同键请求合并和过期结果降级；前端按顺序加载两个接口。

**技术栈：** Go 1.21、ClickHouse 25.x、React 18、TypeScript 5.6、Node test runner。

## 全局约束

- `nat_logs` 原始表结构和日志查询接口不变。
- ClickHouse 冷查询 `max_threads=2`，GeoIP 聚合最多 2 个 worker。
- 排行缓存有效期 5 分钟；有旧值时刷新失败返回 stale 值，禁止回退扫描 `nat_logs`。
- 日期重建同时清理 `nat_logs`、`dashboard_daily_totals`、`dashboard_daily_ip_counts` 的同一来源日期分区。
- 概览 P95 小于 1 秒，缓存排行小于 200 毫秒，页面加载 CPU 峰值小于 30%。
- 保留 `/api/health-dashboard` 一个补丁版本作为兼容入口。

---

### 任务 1：聚合表和物化视图 DDL

**文件：**
- 新建：`internal/storage/clickhouse/dashboard_aggregates.go`
- 修改：`internal/storage/clickhouse/store.go`
- 测试：`internal/storage/clickhouse/dashboard_aggregates_test.go`

**接口：**
- 产出：`DashboardAggregateDDL() []string`
- 产出：`DashboardBackfillSQL(totalsTable, ipCountsTable string, date time.Time) []string`

- [ ] **步骤 1：编写失败测试**

```go
func TestDashboardAggregateDDLDefinesTablesAndViews(t *testing.T) {
    sql := strings.Join(DashboardAggregateDDL(), "\n")
    for _, want := range []string{
        "dashboard_daily_totals", "dashboard_daily_ip_counts",
        "ENGINE = SummingMergeTree", "dashboard_daily_totals_mv",
        "dashboard_daily_src_ip_mv", "dashboard_daily_dst_ip_mv",
        "dashboard_daily_dst_subnet_mv", "max_threads = 2",
    } {
        if !strings.Contains(sql, want) { t.Fatalf("DDL missing %q", want) }
    }
}
```

- [ ] **步骤 2：运行失败测试**

执行：`go test ./internal/storage/clickhouse -run TestDashboardAggregateDDLDefinesTablesAndViews -count=1`

预期：因 `DashboardAggregateDDL` 未定义而编译失败。

- [ ] **步骤 3：实现聚合 DDL**

`dashboard_daily_totals` 使用 `(source_id, log_date)` 分区、`(log_date, source_id, log_tag)` 排序；`dashboard_daily_ip_counts` 使用相同分区并按 `(dimension, log_date, source_id, address, log_tag)` 排序。四个物化视图分别按总量、`src_ip`、`dst_ip`、`cutIPv6(dst_ip, 8, 1)` 汇总，所有目标查询使用 `sum(rows)`。

- [ ] **步骤 4：让建表流程创建聚合对象**

```go
func ClickHouseDDL() []string {
    statements := []string{/* 现有五张表 */}
    return append(statements, DashboardAggregateDDL()...)
}
```

- [ ] **步骤 5：运行测试**

执行：`go test ./internal/storage/clickhouse -count=1`

预期：全部通过。

### 任务 2：聚合概览和排行查询

**文件：**
- 修改：`internal/storage/clickhouse/dashboard_aggregates.go`
- 修改：`internal/storage/clickhouse/store.go`
- 测试：`internal/storage/clickhouse/dashboard_aggregates_test.go`

**接口：**
- 产出：`DashboardSummaryMetrics(ctx context.Context) (DashboardMetrics, error)`
- 产出：`DashboardRankingMetrics(ctx context.Context, since time.Time, sourceID string) (DashboardMetrics, error)`

- [ ] **步骤 1：编写 SQL 约束测试**

```go
func TestDashboardQueriesNeverReadNatLogs(t *testing.T) {
    for _, sql := range []string{DashboardSummarySQL(), dashboardRankingSQL("src_ip", time.Now(), "")} {
        if strings.Contains(sql, "FROM nat_logs") { t.Fatalf("dashboard query scans nat_logs: %s", sql) }
        if !strings.Contains(sql, "sum(rows)") { t.Fatalf("dashboard query must sum rows: %s", sql) }
    }
}
```

- [ ] **步骤 2：运行失败测试**

执行：`go test ./internal/storage/clickhouse -run 'TestDashboardQueries' -count=1`

预期：查询函数未定义。

- [ ] **步骤 3：实现查询**

summary 从 `dashboard_daily_totals` 读取今日、昨日、14 日趋势和总行数；rankings 从 `dashboard_daily_ip_counts` 读取源 IP、目标 IP、目标子网，并从 totals 读取日志标签排行。排行 SQL 均追加 `SETTINGS max_threads = 2`，IP 和标签 Top 10 限制在 ClickHouse 内完成，目标子网保留全部结果供 GeoIP 聚合。

- [ ] **步骤 4：保留系统健康轻查询**

将数据库版本、活动查询、活动 merge、part 数和磁盘占用留在 summary；`DatabaseHealth.TotalRows` 改用 `sum(rows)`，不再执行 `SELECT count() FROM nat_logs`。

- [ ] **步骤 5：运行测试**

执行：`go test ./internal/storage/clickhouse ./internal/dashboard -count=1`

预期：全部通过。

### 任务 3：日期重建一致性

**文件：**
- 修改：`internal/importer/importer.go`
- 修改：`internal/importer/importer_test.go`

**接口：**
- 产出：`dropLogSourceDatePartitionsSQL(sourceID string, date time.Time) []string`

- [ ] **步骤 1：把单分区断言改成三分区断言**

```go
want := []string{
    "ALTER TABLE nat_logs DROP PARTITION ('fw-a', '2026-07-02')",
    "ALTER TABLE dashboard_daily_totals DROP PARTITION ('fw-a', '2026-07-02')",
    "ALTER TABLE dashboard_daily_ip_counts DROP PARTITION ('fw-a', '2026-07-02')",
}
```

- [ ] **步骤 2：运行失败测试**

执行：`go test ./internal/importer -run TestImporterImportDate -count=1`

预期：目前只执行一个 `ALTER TABLE`，测试失败。

- [ ] **步骤 3：实现顺序清理**

```go
for _, statement := range dropLogSourceDatePartitionsSQL(source.SourceID, date) {
    if err := writer.Exec(ctx, statement); err != nil {
        return fmt.Errorf("clear rebuild partition: %w", err)
    }
}
```

任一清理失败立即停止，三项全部成功后才写 importing 状态并重新插入原始日志。

- [ ] **步骤 4：运行测试**

执行：`go test ./internal/importer ./internal/server -count=1`

预期：全部通过。

### 任务 4：拆分接口并增加排行缓存

**文件：**
- 新建：`internal/server/dashboard_cache.go`
- 新建：`internal/server/dashboard_cache_test.go`
- 修改：`internal/server/server.go`
- 修改：`internal/server/dashboard_adapter.go`
- 修改：`internal/server/handlers/dashboard_adapter.go`
- 修改：`internal/server/router.go`
- 修改：`internal/server/handlers_test.go`
- 修改：`internal/server/routes_test.go`

**接口：**
- 产出：`DashboardSummary(*http.Request) (HealthDashboardResponse, error)`
- 产出：`DashboardRankings(*http.Request) (HealthDashboardResponse, error)`
- 产出：`rankingCache.Get(ctx, key, loader) (DashboardMetrics, CacheState, error)`

- [ ] **步骤 1：编写缓存测试**

覆盖有效缓存不重复加载、两个并发同键只加载一次、过期后刷新、刷新失败返回 stale、首次加载失败返回错误。使用可注入 `now func() time.Time`，不使用真实等待。

- [ ] **步骤 2：运行失败测试**

执行：`go test ./internal/server -run 'TestRankingCache|TestDashboardRoutes' -count=1`

预期：缓存和新路由未定义。

- [ ] **步骤 3：实现缓存和请求合并**

```go
type rankingCacheEntry struct {
    value DashboardMetrics
    loadedAt time.Time
    loading chan struct{}
    err error
}

type rankingCache struct {
    mu sync.Mutex
    ttl time.Duration
    now func() time.Time
    entries map[string]*rankingCacheEntry
}
```

等待同键加载时同时监听 `ctx.Done()`；加载失败且已有旧值时返回旧值及 `stale` 状态。

- [ ] **步骤 4：拆分服务方法和路由**

summary 读取日期状态、`DashboardSummaryMetrics`、自动扫描和系统健康；rankings 只读取缓存后的 `DashboardRankingMetrics` 并执行 GeoIP 聚合。路由增加 `/api/health-dashboard/summary` 与 `/api/health-dashboard/rankings`，旧路由组合两者结果。

- [ ] **步骤 5：限制 GeoIP worker**

```go
workers := min(2, len(destinations))
```

- [ ] **步骤 6：运行测试**

执行：`go test ./internal/server ./internal/dashboard -count=1`

预期：全部通过，`-race` 下无数据竞争。

### 任务 5：前端顺序加载和取消旧请求

**文件：**
- 修改：`web/src/api.ts`
- 修改：`web/src/pages/HealthDashboard.tsx`
- 新建：`web/tests/dashboardLoading.test.ts`

**接口：**
- 产出：`apiGet<T>(path: string, options?: RequestInit): Promise<T>` 支持 `AbortSignal`

- [ ] **步骤 1：编写源码约束测试**

```ts
test('dashboard loads summary before rankings and cancels stale requests', () => {
  assert.match(source, /health-dashboard\/summary/);
  assert.match(source, /health-dashboard\/rankings/);
  assert.match(source, /AbortController/);
  assert.doesNotMatch(source, /include_distributions/);
});
```

- [ ] **步骤 2：运行失败测试**

执行：`npm test -- --test-name-pattern="dashboard loads"`

工作目录：`web`

预期：仍调用旧接口，测试失败。

- [ ] **步骤 3：实现前端加载**

首次进入先 await summary 并展示，再请求 rankings；summary 空闲 30 秒、入库中 5 秒刷新，rankings 5 分钟刷新。筛选变化和组件卸载时 abort 旧请求，`AbortError` 不弹错误提示；排行响应只合并分布字段，不能覆盖更新更晚的 summary 状态。

- [ ] **步骤 4：运行前端测试和构建**

执行：`npm test && npm run build`

工作目录：`web`

预期：测试和 TypeScript 构建全部通过。

### 任务 6：历史 staging 回填工具

**文件：**
- 新建：`scripts/backfill-dashboard-aggregates.sh`
- 新建：`scripts/test-backfill-dashboard-aggregates.sh`

**接口：**
- 输入：`CLICKHOUSE_CLIENT`、`CLICKHOUSE_DATABASE`、可选 `START_DATE`、`END_DATE`
- 产出：staging 表、逐日回填记录、总量对账结果

- [ ] **步骤 1：编写脚本静态测试**

测试必须断言脚本包含 staging 表、`max_threads=1`、按日循环、源日期总量对账、失败立即退出和原子 `RENAME TABLE`。

- [ ] **步骤 2：运行失败测试**

执行：`bash scripts/test-backfill-dashboard-aggregates.sh`

预期：回填脚本不存在，测试失败。

- [ ] **步骤 3：实现可恢复回填**

脚本创建 `dashboard_daily_totals_staging` 和 `dashboard_daily_ip_counts_staging`，按 ClickHouse 中存在的日期从旧到新逐日 `INSERT SELECT`，每次使用 `SETTINGS max_threads=1`。每一天对比 totals 的 `sum(rows)` 与 `nat_logs count()`，成功日期写入本地 checkpoint。

- [ ] **步骤 4：实现切换保护**

最终切换前停止 fwlog，重算回填开始后发生变化的日期，执行总量和 Top 10 抽样对账；随后原子重命名 staging 表、创建物化视图并启动服务。任一对账失败不得重命名。

- [ ] **步骤 5：运行脚本测试**

执行：`bash scripts/test-backfill-dashboard-aggregates.sh`

预期：全部断言通过。

### 任务 7：全量验证、生产部署和监控

**文件：**
- 修改：`internal/server/web/dist/**`（由前端构建生成）
- 新建：`scripts/monitor-dashboard-cpu.sh`

- [ ] **步骤 1：全量本地验证**

执行：`go test ./... -count=1`

执行：`go test -race ./internal/dashboard ./internal/server -count=1`

执行：`npm test && npm run build`（工作目录 `web`）

预期：全部通过。

- [ ] **步骤 2：构建 Linux amd64 二进制**

执行：`$env:GOOS='linux'; $env:GOARCH='amd64'; go build -o dist/fwlog-linux-amd64 ./cmd/fwlog`

预期：生成可执行文件且命令退出码为 0。

- [ ] **步骤 3：生产备份和 staging 回填**

备份当前二进制、systemd unit 和配置到带时间戳目录；运行回填脚本并保存逐日行数证据。回填期间每 5 秒检查 fwlog、ClickHouse CPU 和接口延迟，出现持续 CPU 大于 80% 时暂停回填。

- [ ] **步骤 4：短暂停服切换与部署**

停止 fwlog，完成最终增量对账和表切换，部署新二进制与前端静态资源，执行 `systemctl daemon-reload && systemctl start fwlog`。切换窗口目标小于 1 分钟。

- [ ] **步骤 5：数据正确性验收**

至少抽取 3 个日期核对原始表与 totals；核对 30 天 Top 10 源 IP、目标 IP、日志标签和国家/地区；专门核对 `2026-07-16` 的 `3,302,145` 行。

- [ ] **步骤 6：10 分钟 CPU 和延迟复测**

按 1 秒粒度采集系统、fwlog、ClickHouse CPU，同时连续访问 summary 和 rankings，计算 P50/P95/P99、CPU 峰值和超过 80% 的秒数。验收目标：summary P95 < 1 秒、缓存 rankings < 200ms、页面加载 CPU 峰值 < 30%。

- [ ] **步骤 7：回滚边界**

应用异常时恢复旧二进制；聚合数据异常时先删除四个物化视图，再恢复旧二进制。禁止删除或改写 `nat_logs`，保留 staging/正式聚合表供故障分析。
