# v1.0.0 死代码清理与开发文档目录重组

## 1. 问题与范围

`v1.0.0` 发布后仓库同时存在两类冗余：

1. **死代码**：多个包内存在生产不可达函数（含整包无导入者的接口包、仅被测试续命的 helper、非 `_test.go` 的测试转接文件）、未被引用的资源与依赖、以及永不成立的分支；发布出去的 Chrome 扩展 zip 里还打包了 7 个已死的静态文件。
2. **文档结构**：设计文档 / 实施计划 / 报告混在 `docs/superpowers/` 下，`AGENTS.md` 中 `{{报告目录}}`、`{{当前版本}}`、`{{版本文件}}`、`{{README版本位置}}` 四处占位符未落地，目录说明与实际不符（写着“尚未建立固定报告目录”，实际已积累 23 篇报告）。

本轮范围：清理可证明无引用的代码与资源、重建文档目录结构并落地索引与占位符。**不改变任何用户可见行为**（删除项均为不可达路径），**不修改版本号**。

判定方法：`golang.org/x/tools/cmd/deadcode`（两种模式：以 `main` 为根、以及把测试也作为根）+ 自写引用计数（对 903 个 Go 生产声明统计跨文件引用）+ `go list -deps ./cmd/fwlog` 导入反向图 + `main.tsx` 前端 import 图 DFS + manifest/html 引用链。

## 2. 改动文件清单

### 2.1 删除（24 个）

| 文件 | 判定依据 |
| --- | --- |
| `internal/storage/ingest_state_store.go`、`internal/storage/settings_store.go` | 整包零导入者：`go list -deps ./cmd/fwlog` 不含该包，`grep '"fwlog/internal/storage"'` 零命中 |
| `internal/dashboard/compat_test_helpers.go` | 非 `_test.go` 文件却只为测试转发；删除后测试直接调用 `ip.NewIPEngine()` / `importer.BuildAutoScanPlan()` |
| `internal/security/service.go` | `Service` 接口与 `PasswordService` 全仓零引用（生产直接用包级 `HashPassword`/`VerifyPassword`） |
| `scripts/deploy-142-from-release.sh` | 一次性部署脚本，非文档引用只有“读文件断言”测试（用户确认删除） |
| `scripts/monitor-dashboard-cpu.sh`、`scripts/test-backfill-dashboard-aggregates.sh`、`scripts/test-backfill-access-trend-aggregates.sh` | 仅被计划/报告文档提及，未接入 CI 或打包流程 |
| `chrome-extension/login-globe.js`、`globe-land-points.json`、`vendor/three.module.min.js`、`vendor/THREE-LICENSE.txt`、`options.html`、`options.js`、`logo.svg` | manifest 未声明 options 页；`popup.html` 的 classic script 链不含 `login-globe.js`；`vendor/*` 只被已死脚本引用 |
| `web/src/components/LoginGlobeScene.tsx`、`web/src/components/loginGlobeGeometry.ts`、`web/src/assets/globe-land-points.json` | 登录页已改用 `LoginDataFlowScene`，三者从 `main.tsx` import 图不可达 |
| `web/src/assets/threat-intelligence/{nsfocus,qianxin,tencent,threatbook}.ico` | 前端 import 的同名 `.svg`，`.ico` 全仓零引用且未进构建产物 |
| `web/tests/loginGlobe.test.ts` | 全部断言基于“读源码文本/断言文件存在”，对象是本轮删除的死文件 |
| `web/project_report.html` | 被 git 跟踪但零引用，Vite 入口固定为 `index.html` |

### 2.2 新增

| 文件 | 说明 |
| --- | --- |
| [`scripts/build-query-extension.sh`](../../scripts/build-query-extension.sh) | Chrome 扩展打包脚本：确定成员集、`/` 分隔、缺失 `zip` 时明确报错退出；此前仓库没有生成该 zip 的任何脚本 |
| [`docs/README.md`](../../docs/README.md) | 文档唯一索引：目录职责、命名规则、维护约定、全部文档链接 |
| [`docs/architecture/overview.md`](../../docs/architecture/overview.md) | 后端调用链、包分层、数据流、ClickHouse 表与引擎、HTTP 契约分组、升级打包链路 |
| [`docs/reports/fix-report-v1.0.0-2026-10-14-dead-code-cleanup.md`](fix-report-v1.0.0-2026-10-14-dead-code-cleanup.md) | 本报告 |

### 2.3 文档目录重组（29 个文件重命名，内容不变）

```
docs/superpowers/specs/*.md    -> docs/specs/*.md      (3)
docs/superpowers/plans/*.md    -> docs/plans/*.md      (3)
docs/superpowers/reports/*.md  -> docs/reports/*.md    (23)
docs/superpowers/              -> 已删除空目录
```

移动后文档层级由 3 层变 2 层，`docs/reports/*.md` 内的 `../../../` 交叉引用统一改为 `../../`（涉及 21 个报告、共约 190 处），另有 2 处纯文本路径同步修正。

### 2.4 主要修改（49 个文件，+365 / -1942 行）

