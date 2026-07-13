# RSyslog Source Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 RSyslog 日志源改造为列表/弹窗式 CRUD，按真实客户端 IP 共享端口路由，并安全地自动压缩、移动和清理归档。

**Architecture:** `log_sources` 设置 JSON 继续作为唯一配置协议，服务端在持久化和替换运行状态前做完整归一化/校验。`receiver.Manager` 按 `protocol+host+port` 持有共享监听器，用不可变路由快照按 IP/CIDR 匹配日志源。独立 `receiver.Archiver` 完成幂等 gzip/移动/清理，由 server 调度器注入入库已完成日期。

**Tech Stack:** Go 1.21, React 18, TypeScript, Ant Design 5, ClickHouse, Node test runner, Playwright.

---

## File Map

- Modify `internal/model/types.go`: 扩展日志源配置字段。
- Create `internal/receiver/routing.go`: 端点分组、IP/CIDR 解析与最长前缀路由。
- Create `internal/receiver/routing_test.go`: 共享端口和路由优先级测试。
- Modify `internal/receiver/rsyslog.go`: 按端点管理 UDP/TCP 监听器和每源状态。
- Modify `internal/receiver/rsyslog_test.go`: 真实 UDP/TCP 远端匹配、拒绝和重载测试。
- Create `internal/receiver/archive.go`: 关闭日文件压缩、可选移动、幂等恢复和保留期清理。
- Create `internal/receiver/archive_test.go`: gzip 内容、命名、原地/移动、永久保留和 ready 保护测试。
- Modify `internal/server/import_controller.go`: 扩展 payload 归一化。
- Create `internal/server/log_source_settings.go`: 日志源列表校验和设置原子替换。
- Create `internal/server/log_source_settings_test.go`: 非法 IP/路径/重复路由和持久化失败不生效测试。
- Create `internal/server/rsyslog_archive_scheduler.go`: 启动补扫、每分钟归档和 ready 日期加载。
- Create `internal/server/rsyslog_archive_scheduler_test.go`: 调度和 ready 集合测试。
- Modify `internal/server/server.go`, `internal/server/router.go`, `internal/server/settings_controller.go`: 挂载调度器、状态和原子设置流程。
- Modify `web/src/pages/SystemMaintenancePage.tsx`: 列表、弹窗、立即保存、状态和移动摘要。
- Modify `web/src/styles.css`: 日志源列表/弹窗响应式布局。
- Modify `web/tests/maintenanceLayout.test.ts`: 前端结构和文案回归。
- Create `web/tests/logSourceManagement.test.ts`: CRUD 状态转换与 payload 帮助函数测试。

### Task 1: Extend and validate log source settings

**Files:**
- Modify: `internal/model/types.go`
- Modify: `internal/server/import_controller.go`
- Create: `internal/server/log_source_settings.go`
- Create: `internal/server/log_source_settings_test.go`
- Modify: `internal/server/settings_controller.go`

- [ ] **Step 1: Write failing normalization and validation tests**

```go
func TestNormalizeRSyslogSourceUsesArchiveDirectoryAsLogDirectory(t *testing.T) {
	sources := normalizeLogSourcePayloads([]logSourcePayload{{
		SourceID: "fw-a", SourceType: "rsyslog", ClientIP: "192.168.10.20",
		SpoolDir: "/data/fwlog/received/fw-a", ArchiveDir: "/data/fwlog/archive/fw-a",
	}}, false)
	if len(sources) != 1 || sources[0].LogDir != "/data/fwlog/archive/fw-a" || sources[0].ArchiveRetentionDays != 0 {
		t.Fatalf("normalized source = %#v", sources)
	}
}

func TestValidateLogSourcesRejectsDuplicateEndpointRoute(t *testing.T) {
	err := validateLogSources([]model.LogSource{
		{SourceID: "a", SourceType: "rsyslog", ListenProtocol: "udp", ListenHost: "0.0.0.0", ListenPort: 5514, ClientIP: "10.0.0.0/24", SpoolDir: "/data/a"},
		{SourceID: "b", SourceType: "rsyslog", ListenProtocol: "udp", ListenHost: "0.0.0.0", ListenPort: 5514, ClientIP: "10.0.0.0/24", SpoolDir: "/data/b"},
	})
	if err == nil || !strings.Contains(err.Error(), "重复的客户端 IP") {
		t.Fatalf("validateLogSources error = %v", err)
	}
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/server -run 'TestNormalizeRSyslogSourceUsesArchiveDirectoryAsLogDirectory|TestValidateLogSourcesRejectsDuplicateEndpointRoute' -count=1`

