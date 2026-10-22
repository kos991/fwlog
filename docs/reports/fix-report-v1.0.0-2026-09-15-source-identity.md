# 日志来源重命名重复入库修复

## 问题与范围

日志来源标识由“外网”改为 `W-1` 后，旧实现同时把该字段作为界面名称和 ClickHouse 数据身份。自动扫描因此把同一日志目录识别为新来源，重复写入 `W-1` 分区，并留下超时的“正在入库”状态。

本轮只处理来源身份、查询映射、进度映射和测试机误数据，不改写原“外网”数据。

## 改动文件

- [`internal/model/types.go`](../../internal/model/types.go)：为日志来源增加稳定存储身份 `storage_source_id`。
- [`internal/server/import_controller.go`](../../internal/server/import_controller.go)：导入和自动扫描使用稳定存储身份，界面标识只用于展示和任务选择。
- [`internal/server/settings_controller.go`](../../internal/server/settings_controller.go)：旧配置首次改名时，按原来源 ID 或唯一相同目录自动继承存储身份。
- [`internal/server/query_adapter.go`](../../internal/server/query_adapter.go)：查询前将 `W-1` 转为“外网”，返回前再映射为 `W-1`；游标仍保留数据库真实身份。
- [`internal/server/dashboard_adapter.go`](../../internal/server/dashboard_adapter.go)：进度与排行使用稳定存储身份并映射显示名称。
- [`internal/server/access_trend_adapter.go`](../../internal/server/access_trend_adapter.go)：访问趋势按稳定存储身份筛选。
- [`web/src/pages/SystemMaintenancePage.tsx`](../../web/src/pages/SystemMaintenancePage.tsx)：编辑来源时保留后端返回的隐藏存储身份。
- [`internal/server/log_source_settings_test.go`](../../internal/server/log_source_settings_test.go)：覆盖稳定身份保留、进度映射和查询结果映射。
- [`web/tests/maintenanceLayout.test.ts`](../../web/tests/maintenanceLayout.test.ts)：约束前端保留稳定身份字段。

## 关键修复

测试机将可见来源标识保留为 `W-1`，存储身份固定为“外网”。后续自动扫描、状态读取、日志查询、排行和趋势继续使用原“外网”分区，接口返回时展示 `W-1`。

清理前逐日对账确认：`W-1` 共 81 天、180,248,052 行；对应“外网”日期无缺失，且每一天的“外网”行数都不少于 `W-1`。随后按 `(source_id, log_date)` 精确删除四张表的 `W-1` 分区，并清理 `ingest_dates`、`ingest_files` 中仅属于 `W-1` 的状态记录。

## 验收与测试

- `go test -count=1 ./...`：通过。
- `npm.cmd test`：79 项通过。
- `npm.cmd run build`：通过；Vite 仅报告既有的大分块提示。
- 测试机 `/api/settings`：返回 `source_id=W-1`、`storage_source_id=外网`。
- 测试机 `/api/ingest-progress?range=all&include_ready=true`：`2026-07-12` 显示 `ready`，行数为 1,025,181。
- 测试机 `/api/query`：以 `source_id=W-1` 查询 `2026-07-12` 返回总数 1,025,181，记录来源显示为 `W-1`。
- 部署后日志：未出现新的 `import source started`。

## 数据与回滚

- 清理后 `W-1` 在原始表、三张聚合表和两张状态表中的记录均为 0。
- 原“外网”数据保持 328,455,035 行，覆盖日期为 2026-04-23 至 2026-09-13。
- 部署前配置与数据对账保存在测试机 `/data/fwlog/backups/source-identity-before-20260915-132122.json`。
- 旧二进制保存在测试机 `/opt/fwlog/fwlog.rollback-source-identity-20260915-132122`。

## 版本同步

本轮属于现有 `v1.0.0` 测试版本上的缺陷修复，未创建新发布版本，也未修改 release notes、安装包版本或 GitHub Release。

## 风险与备注

稳定存储身份是内部兼容字段，不在来源编辑界面中展示。修改来源名称、设备序列号、显示名称或启用状态不会改变存储身份；旧配置没有该字段时，后端会按原来源 ID 或唯一相同目录自动继承。只有新建来源时才以首次保存的来源标识建立存储身份。

## 结论

重复入库数据已清理，原数据完整保留；`W-1` 已作为显示标识接入原“外网”数据和状态，下一次自动扫描不会因本次重命名再次全量入库。
