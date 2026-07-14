# 后台文案与入库终态修复实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复入库完成后仍显示 100%“入库中”的状态竞争，并统一后台所有用户可见文案。

**Architecture:** 新建的 ClickHouse 入库状态表使用微秒级 `DateTime64(6)`；现有表保持原版本列类型，通过终态跨秒时间、确定性状态排序和启动补写修复同秒竞争，避免修改 ReplacingMergeTree 版本列导致升级失败。前端新增集中状态文案函数，各页面使用同一套术语。页面布局、API 字段和业务行为保持不变。

**Tech Stack:** Go、ClickHouse、React、TypeScript、Ant Design、Node Test Runner。

---

### Task 1: 修复入库状态同秒竞争

**Files:**
- Modify: `internal/storage/clickhouse/store.go`
- Modify: `internal/storage/clickhouse/ingest_state_store.go`
- Test: `internal/storage/clickhouse/ingest_state_store_test.go`

- [ ] **Step 1: 写失败测试**

断言新建的 `ingest_dates`、`ingest_files` 使用 `DateTime64(6) DEFAULT now64(6)`；状态查询在时间相同时优先终态；启动迁移会把已完成文件处理但仍显示 `importing` 的历史日期补写为 `ready`，且不修改 ReplacingMergeTree 版本列类型。

- [ ] **Step 2: 运行测试确认红灯**

Run: `go test ./internal/storage/clickhouse -run "TestIngestStateTimestampsUseMicroseconds|TestIngestStateTimestampMigration" -count=1`

Expected: FAIL，当前 DDL 仍为秒级 `DateTime`，查询缺少确定性排序，且没有历史状态修复。

- [ ] **Step 3: 实现兼容新旧状态表的终态修复**

把两张状态表的版本列改为：

```sql
updated_at DateTime64(6) DEFAULT now64(6)
```

现有表不执行 `ALTER ... MODIFY COLUMN updated_at`，因为 ClickHouse 禁止原地修改 ReplacingMergeTree 版本列类型。点查询和列表查询改为按“更新时间 + 状态优先级”确定最新记录；导入器保证 `ready/failed` 终态晚于同轮非终态的秒级时间；`EnsureTables` 启动时补写已完成文件处理但仍停留在 `importing` 的日期终态。

- [ ] **Step 4: 运行存储层测试**

Run: `go test ./internal/storage/clickhouse -count=1`

Expected: PASS。

### Task 2: 统一入库状态与进度文案

**Files:**
- Create: `web/src/uiCopy.ts`
- Modify: `web/src/ingestPresentation.ts`
- Modify: `web/src/pages/HealthDashboard.tsx`
- Modify: `web/src/pages/IncrementalProgressPage.tsx`
- Modify: `web/src/pages/LogSearchPage.tsx`
- Test: `web/tests/ingestPresentation.test.ts`
- Test: `web/tests/uiCopy.test.ts`

- [ ] **Step 1: 写状态词和终态展示失败测试**

覆盖统一映射：`idle=暂无任务`、`pending=等待处理`、`scanning=正在扫描`、`importing=正在入库`、`ready/succeeded=已完成`、`failed=处理失败`。覆盖 `importing + 100%` 显示“收尾中”，`ready + 100%` 显示“已完成”。

- [ ] **Step 2: 运行前端定向测试确认红灯**

Run: `npm.cmd test -- --test-name-pattern="统一入库状态|入库收尾|已完成入库"`

Expected: FAIL，集中映射不存在且 100% 仍显示数字。

- [ ] **Step 3: 实现集中状态文案并替换页面重复映射**

新增：

```ts
export function ingestStatusText(status?: string) {
  return INGEST_STATUS_TEXT[status || 'idle'] || status || '暂无任务';
}
```

页面统一调用该函数；`buildIngestProgressView` 根据状态先判断终态和收尾态，再生成详情文字。

- [ ] **Step 4: 运行相关前端测试**

Run: `npm.cmd test -- --test-name-pattern="统一入库状态|入库收尾|已完成入库"`

Expected: PASS。

### Task 3: 按任务语义整理所有后台页面文案

**Files:**
- Modify: `web/src/pages/HealthDashboard.tsx`
- Modify: `web/src/pages/IncrementalProgressPage.tsx`
- Modify: `web/src/pages/LogSearchPage.tsx`
- Modify: `web/src/pages/SystemMaintenancePage.tsx`
- Modify: `web/src/upgradePresentation.ts`
- Test: `web/tests/maintenanceLayout.test.ts`
- Test: `web/tests/upgradePresentation.test.ts`
- Test: `web/tests/uiCopy.test.ts`

- [ ] **Step 1: 写页面文案契约测试**

断言以下关键改写存在，并禁止旧歧义词继续作为字段主标签：

```text
可查询日期范围
今日新增日志
接收文件保存目录
允许的发送端地址（可选）
接受任意发送端
导入新增日志
重新导入所选日期
发布文件
运行组件版本
```

- [ ] **Step 2: 运行页面文案测试确认红灯**

Run: `npm.cmd test -- --test-name-pattern="后台文案|维护页文案|升级文案"`

Expected: FAIL，当前页面仍使用旧术语。

- [ ] **Step 3: 修改页面可见文案**

执行 Issue #8 中确认的字段映射；不改组件结构、请求参数和业务条件。操作反馈采用“结果 + 原因/下一步”的格式，技术名称作为次级信息保留。

- [ ] **Step 4: 运行全部前端测试和生产构建**

Run: `npm.cmd test`

Run: `npm.cmd run build`

Expected: 全部 PASS，Vite 仅允许既有大 chunk 警告。

### Task 4: 整理页面直接展示的 API 错误

**Files:**
- Modify: `internal/server/import_controller.go`
- Modify: `internal/server/upgrade_service.go`
- Test: `internal/server/import_controller_test.go`
- Test: `internal/server/upgrade_service_test.go`

- [ ] **Step 1: 写失败测试**

覆盖数据库未连接、日期格式错误、日志源不可用、发布文件缺失和运行组件不兼容的中文可操作提示。

- [ ] **Step 2: 运行定向测试确认红灯**

Run: `go test ./internal/server -run "TestImport.*Message|TestUpgrade.*Message" -count=1`

Expected: FAIL，当前仍返回英文或技术术语。

- [ ] **Step 3: 实现用户可见错误文案**

仅修改响应 `message`，保留 HTTP 状态码和 `error` 机器码。

- [ ] **Step 4: 运行服务端全量测试**

Run: `go test -count=1 ./...`

Expected: PASS。

### Task 5: 发布与部署验证

**Files:**
- Create: `release-notes/v2.0.0.5.md`

- [ ] **Step 1: 全量验证**

Run: `go test -count=1 ./...`

Run: `npm.cmd test`

Run: `npm.cmd run build`

- [ ] **Step 2: 提交、打标签并推送**

Commit: `fix(ui): clarify admin copy and ingest completion state`

Tag: `v2.0.0.5`

- [ ] **Step 3: 验证 Release**

确认 GitHub Actions 成功、Release 为 prerelease、10 个资产完整且 `latest.json.app_version=v2.0.0.5`。

- [ ] **Step 4: 部署验证**

先在 142 安装升级包验证普通升级，再在 244 安装薄 RPM；检查服务、版本、状态表精度和后台关键页面。
