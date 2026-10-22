# 访问趋势 mode 正确性修复报告

> 状态：本报告记录的是中间修复方案。当前实现已由 [`fix-report-v2.1.2-2026-09-11-access-trend-preaggregation.md`](./fix-report-v2.1.2-2026-09-11-access-trend-preaggregation.md) 中的“目标网段地域节点”方案取代；请求路径不再读取五元组聚合表，响应也已由 `flows` 调整为 `points`。以下内容仅保留为历史验证记录，不代表当前线上实现。

## 1. 问题与范围

本轮修复访问趋势接口在预聚合后仍保留全局 `LIMIT 500` 导致的 mode 筛选与汇总错误。范围限定为访问趋势后端查询、HTTP 适配层与对应测试；不改前端交互，不改 API 响应结构，不改 `access_trend_daily_flows` 的 DDL、物化视图、回填脚本或安装流程。

相关入口：[`internal/storage/clickhouse/access_trend.go`](../../internal/storage/clickhouse/access_trend.go)、[`internal/server/access_trend_adapter.go`](../../internal/server/access_trend_adapter.go)。

## 2. 方案选择

采用方案 2：不改 schema，去掉会截断正确结果集的 ClickHouse `LIMIT`，由 Go 层基于完整预聚合结果做 mode 分类、汇总和最终 Top 50 截断。

没有采用方案 1 的原因：当前项目的 GeoIP 来源是 Go 侧 `geoip2` mmdb，默认路径来自 [`internal/config/config.go`](../../internal/config/config.go)，安装包把 `GeoLite2-City.mmdb` 放到 `/opt/fwlog/GeoLite2-City.mmdb`，见 [`packaging/build-server-packages.sh`](../../packaging/build-server-packages.sh:196)。仓库内尚无 ClickHouse 侧 `ip_trie` 字典、前缀表、mmdb 导入器、字典刷新脚本或针对已有 `AggregatingMergeTree` 的安全迁移路径。若本轮强行下推，需要新增国家维度、重建物化视图、全量回填、安装/升级脚本和部署验证，变更面明显超出“正确性热修”的可回滚边界。

方案 2 的代价已经写入代码注释：[`internal/storage/clickhouse/access_trend.go`](../../internal/storage/clickhouse/access_trend.go:53)。代价是大窗口或高基数五元组下，ClickHouse 到 Go 的传输行数和 Go 进程内存会增加；收益是本轮不做高风险 DDL/部署变更，同时保证国内/国外筛选和汇总不再被错误截断。

## 3. 改动文件

- [`internal/storage/clickhouse/access_trend.go`](../../internal/storage/clickhouse/access_trend.go)：移除 `accessTrendQueryLimit` 与 SQL `LIMIT 500`，保持从 `access_trend_daily_flows` 聚合表读取并按 `rows DESC, last_seen DESC` 排序；保留 `argMaxMerge(nat_state)`，不改变 NAT 地址/端口同一 tuple 的确定性语义。
- [`internal/storage/clickhouse/access_trend_test.go`](../../internal/storage/clickhouse/access_trend_test.go)：新增 SQL 不得提前 `LIMIT` 的断言，防止回归。
- [`internal/server/access_trend_adapter.go`](../../internal/server/access_trend_adapter.go)：把 `Summary.Total/Domestic/Overseas/Unknown` 的累加移动到 mode 筛选之前；保留响应 `flows` 最多 50 条，由 `accessTrendFlowLimit` 控制。
- [`internal/server/access_trend_adapter_test.go`](../../internal/server/access_trend_adapter_test.go)：新增 `mode=overseas` 时 summary 仍统计完整窗口的回归测试。
- [`docs/reports/fix-report-v2.1.2-2026-09-11-access-trend-mode-correctness.md`](./fix-report-v2.1.2-2026-09-11-access-trend-mode-correctness.md)：本报告。

## 4. 三个缺陷如何消除

