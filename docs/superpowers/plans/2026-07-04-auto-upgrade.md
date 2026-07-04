# 自动升级功能 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 增加不发布版本的手动自动升级功能，让管理员可在系统设置页检查 GitHub Release 并触发 Linux 二进制升级。

**Architecture:** 后端新增独立 `upgrade_service.go` 封装 Release 检查、状态机、后台执行和系统命令边界。`App` 持有内存升级状态，路由层要求登录后访问升级 API。前端在系统设置页维护标签内新增升级卡片，调用检查、状态和运行接口。

**Tech Stack:** Go `net/http`、`os/exec`、GitHub Releases API、React、Ant Design。

## Global Constraints

- 使用简体中文回复和用户可见文案。
- 本轮只提交源码，不发布新 tag，不部署 142。
- 升级资产只支持 Linux amd64：`nat-query-service_linux_amd64`。
- 升级接口必须要求登录态。
- 后端新增行为按 TDD 实现，先写失败测试再写生产代码。

---

### Task 1: 后端升级服务与 API

**Files:**
- Create: `upgrade_service.go`
- Modify: `app.go`
- Modify: `handlers.go`
- Test: `upgrade_service_test.go`
- Test: `routes_test.go`

**Interfaces:**
- Produces: `type UpgradeStatus struct`
- Produces: `type UpgradeCheckResponse struct`
- Produces: `func validUpgradeVersion(version string) bool`
- Produces: `func (a *App) upgradeCheckHandler() http.Handler`
- Produces: `func (a *App) upgradeStatusHandler() http.Handler`
- Produces: `func (a *App) upgradeRunHandler() http.Handler`

- [ ] **Step 1: Write failing tests**

Add tests for version validation, release asset validation, unauthenticated API rejection, route registration, and running-state conflict.

- [ ] **Step 2: Run tests to verify failure**

Run: `go test -count=1 ./...`

Expected: FAIL because upgrade types and routes do not exist.

- [ ] **Step 3: Implement minimal backend**

Create `upgrade_service.go`, extend `App`, register the three `/api/upgrade/*` routes, require session authentication, and implement in-memory upgrade state.

- [ ] **Step 4: Run backend tests**

Run: `go test -count=1 ./...`

Expected: PASS.

---

### Task 2: 前端系统设置页升级卡片

**Files:**
- Modify: `web/src/api.ts`
- Modify: `web/src/pages/SystemMaintenancePage.tsx`
- Modify: `web/src/styles.css`

**Interfaces:**
- Consumes: `GET /api/upgrade/check`
- Consumes: `GET /api/upgrade/status`
- Consumes: `POST /api/upgrade/run`

- [ ] **Step 1: Add API types and UI state**

Add `UpgradeStatus` and `UpgradeCheckResponse` types in `web/src/api.ts`, then add local state in `SystemMaintenancePage`.

- [ ] **Step 2: Add maintenance card**

Add a compact upgrade card under the maintenance tab with current version, latest version, state, version input, check button, and upgrade button.

- [ ] **Step 3: Build frontend**

Run: `npm.cmd run build --prefix web`

Expected: PASS.

---

### Task 3: 完整验证

**Files:**
- No additional files.

- [ ] **Step 1: Run full verification**

Run:

```powershell
npm.cmd run build --prefix web
go test -count=1 ./...
```

Expected: both PASS.

- [ ] **Step 2: Commit**

Commit source changes only. Do not create tag, do not publish Release, do not deploy 142.

