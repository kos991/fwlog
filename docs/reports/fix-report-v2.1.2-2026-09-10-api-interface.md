# API 与 MCP 接口设置修复报告

## 1. 问题、目标与范围

原实现将 API 接口作为独立一级菜单，页面样式与系统设置不一致，MCP 仅显示预留文案且没有真实服务。主要业务页面标题上方还存在重复的类别描述。

本轮完成以下调整：

- 删除主要业务页面标题上方的重复类别描述。
- 将 API 和 MCP 统一移入“系统设置 > 接口服务”。
- 为 API 与 MCP 增加可持久化开关，并让开关约束真实接口行为。
- 提供最小只读 MCP JSON-RPC 服务，仅暴露 `query_logs` 查询工具。

## 2. 改动文件

- [internal/server/interface_access.go](../../internal/server/interface_access.go)：定义接口设置键、Bearer API 开关和 MCP 开关中间件。
- [internal/server/mcp_controller.go](../../internal/server/mcp_controller.go)：实现 MCP `initialize`、`tools/list`、`tools/call` 与 `query_logs`。
- [internal/server/router.go](../../internal/server/router.go#L25)：注册受开关和认证保护的查询 API 与 MCP 路由。
- [internal/server/server.go](../../internal/server/server.go#L248)：增加 API 默认启用、MCP 默认停用的设置项。
- [internal/server/session_auth.go](../../internal/server/session_auth.go#L200)：允许 `api`、`mcp` 客户端按现有登录流程取得 Bearer JWT。
- [internal/server/interface_access_test.go](../../internal/server/interface_access_test.go)：覆盖默认值、启停、Cookie 查询隔离、MCP 工具和认证。
- [web/src/pages/SystemMaintenancePage.tsx](../../web/src/pages/SystemMaintenancePage.tsx#L1136)：新增统一的“接口服务”页签和两个设置开关。
- [web/src/layout/AppLayout.tsx](../../web/src/layout/AppLayout.tsx)：移除独立 API 一级菜单。
- [web/src/main.tsx](../../web/src/main.tsx)：移除独立 API 页面渲染分支。
- [web/src/styles.css](../../web/src/styles.css#L2347)：增加紧凑、响应式的接口设置布局。
- [web/tests/maintenanceLayout.test.ts](../../web/tests/maintenanceLayout.test.ts)：覆盖入口合并、开关字段、重复描述清理和预览状态。

## 3. 关键修复

### 3.1 接口开关语义

- `external_api_enabled=true`：Bearer JWT 客户端可访问 `/api/query`。
- `external_api_enabled=false`：Bearer JWT 查询返回 HTTP `503`、错误码 `external_api_disabled`。
- 网页控制台使用 Cookie 调用同一路由，不受外部 API 开关影响，避免关闭插件接口后导致控制台日志查询失效。
- `mcp_enabled=false`：`/api/mcp` 返回 HTTP `503`、错误码 `mcp_disabled`。
- MCP 无论开关状态均要求有效 Bearer JWT，不接受 Cookie 代替外部客户端认证。

该保护逻辑来自用户明确要求。触发时调用方收到明确错误，不会返回空结果冒充成功。受影响的合法调用仅限被管理员主动停用的外部接口，可在系统设置重新启用。

### 3.2 MCP 只读能力

`/api/mcp` 使用 JSON-RPC 2.0，支持协议版本 `2025-06-18`，提供：

- `initialize`
- `notifications/initialized`
- `tools/list`
- `tools/call`

唯一工具 `query_logs` 复用现有日志查询服务及其时间范围、并发、分页和参数校验，不新增写入、导出或任务管理能力。

### 3.3 页面与文案

- 系统设置新增“接口服务”页签，沿用现有页签、面板、保存按钮、字体和颜色规范。
- 接口地址支持复制，启停状态和开关在桌面及窄屏下保持对齐。
- 删除“系统状态”“NAT 日志检索”“入库管理”“系统管理”等标题上方重复描述。

## 4. 验收与证据

- `go test -count=1 ./...`：通过。
- `go build ./...`：通过。
- `npm.cmd test`：75 项全部通过。
- `npm.cmd run build`：通过，产物写入 `internal/server/web/dist/`。
- 1440px 浏览器检查：页面宽度与视口均为 1440px，无横向溢出。
- 390px 浏览器检查：页面宽度与视口均为 390px，四个系统设置页签完整显示。
- [桌面截图](../../.tmp-preview/interface-settings-desktop-20260910.png)
- [390px 截图](../../.tmp-preview/interface-settings-390-20260910.png)

Windows 本机 `CGO_ENABLED=0` 且没有可用 GCC，`go test -race -count=1 ./...` 无法执行；本轮使用普通全量测试与编译替代，竞态检测仍由 Linux CI 执行。Vite 构建保留项目已有的大分包提示，本轮未扩大处理范围。

## 5. 版本同步

版本保持 `v2.1.2`，未修改 release tag、安装包版本、`VERSION` 或 release notes。本轮在当前版本工作区继续修复，用户未要求升版。

## 6. 风险与回滚边界

- MCP 当前只提供同步 JSON 响应和单个只读查询工具，不包含资源、提示词或写操作。
- 回滚可按职责分别撤销接口路由与控制器、设置默认值、系统设置页签及导航入口，不影响日志表结构与已有数据。

## 7. 结论

独立 API 页面已合并到系统设置，API/MCP 开关已连接真实后端行为，MCP 查询能力与现有日志查询约束保持一致，主要页面重复标题描述已清理。
