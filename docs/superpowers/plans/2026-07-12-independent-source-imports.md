# Independent Source Imports Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow each configured log source to be imported and rebuilt independently, while different sources may run in parallel under a bounded ClickHouse write concurrency.

**Architecture:** The scheduling key is `(source_id, log_date)`. The existing `nat_logs` table must be changed from `PARTITION BY log_date` to `PARTITION BY (source_id, log_date)`, otherwise rebuilding one source deletes another source's same-day data. Source workers are independent and process their own dates/files sequentially. A global write semaphore limits ClickHouse batch writes to one at a time initially; no task platform or unbounded queue is introduced.

**Tech Stack:** Go 1.21, ClickHouse MergeTree, `clickhouse-go/v2`, React 18, TypeScript 5.6.

---

## Explicit Scope

In scope:

- Independent source/date storage isolation.
- Independent sync and rebuild execution.
- At most one active import for each source.
- At most two source workers by default.
- At most one ClickHouse batch write by default.
- Per-source progress display using existing ingest state tables.
- Tests for same-source exclusion, cross-source concurrency, write limiting, and source isolation.

Out of scope:

- A persistent job queue.
- A new job history system or run lifecycle table.
- Round-robin batch queues in the first implementation.
- Rebuilding the HTTP API around a new task resource.
- A full staging pipeline unless ClickHouse tests prove direct source/date replacement cannot preserve existing data safely.
- Unrelated release workflow changes.

## Invariants

- `source_id + log_date` is the smallest replacement unit.
- Rebuilding `fw-a / 2026-07-01` cannot delete or alter `fw-b / 2026-07-01`.
- A source worker owns only its source; an error stops that source and does not cancel siblings.
- A source worker releases its claim on success, error, cancellation, or panic.
- The HTTP handler reports whether the requested source was started or already busy.
- Existing `ingest_dates` and `ingest_files` remain the progress store; add no job database.

## Files

Create:

- `internal/app/import_coordinator.go`
- `internal/app/import_coordinator_test.go`
- `internal/app/clickhouse_migrations.go`
- `internal/app/clickhouse_migrations_test.go`

Modify:

- `internal/app/app.go`
- `internal/app/import_controller.go`
- `internal/app/importer.go`
- `internal/app/clickhouse_store.go`
- `internal/app/clickhouse_store_test.go`
- `internal/app/importer_test.go`
- `internal/app/routes_test.go`
- `internal/app/auto_scan_scheduler.go`
- `internal/app/auto_scan_scheduler_test.go`
- `internal/app/dashboard_service.go`
- `internal/app/dashboard_service_test.go`
- `web/src/pages/SystemMaintenancePage.tsx`
- `web/src/pages/HealthDashboard.tsx`
- `web/src/pages/IncrementalProgressPage.tsx`

## Task 1: Restore A Clean Baseline

- [ ] Remove the current probe-then-release logic in `startBackgroundImport` and the current nested source goroutines in `importConfiguredSources`.
- [ ] Keep the existing `importRunner` signature and existing sync/rebuild routes during the first step.
- [ ] Restore a single global import guard temporarily so the current behavior remains correct while the new coordinator is added.
- [ ] Run:

```bash
gofmt -w internal/app/app.go internal/app/import_controller.go
go test ./internal/app -count=1
```

- [ ] Commit `refactor: restore single import baseline`.

## Task 2: Change ClickHouse Isolation To Source-Date

- [ ] Add failing tests requiring `nat_logs` DDL to contain `PARTITION BY (source_id, log_date)`.
- [ ] Add migration helpers that inspect `system.tables` and distinguish old/new partition keys.
- [ ] Create a replacement table with the new key, copy existing rows, compare total row counts and grouped source/date counts, then atomically rename the tables.
- [ ] Keep the old table as `nat_logs_date_partition_backup`; never delete it automatically.
- [ ] Refuse startup when an incomplete migration is detected instead of guessing.
- [ ] Run:

```bash
go test ./internal/app -run 'Test.*(DDL|Partition|Migration)' -count=1
```

- [ ] Commit `feat: isolate nat logs by source and date`.

## Task 3: Make ImportDate Source-Scoped

- [ ] Change the partition helper from date-only to source/date:

```go
func dropLogSourceDatePartitionSQL(sourceID string, date time.Time) string
```

