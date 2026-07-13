# Maintenance Ingest Action Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将维护页「入库操作」改成方案 3：先选日志源、再选单日/日期范围/所有历史日期、再选手动入库/全量重建，并补齐后端日期范围参数。

**Architecture:** 前端在 `SystemMaintenancePage.tsx` 内把 `fullRebuild` 开关替换成 `ingestAction` 与 `dateMode` 表单状态，统一生成 `/api/sync` 或 `/api/rebuild` 请求。后端保留现有 `/api/sync`、`/api/rebuild` 入口，将日期参数解析扩展为单日、闭区间范围、所有历史日期三种模式，执行层复用单日导入函数逐日处理范围。

**Tech Stack:** Go `net/http` + 现有 server 测试；React + TypeScript + Ant Design + dayjs；Node 内置测试。

---

### Task 1: 后端日期范围解析测试

**Files:**
- Modify: `internal/server/routes_test.go`
- Modify: `internal/server/import_controller.go`

- [ ] **Step 1: 写失败测试**

在 `internal/server/routes_test.go` 的日期解析测试旁新增：

```go
func TestParseImportTargetDateAcceptsDateRangeQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/rebuild?date_from=2026-06-13&date_to=2026-06-15", nil)

	got, err := parseImportTargetDate(req)
	if err != nil {
		t.Fatalf("parseImportTargetDate returned error: %v", err)
	}

	wantStart := time.Date(2026, 6, 13, 0, 0, 0, 0, time.Local)
	wantEnd := time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local)
	if !got.Start.Equal(wantStart) || !got.End.Equal(wantEnd) {
		t.Fatalf("target range = %#v, want %v - %v", got, wantStart, wantEnd)
	}
}

func TestParseImportTargetDateRejectsInvertedDateRangeQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/rebuild?date_from=2026-06-15&date_to=2026-06-13", nil)

	if _, err := parseImportTargetDate(req); err == nil {
		t.Fatal("inverted target date range should return an error")
	}
}

func TestParseImportTargetDateRejectsMixedDateAndRangeQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/rebuild?date=2026-06-13&date_from=2026-06-13&date_to=2026-06-15", nil)

	if _, err := parseImportTargetDate(req); err == nil {
		t.Fatal("mixed date and date range should return an error")
	}
}
```

同时更新现有单日测试断言为 `got.Start` 和 `got.End`。

- [ ] **Step 2: 运行失败测试**

Run: `go test ./internal/server -run "TestParseImportTargetDate" -count=1`

Expected: FAIL，因为当前 `parseImportTargetDate` 返回 `time.Time`，没有 `Start` / `End`，也不支持 `date_from/date_to`。

- [ ] **Step 3: 实现最小后端解析**

在 `internal/server/import_controller.go` 中新增：

```go
type importTargetDateRange struct {
	Start time.Time
	End   time.Time
}

func (r importTargetDateRange) IsZero() bool {
	return r.Start.IsZero() && r.End.IsZero()
}
```

把 `parseImportTargetDate` 改为返回 `importTargetDateRange`：

```go
func parseImportTargetDate(r *http.Request) (importTargetDateRange, error) {
	values := r.URL.Query()
	dateValue := strings.TrimSpace(values.Get("date"))
	fromValue := strings.TrimSpace(values.Get("date_from"))
	toValue := strings.TrimSpace(values.Get("date_to"))

	if dateValue != "" && (fromValue != "" || toValue != "") {
		return importTargetDateRange{}, errors.New("date cannot be combined with date_from/date_to")
	}
	if dateValue != "" {
		date, err := time.ParseInLocation("2006-01-02", dateValue, time.Local)
		if err != nil {
			return importTargetDateRange{}, err
		}
		return importTargetDateRange{Start: date, End: date}, nil
	}
	if fromValue == "" && toValue == "" {
		return importTargetDateRange{}, nil
	}
	if fromValue == "" || toValue == "" {
		return importTargetDateRange{}, errors.New("date_from and date_to must be provided together")
	}
	start, err := time.ParseInLocation("2006-01-02", fromValue, time.Local)
	if err != nil {
		return importTargetDateRange{}, err
	}
	end, err := time.ParseInLocation("2006-01-02", toValue, time.Local)
	if err != nil {
		return importTargetDateRange{}, err
	}
	if start.After(end) {
		return importTargetDateRange{}, errors.New("date_from cannot be after date_to")
	}
	return importTargetDateRange{Start: start, End: end}, nil
}
```