Expected: compile failure for missing `ClientIP`, `ArchiveDir`, `ArchiveRetentionDays`, and `validateLogSources`.

- [ ] **Step 3: Add model and payload fields**

```go
type LogSource struct {
	// existing fields
	ClientIP             string `json:"client_ip,omitempty"`
	ArchiveDir           string `json:"archive_dir,omitempty"`
	ArchiveRetentionDays int    `json:"archive_retention_days,omitempty"`
}
```

Add identical JSON fields to `logSourcePayload`. Normalize `ClientIP` with `strings.TrimSpace`, default retention to `0`, and assign `LogDir = ArchiveDir` when non-empty, otherwise `LogDir = SpoolDir` for RSyslog sources.

- [ ] **Step 4: Implement complete validation**

```go
func validateLogSources(sources []model.LogSource) error {
	ids := map[string]struct{}{}
	routes := map[string]map[string]string{}
	for _, source := range sources {
		if !regexp.MustCompile(`^[A-Za-z0-9._-]+$`).MatchString(source.SourceID) {
			return fmt.Errorf("设备 ID %q 只允许字母、数字、点、下划线和连字符", source.SourceID)
		}
		if _, exists := ids[source.SourceID]; exists { return fmt.Errorf("设备 ID %q 重复", source.SourceID) }
		ids[source.SourceID] = struct{}{}
		if source.SourceType != "rsyslog" { continue }
		if source.ListenPort < 1 || source.ListenPort > 65535 { return fmt.Errorf("监听端口必须为 1-65535") }
		if source.ClientIP != "" {
			if net.ParseIP(source.ClientIP) == nil { if _, _, err := net.ParseCIDR(source.ClientIP); err != nil { return fmt.Errorf("客户端 IP/网段 %q 无效", source.ClientIP) } }
		}
		for _, path := range []string{source.SpoolDir, source.ArchiveDir} {
			if path != "" && !filepath.IsAbs(path) { return fmt.Errorf("路径 %q 必须为绝对路径", path) }
		}
		if source.ArchiveRetentionDays < 0 || source.ArchiveRetentionDays > 3650 { return fmt.Errorf("归档保留天数必须为 0-3650") }
		endpoint := strings.Join([]string{source.ListenProtocol, source.ListenHost, strconv.Itoa(source.ListenPort)}, "|")
		if routes[endpoint] == nil { routes[endpoint] = map[string]string{} }
		if previous := routes[endpoint][source.ClientIP]; previous != "" { return fmt.Errorf("重复的客户端 IP %q", source.ClientIP) }
		routes[endpoint][source.ClientIP] = source.SourceID
	}
	return nil
}
```

- [ ] **Step 5: Make settings save atomic**

Add `normalizeSettingsPayload(payload) (map[string]string, error)` that normalizes and validates `log_sources` without mutating `a.settings`. In `settingsHandler`, call `store.SaveSettings` with the candidate values first; only after success merge them under `a.mu`, reload receiver, and return the new settings. Invalid settings return HTTP 400 and storage failure returns HTTP 500 without changing the running receiver.

- [ ] **Step 6: Run server tests and verify GREEN**

Run: `go test ./internal/server -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/model/types.go internal/server/import_controller.go internal/server/log_source_settings.go internal/server/log_source_settings_test.go internal/server/settings_controller.go
git commit -m "feat: validate rsyslog source settings"
```

### Task 2: Build shared endpoint IP routing

**Files:**
- Create: `internal/receiver/routing.go`
- Create: `internal/receiver/routing_test.go`

- [ ] **Step 1: Write failing route tests**