- [ ] Ensure the generated operation affects only the requested source/date partition.
- [ ] Keep the current importer flow otherwise unchanged in this task: scan files, write batches, update `ingest_files`, update `ingest_dates`.
- [ ] Add tests proving `fw-a / date` generates a different partition target from `fw-b / date` and never emits a date-only drop.
- [ ] Document the temporary failure behavior: direct replacement may leave an empty source/date partition; staging is a separate follow-up only if required by the product's recovery needs.
- [ ] Run importer tests and commit `fix: scope import replacement by source`.

## Task 4: Add A Small Source Coordinator

- [ ] Write tests for:

```text
different sources can run concurrently
the same source is rejected while active
one source failure does not stop another
panic releases the source claim
cancellation releases the source claim
```

- [ ] Implement a coordinator with only these responsibilities:

```go
type ImportCoordinator struct {
	mu      sync.Mutex
	running map[string]struct{}
	sourceSem chan struct{}
	writeSem  chan struct{}
}
```

- [ ] Claim all requested source IDs under one mutex before starting workers. Do not probe and release.
- [ ] Use `sourceSem` with default capacity `2`.
- [ ] Add `WithWriteSlot(ctx, fn)` with default capacity `1`; hold it only around ClickHouse batch creation and send.
- [ ] Recover panics inside each source worker and always release its source claim.
- [ ] Inject the write gate into `Importer.AppendBatch` without changing existing test fakes that do not provide one.
- [ ] Run:

```bash
go test ./internal/app -run 'Test(ImportCoordinator|Importer)' -count=1 -race
```

- [ ] Commit `feat: coordinate independent source imports`.

## Task 5: Connect Existing Routes And Scheduler

- [ ] Keep `/api/sync` and `/api/rebuild` paths unchanged.
- [ ] Add optional `source_id` query selection:

```text
/api/sync                         all enabled sources
/api/sync?source_id=fw-a          fw-a only
/api/rebuild?date=DATE            all enabled sources for DATE
/api/rebuild?source_id=fw-a&date=DATE  fw-a for DATE
/api/rebuild?source_id=fw-a       fw-a all dates
```

- [ ] Return `202` for accepted work and include `busy_sources` when some requested sources are already running. Return `400` for an explicitly unknown or disabled source.
- [ ] Make auto scan submit enabled sources through the same coordinator. A busy source must not prevent an idle source from running.
- [ ] Do not add `run_id`, persistent job history, or new job endpoints in this task.
- [ ] Add route and scheduler tests, run `go test ./internal/app -run 'Test(Router|RunDueAutoScan)' -count=1 -race`, and commit `feat: support source-scoped imports`.

## Task 6: Show Existing Progress Per Source

- [ ] Extend the existing progress response with a `sources` array derived from latest `ingest_dates` rows grouped by `source_id`.
- [ ] Retain current aggregate fields for compatibility; calculate aggregate status from all source states.
- [ ] Show source selection in `SystemMaintenancePage` without changing the existing workflow.
- [ ] Add source rows/filtering to the progress page and source-level active progress to the dashboard.
- [ ] Keep polling at five seconds while any source is importing.
- [ ] Add backend and frontend tests for two simultaneous source imports.
- [ ] Run:

```bash
go test ./internal/app -count=1
cd web && npm test
cd web && npm run build
```

- [ ] Commit `feat: display progress per source`.

## Task 7: Verify Isolation And Write Pressure

- [ ] Add a two-source same-date integration test:

```text
import fw-a / DATE
import fw-b / DATE
rebuild fw-a / DATE
assert fw-b rows and sample data are unchanged
```

- [ ] Add deterministic tests proving maximum source concurrency is `2`, maximum ClickHouse batch write concurrency is `1`, and no worker queues more than one batch in memory.
- [ ] Add failure tests proving one source error does not cancel another source.
- [ ] Run:

```bash
git diff --check
go test -race -count=1 ./...
cd web && npm test
cd web && npm run build
```

- [ ] Run a real ClickHouse smoke test before enabling source concurrency in production.
- [ ] Commit `test: verify source isolation and write limits`.

## Release Criteria

- Same-date sources survive each other's rebuilds.
- Existing routes continue to work without a new job API.
- Same source cannot run twice concurrently.
- Different sources can overlap.
- ClickHouse writes are globally limited to one batch at a time.
- Existing ingest state tables show independent source progress.
- No persistent task platform or unrelated subsystem is introduced.
