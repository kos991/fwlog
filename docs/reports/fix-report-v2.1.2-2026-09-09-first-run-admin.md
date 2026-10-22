# 首次安装管理员初始化修复报告

## 1. 目标与范围

首次安装没有管理员密码时，服务仍可启动并通过浏览器创建固定账号 `admin`；保留已有密码、环境变量和持久化哈希兼容。同时完成初始化页接入、账号安全页面宽度对齐和安装包密码行为调整。

## 2. 改动文件

- [`internal/server/server.go`](../../internal/server/server.go)
- [`internal/server/setup_controller.go`](../../internal/server/setup_controller.go)
- [`internal/server/setup_controller_test.go`](../../internal/server/setup_controller_test.go)
- [`internal/server/settings_controller.go`](../../internal/server/settings_controller.go)
- [`internal/server/router.go`](../../internal/server/router.go)
- [`web/src/pages/SetupPage.tsx`](../../web/src/pages/SetupPage.tsx)
- [`web/src/main.tsx`](../../web/src/main.tsx)
- [`web/src/api.ts`](../../web/src/api.ts)
- [`web/src/styles.css`](../../web/src/styles.css)
- [`web/tests/setupPage.test.ts`](../../web/tests/setupPage.test.ts)
- [`web/tests/maintenanceLayout.test.ts`](../../web/tests/maintenanceLayout.test.ts)
- [`packaging/build-server-packages.sh`](../../packaging/build-server-packages.sh)
- [`packaging/rpm/fwlog.spec`](../../packaging/rpm/fwlog.spec)
- `scripts/deploy-142-from-release.sh`（v1.0.0 死代码清理中删除，见 [fix-report-v1.0.0-2026-10-14-dead-code-cleanup.md](fix-report-v1.0.0-2026-10-14-dead-code-cleanup.md)）

## 3. 关键修复

- 新增公开 `GET /api/setup/status` 和 `POST /api/setup`，使用互斥锁保证仅一次成功。
- 初始化密码使用既有 PBKDF2-SHA256 哈希；持久化成功后才更新内存哈希。
- 启动时即使没有密码也继续提供 HTTP/HTTPS，业务接口仍由认证中间件保护。
- 前端启动并行读取初始化状态和会话状态，初始化优先显示 `SetupPage`。
- 本地 Mock 默认模拟已有安装；只有显式设置 `VITE_MOCK_SETUP_REQUIRED=true` 才显示初始化预览，避免演示状态混入常规预览。
- 完整安装包停止生成随机管理员密码；升级脚本继续兼容旧密码文件。
- 账号安全外层面板改为与维护页面一致的全宽布局，内部密码字段继续限宽。

## 4. 验收与验证

- `npm.cmd test`：66 项通过。
- `npm.cmd run build`：通过。
- `go test ./internal/server -count=1`：通过。
- `go test ./internal/server ./packaging -run 'TestSetup|TestRoutes|TestServerPackages|TestSystemdServices' -count=1`：通过。
- `go build ./...`：通过。
- `bash -n packaging/build-server-packages.sh scripts/deploy-142-from-release.sh`：通过。
- 测试机 `192.168.0.130`：服务 `active`，`https://127.0.0.1:8080/` 返回 HTTP 200，`/api/setup/status` 返回 `{"required":false,"username":"admin"}`；原密码文件未删除（备份目录 `/opt/fwlog/fwlog.backup-20260909132712`）。

`go test -race` 在本机不可用，原因是 Windows 环境缺少 `gcc`，已使用非 race 全量测试和构建替代验证。

## 5. 版本与报告

版本保持 `v2.1.2`，未升级版本号。本报告记录功能和可执行行为变化。

## 6. 风险与备注

新安装首次访问必须能够连接 ClickHouse，初始化密码才可持久化；连接失败会明确返回错误，不会伪造成功。测试机已有管理员凭据未被重置。

## 7. 结论

首次初始化、既有账号兼容、前端启动分支、布局对齐和安装包行为均已完成并通过本轮验证。