1. `mode=overseas` 不再只看“全局 Top500 五元组”。
   - 修复前：SQL 在 [`AccessTrendRows`](../../internal/storage/clickhouse/access_trend.go) 阶段 `LIMIT 500`，Go 层只能在这 500 行内筛 overseas。
   - 修复后：SQL 没有 `LIMIT`，完整窗口内所有预聚合五元组都会进入 Go 层；adapter 在完整集合上先判定 `flowMode`，再按 mode 筛选，最后才用 [`accessTrendFlowLimit = 50`](../../internal/server/access_trend_adapter.go:14) 截断展示列表。

2. `response.Summary.Total/Domestic/Overseas/Unknown` 不再统计被截断或被 mode 过滤后的子集。
   - 修复前：`continue` 发生在 summary 累加之前，`mode=overseas` 时 summary 只统计海外行。
   - 修复后：summary 在 [`internal/server/access_trend_adapter.go`](../../internal/server/access_trend_adapter.go:52) 至 [`internal/server/access_trend_adapter.go`](../../internal/server/access_trend_adapter.go:59) 先基于完整行集累加，然后才执行 mode 过滤。
   - 回归测试见 [`internal/server/access_trend_adapter_test.go`](../../internal/server/access_trend_adapter_test.go:48)，构造 domestic=100、overseas=90、unknown=80，`mode=overseas` 返回的 summary 仍必须是 total=270/domestic=100/overseas=90/unknown=80。

3. 切换到 overseas 时列表不再因为全局 Top500 先截断而凑不满 50 条。
   - 修复前：排名在全局 500 之外的海外目标永远不会进入 Go 层，页面最多只能从全局 Top500 中挑海外行。
   - 修复后：Go 层拿到完整预聚合结果后再按 mode 过滤，`flows` 的 50 条上限只作用于已经符合 mode 的行，见 [`internal/server/access_trend_adapter.go`](../../internal/server/access_trend_adapter.go:61) 至 [`internal/server/access_trend_adapter.go`](../../internal/server/access_trend_adapter.go:65)。

## 5. 实际执行命令与输出

### RED：修复前回归测试失败

命令：

```powershell
go test -count=1 ./internal/storage/clickhouse ./internal/server
```

关键原始输出：

```text
--- FAIL: TestAccessTrendSQLAggregatesFiveTupleAndSeparatesSources (0.01s)
    access_trend_test.go:19: access trend query must not truncate before Go mode filtering: SELECT source_id, src_ip, src_port, dst_ip,
...
        ORDER BY rows DESC, last_seen DESC
        LIMIT 500 SETTINGS max_threads = 1
FAIL
FAIL    fwlog/internal/storage/clickhouse  0.055s
--- FAIL: TestAccessTrendModeFilterKeepsWindowSummary (0.06s)
    access_trend_adapter_test.go:49: summary = model.AccessTrendSummary{Total:0x5a, Domestic:0x0, Overseas:0x5a, Unknown:0x0}, want full-window total/domestic/overseas/unknown counts
FAIL    fwlog/internal/server  9.099s
```

### GREEN：相关包通过

命令：

```powershell
go test -count=1 ./internal/storage/clickhouse ./internal/server
```

原始输出：

```text
ok      fwlog/internal/storage/clickhouse  0.057s
ok      fwlog/internal/server              8.786s
```

### Go 全量测试

命令：

```powershell
go test -count=1 ./...
```

原始输出摘要：

```text
?       fwlog/cmd/fwlog                    [no test files]
ok      fwlog/internal/config              0.048s
ok      fwlog/internal/dashboard           0.083s
ok      fwlog/internal/health              0.042s
ok      fwlog/internal/importer            0.199s
ok      fwlog/internal/ip                  0.049s
?       fwlog/internal/model               [no test files]
ok      fwlog/internal/query               0.077s
ok      fwlog/internal/receiver            0.520s
ok      fwlog/internal/security            0.052s
ok      fwlog/internal/server              10.002s
?       fwlog/internal/server/handlers     [no test files]
?       fwlog/internal/storage             [no test files]
ok      fwlog/internal/storage/clickhouse  0.069s
ok      fwlog/internal/threatintel         0.259s
?       fwlog/internal/upgrade             [no test files]
?       fwlog/internal/version             [no test files]
ok      fwlog/packaging                    0.906s
```

