# fwlog 项目级 Agent 指令

本文件只约束当前项目 `fwlog`。若与全局偏好冲突，以更具体、更贴近当前项目真实约束的规则为准。

## 0. 执行环境前置规则

- 执行命令、测试或脚本前，先识别当前操作系统与 shell。本项目的实际运行面分三处，命令语法不可混用：
  - 本地开发：Windows（win32）、默认 PowerShell。
  - CI / 构建 / 打包：`ubuntu-24.04`、bash（见 `.github/workflows/ci.yml`、`.github/workflows/release-build.yml`）。
  - 部署目标：Linux（Kylin 麒麟、Debian/Ubuntu、Rocky），systemd + bash（见 `packaging/build-server-packages.sh`、`scripts/*.sh`）。
- Windows PowerShell、Linux Bash、macOS 的命令语法、路径分隔符、环境变量写法、虚拟环境路径不同，不要把 Bash heredoc、`source`、`/` 路径习惯或 shell 语法直接复制到 PowerShell。
- 包管理与运行环境命令必须从项目真实文件识别：Go 依赖见 `go.mod`（module `fwlog`，Go 1.21），前端依赖见 `web/package.json` + `web/package-lock.json`（npm，Node 22）。不要写死不存在的命令。
- 本项目已知的两个本机执行陷阱，遇到时先按此处理，不要当作代码错误：
  - 本地 PowerShell 下 `git` 若报 `missing config key GIT_CONFIG_KEY_0` / `unable to parse command-line config`，是残留的 `GIT_CONFIG_COUNT`/`GIT_CONFIG_VALUE_0` 环境变量缺少对应 key 导致，执行前先 `Remove-Item Env:GIT_CONFIG_COUNT,Env:GIT_CONFIG_VALUE_0 -ErrorAction SilentlyContinue`。
  - 本地 PowerShell 下 `npm` 若报 `npm.ps1 cannot be loaded because running scripts is disabled`，改用 `npm.cmd` 调用（如 `npm.cmd run build`、`npm.cmd test`）。
- 跨平台文件读写必须显式考虑 UTF-8。涉及 Python 时按需启用 `PYTHONUTF8=1` / `python -X utf8`；涉及 Go 源文件与 Web 源文件时保持 UTF-8，注意 Windows 下 Git 可能提示 `LF will be replaced by CRLF`，纯换行差异不作为实质改动提交。
- 修改文本文件前确认原文件编码和换行风格；发现乱码时先判断是读取方式问题还是文件内容损坏，再继续修改（参考 `packaging/build-server-packages.sh` 中 systemd 脚本内嵌的中文文案曾出现乱码的情况，定位问题再动）。

## 1. 语言与表达

- 默认使用简体中文回复。
- 代码、命令、报错、路径、字段名、域名、库名和专有名词可以保留英文原文。
- 不要把未验证内容写成已验证事实，不要把推测写成事实。
- 面向用户的说明要自然、清楚、可执行；不要写成调试日志、机器翻译或内部变量说明。

## 2. 项目定位

- 本项目类型：防火墙 / NAT 日志的导入、接收、查询与运维服务；Go 后端 + React/TypeScript/Vite 前端 + ClickHouse 存储，systemd 服务 + deb/rpm/tar.gz 安装包交付。
- 项目主目标：提供离线可用的日志入库、检索、概览/排行分析、威胁情报联动与在线升级能力。
- 核心边界：V2 是唯一主线，代码入口、服务名、安装路径、打包产物均已迁移到 `fwlog`（旧 `nat-query` 仅作为历史兼容引用，不作为新功能目标）。
- 回答、计划或修改时，不要把项目改写成不符合事实的技术栈、业务场景或发布形态。

## 3. 顶层代码生成约束