并加入 `errors` import。

- [ ] **Step 4: 运行解析测试通过**

Run: `go test ./internal/server -run "TestParseImportTargetDate" -count=1`

Expected: PASS。

### Task 2: 后端日期范围执行测试

**Files:**
- Modify: `internal/server/routes_test.go`
- Modify: `internal/server/import_controller.go`

- [ ] **Step 1: 写失败测试**

在 `routes_test.go` 新增单元测试，测试范围会展开为闭区间逐日列表：

```go
func TestTargetDateRangeDatesReturnsInclusiveDays(t *testing.T) {
	target := importTargetDateRange{
		Start: time.Date(2026, 6, 13, 0, 0, 0, 0, time.Local),
		End:   time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local),
	}

	var got []string
	for _, date := range target.Dates() {
		got = append(got, formatDate(date))
	}

	want := []string{"2026-06-13", "2026-06-14", "2026-06-15"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dates = %#v, want %#v", got, want)
	}
}
```

如果文件还没有 `reflect` import，加入 `reflect`。

- [ ] **Step 2: 运行失败测试**

Run: `go test ./internal/server -run "TestTargetDateRangeDatesReturnsInclusiveDays" -count=1`

Expected: FAIL，因为 `importTargetDateRange` 当前没有 `Dates()`。

- [ ] **Step 3: 实现日期范围执行**

把 `startBackgroundImport`、`startBackgroundImportSources`、`importSourceDates` 的目标日期参数改成 `importTargetDateRange`。在 `importSourceDates` 中：

- `target.IsZero()` 时保持现有所有历史日期逻辑。
- 非零范围时，从 `target.Start` 到 `target.End` 闭区间逐日循环。
- 每天复用现有单日逻辑；重建模式直接导入该日；非重建模式仍检查 `LatestDateState`，已完成则加入 skipped。

范围模式复用 `target.Dates()` 逐日执行；`target.IsZero()` 时仍可调用 `importRunner`，因为 `importRunner` 表示所有历史日期的测试/替换入口。

- [ ] **Step 4: 运行执行测试通过**

Run: `go test ./internal/server -run "TestTargetDateRangeDatesReturnsInclusiveDays|TestParseImportTargetDate" -count=1`

Expected: PASS。

### Task 3: 前端维护页测试

**Files:**
- Modify: `web/tests/maintenanceLayout.test.ts`
- Modify: `web/tests/sourceScopedMaintenance.test.ts`
- Modify: `web/src/pages/SystemMaintenancePage.tsx`

- [ ] **Step 1: 写失败测试**

把 `maintenanceLayout.test.ts` 中旧的 `full rebuild switch makes all-date rebuild explicit` 改为：

```ts
test('maintenance ingest action uses source date range and action type', () => {
  const page = fs.readFileSync(pagePath, 'utf8');
  const styles = fs.readFileSync(stylesPath, 'utf8');

  assert.doesNotMatch(page, /const \[fullRebuild, setFullRebuild\]/);
  assert.doesNotMatch(page, /<Switch checked=\{fullRebuild\}/);
  assert.match(page, /type IngestAction = 'sync' \| 'rebuild';/);
  assert.match(page, /type IngestDateMode = 'all' \| 'single' \| 'range';/);
  assert.match(page, /const \[ingestAction, setIngestAction\]/);
  assert.match(page, /const \[dateMode, setDateMode\]/);
  assert.match(page, /DatePicker\.RangePicker/);
  assert.match(page, /所有历史日期/);
  assert.match(page, /日期范围/);
  assert.match(page, /全量重建当前日志源的所选日期范围/);
  assert.match(page, /本次操作：日志源 =/);
  assert.match(styles, /\.maintenance-action-summary\s*\{/);
});
```