```go
func TestRouteTablePrefersExactIPThenLongestCIDRThenCatchAll(t *testing.T) {
	table, err := buildRouteTable([]model.LogSource{
		{SourceID: "all", ClientIP: "", SpoolDir: "/all"},
		{SourceID: "network", ClientIP: "10.0.0.0/8", SpoolDir: "/network"},
		{SourceID: "site", ClientIP: "10.20.0.0/16", SpoolDir: "/site"},
		{SourceID: "exact", ClientIP: "10.20.30.40", SpoolDir: "/exact"},
	})
	if err != nil { t.Fatal(err) }
	for ip, want := range map[string]string{"10.20.30.40":"exact", "10.20.1.2":"site", "10.9.1.2":"network", "192.0.2.1":"all"} {
		got, ok := table.Match(net.ParseIP(ip))
		if !ok || got.SourceID != want { t.Fatalf("Match(%s) = %#v, %v", ip, got, ok) }
	}
}
```

- [ ] **Step 2: Run test and verify RED**

Run: `go test ./internal/receiver -run TestRouteTable -count=1`

Expected: compile failure for missing `buildRouteTable`.

- [ ] **Step 3: Implement endpoint and immutable route table**

```go
type endpointKey struct { Protocol, Host string; Port int }
type route struct { Source model.LogSource; Network *net.IPNet; Prefix int; Exact bool }
type routeTable struct { exact map[string]route; networks []route; catchAll *route }

func (t routeTable) Match(ip net.IP) (model.LogSource, bool) {
	if ip == nil { return model.LogSource{}, false }
	if matched, ok := t.exact[ip.String()]; ok { return matched.Source, true }
	for _, candidate := range t.networks { if candidate.Network.Contains(ip) { return candidate.Source, true } }
	if t.catchAll != nil { return t.catchAll.Source, true }
	return model.LogSource{}, false
}
```

Sort CIDR routes by prefix descending. Normalize exact IP keys through `net.ParseIP(...).String()` and reject duplicate exact/CIDR/catch-all rules.

- [ ] **Step 4: Run receiver route tests and verify GREEN**

Run: `go test ./internal/receiver -run 'TestRouteTable' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/receiver/routing.go internal/receiver/routing_test.go
git commit -m "feat: route rsyslog clients by ip"
```

### Task 3: Refactor UDP/TCP listeners around shared endpoints

**Files:**
- Modify: `internal/receiver/rsyslog.go`
- Modify: `internal/receiver/rsyslog_test.go`

- [ ] **Step 1: Write failing shared listener and status tests**

Create tests that configure two sources on one UDP endpoint with `127.0.0.1/32` and a nonmatching CIDR, send a real datagram, and assert only the matching spool contains the message. Add the TCP equivalent and a nonmatching test asserting no spool file is created.

```go
status := manager.Status()["local"]
if status.ClientIP != "127.0.0.1/32" || status.LastClientIP != "127.0.0.1" || status.ReceivedMessages != 1 {
	t.Fatalf("status = %#v", status)
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/receiver -run 'TestManagerRoutes|TestManagerRejects' -count=1`

Expected: port conflict or missing status fields.

- [ ] **Step 3: Reconcile endpoint listeners**

Change `Manager.listeners` to `map[endpointKey]*endpointListener`. An endpoint listener owns one UDP `PacketConn` or TCP `Listener`, an `atomic.Value` containing `routeTable`, and TCP connection tracking. `ApplySources([]model.LogSource) error` groups enabled RSyslog sources by endpoint, reuses unchanged endpoints by swapping route snapshots, binds new endpoints before publishing them, closes removed endpoints after successful preparation, and returns binding errors.

- [ ] **Step 4: Route both transports using remote host**

```go
func remoteIP(address net.Addr) net.IP {
	host, _, err := net.SplitHostPort(address.String())
	if err != nil { return nil }
	return net.ParseIP(strings.Trim(host, "[]"))
}
```

UDP passes the address from `ReadFrom`; TCP captures `conn.RemoteAddr()` once and routes every line on that connection. On a match, call `appendMessage(source.SpoolDir, message)` and update per-source status under the manager mutex. On no match, log a rate-limited warning and do not create any spool file.

- [ ] **Step 5: Run receiver tests with race detection**

Run: `go test -race ./internal/receiver -count=1`

Expected: PASS with no race warnings.

- [ ] **Step 6: Commit**