- 默认做最小必要改动，但“最小”不是盲目保守。若面向真实用户需求、解决反复多次仍未修好的 bug，或为了贴合真实工业场景 / 业务实践，允许进行更大范围重构。
- 大改前必须说明根因、必要性、影响范围、回归方案和可回滚边界。
- 代码生成必须以真实仓库结构、真实调用链、真实测试入口为依据，不要凭记忆或参考项目路径编造实现。后端调用链从 `cmd/fwlog/main.go` 起步：`config.LoadConfig()` → `server.NewApp(cfg)` → `app.Run(ctx)`；内部包分层见 `internal/`（config、server、storage/clickhouse、importer、dashboard、query、threatintel、upgrade、version 等）。
- 禁止为了显得完整而堆空壳、堆模板、堆不可验证约束。
- 禁止一次性生成超长单文件代码。复杂逻辑应按职责边界拆成模块、函数或配置，保证可审查、可测试、可回滚。
- 不要复制粘贴大段近似逻辑；出现重复分支时，优先抽取明确 helper、配置表或领域模块。
- 新增文件、目录、测试和报告命名要体现职责与场景，避免只有时间戳、缩写或模糊命名。

## 4. 修改前必读

开始任何修改前，至少先读与本轮任务直接相关的最小上下文：

- 项目总览：`README.md`（当前极简，仅说明 V2 定位；真正的设计依据在下面两项）。
- 设计文档：`docs/superpowers/specs/*.md`（如 `2026-08-31-threat-intelligence-design.md`）。
- 实现计划：`docs/superpowers/plans/*.md`（如 `2026-08-31-threat-intelligence.md`）。
- 发布 / 升级说明：`release-notes/*.md`。
- 本轮相关入口、调用链、配置、脚本：`cmd/fwlog/main.go`、`internal/server/*.go`、`internal/storage/clickhouse/*.go`、`web/src/**`、`packaging/build-server-packages.sh` 等，以本轮改动为准。

原则：先理解入口、调用链、数据结构、依赖顺序和验证方式，再动手改文件。不要用过期文件清单替代当前仓库事实。

## 5. 文档与报告目录规则

- `README.md` 面向首次接触本项目的使用 / 运维方，当前极简，不建议把实现细节塞进去。
- `docs/superpowers/specs/*.md` 面向开发方，记录领域模型与关键设计决策。
- `docs/superpowers/plans/*.md` 面向执行方，记录可执行的实现计划。
- `{{报告目录}}` 用于保存修复报告、验证记录或交接记录。本项目尚未建立固定报告目录；在确定前使用占位 `{{报告目录}}`，落地前与用户确认并同步 `.gitignore`（当前 `.gitignore` 只放行 `docs/superpowers/plans/*.md`）。
- 新增文档前先确认是否已有合适位置；不要把一次性过程记录塞进用户文档，也不要把长期规则藏在临时报告里。

## 6. 修复报告规则

修复报告只用于记录核心代码、可执行能力、评测闭环或项目行为的实质变化。纯文档修改、README 更新、说明文字修正、技能 / 指令文件调整、格式整理、注释修正，默认不触发修复报告，也不触发版本号变更。

只有本次改动涉及以下任一类型，完成后才需要在 `{{报告目录}}` 下新增或更新修复报告：

- 功能修复或行为变更。
- 核心代码、运行入口、任务契约、数据结构、接口协议或可执行能力变化。
- 测试、评测、验证闭环、发布流程或真实服务调用方式变化。
- 影响项目架构分层、目录结构、用户可见行为、权限 / 隐私 / 审核风险的变化。
- 用户明确要求生成修复报告。

报告命名规则：

```text
{{报告目录}}/fix-report-v{{版本号}}-{{日期}}.md
```

如项目需要细分主题，可使用：

```text
{{报告目录}}/fix-report-v{{版本号}}-{{日期}}-{{主题slug}}.md
```

报告至少覆盖：

1. 本轮问题 / 目标与范围。
2. 改动文件清单。
3. 关键修复内容。
4. 验收方式 / 手测步骤 / 自动化测试情况。
5. 版本同步清单。
6. 风险与备注。
7. 结论。

