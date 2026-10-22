# 查询结果插件与 API 认证修复报告

## 1. 目标与范围

为 fwlog 增加一个只查询结果的 Chrome MV3 插件。插件仅负责登录、提交 `/api/query` 条件和展示分页结果，不包含下载、导出、任务管理或文件操作。

## 2. 改动文件

- [chrome-extension/manifest.json](../../chrome-extension/manifest.json)
- [chrome-extension/background.js](../../chrome-extension/background.js)
- [chrome-extension/popup.html](../../chrome-extension/popup.html)
- [chrome-extension/popup.js](../../chrome-extension/popup.js)
- [chrome-extension/popup.css](../../chrome-extension/popup.css)
- [chrome-extension/status-metrics.js](../../chrome-extension/status-metrics.js)
- `chrome-extension/options.html`、`chrome-extension/options.js`（v1.0.0 死代码清理中删除，见 [fix-report-v1.0.0-2026-10-14-dead-code-cleanup.md](fix-report-v1.0.0-2026-10-14-dead-code-cleanup.md)）
- [chrome-extension/README.md](../../chrome-extension/README.md)
- [internal/security/session.go](../../internal/security/session.go)
- [internal/server/session_auth.go](../../internal/server/session_auth.go)
- [internal/server/cors.go](../../internal/server/cors.go)
- [internal/server/router.go](../../internal/server/router.go)
- [internal/server/routes_test.go](../../internal/server/routes_test.go)
- [internal/model/types.go](../../internal/model/types.go)
- [internal/storage/clickhouse/store.go](../../internal/storage/clickhouse/store.go)
- [internal/storage/clickhouse/store_test.go](../../internal/storage/clickhouse/store_test.go)
- [web/tests/queryExtensionStatus.test.ts](../../web/tests/queryExtensionStatus.test.ts)

## 3. 关键修复

- 登录请求使用 `client=chrome_extension` 时返回短期 JWT Bearer 令牌；管理员密码不返回、不保存。
- `/api/query` 认证同时支持现有 HttpOnly Cookie 和 `Authorization: Bearer <JWT>`。
- 为 Chrome 扩展提供受限的跨源响应和 OPTIONS 预检处理。
- 插件支持密码登录、MFA 登录、时间范围、统一 IP 地址和分页结果展示。
- 登录态改为 360px 紧凑弹窗，工具栏与 Chrome 扩展图标统一使用项目 FWLOG Logo。
- 修复 `[hidden]` 被表单布局覆盖导致密码和 MFA 输入框同时显示的问题。
- 登录表单支持 Enter 提交、重复提交拦截、输入校验、错误焦点和退出后的状态重置。
- 查询表单收敛为开始时间、结束时间和统一 IP 地址三个条件，分页大小固定为 50。
- 密码框增加可见性切换；页面内不再重复展示 Logo，Logo 仅作为 Chrome 扩展图标。
- JWT 保存到 `chrome.storage.session`，关闭弹窗后再次打开可继续查询；登出、浏览器会话结束或令牌过期后清除。
- 服务地址设置改为插件内嵌面板，不再跳转独立 options 页面。
- 标题栏增加独立的 API 连接状态和插件启停开关：状态胶囊负责展示服务连通性；开关负责启用或停用整个插件，停用后隐藏登录/查询区并禁止请求，同时持久化插件状态。
- 停用开关使用深色关闭态、滑块回到左侧，并通过 MV3 后台 Service Worker 将 Chrome 工具栏图标同步为灰色；浏览器重启后从本地状态恢复。
- 已认证区域拆为“日志查询”和“主机状态”两页，状态页通过 `/api/health-dashboard/summary?range=all` 展示主机 CPU、内存使用率和 ClickHouse 数据库占用空间。
- 标题区增加克制的 SVG 数据流粒子；API 已连接指示点增加呼吸动画，并兼容 `prefers-reduced-motion`。
- 删除标题栏“日志查询”和登录框“连接 FWLOG 查询服务”两处重复说明，压缩登录页垂直空间。
- ClickHouse 健康查询读取 `system.disks` 中默认数据盘的总空间和剩余空间，健康响应新增数据盘总量、剩余量和剩余百分比。
- 主机状态页通过现有 `/api/receiver/status` 汇总日志来源运行数、异常数、最近接收时间和服务本次运行累计接收量；不将累计接收量误写成今日数量。
- 查询结果由横向字段表格调整为紧凑的两行日志列表：首行展示源地址、目标地址和协议，次行展示时间、转换地址和日志来源；点击单条日志可展开完整时间、动作、日志标识和来源 ID。
- 查询筛选区使用可收缩网格，避免 360px 弹窗中日期控件撑开布局并裁切查询按钮。
- 查询页与主机状态页使用带滑动指示器的双页签切换，并为减少动态效果的系统设置保留静态回退。
- 状态页调整为接收状态概览和横向遥测列表，集中展示 CPU、内存、磁盘剩余与 ClickHouse 占用，移除占用空间较大的 2×2 指标矩阵。
- 统一界面字阶和数字字体：操作文案使用系统 UI 字体，IP、端口、时间和主机指标使用等宽字体与等宽数字。
- 时间条件改为可直接编辑的文本输入，支持带秒、斜杠和紧凑数字格式；提交查询前统一规范化为 `YYYY-MM-DD HH:mm:ss`，并显式提示非法时间或反向时间范围。
- 插件内增加紧凑日期面板，通过现有 `/api/ingest-progress?range=all&include_ready=true` 同步主程序日期状态；多来源同日状态沿用主程序的失败、正在入库、等待处理、无数据、可查询优先级。
- 日期面板以任一来源已完成的日期计算可查询起止范围，同时保留同日其他来源正在入库或失败的状态标识；最新可查询日期作为插件默认查询日。
- 登录、MFA、接口设置、停用提示、查询与主机状态统一使用同一套字阶、行高和数字排版，避免不同页面字号跳变。

