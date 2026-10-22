# 文档索引

本文件是 `docs/` 的唯一入口，记录目录职责、命名规则、维护约定与全部文档链接。新增文档前先看本文件，确认是否已有合适位置。

配套规则见 [`AGENTS.md`](../AGENTS.md) 第 5 节「文档与报告目录规则」与第 6 节「修复报告规则」。

## 1. 目录职责

| 目录 | 职责 | 面向 | 现状 |
| --- | --- | --- | --- |
| [`specs/`](./specs/) | 设计文档：领域模型、关键设计决策、成功标准 | 开发方 | 3 篇 |
| [`plans/`](./plans/) | 实施计划：可逐项执行的改造步骤与复选框进度 | 执行方 | 3 篇 |
| [`reports/`](./reports/) | 修复报告、发布准备报告与验证记录 | 开发 / 运维 | 24 篇 |
| `docs/architecture/` | 调用链与分层说明、数据流、关键契约 | 开发方 | 已有 1 篇：[`overview.md`](./architecture/overview.md) |
| `docs/guides/` | 运维与构建指南 | 运维 / 执行方 | 暂无文档；首个运维或构建指南落地时创建该目录，不预先造空目录 |

设计文档与实施计划一一对应（同一主题、同一日期），报告与 `release-notes/` 记录发布侧结果。

## 2. 命名规则

| 类型 | 路径与命名 | 说明 |
| --- | --- | --- |
| 设计文档 | `docs/specs/YYYY-MM-DD-<主题>-design.md` | 日期为主题立项日，主题用简短 ASCII slug，例如 [`specs/2026-08-31-threat-intelligence-design.md`](./specs/2026-08-31-threat-intelligence-design.md)。 |
| 实施计划 | `docs/plans/YYYY-MM-DD-<主题>.md` | 与同名设计文档成对，例如 [`plans/2026-08-31-threat-intelligence.md`](./plans/2026-08-31-threat-intelligence.md)。 |
| 修复 / 发布报告 | `docs/reports/fix-report-v<版本>-<日期>[-<主题slug>].md` | 版本号取报告所属版本（如 `v1.0.0`、`v2.1.2`）；日期为报告完成日；同一版本同一天有多份报告时用主题 slug 区分，例如 [`reports/fix-report-v1.0.0-2026-09-15-source-identity.md`](./reports/fix-report-v1.0.0-2026-09-15-source-identity.md)。发布准备报告可用主题位 `release-readiness`，例如 [`reports/release-readiness-v1.0.0-2026-09-13.md`](./reports/release-readiness-v1.0.0-2026-09-13.md)。 |
| 架构文档 | `docs/architecture/<主题>.md` | 主题为分层或调用链名称，例如 [`architecture/overview.md`](./architecture/overview.md)。 |
| 运维 / 构建指南 | `docs/guides/<主题>.md` | 主题为操作对象，如部署、升级、回填。 |
| 发布说明 | `release-notes/v<版本>.md` | 由 CI 按 release tag 查找：`release-notes/<version>.md`（见 [`.github/workflows/release-build.yml`](../.github/workflows/release-build.yml)）。 |

## 3. 维护约定

- **必须新增或更新报告的情形**：本次改动涉及功能修复或行为变更、核心代码 / 运行入口 / 任务契约 / 数据结构 / 接口协议 / 可执行能力变化、测试与验证闭环 / 发布流程 / 真实服务调用方式变化、影响架构分层 / 目录结构 / 用户可见行为 / 权限与隐私风险，或用户明确要求。判定口径与报告必需章节见 [`AGENTS.md`](../AGENTS.md) 第 6 节「修复报告规则」。
- **纯文档改动不触发报告**：文档、README、说明文字、技能 / 指令文件、格式整理与注释修正默认不新增报告、不变更版本号。
- **链接必须可跳转**：交叉引用一律使用相对路径（同目录 `./x.md`，上一级 `../x.md`，仓库根 `../../x.md`），优先指向具体文件并在能定位时带行号锚点；不要只写目录名，也不要使用绝对路径（同见 [`AGENTS.md`](../AGENTS.md) 第 6 节）。
- **本索引需同步**：新增 spec / plan / report 后，在本文件对应小节补一行链接，并保持时间倒序。
- **目录渐进创建**：`architecture/`、`guides/` 不预先创建空目录，首个文档落地时一并建目录。

## 4. 设计文档（specs）