生成报告时必须使用可跳转的 Markdown 相对路径交叉引用。链接优先落到具体文件名，能定位到行号时写到行号；不要只写文件夹名代替关键证据，也不要使用当前 IDE 无法跳转的绝对路径，必须强制使用相对路径。

如果本轮不触发修复报告，也要在最终总结中明确说明“不触发报告”的原因。若用户明确要求生成报告，即使是文档或指令文件治理，也必须按用户指定路径生成。

## 7. 版本号演进规则

- 当前版本：`{{当前版本}}`（待确认；本项目以 GitHub release tag 为准，`release-notes/` 目前最新为 `v2.1.2`）。
- 版本起点：V2 主线（服务名、安装路径、打包产物均为 `fwlog`）。
- 版本文件：`{{版本文件}}`。本项目实际版本来源有两处，改动时必须保持一致：
  - 编译期：`internal/server/upgrade_service.go` 中 `var appVersion = "dev"`；正式包由 release 构建通过 ldflags `-X fwlog/internal/server.appVersion=$version` 注入（见 `.github/workflows/release-build.yml`）。
  - 安装期：`packaging/build-server-packages.sh` 写入 `/opt/fwlog/VERSION`（`VERSION=vX.Y.Z`）与 `/opt/fwlog/RUNTIME_VERSION`（`RUNTIME_VERSION=clickhouse-<ver>`），运行时由 `internal/version/service.go` 读取。
- README 版本位置：`{{README版本位置}}`（当前 `README.md` 未标注版本；若引入版本号需与 release tag 同步）。
- 其他版本位置：`release-notes/vX.Y.Z.md`、release 产物 `latest.json` 的 `app_version`/`runtime_version`、RPM/DEB 包版本号（`fwlog-upgrade_<ver>_amd64.deb` 等）。

版本含义：

- `PATCH`：小范围 bug fix、小范围行为修正、局部测试补强或不改变核心能力的兼容性修复。
- `MINOR`：新增可用能力、新增较完整工作流、增加用户可见功能或扩展可执行能力。
- `MAJOR`：明显改变核心调用链、接口契约、数据格式、发布方式或旧用法需要迁移。

默认规则：

- 用户没有明确要求 bump 时，默认在当前版本继续修改。
- 纯文档、说明文字、技能 / 指令文件、格式整理、注释修正默认不修改版本号。
- 不要为了显得进展大而随意升版；版本号必须对应真实改动。
- 每次修改项目版本号时，必须同步检查所有写有当前版本号的位置，包括 `{{版本文件}}`、`{{README版本位置}}`、`release-notes/`、修复报告文件名、`latest.json`、用户可见版本显示、RPM/DEB 包版本或其他项目实际版本位置。
- 如果某些版本位置历史上不一致，必须说明处理方式，不能静默跳过。

## 8. 保护逻辑与错误处理原则

允许必要的安全边界，但不要为了“看起来更稳”而加入没有依据的保护逻辑。这里的保护逻辑是通用判断框架，不绑定任何特定技术栈。

禁止无依据新增的典型保护逻辑包括但不限于：固定超时、长度截断、条数上限、重试上限、静默降级、隐藏兜底、broad try/catch 后吞异常、失败后伪造成功或返回空结果冒充正常。

保护逻辑包括但不限于：

- 中断条件：导入扫描跳过 0 字节文件、跳过近 5 分钟内未稳定的文件等（见 `internal/importer/log_scanner.go`）。
- 容量边界：升级包上限 `maxUpgradePackageBytes = 512 MiB`、升级 HTTP 超时 `10m`（见 `internal/server/upgrade_service.go`）。
- 输入输出限制：在线升级版本号校验 `^v\d+\.\d+\.\d+(?:\.\d+)?$`、IP 校验、威胁情报凭据加密等。
- 重试 / 轮询策略：ClickHouse 就绪轮询、升级状态轮询。
- 降级 / 回退策略：概览排行 5 分钟缓存 + 过期结果降级（见 `internal/storage/clickhouse/dashboard_aggregates.go`）。
- 异常捕获策略：威胁情报各适配器对第三方响应的错误分类（见 `internal/threatintel/`）。
- 默认值策略：`appVersion` 默认 `dev`、运行时版本缺省 `unknown`（见 `internal/version/service.go`）。