```bash
git add internal/receiver/rsyslog.go internal/receiver/rsyslog_test.go
git commit -m "feat: share rsyslog endpoints across devices"
```

### Task 4: Add idempotent automatic archiving and retention

**Files:**
- Create: `internal/receiver/archive.go`
- Create: `internal/receiver/archive_test.go`
- Modify: `internal/receiver/rsyslog.go`

- [ ] **Step 1: Write failing archive tests**

```go
func TestArchiverCompressesClosedDayInPlaceWithImportableName(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "2026-07-13.log"), []byte("first\nsecond\n"), 0o644)
	archiver := NewArchiver()
	results := archiver.Run([]model.LogSource{{SourceID:"fw-a", SourceType:"rsyslog", SpoolDir:dir}}, nil, time.Date(2026,7,14,1,0,0,0,time.Local))
	want := filepath.Join(dir, "fw-a_2026-07-13.log-20260714.gz")
	assertGzipContent(t, want, "first\nsecond\n")
	if _, ok := importer.ExtractLogDate(filepath.Base(want)); !ok { t.Fatal("archive name is not importable") }
	if len(results) != 1 || results[0].Error != "" { t.Fatalf("results = %#v", results) }
}

func TestArchiverRetentionZeroNeverDeletes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fw-a_2026-01-01.log-20260102.gz")
	writeTestGzip(t, path, "old\n")
	NewArchiver().Run([]model.LogSource{{SourceID:"fw-a", SourceType:"rsyslog", SpoolDir:dir, ArchiveRetentionDays:0}}, nil, time.Date(2026,7,14,1,0,0,0,time.Local))
	if _, err := os.Stat(path); err != nil { t.Fatalf("permanent archive was removed: %v", err) }
}

func TestArchiverDeletesOnlyReadyExpiredArchive(t *testing.T) {
	dir := t.TempDir()
	readyPath := filepath.Join(dir, "fw-a_2026-06-01.log-20260602.gz")
	failedPath := filepath.Join(dir, "fw-a_2026-06-02.log-20260603.gz")
	writeTestGzip(t, readyPath, "ready\n")
	writeTestGzip(t, failedPath, "failed\n")
	ready := map[ArchiveReadyKey]bool{{SourceID:"fw-a", Date:"2026-06-01"}:true}
	NewArchiver().Run([]model.LogSource{{SourceID:"fw-a", SourceType:"rsyslog", SpoolDir:dir, ArchiveRetentionDays:7}}, ready, time.Date(2026,7,14,1,0,0,0,time.Local))
	if _, err := os.Stat(readyPath); !os.IsNotExist(err) { t.Fatalf("ready archive still exists: %v", err) }
	if _, err := os.Stat(failedPath); err != nil { t.Fatalf("non-ready archive was removed: %v", err) }
}

func writeTestGzip(t *testing.T, path, content string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil { t.Fatal(err) }
	writer := gzip.NewWriter(file)
	if _, err := writer.Write([]byte(content)); err != nil { t.Fatal(err) }
	if err := writer.Close(); err != nil { t.Fatal(err) }
	if err := file.Close(); err != nil { t.Fatal(err) }
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/receiver -run 'TestArchiver' -count=1`

Expected: compile failure for missing `NewArchiver`.

- [ ] **Step 3: Implement atomic gzip production**

Define:

```go
type ArchiveReadyKey struct { SourceID, Date string }
type ArchiveResult struct { SourceID string; Date time.Time; Path string; Deleted bool; Error string; CompletedAt time.Time }
type Archiver struct{}
func (a *Archiver) Run(sources []model.LogSource, ready map[ArchiveReadyKey]bool, now time.Time) []ArchiveResult
```

Scan only `YYYY-MM-DD.log` files older than today. Write gzip content to `<target>.tmp`, close it, rename to the final import-compatible name, and remove the source only after success. If `ArchiveDir` is empty or cleans to `SpoolDir`, write in place. Create the target directory with `0755`.

- [ ] **Step 4: Implement idempotency and safe retention**

If the final archive exists, open it with `gzip.NewReader`; only after a successful read remove a duplicate source `.log`. For cleanup, return immediately for retention `0`; otherwise parse event dates from matching archives and delete only when expired and `ready[ArchiveReadyKey{sourceID,date}]` is true.