| 区域 | 代表改动 |
| --- | --- |
| Go 生产代码 | [internal/server/import_controller.go](../../internal/server/import_controller.go)（删 `currentLogSource`/`legacyLogSource`/`importArchivedDates`）、[internal/server/session_auth.go](../../internal/server/session_auth.go)（删 `newSessionToken`）、[internal/server/upgrade_service.go](../../internal/server/upgrade_service.go)（删 `copyFile`/`replaceFileAtomic`/`UpgradeState` 别名/`installedBinaryPath`）、[internal/storage/clickhouse/store.go](../../internal/storage/clickhouse/store.go)（删 `distribution`/`destinationSubnetDistribution` 及两个 SQL helper）、[internal/importer/scheduler.go](../../internal/importer/scheduler.go)（删无调用者的 `ShouldRetryDate`） |
| Go 测试 | 9 个 `internal/server/*_test.go` 中 71 处 `LoadConfig()` 改为 `config.LoadConfig()`（`internal/server/compat.go` 的 `LoadConfig` 转接层删除）；`dateOnly` 等价 helper 移入 [internal/server/test_helpers_test.go](../../internal/server/test_helpers_test.go)；TOTP 测试改用 `pquerna/otp` 直接生成验证码 |
| 前端 | 删 `visibleThreatProviders`（生产零调用）、收回 3 个多余 `export`、移除 `three` / `@types/three` 依赖（`web/package.json` 与 lock 同步） |
| 构建配置 | [web/tsconfig.node.json](../../web/tsconfig.node.json) 增加 `outDir`/`tsBuildInfoFile`，[web/tsconfig.json](../../web/tsconfig.json) 增加 `tsBuildInfoFile`，`tsc -b` 不再把 `vite.config.js`/`vite.config.d.ts`/`*.tsbuildinfo` 写回 `web/` 根目录 |
| 分发产物 | 重新生成 [web/public/downloads/fwlog-query-extension.zip](../../web/public/downloads/fwlog-query-extension.zip)：23 → 16 个成员，且成员路径由 Windows 反斜杠改为 ZIP 规范的 `/` |
| 治理文件 | [AGENTS.md](../../AGENTS.md) 占位符全部落地并改写第 5 节目录规则；[.gitignore](../../.gitignore) 放行规则改为 `docs/{README.md,architecture,specs,plans,reports}` |

## 3. 关键修复内容

1. **整包死代码**：删除 `internal/storage` 包（两个只有接口声明、实现方与调用方都不存在的文件）。
2. **生产不可达函数**：删除 deadcode 在“含测试为根”模式下仍判定 unreachable 的函数（`importArchivedDates`、`newSessionToken`、`copyFile`、`ClickHouseStore.distribution` 链等 10 项）。
3. **仅测试可达的 helper**：删除后同步改写调用它们的测试，使测试直接依赖生产路径，而不是依赖为测试保留的生产代码。
4. **非测试文件的测试转接层**：`internal/dashboard/compat_test_helpers.go` 会编进生产二进制，删除后测试直接调用真实包函数；同类的 `internal/server/compat.go` 只保留生产确实在用的 wrapper。
5. **死分支**：[internal/server/server.go](../../internal/server/server.go) 中 `passwordConfigured := true` 与 `if false && !passwordConfigured` 永假分支及其重复注释删除。
6. **资源与依赖**：4 个 `.ico`、`three`/`@types/three`、`project_report.html`、扩展内 7 个死文件清理；扩展 zip 由新脚本可复现生成。
7. **前端死文件与其测试**：删除不可达的登录点阵地球三联及 `loginGlobe.test.ts`（该文件以及 `loginAnimation.test.ts` 的反向断言都只针对已删实现）。
8. **构建产物污染**：修复 `tsc -b` 把编译产物写到 `web/` 根目录（其中 `vite.config.js` 会**遮蔽** `vite.config.ts`，使后者改动不生效）。
9. **文档结构**：按主题重组为 `docs/{architecture,guides,specs,plans,reports}`（`guides/` 无内容不建空目录），新增索引与架构说明，修复全部因移动失效的交叉引用。
10. **治理落地**：`AGENTS.md` 的 `{{报告目录}}`→`docs/reports/`、`{{当前版本}}`→`v1.0.0`、`{{版本文件}}`→`appVersion` + `/opt/fwlog/VERSION`、`{{README版本位置}}`→`README.md`；`.gitignore` 与新目录结构同步。

## 4. 验收方式与测试情况

本轮实际执行的命令与结果：