### Go 编译

命令：

```powershell
go build ./...
```

原始输出：空输出，退出码 0。

### Web 测试

命令：

```powershell
cd web
npm.cmd test
```

原始输出摘要：

```text
> fwlog-v2-web@0.1.0 test
> node --test "tests/*.test.ts"
...
ℹ tests 75
ℹ suites 0
ℹ pass 75
ℹ fail 0
ℹ cancelled 0
ℹ skipped 0
ℹ todo 0
ℹ duration_ms 1509.8568
```

### Web 构建

命令：

```powershell
cd web
npm.cmd run build
```

原始输出摘要：

```text
> fwlog-v2-web@0.1.0 build
> tsc -b && vite build

vite v5.4.21 building for production...
✓ 4720 modules transformed.
...
(!) Some chunks are larger than 500 kB after minification.
✓ built in 12.44s
```

### Race 测试尝试

命令：

```powershell
go test -race -count=1 ./...
```

原始输出：

```text
go: -race requires cgo; enable cgo by setting CGO_ENABLED=1
```

继续尝试：

```powershell
$env:CGO_ENABLED='1'; go test -race -count=1 ./...
```

原始输出：

```text
# runtime/cgo
cgo: C compiler "gcc" not found: exec: "gcc": executable file not found in %PATH%
FAIL    fwlog/cmd/fwlog [build failed]
...
FAIL    fwlog/packaging [build failed]
FAIL
```

结论：本机 Windows 环境没有 GCC，`go test -race -count=1 ./...` 未完成；非 race 的 Go 全量测试、Go 编译、前端测试和前端构建均已通过。

## 6. 未验证部分

- 未部署到 Kylin/Debian/Rocky 或 systemd 环境。
- 未连接真实 ClickHouse 实例执行访问趋势查询，也未用真实生产数据验证大窗口传输量、内存占用和接口耗时。
- 未执行真实 `scripts/backfill-access-trend-aggregates.sh` 回填；本轮没有修改 DDL 或回填脚本。
- 未实测浏览器页面、Header、metrics、trace 或真实接口响应耗时。
- 未验证方案 1 的 ClickHouse 字典/前缀表路径；本轮明确没有实现该方案。
- `go test -race -count=1 ./...` 因本机缺少 CGO/GCC 未完成。

## 7. 版本同步清单

版本保持 `v2.1.2`。本轮是当前版本内的访问趋势正确性修复，未修改 release tag、`release-notes/`、安装包版本号、`latest.json` 或运行时版本文件。

## 8. 回滚方式

如需回滚本轮修复：

1. 恢复 [`internal/storage/clickhouse/access_trend.go`](../../internal/storage/clickhouse/access_trend.go) 中的 `LIMIT 500` 与固定容量读取。
2. 恢复 [`internal/server/access_trend_adapter.go`](../../internal/server/access_trend_adapter.go) 中 summary 在 mode 过滤之后累加的旧逻辑。
3. 移除本轮新增/调整的两个回归测试断言。

回滚后 API 形状仍兼容，但会重新引入本报告描述的三个正确性缺陷；不建议在没有 ClickHouse 下推国家维度的替代方案前回滚。

## 9. 结论

本轮采用方案 2 完成正确性修复：访问趋势查询不再在 ClickHouse 侧提前 `LIMIT 500`，mode 筛选发生在最终 Top 50 截断之前，summary 始终基于完整窗口预聚合行计算。自动化验证除本机缺少 GCC 导致的 race 测试外均已通过；部署、真实回填和真实 ClickHouse 性能仍未验证。