## 4. 验收方式

- `go test -count=1 ./...`：本轮后端测试通过。
- `node --check chrome-extension/popup.js`：通过。
- `node --check chrome-extension/background.js`：通过。
- `node --test web/tests/queryExtensionStatus.test.ts`：通过，覆盖多来源接收汇总、未配置/等待接收状态和磁盘剩余百分比。
- `go test ./internal/storage/clickhouse -run 'TestDatabaseHealthSQLReadsDefaultDataDiskCapacity|TestDiskAvailablePercent' -count=1`：通过。
- `go test -count=1 ./...`：通过；`go build ./...`：通过。
- `go test -race -count=1 ./...`：本机因 `CGO_ENABLED=0` 无法启动，Go 返回 `-race requires cgo`；已使用全量非 Race 测试和全量编译替代，本轮未完成 Race 检测。
- `npm.cmd run build`：通过；`npm.cmd test`：72 项全部通过，其中覆盖紧凑日志列表、状态遥测列表、滑动页签、360px 可收缩查询区、手动时间解析及日期状态优先级。
- Node `JSON.parse` 校验 `manifest.json`：通过。
- Edge 无头浏览器已生成登录态、停用态、查询页和主机状态页截图：`.tmp-preview/query-extension-login-v4.png`、`.tmp-preview/query-extension-disabled-v4.png`、`.tmp-preview/query-extension-query-v4.png`、`.tmp-preview/query-extension-status-v4.png`。
- 删除冗余文案并增加接收/磁盘状态后的截图为 `.tmp-preview/query-extension-login-v5.png` 和 `.tmp-preview/query-extension-status-v5.png`。
- 正式插件展示代码通过模拟 API 数据完成浏览器实拍：`.tmp-preview/query-extension-formal-check/login.png`、`.tmp-preview/query-extension-formal-check/query.png`、`.tmp-preview/query-extension-formal-check/status.png`、`.tmp-preview/query-extension-formal-check/expanded.png`。
- 时间输入与整体排版复测截图：`.tmp-preview/query-extension-formal-check/login-v2.png`、`.tmp-preview/query-extension-formal-check/mfa-v3.png`、`.tmp-preview/query-extension-formal-check/settings-v3.png`、`.tmp-preview/query-extension-formal-check/disabled-v3.png`、`.tmp-preview/query-extension-formal-check/manual-v3.png`、`.tmp-preview/query-extension-formal-check/calendar-final.png`、`.tmp-preview/query-extension-formal-check/status-v2.png`。
- 本地分发目录与源码文件 SHA-256 一致；重新生成的 `dist/fwlog-query-extension.zip` SHA-256 为 `EB562CC8ECD76FAC9CA5B06CAD19B0EF695E079F463E147DE29F2238DC9CFA86`。
- Linux amd64 二进制已构建并部署到测试机，服务状态为 `active`。
- 测试机 `https://127.0.0.1:8080/` 返回 `200`，未认证 `/api/query` 返回 `401`。

本轮插件包仅同步到本地 `dist/fwlog-query-extension.zip`，未上传测试机。

## 5. 版本同步

未修改版本号，继续使用当前 `v2.1.2` 代码线。

## 6. 风险与备注

- 测试机使用内网自签名证书，首次使用扩展前必须在 Chrome 中打开并信任 `https://192.168.0.130:8080/`。
- JWT 只存于扩展当前会话内；服务重启或登出后令牌失效。
- 测试机旧二进制备份为 `/opt/fwlog/fwlog.backup-query-20260910095028`。

## 7. 结论

查询结果插件与服务端认证链路已完成，未新增下载或其他非查询功能。
