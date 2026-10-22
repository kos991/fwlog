# 修复报告：导入提速与逻辑错误修复（v1.0.1，2026-10-22）

## 1. 目标与范围

- 提升日志导入速度（解析、批量写入）。
- 清理死代码。
- 排查并修复导入、存储、查询、调度链路中的逻辑错误。

版本号：`v1.0.0` → `v1.0.1`（PATCH：缺陷修复与性能改进，不改接口契约与数据格式）。

## 2. 改动文件

- [internal/importer/log_parser.go](../../internal/importer/log_parser.go)
- [internal/importer/importer.go](../../internal/importer/importer.go)
- [internal/importer/coordinator.go](../../internal/importer/coordinator.go)
- [internal/query/service.go](../../internal/query/service.go)
- [internal/server/query_adapter.go](../../internal/server/query_adapter.go)
- [internal/server/auto_scan_scheduler.go](../../internal/server/auto_scan_scheduler.go)
- [internal/server/log_source_settings.go](../../internal/server/log_source_settings.go)
- [internal/storage/clickhouse/store.go](../../internal/storage/clickhouse/store.go)
- [internal/server/import_controller.go](../../internal/server/import_controller.go)、[internal/server/server.go](../../internal/server/server.go)、[internal/server/helpers.go](../../internal/server/helpers.go)
- [release-notes/v1.0.1.md](../../release-notes/v1.0.1.md)、[docs/architecture/overview.md](../architecture/overview.md)
- 测试：`internal/importer/*_test.go`（新增 [log_parser_bench_test.go](../../internal/importer/log_parser_bench_test.go)、[importer_bench_test.go](../../internal/importer/importer_bench_test.go)）、[internal/query/service_test.go](../../internal/query/service_test.go)、[internal/server/auto_scan_scheduler_test.go](../../internal/server/auto_scan_scheduler_test.go)、[internal/server/log_source_settings_test.go](../../internal/server/log_source_settings_test.go)、[internal/storage/clickhouse/store_test.go](../../internal/storage/clickhouse/store_test.go)

## 3. 关键修复

### 3.1 逻辑错误

| 问题 | 影响 | 修复 |
|---|---|---|
| `natLabelSrcPort` 常量是乱码字节（`源端\uFFFD?`） | 深信服日志 `src_port` 永远为 0。线上查询结果已确认 `"src_port":0` | [log_parser.go:16](../../internal/importer/log_parser.go#L16) 改回 `源端口:`，测试样例同步 |
| `BuildVisibleRanges` 按日期 map 覆盖多源状态 | 同一天多个日志源时，一个源的状态决定所有源是否可查 | 按日期保留全部源状态，范围带 `source_id` 条件（[service.go](../../internal/query/service.go)） |
| 不同日志源可配置相同 `storage_source_id` | 导入、分区删除、状态互相覆盖 | [log_source_settings.go](../../internal/server/log_source_settings.go) 拒绝重复 |
| 偏移分页 `has_more` 恒为 false | 第 2 页以后无法翻页 | 多取 1 行判断（[store.go:150](../../internal/storage/clickhouse/store.go#L150)） |
| 自动扫描失败仍写入 `last_auto_scan_at` | 失败的时段不再重试 | 协调器新增 `Succeeded` 通道（[coordinator.go:19](../../internal/importer/coordinator.go#L19)），仅全部成功才记录 |
| 重建时删除分区失败不写状态 | 部分数据已删但状态仍为 ready，之后同步跳过该日期 | 写入 failed 状态（[importer.go:161](../../internal/importer/importer.go#L161)） |
| `endOfDayTime` 固定加 24h | 夏令时日边界错误 | 按日历日计算 |

### 3.2 导入提速

基准（同机，修改前后）：

| 基准 | 修改前 | 修改后 |
|---|---|---|
| 深信服行解析 | 1444 ns，1 alloc | 617 ns，0 alloc |
| 华为行 | ~2.4 µs，2 alloc | 578 ns，0 alloc |
| H3C 行 | ~2.2 µs，2 alloc | 561 ns，0 alloc |
| FortiGate 行 | ~2.6 µs，4 alloc | 986 ns，0 alloc |
| `AppendBatch` 每行 | 505 ns，100k alloc/批 | 371 ns，419 alloc/批 |

- 时间戳解析用手写匹配替换三个正则，语义与原正则一致（300 万随机行差分对比无差异，`TestParseNATTimestampMatchesFormerRegexpGrammar` 固定边界）。
- FortiGate 解析不再构建 map。
- 批量写入复用参数切片、对非 LowCardinality 列传指针，避免逐值装箱。
- `WORKERS` 生效范围扩大：原来只限制同时导入的日志源数（单源时等于无效），写入槽固定为 1。现在同一来源内按 `WORKERS` 并行导入多个日期，写入槽也等于 `WORKERS`（[import_controller.go `runImportDates`](../../internal/server/import_controller.go)、[server.go:99](../../internal/server/server.go#L99)）。每个日期独占自己的 `(source_id, log_date)` 分区和状态行，日期之间不共享写入；结果与错误仍按日期顺序汇总。`WORKERS=1` 时行为与之前一致。安装包默认仍为 `WORKERS=1`。

### 3.3 死代码

- `deadcode ./...` 与 `deadcode -test ./...` 均无输出。
- 删除未跟踪的残留测试二进制 `internal/importer/tmp/importer.test`（8.6 MB）。
- 修复 [store_test.go](../../internal/storage/clickhouse/store_test.go) 中乱码的失败提示。

## 4. 验证

- `go build ./...`、`go vet ./...`：通过。
- `go test -count=1 ./...`：全部通过。
- `-race` 在本机不可用（CGO 未启用），由 CI 执行。
- `TestRunImportDatesBoundsParallelismAndKeepsDateOrder`：并行峰值等于 `WORKERS`，结果顺序与失败汇总正确。
- Web：`npm ci`、`npm run build`、`npm test`（76 通过）。
- 未在线上主机验证：Netcatty External MCP 在本轮中途被关闭。

## 5. 版本同步

- `release-notes/v1.0.1.md` 新增；release 构建通过 tag `v1.0.1` 注入 `appVersion` 并写入 `/opt/fwlog/VERSION`，`latest.json` 与 RPM/DEB 包版本由 CI 按 tag 生成。
- `README.md` 未写具体版本号，无需修改。

## 6. 风险与备注

- 修复 `src_port` 后，**历史已入库数据的 `src_port` 仍为 0**，需要对历史日期执行重建才能补齐。
- 多源查询现在 SQL 中多出 `source_id = ?` 条件，与分区键一致，不影响性能。
- 新增的存储 ID 重复校验可能拒绝已有的错误配置；线上当前只有一个日志源，不受影响。
- `WORKERS` 调大后，导入时 CPU、内存和 ClickHouse 写入压力随之上升；建议按主机资源逐步调整并观察。

## 7. 结论

7 个逻辑问题已修复并有回归测试；解析速度提升约 2–4 倍并消除逐行分配；未发现可删除的生产死代码。