| 日期 | 文档 | 说明 |
| --- | --- | --- |
| 2026-09-09 | [`2026-09-09-first-run-admin-design.md`](./specs/2026-09-09-first-run-admin-design.md) | 首次安装管理员初始化：首次启动强制创建管理员的设计与成功标准。 |
| 2026-08-31 | [`2026-08-31-threat-intelligence-design.md`](./specs/2026-08-31-threat-intelligence-design.md) | 目标 IP 威胁情报分析：情报源接入、查询与展示的领域模型。 |
| 2026-07-20 | [`2026-07-20-dashboard-preaggregation-design.md`](./specs/2026-07-20-dashboard-preaggregation-design.md) | 数据概览预聚合：ClickHouse 聚合表与物化视图的设计取舍。 |

## 5. 实施计划（plans）

| 日期 | 计划 | 说明 |
| --- | --- | --- |
| 2026-09-09 | [`2026-09-09-first-run-admin.md`](./plans/2026-09-09-first-run-admin.md) | 首次安装管理员初始化的逐任务实施步骤。 |
| 2026-08-31 | [`2026-08-31-threat-intelligence.md`](./plans/2026-08-31-threat-intelligence.md) | 威胁情报分析的后端、前端与配置改造任务清单。 |
| 2026-07-20 | [`2026-07-20-dashboard-preaggregation.md`](./plans/2026-07-20-dashboard-preaggregation.md) | 数据概览预聚合的建表、回填与查询切换步骤。 |

## 6. 报告（reports）