| 命令 | 结果 |
| --- | --- |
| `go build ./...` | 通过 |
| `go test -count=1 ./...` | 全部包 `ok`（含 `internal/importer` 5.965s、`internal/server` 12.062s、`packaging` 1.490s），总耗时 26.4s |
| `deadcode ./...` | **0 条** unreachable（清理前 25 条） |
| `deadcode -test ./...` | **0 条** unreachable（清理前 10 条） |
| `cd web && npm.cmd test` | 76 项通过 / 0 失败（清理前 79 项；删除 2 项 `loginGlobe` 用例与 1 项 `visibleThreatProviders` 用例） |
| `cd web && npm.cmd run build` | 通过（`tsc -b && vite build`，4721 modules，12.16s），产物输出到 `internal/server/web/dist/`；构建后 `web/` 根目录不再生成 `vite.config.js` / `*.tsbuildinfo` |
| `unzip -l web/public/downloads/fwlog-query-extension.zip` | 16 个成员，均为 `/` 分隔；`popup.html` 引用的 4 个资源与 `manifest.json` 引用的入口/图标全部在包内 |
| 文档链接检查脚本 | 全部 `docs/**`、`AGENTS.md`、`README.md`、`release-notes/**` 的 markdown 链接可解析（9 处指向本轮删除文件的旧链接已改为不失效的引用） |

未执行：`go test -race`（本机 Windows 缺 gcc，与既往报告一致，由 CI Ubuntu 承担）；`scripts/build-query-extension.sh` 的 `zip` 主路径未能本机执行（本机 Git Bash 无 `zip` 命令，脚本会明确报错退出），zip 由等价的 Python `zipfile` 生成并已按成员清单校验。

## 5. 版本同步清单

- 本轮**未修改版本号**，沿用 `v1.0.0`（用户未要求 bump，且改动为删除不可达代码与文档重组）。
- 已核对：[internal/server/upgrade_service.go](../../internal/server/upgrade_service.go) 的 `appVersion = "dev"` 保持默认；`.github/workflows/release-build.yml` 的 ldflags 注入路径未变；`release-notes/` 未变；`packaging/build-server-packages.sh` 的 `/opt/fwlog/VERSION` 与 `RUNTIME_VERSION` 写入逻辑未变；仓库内无 `latest.json`。
- `AGENTS.md` 第 7 节原先“`release-notes/` 目前最新为 `v2.1.2`”的过期描述已更正为 V2 主线 `v1.0.0`，并说明 `v2.1.x` 为旧编号遗留。

## 6. 风险与备注

1. **重试窗口策略无消费者（既有事实，非本轮引入）**：`NextRetryAt` 仍由 [internal/importer/importer.go](../../internal/importer/importer.go) 在失败时写入状态，但判断“是否该重试”的 `ShouldRetryDate` 从未被生产代码调用，本轮已删除。即“失败日期最多重试 3 次 + 退避窗口”目前只体现在状态字段上，没有代码消费它。本轮**未**擅自补上该逻辑（属行为变更，需产品确认）。
2. **`/api/export` 仍是 501 占位**：[internal/server/router.go](../../internal/server/router.go) 中该路由固定返回 `not_implemented`，除 `routes_test.go` 外无调用方。属未实现能力而非死代码，未删除，建议后续明确取舍。
3. **历史报告中的散文提及**：`docs/plans/2026-09-09-first-run-admin.md`（3 处）、`docs/reports/fix-report-v2.1.2-2026-09-06-paths.md`、`docs/reports/fix-report-v2.1.2-2026-09-09-first-run-admin.md`、`docs/reports/fix-report-v2.1.2-2026-09-11-login-globe-preview.md` 中仍有对已删脚本/扩展文件的文字描述（已改为非链接形式，不会点开 404）。历史记录按原貌保留，未改写结论。
4. **`web/tests/dashboardTrendTicks.test.ts` 全部为源码正则断言**（7 条用例，逐条 `assert.match(source, ...)`）。它把 `HealthDashboard.tsx` 中 `recentDateKeys` 的 `export` 关键字钉住，导致该多余导出无法收回；同时这类断言不覆盖行为。本轮未动该文件，建议单独一轮改造为行为测试。
5. **本机遗留产物未删除**（均在 `.gitignore` 覆盖范围内、非仓库内容）：`.tmp-preview/` 428MB、`.tmp-deploy/` 192MB、`dist/` 127MB（含 `nat-query-service-*` 旧二进制）、`.tmp-docx-structure/` 365KB、`web/*.png` 截图约 40 张、`web/src/pages/HealthDashboard.tsx.backup`、`assets.db*`。删除属不可逆操作且可能包含部署/会话数据，留给仓库所有者决定。
6. **扩展 zip 的成员校验**：脚本成员集来自 `chrome-extension/**` 全量文件，新增扩展文件会自动进包；若将来需要排除某些文件，需在脚本内显式加排除规则（当前保留 `README.md`，与历史分发包一致）。

## 7. 结论

- Go 侧：`deadcode` 两种模式均从 25/10 条降为 **0 条**，`go build` 与全量测试通过；删除的全部是不可达路径，无用户可见行为变化。
- 前端侧：删除不可达页面组件、无用图标与依赖，构建与测试通过（76 项），产物正常重建。
- 分发侧：Chrome 扩展 zip 从 23 个成员收敛到 16 个，并首次有了可复现的打包脚本。
- 文档侧：`docs/` 重组为按主题分层的五个目录，索引、架构说明、占位符与 `.gitignore` 全部对齐，跨文档链接经脚本校验无失效。