把 `sourceScopedMaintenance.test.ts` 第一条测试改为匹配 `buildIngestPath`：

```ts
test('source selection applies to sync and rebuild ingest operations', () => {
  assert.match(source, /const \[savedLogSources, setSavedLogSources\]/);
  assert.match(source, /function buildIngestPath/);
  assert.match(source, /sourceID/);
  assert.match(source, /date_from/);
  assert.match(source, /date_to/);
  assert.match(source, /apiPost\(buildIngestPath\(\)\)/);
  assert.match(source, /trigger\('\/api\/ip-data\/reload',[^\n]+\)/);
});
```

- [ ] **Step 2: 运行失败测试**

Run: `cd web; npm test -- tests/maintenanceLayout.test.ts tests/sourceScopedMaintenance.test.ts`

Expected: FAIL，因为当前页面仍使用 `fullRebuild`、单日 `rebuildDate` 和分散按钮。

- [ ] **Step 3: 实现维护页状态和路径构造**

在 `SystemMaintenancePage.tsx` 中：

- 增加 `type IngestAction = 'sync' | 'rebuild';`
- 增加 `type IngestDateMode = 'all' | 'single' | 'range';`
- 把 `rebuildDate/fullRebuild` 替换为 `ingestAction/dateMode/singleDate/dateRange`。
- 增加 `enabledLogSources`、`selectedSourceLabel`、`dateScopeLabel`、`buttonLabel`、`actionSummary`。
- 增加 `buildIngestPath()`，根据 `source_id`、`date`、`date_from/date_to` 生成请求。
- 增加 `triggerIngestAction()`，全量重建走 `Popconfirm`，手动入库直接请求。

- [ ] **Step 4: 实现 JSX 和样式**

把入库操作卡片改为：

- 日志源 Select。
- 日期范围 Select。
- 单日 DatePicker 或 RangePicker。
- 操作类型 Select。
- 摘要 `.maintenance-action-summary`。
- 主按钮：手动入库使用 primary，全量重建使用 danger。

在 `web/src/styles.css` 中保留现有 `.maintenance-run-grid`，新增 `.maintenance-action-summary`，删除或停止使用 `.maintenance-rebuild-mode`。

- [ ] **Step 5: 运行前端定向测试通过**

Run: `cd web; npm test -- tests/maintenanceLayout.test.ts tests/sourceScopedMaintenance.test.ts`

Expected: PASS。

### Task 4: 全量验证

**Files:**
- Modified files from previous tasks

- [ ] **Step 1: 运行后端测试**

Run: `go test ./internal/server -count=1`

Expected: PASS。

- [ ] **Step 2: 运行前端测试**

Run: `cd web; npm test`

Expected: PASS。

- [ ] **Step 3: 运行前端构建**

Run: `cd web; npm run build`

Expected: PASS。

- [ ] **Step 4: 检查工作区**

Run: `git status --short`

Expected: 只包含本次计划、测试、前后端实现相关文件。

- [ ] **Step 5: 提交实现**

Run:

```bash
git add internal/server/import_controller.go internal/server/routes_test.go web/src/pages/SystemMaintenancePage.tsx web/src/styles.css web/tests/maintenanceLayout.test.ts web/tests/sourceScopedMaintenance.test.ts docs/superpowers/plans/2026-07-13-maintenance-ingest-action-plan.md
git commit -m "feat: clarify maintenance ingest actions"
```

Expected: 提交成功。

## 自检

- Spec 覆盖：日志源、单日、日期范围、所有历史日期、手动入库、全量重建、确认文案、测试验证均有任务覆盖。
- 占位扫描：无待补实现项。
- 类型一致性：前端统一使用 `IngestAction`、`IngestDateMode`；后端统一使用 `importTargetDateRange`。