| 日期 | 版本 | 报告 | 说明 |
| --- | --- | --- | --- |
| 2026-10-22 | v1.0.1 | [`fix-report-v1.0.1-2026-10-22-import-speed-logic-fixes.md`](./reports/fix-report-v1.0.1-2026-10-22-import-speed-logic-fixes.md) | 导入解析提速 2–4 倍、`WORKERS` 并行导入日期；修复源端口乱码、多源查询可见性、分页 has_more、自动扫描失败记录等逻辑错误。 |
| 2026-10-14 | v1.0.0 | [`fix-report-v1.0.0-2026-10-14-dead-code-cleanup.md`](./reports/fix-report-v1.0.0-2026-10-14-dead-code-cleanup.md) | 死代码清理与文档目录重组：deadcode 归零、文档分层与索引落地。 |
| 2026-09-15 | v1.0.0 | [`fix-report-v1.0.0-2026-09-15-mcp-config.md`](./reports/fix-report-v1.0.0-2026-09-15-mcp-config.md) | MCP 接入配置格式修复：下载内容改为单实例顶层配置。 |
| 2026-09-15 | v1.0.0 | [`fix-report-v1.0.0-2026-09-15-source-identity.md`](./reports/fix-report-v1.0.0-2026-09-15-source-identity.md) | 日志来源重命名重复入库修复：引入稳定存储身份。 |
| 2026-09-15 | v1.0.0 | [`fix-report-v1.0.0-2026-09-15-source-path-scan.md`](./reports/fix-report-v1.0.0-2026-09-15-source-path-scan.md) | 来源配置变更避免无关扫描：仅目录变化才触发归档。 |
| 2026-09-13 | v1.0.0 | [`release-readiness-v1.0.0-2026-09-13.md`](./reports/release-readiness-v1.0.0-2026-09-13.md) | v1.0.0 发布准备报告：交付物、发布流程与验证记录。 |
| 2026-09-12 | v2.1.2 | [`fix-report-v2.1.2-2026-09-12-log-management-navigation.md`](./reports/fix-report-v2.1.2-2026-09-12-log-management-navigation.md) | 日志管理导航与接口下载修复：扁平化入口与统一下载按钮。 |
| 2026-09-11 | v2.1.2 | [`fix-report-v2.1.2-2026-09-11-access-trend-mode-correctness.md`](./reports/fix-report-v2.1.2-2026-09-11-access-trend-mode-correctness.md) | 访问趋势 mode 正确性修复：去掉 SQL 提前 LIMIT。 |
| 2026-09-11 | v2.1.2 | [`fix-report-v2.1.2-2026-09-11-access-trend-preaggregation.md`](./reports/fix-report-v2.1.2-2026-09-11-access-trend-preaggregation.md) | 访问趋势预聚合修复：聚合表、缓存与历史回填。 |
| 2026-09-11 | v2.1.2 | [`fix-report-v2.1.2-2026-09-11-log-management-navigation.md`](./reports/fix-report-v2.1.2-2026-09-11-log-management-navigation.md) | 日志管理页面合并：四个二级入口收敛为两项。 |
| 2026-09-11 | v2.1.2 | [`fix-report-v2.1.2-2026-09-11-login-and-reception-split.md`](./reports/fix-report-v2.1.2-2026-09-11-login-and-reception-split.md) | 登录流程图与接收页面调整记录。 |
| 2026-09-11 | v2.1.2 | [`fix-report-v2.1.2-2026-09-11-login-globe-preview.md`](./reports/fix-report-v2.1.2-2026-09-11-login-globe-preview.md) | 登录地球预览：登录页与插件统一为点阵地球。 |
| 2026-09-11 | v2.1.2 | [`fix-report-v2.1.2-2026-09-11-reception-schedule.md`](./reports/fix-report-v2.1.2-2026-09-11-reception-schedule.md) | 接收侧扫描计划调整记录。 |
| 2026-09-10 | v2.1.2 | [`fix-report-v2.1.2-2026-09-10-api-interface.md`](./reports/fix-report-v2.1.2-2026-09-10-api-interface.md) | API 与 MCP 接口设置修复：开关、中间件与统一页签。 |
| 2026-09-10 | v2.1.2 | [`fix-report-v2.1.2-2026-09-10-query-extension.md`](./reports/fix-report-v2.1.2-2026-09-10-query-extension.md) | 查询结果插件与 API 认证修复：CORS 与 Bearer JWT。 |
| 2026-09-09 | v2.1.2 | [`fix-report-v2.1.2-2026-09-09-access-trend-data.md`](./reports/fix-report-v2.1.2-2026-09-09-access-trend-data.md) | 访问趋势正式数据接入：ClickHouse 查询与真实接口。 |
| 2026-09-09 | v2.1.2 | [`fix-report-v2.1.2-2026-09-09-first-run-admin.md`](./reports/fix-report-v2.1.2-2026-09-09-first-run-admin.md) | 首次安装管理员初始化修复报告。 |
| 2026-09-09 | v2.1.2 | [`fix-report-v2.1.2-2026-09-09-task-security-layout.md`](./reports/fix-report-v2.1.2-2026-09-09-task-security-layout.md) | 任务调度与账号安全布局修复报告。 |
| 2026-09-08 | v2.1.2 | [`fix-report-v2.1.2-2026-09-08-access-trend-preview.md`](./reports/fix-report-v2.1.2-2026-09-08-access-trend-preview.md) | 访问趋势地图预览报告。 |
| 2026-09-06 | v2.1.2 | [`fix-report-v2.1.2-2026-09-06-ingest-menu.md`](./reports/fix-report-v2.1.2-2026-09-06-ingest-menu.md) | 入库进度二级菜单调整报告。 |
| 2026-09-06 | v2.1.2 | [`fix-report-v2.1.2-2026-09-06-mfa.md`](./reports/fix-report-v2.1.2-2026-09-06-mfa.md) | 管理员 MFA 与 JWT 登录修复报告。 |
| 2026-09-06 | v2.1.2 | [`fix-report-v2.1.2-2026-09-06-paths.md`](./reports/fix-report-v2.1.2-2026-09-06-paths.md) | IP 数据路径统一修复报告。 |
| 2026-09-06 | v2.1.2 | [`fix-report-v2.1.2-2026-09-06-responsive-defaults.md`](./reports/fix-report-v2.1.2-2026-09-06-responsive-defaults.md) | 响应式布局与默认日志来源修复报告。 |
| 2026-09-05 | v2.1.2 | [`fix-report-v2.1.2-2026-09-05-log-parser.md`](./reports/fix-report-v2.1.2-2026-09-05-log-parser.md) | 日志解析格式扩展修复报告。 |
| 2026-09-05 | v2.1.2 | [`fix-report-v2.1.2-2026-09-05-login-scene.md`](./reports/fix-report-v2.1.2-2026-09-05-login-scene.md) | 登录流程图视觉优化修复报告。 |

## 7. 发布说明（release-notes）

发布说明位于仓库根目录的 [`release-notes/`](../release-notes/)，按 tag 查找，不由本索引逐条维护。

| 版本 | 文件 |
| --- | --- |
| v1.0.1 | [`release-notes/v1.0.1.md`](../release-notes/v1.0.1.md) |
| v2.1.2 | [`release-notes/v2.1.2.md`](../release-notes/v2.1.2.md) |
| v2.1.1 | [`release-notes/v2.1.1.md`](../release-notes/v2.1.1.md) |
| v2.1.0 | [`release-notes/v2.1.0.md`](../release-notes/v2.1.0.md) |
| v1.0.0 | [`release-notes/v1.0.0.md`](../release-notes/v1.0.0.md) |