只有存在以下依据之一时，才允许新增保护逻辑：

1. 产品需求或用户明确要求。
2. 协议、平台、运行环境或第三方服务存在客观限制。
3. 项目已有同类约定，本轮只是沿用并保持一致。
4. 真实故障证据或测试结果证明不加会稳定导致故障、卡死、数据损坏、安全问题或严重体验问题。

新增保护逻辑后，必须说明：

1. 依据是什么。
2. 触发时用户、日志或调用方能看到什么。
3. 可能误伤哪些合法输入、输出、数据规模或慢路径。
4. 如何验证，如何记录风险，后续如何调整。

错误处理应优先显式暴露问题、便于排查，而不是隐藏错误。禁止吞异常、伪造成功、把真实失败包装成正常空结果，除非项目协议明确要求且已经记录依据和可观测方式。

## 9. 测试与验证

- 不要把“代码已写”当作“功能已完成”。
- 当前项目最小验证命令：
  - Go 全量：`go test -race -count=1 ./...`（CI 使用）；单包：`go test ./internal/<pkg>/...`；编译：`go build ./...`。
  - Web（在 `web/` 下）：`npm ci`、`npm run build`（`tsc -b && vite build`）、`npm test`（`node --test "tests/*.test.ts"`）。
- 按本轮实际改动选择验证方式：改 Go 用 Go 测试，改前端用 `npm run build` + `npm test`，改打包用 `packaging/build_server_packages_test.go` 与 `packaging/build-server-packages.sh` 的产物检查。
- 验证证据可以包括本轮命令输出、日志、截图、trace、报告、构建产物、静态检查结果、解析校验结果或人工验证记录。
- 项目专项验证段落：见下一节的各专项段。
- 如果验证不可用，必须说明不可用的命令、原因、替代验证和仍未验证的边界。
- 历史测试结果不能写成本轮验证结果；本轮验证必须有本轮命令、日志、截图、trace、报告或明确的静态检查证据。

## 10. 可选专项约束段

### Go 后端专项

- 适用条件：改动 `internal/`、`cmd/` 或 `go.mod`/`go.sum` 时启用。
- 必须确认：改动是否落在既有包职责内；`internal/server` 的 handler 是否已在 `router.go` 注册；新增外部依赖是否已进入 `go.mod`（如 `golang.org/x/sync`）。
- 禁止事项：不要在 handler 层直接拼 ClickHouse SQL 而绕过 `internal/storage/clickhouse`；不要吞数据库错误返回空结果冒充正常。
- 验证方式：`go test -race -count=1 ./...`；涉及升级 / 打包逻辑时补 `go test ./internal/server/... ./internal/upgrade/... ./packaging/...`。
- 证据要求：本轮 `go test` 输出；改动文件的相对路径与行号。

### Web 前端专项

- 适用条件：改动 `web/src/`、`web/tests/` 或 `web/package.json` 时启用。
- 必须确认：新页面是否已在 `web/src/main.tsx` 路由/渲染逻辑接入；类型是否通过 `tsc -b`；文案是否符合项目统一中文口径（见 `web/tests/` 中的 wording 测试）。
- 禁止事项：不要绕过现有组件规范硬写内联样式大块堆砌；不要在未登录分支泄漏受保护接口数据。
- 验证方式：`npm.cmd run build`（类型检查 + vite 构建）与 `npm.cmd test`；产物默认落到 `internal/server/web/dist/`（已被 `.gitignore` 忽略，不提交）。
- 证据要求：`npm run build` 输出与 `npm test` 通过结果；改动文件相对路径。

### 数据存储专项（ClickHouse / GeoIP）