- [ ] **Step 5: Feed archive results into receiver status**

Add `Manager.UpdateArchiveResults([]ArchiveResult)` to update `LastArchiveAt` and `ArchiveError` per source without resetting receive counters.

- [ ] **Step 6: Run archive and receiver tests**

Run: `go test -race ./internal/receiver -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/receiver/archive.go internal/receiver/archive_test.go internal/receiver/rsyslog.go
git commit -m "feat: archive received syslog files"
```

### Task 5: Schedule archive runs with ingest readiness

**Files:**
- Create: `internal/server/rsyslog_archive_scheduler.go`
- Create: `internal/server/rsyslog_archive_scheduler_test.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/router.go`
- Modify: `internal/server/settings_controller.go`

- [ ] **Step 1: Write failing scheduler readiness test**

```go
func TestArchiveReadyMapIncludesOnlyReadySourceDates(t *testing.T) {
	states := []DateIngestState{
		{SourceID:"a", LogDate:dateOnly(2026,7,1), Status:StatusReady},
		{SourceID:"a", LogDate:dateOnly(2026,7,2), Status:StatusFailed},
	}
	ready := archiveReadyMap(states)
	if !ready[receiver.ArchiveReadyKey{SourceID:"a", Date:"2026-07-01"}] || len(ready) != 1 { t.Fatalf("ready = %#v", ready) }
}
```

- [ ] **Step 2: Run test and verify RED**

Run: `go test ./internal/server -run TestArchiveReadyMap -count=1`

Expected: compile failure for missing `archiveReadyMap`.

- [ ] **Step 3: Add scheduler lifecycle**

Add `archiver *receiver.Archiver` to `App`, initialize it in `NewApp`, and call `startRSyslogArchiveScheduler(ctx)` next to `startAutoScanScheduler(ctx)` in `Run`. The scheduler runs once immediately, then on a one-minute ticker until context cancellation.

- [ ] **Step 4: Load readiness and execute one archive pass**

`runRSyslogArchive(ctx, now)` reads all configured RSyslog sources, calculates the maximum nonzero retention, loads date states from `now - maxRetention - 1 day`, builds the ready map, calls `archiver.Run`, updates receiver status, and logs each error with `source_id/date/path`.

- [ ] **Step 5: Trigger a nonblocking pass after source settings change**

After a successful `log_sources` save and receiver apply, invoke one archive pass in a goroutine with a bounded context so a newly configured archive directory is applied without waiting a minute.

- [ ] **Step 6: Run server tests**

Run: `go test -race ./internal/server -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/server/rsyslog_archive_scheduler.go internal/server/rsyslog_archive_scheduler_test.go internal/server/server.go internal/server/router.go internal/server/settings_controller.go
git commit -m "feat: schedule rsyslog archiving"
```

### Task 6: Replace expanded source forms with list and modal CRUD

**Files:**
- Modify: `web/src/pages/SystemMaintenancePage.tsx`
- Modify: `web/src/styles.css`
- Modify: `web/tests/maintenanceLayout.test.ts`
- Create: `web/tests/logSourceManagement.test.ts`

- [ ] **Step 1: Write failing structural and copy tests**

```ts
test('log source management uses a compact list and modal editor', () => {
  const page = fs.readFileSync(pagePath, 'utf8');
  assert.match(page, /className="source-management-list"/);
  assert.match(page, /<Modal/);
  assert.match(page, /编辑 RSyslog 接收源/);
  assert.match(page, /客户端 IP \/ 网段/);
  assert.match(page, /留空时压缩文件保留在落盘目录，不移动/);
  assert.match(page, /0 表示永久保留/);
  assert.doesNotMatch(page, /<Form\.List name="log_sources">/);
});
```

- [ ] **Step 2: Run frontend tests and verify RED**

Run: `npm.cmd test -- tests/maintenanceLayout.test.ts tests/logSourceManagement.test.ts`

Expected: assertions fail because the expanded `Form.List` is still present.

- [ ] **Step 3: Isolate source list state and persistence**

Use `savedLogSources` as the displayed source list. Implement:

```ts
async function persistLogSources(next: LogSourceSetting[]) {
  const response = await apiPost<Settings>('/api/settings', { log_sources: JSON.stringify(next) });
  const normalized = normalizeLogSourcesForForm(parseLogSources(response.log_sources));
  setSavedLogSources(normalized);
  return normalized;
}
```

Remove `log_sources` from the main settings `Form` save payload so source CRUD cannot be overwritten by stale form state.

- [ ] **Step 4: Implement add/edit modal**

Add a dedicated `Form<LogSourceSetting>` inside `Modal`. File source fields are device ID, log name, directory, enabled. RSyslog adds client IP/CIDR, protocol, port, spool directory, optional archive directory, retention days default `0`, and enabled. “保存并应用” validates the modal form, replaces or appends one item, calls `persistLogSources`, then closes only on success.

- [ ] **Step 5: Implement immediate toggle and confirmed delete**

Toggle clones the list with one `enabled` change and reverts on request failure. Delete uses `Popconfirm`, persists the filtered list, and states that files are not deleted. Add edit/delete icon buttons with `Tooltip` and `aria-label`.

- [ ] **Step 6: Render desktop and mobile summaries**

Desktop uses a compact comparison grid with headers for device, name, type, client/directory, receive/archive, state, actions. Mobile hides headers and renders the same data as a compact source summary without horizontal scrolling. Fetch `/api/receiver/status` after load and after each persistence to show configured client, last client, last received time, received messages, and archive error/time.

- [ ] **Step 7: Run tests and production build**

Run: `npm.cmd test`

Expected: all frontend tests PASS.

Run: `npm.cmd run build`

Expected: TypeScript and Vite build exit 0.

- [ ] **Step 8: Commit**

```bash
git add web/src/pages/SystemMaintenancePage.tsx web/src/styles.css web/tests/maintenanceLayout.test.ts web/tests/logSourceManagement.test.ts
git commit -m "feat: simplify log source management"
```

### Task 7: Full verification, browser QA, CI, and deployment

**Files:**
- Verify all modified files
- Generated binary: `dist/fwlog_linux_amd64`

- [ ] **Step 1: Run complete local verification**

Run: `go test -race ./... -count=1`

Expected: all Go packages PASS with no race reports.

Run: `npm.cmd test && npm.cmd run build` from `web/` (PowerShell executes them as separate commands).

Expected: all frontend tests PASS and build exits 0.

Run: `git diff --check`

Expected: no output.

- [ ] **Step 2: Run Playwright visual and interaction QA**

At 1440x1000 and 390x844 verify: source list has no horizontal overflow; add/edit modal fields fit; archive copy is visible; edit updates one row; toggle rolls back on simulated failure; delete confirmation text states files remain; RSyslog runtime shows configured and last client IP.

- [ ] **Step 3: Push and wait for GitHub CI**

Push `main`, wait for the latest CI run, and require Go race tests, frontend tests/build, full/upgrade packages, DEB transaction, RPM transaction, and artifact upload all to succeed.

- [ ] **Step 4: Build and deploy the Linux binary**

Build with `GOOS=linux GOARCH=amd64 CGO_ENABLED=0`, upload to a versioned temporary path on `192.168.0.142`, verify SHA-256, preserve the current binary as a rollback copy, atomically replace `/opt/fwlog/fwlog`, update `/opt/fwlog/VERSION`, and restart `fwlog`.

- [ ] **Step 5: Verify client routing and restore production configuration**

Temporarily add a test RSyslog source with a loopback/client route and unused port. Send UDP and TCP messages, confirm only the matched spool receives them, confirm `last_client_ip`, then restore the exact original `log_sources` JSON and remove only the exact test files/directories.

- [ ] **Step 6: Verify auto archive and safety**

Create a previous-day test `.log`, run/wait for the archive pass, verify gzip contents and importer-visible name. Verify empty archive directory keeps the gzip in spool. Verify retention `0` does not delete it. Restore settings and remove the exact test artifact.

- [ ] **Step 7: Final service check**

Require homepage 200, expected app version, original source IDs restored, `systemctl is-active fwlog` = `active`, no error-level journal entries since restart, and a clean local worktree.
