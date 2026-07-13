# V2 日志趋势和 RSyslog 接收 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 V2 测试版的趋势图、维护页、中文文案，并加入 UDP RSyslog 接收落盘能力。

**Architecture:** 后端扩展 dashboard 趋势数据契约为按天、按设备维度；新增轻量 UDP RSyslog receiver，把接收日志写入每个 source 的 spool 目录，导入仍复用现有文件导入链路。前端维护页改成纵向方案 C，并统一中文文案。

**Tech Stack:** Go 1.x、ClickHouse、React、Ant Design、Vite、systemd、GitHub Actions 打包。

## Global Constraints

- 所有用户可见文案必须使用简体中文，禁止维护页出现 `Source`、`All enabled sources`、`Manual`、`Auto upgrade`、`自动升级`。
- RSyslog 接收器默认端口为 `5514`，必须支持自定义端口。
- 第一版 RSyslog 只实现 UDP；配置模型保留协议字段，但页面不承诺 TCP 已可用。
- 接收器只负责落盘到 `/data/fwlog/received/<source_id>/YYYY-MM-DD.log`，入库继续走现有 importer。
- 不引入外部 rsyslog 进程管理，不直接写 ClickHouse。

---

### Task 1: 后端趋势数据按天和设备返回

**Files:**
- Modify: `internal/model/types.go`
- Modify: `internal/dashboard/service.go`
- Modify: `internal/storage/clickhouse/store.go`
- Modify: `internal/storage/clickhouse/store_test.go`
- Modify: `internal/dashboard/service_test.go`

**Interfaces:**
- Produces: `model.LogTrendPoint { Date, SourceID, LogTag string; Value uint64 }`
- Produces: `DashboardMetrics.LogTrend []LogTrendPoint`
- Produces: API JSON `log_trend[].date/source_id/log_tag/value`

- [ ] 写测试：ClickHouse 趋势 SQL 必须 `GROUP BY log_date, source_id, log_tag`。
- [ ] 写测试：dashboard response 复制 `LogTrendPoint`。
- [ ] 实现 `LogTrendPoint` 类型并替换 `LogTrend []DistributionItem`。
- [ ] 修改 `ClickHouseLogTrendSQL` 和 `dailyLogTrend`。
- [ ] 跑 `go test ./internal/storage/clickhouse ./internal/dashboard`。
- [ ] 提交：`feat: return source scoped log trend`

### Task 2: 前端趋势图使用日期和设备筛选

**Files:**
- Modify: `web/src/pages/HealthDashboard.tsx`
- Add/Modify: `web/tests/dashboardTrendTicks.test.ts`

**Interfaces:**
- Consumes: `log_trend: Array<{ date: string; source_id: string; log_tag: string; value: number }>`
- Produces: 设备筛选选项 `全部设备` 和每个设备。

- [ ] 写测试：趋势图不能使用固定小时标签。
- [ ] 写测试：0 数据只显示 0 刻度，不显示重复 1。
- [ ] 修改前端类型。
- [ ] 实现日期序列补齐、全部设备聚合和设备筛选。
- [ ] 跑 `npm test` 和 `npm run build`。
- [ ] 提交：`feat: show daily source log trend`

### Task 3: 维护页方案 C 和统一文案

**Files:**
- Modify: `web/src/pages/SystemMaintenancePage.tsx`
- Modify: `web/src/styles.css`
- Modify: `web/tests/maintenanceLayout.test.ts`

**Interfaces:**
- Produces: 纵向卡片 `.maintenance-ops-card`。
- Produces: 中文文案 `日志源`、`全部启用日志源`、`执行入库`、`手动升级`。

- [ ] 写测试：CSS 不再使用左右跨行布局。
- [ ] 写测试：维护页不含禁止文案。
- [ ] 改 JSX 文案和按钮。
- [ ] 改 CSS 为纵向布局。
- [ ] 跑 `npm test` 和 `npm run build`。
- [ ] 提交：`fix: unify maintenance page layout and copy`

### Task 4: 日志源乱码和 RSyslog 配置模型

**Files:**
- Modify: `web/src/api.ts`
- Modify: `web/src/pages/SystemMaintenancePage.tsx`
- Modify: `internal/model/types.go`
- Modify: `internal/server/import_controller.go`
- Modify: `internal/server/settings_controller.go`
- Modify: tests under `internal/server` and `web/tests`

**Interfaces:**
- Produces: `source_type=file|rsyslog`
- Produces: `listen_protocol=udp`
- Produces: `listen_host=0.0.0.0`
- Produces: `listen_port=5514`
- Produces: `spool_dir=/data/fwlog/received/<source_id>`

- [ ] 写测试：乱码默认值清洗为 `深信服 NAT`。
- [ ] 写测试：RSyslog 源保存后会补齐默认端口和 spool 目录。
- [ ] 修改前端 mock 乱码。
- [ ] 修改日志源表单，根据 source type 展示文件目录或接收端口。
- [ ] 修改后端 log source 解析和规范化。
- [ ] 跑 `go test ./internal/server ./internal/config` 和 `npm test`。
- [ ] 提交：`feat: add rsyslog log source settings`

### Task 5: UDP RSyslog 接收器落盘

**Files:**
- Create: `internal/receiver/rsyslog.go`
- Create: `internal/receiver/rsyslog_test.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/settings_controller.go`
- Modify: `internal/server/router.go` 或现有 settings/status handler
- Modify: `fwlog.service`
- Modify: `packaging/systemd/fwlog.service`

**Interfaces:**
- Produces: `receiver.Manager` with `ApplySources([]model.LogSource)` and `Status()`.
- Consumes: enabled `source_type=rsyslog` sources.
- Produces: files under `/data/fwlog/received/<source_id>/YYYY-MM-DD.log`.

- [ ] 写 UDP 接收器测试：发送 syslog 后落盘。
- [ ] 写端口冲突测试：源状态为错误但 manager 不 panic。
- [ ] 实现 receiver manager。
- [ ] App 启动和设置保存后应用 receiver 配置。
- [ ] systemd service 增加可选 UDP 5514 说明，不强依赖 root 低端口。
- [ ] 跑 `go test ./internal/receiver ./internal/server ./...`。
- [ ] 提交：`feat: receive rsyslog messages to spool`

### Task 6: 打包、发布测试版并部署 142

**Files:**
- Modify: `release-notes/v2.0.0.2.md`

**Interfaces:**
- Produces: prerelease `v2.0.0.2`
- Deploy target: `root@192.168.0.142`

- [ ] 新增 release notes。
- [ ] 跑 `go test ./...`、`npm test`、`npm run build`。
- [ ] 推送 main 和 tag `v2.0.0.2`。
- [ ] 等 Release Build 成功。
- [ ] 在 142 下载 upgrade DEB 安装。
- [ ] 验证页面、`/api/version`、UDP 5514 落盘。
- [ ] 提交或记录部署结果。