- 适用条件：改动 `internal/storage/clickhouse/`、聚合 SQL、迁移逻辑或 GeoIP 读取时启用。
- 必须确认：SQL 是否限定状态版本列、是否遵循 `ReplacingMergeTree` 版本列约定（见 `internal/storage/clickhouse/ingest_state_store_test.go` 的约束测试）；聚合是否遵循“常态走预聚合、冷排行单 worker”的性能边界。
- 禁止事项：不要在迁移中原地改动版本列；不要把冷刷新放到多 worker 抢占多核 CPU。
- 验证方式：对应包测试 `go test ./internal/storage/clickhouse/...`；涉及真实对账时以 `scripts/backfill-dashboard-aggregates.sh` 输出为证据。
- 证据要求：测试输出 + 对账日志（若涉及真实数据）。

### 打包与发布专项

- 适用条件：改动 `packaging/`、`fwlog.service`、`packaging/systemd/`、`scripts/` 或 `.github/workflows/*.yml` 时启用。
- 必须确认：full 与 upgrade 两种模式是否都覆盖；`/opt/fwlog/VERSION`、`/opt/fwlog/RUNTIME_VERSION` 是否只在 full 包出现、upgrade 包必须不含 `RUNTIME_VERSION` 与内嵌 ClickHouse（对照 `ci.yml` 的 smoke test 断言）。
- 禁止事项：不要只改打包脚本不跑产物内容断言；不要把升级包做成带内嵌运行时。
- 验证方式：`go test ./packaging/...`；有真实二进制时跑 `packaging/build-server-packages.sh --version <vX.Y.Z> --binary <path> --output <dir>`，并检查 deb/rpm 内容。
- 证据要求：打包测试输出 + 产物清单（`dpkg-deb --contents` / `rpm -qpl` 结果）。

## 11. Git 与用户改动

- 不要回滚用户已有改动；遇到不相关 dirty 文件，记录并避开。
- 不要自动提交或 push，除非用户明确要求。
- 不要提交密钥、token、`.env`、本地虚拟环境、缓存目录（`.cache/`、`.tmp-docx-structure/`）或无关生成文件（`web/dist`、`internal/server/web/dist` 已被忽略）。
- 高风险删除、迁移、恢复操作必须先只读体检，再说明方案和备份边界。
- 本机执行 `git` 前若遇 `missing config key GIT_CONFIG_KEY_0`，按第 0 节清理 `GIT_CONFIG_*` 环境变量后再跑，不要据此误判仓库状态。

## 12. 输出与验收格式

最终总结尽量按以下顺序组织：

1. 文件改动清单：逐文件说明改了什么。
2. 运行方法 / 验证方式：列出实际运行命令，不把未运行命令写成已运行。
3. 证据路径：截图、日志、trace、报告、构建产物或命令输出摘要；生成报告时必须使用可跳转 Markdown 相对路径交叉引用，链接优先落到具体文件名，能定位到行号时写到行号。
4. 问题与修复闭环：说明定位、修复、复测和结果。
5. 版本同步清单：说明是否升级了版本，检查了哪些版本位置。
6. 修复报告路径：说明是否新增 / 更新报告；不触发时说明原因。
7. 最终结论：是否达标，仍有什么边界风险或未验证项。

## 13. 进度播报格式

在执行命令、读写文件、测试页面、查看日志时，使用简洁中文进度块：

> 🧩 步骤：{一句话描述正在做什么}
> 🎯 目的：{为什么要做}
> ▶️ 执行：{命令、页面、文件路径或操作}
> ✅ 结果：{当前状态}
> 🧾 证据：{可验证证据路径}
> 📝 备注：{可选，最多一句}

## 14. 默认协作风格

- 优先给出可直接落地的改法。
- 优先给出能闭环真实问题的可行方案；如果最小方案会导致功能割裂、不符合业务实践或留下反复返工风险，要明确说明并给出更合适的调整范围。
- 发现风险要明确指出，不要模糊带过。
- 不为了省事跳过测试。
- 不把未验证写成已验证。
- 不把推测写成事实；有证据给证据，没证据就明确说明。
