# 首次安装管理员初始化实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新安装首次访问时在浏览器创建固定管理员 `admin`，并让账号安全页与其他系统设置页面使用一致的内容边界，同时保留已有安装升级兼容性。

**Architecture:** 后端在服务启动并完成 ClickHouse 连接后暴露未初始化状态和一次性创建接口，使用 App 内互斥锁保证只成功一次；初始化成功后复用现有 PBKDF2 密码哈希、JWT、MFA 和持久化设置。前端在登录分支并行检查初始化状态和会话状态，初始化优先显示独立 SetupPage；账号安全页只做布局边界调整，不改变 `/api/password` 和 MFA 契约。完整安装包停止生成随机管理员密码，旧密码文件和已持久化哈希继续兼容。

**Tech Stack:** Go 1.21、`net/http`、ClickHouse `app_settings`、React 18、TypeScript、Ant Design、Vite、Node test runner、DEB/RPM shell packaging。

---

### Task 1: 固化后端初始化状态与接口契约

**Files:**
- Modify: `internal/server/server.go`（App 状态字段、初始化状态辅助方法）
- Create: `internal/server/setup_controller.go`（`GET /api/setup/status`、`POST /api/setup`）
- Modify: `internal/server/router.go`（注册公开初始化路由）
- Modify: `internal/server/settings_controller.go`（补充严格密码持久化 helper，不能在 Store 未就绪时静默成功）
- Test: `internal/server/setup_controller_test.go`
- Test: `internal/server/routes_test.go`（已有密码和受保护路由回归）

- [ ] **Step 1: 写失败测试，覆盖未初始化、成功初始化和重复初始化**

在 `internal/server/setup_controller_test.go` 中构造 `NewApp(Config{})`，注入可控 `settingsSaver` 和 ready 的测试 Store；断言：

```go
func TestSetupStatusAndCreateAdmin(t *testing.T) {
    app := newSetupTestApp(t)
    status := requestJSON(t, app.Router(), http.MethodGet, "/api/setup/status", "")
    if status.Code != http.StatusOK || !jsonFieldBool(t, status.Body.Bytes(), "required") {
        t.Fatalf("initial setup status = %d %s", status.Code, status.Body.String())
    }

    created := requestJSON(t, app.Router(), http.MethodPost, "/api/setup", `{"password":"first-admin"}`)
    if created.Code != http.StatusCreated { t.Fatalf("create status = %d %s", created.Code, created.Body.String()) }
    if !app.verifyPassword("first-admin") { t.Fatal("created password was not installed") }

    second := requestJSON(t, app.Router(), http.MethodPost, "/api/setup", `{"password":"second-admin"}`)
    if second.Code != http.StatusConflict { t.Fatalf("second setup status = %d", second.Code) }
}
```

同时增加密码确认不一致/过短、已存在 `ADMIN_PASSWORD_FILE` 或持久化哈希时 `required=false`、未初始化时业务 API 仍返回 401、持久化失败不更新内存哈希、并发两个创建请求只有一个 201 的测试。

- [ ] **Step 2: 运行后端专项测试确认失败**

运行：`go test ./internal/server -run 'TestSetup|TestRoutes' -count=1`

预期：FAIL，原因是初始化路由、状态字段和严格持久化逻辑尚不存在。

- [ ] **Step 3: 实现 App 初始化状态和严格持久化**

在 `App` 增加：

```go
setupMu sync.Mutex
```

状态判定只在 `passwordHash` 非空时返回已初始化；`NewApp` 继续优先读取 `ADMIN_PASSWORD`/`ADMIN_PASSWORD_FILE`，`Connect` 继续读取 `admin_password_hash`，不改变旧部署优先级。新增严格 helper：检查 `currentStore() != nil` 且 `Ready()`，然后调用 `SaveSettings`; 未连接或未就绪返回明确错误，不复用会静默返回的普通设置保存路径。

- [ ] **Step 4: 实现公开初始化 handler**

`setup_controller.go` 提供两个 handler：

```go
GET /api/setup/status -> {"required": bool, "username":"admin"}
POST /api/setup      -> 201 {"created":true,"username":"admin"}
```

POST 使用 `decodeJSONBodyLimit`、复用 `validateNewAdminPassword`，在 `setupMu` 内再次检查 `passwordHash`，哈希成功后先严格持久化，持久化成功才写入 `a.passwordHash`。已初始化返回 409，校验或存储错误返回 400/500，错误体不包含密码。路由必须放在 `requireAuth` 之外；其余业务路由不改变。

- [ ] **Step 5: 运行后端测试确认通过**

运行：`go test ./internal/server -run 'TestSetup|TestRoutes' -count=1`

预期：新增初始化、并发、持久化失败和旧密码兼容测试全部 PASS。

---

### Task 2: 构建首次访问初始化页面和启动分支

**Files:**
- Create: `web/src/pages/SetupPage.tsx`
- Modify: `web/src/main.tsx`（并行状态检查、初始化优先级）
- Modify: `web/src/api.ts`（Mock setup status/create）
- Modify: `web/src/styles.css`（沿用 FWLOG 蓝白登录结构，增加初始化页面样式）
- Create: `web/tests/setupPage.test.ts`

- [ ] **Step 1: 写失败的前端契约测试**

在 `web/tests/setupPage.test.ts` 断言：

```ts
const page = fs.readFileSync(path.resolve('src/pages/SetupPage.tsx'), 'utf8');
const main = fs.readFileSync(path.resolve('src/main.tsx'), 'utf8');
assert.match(page, /初始化管理员/);
assert.match(page, /admin/);
assert.match(page, /创建管理员/);
assert.match(page, /confirm_password/);
assert.match(main, /\/api\/setup\/status/);
assert.match(main, /<SetupPage/);
```

测试还要断言初始化页包含密码可见性切换、确认密码校验和成功后触发 `onComplete`，且不直接调用受保护接口。

- [ ] **Step 2: 运行前端专项测试确认失败**

运行：`npm.cmd test -- tests/setupPage.test.ts`

预期：FAIL，因为 SetupPage 和初始化状态分支尚不存在。

- [ ] **Step 3: 实现 SetupPage**

使用现有 `BrandLogo`、Ant Design `Form`、`Input.Password` 和 `apiPost`。表单固定显示只读账号 `admin`，字段为 `password`、`confirm_password`，密码至少 6 个字符；提交 `/api/setup` 成功后调用 `onComplete`，失败显示后端错误。布局使用现有登录页的白色面板、蓝色主按钮和 `prefers-reduced-motion` 规则，不复制参考图黑色主题。

- [ ] **Step 4: 修改 main.tsx 为并行初始化检查**

增加 `SetupStatus` 类型和 `setupRequired` 状态。应用启动时使用 `Promise.all([apiGet('/api/setup/status'), apiGet('/api/session')])`，初始化状态请求失败时明确显示错误状态，不把失败伪装成已初始化。渲染优先级固定为：升级预览、访问趋势预览、初始化页、未登录页、已登录控制台。初始化成功回调只重新查询状态并显示登录页。

- [ ] **Step 5: 补充 Mock 与前端测试**

在 `web/src/api.ts` 增加 Mock 状态变量 `mockSetupRequired = true`；`GET /api/setup/status` 返回状态，`POST /api/setup` 首次返回成功并切换为 false，第二次返回冲突错误。运行：`npm.cmd test -- tests/setupPage.test.ts tests/loginMFA.test.ts`，预期全部 PASS。

---

### Task 3: 让账号安全页与系统设置页面对齐

**Files:**
- Modify: `web/src/pages/SystemMaintenancePage.tsx`（移除账号安全专用居中宽度类，保留内部列宽）
- Modify: `web/src/styles.css`（外层卡片继承 page-stack 内容宽度，内部密码/MFA 控件保留 max-width）
- Modify: `web/tests/maintenanceLayout.test.ts`

- [ ] **Step 1: 写失败布局断言**

把测试从“存在 `max-width: 1120px`”改为断言账号安全面板没有居中限宽，并且拥有与其他维护面板相同的 `width: 100%`/`max-width: none` 语义；内部 `.account-password-fields` 继续断言 `max-width: 520px`。

- [ ] **Step 2: 实现最小 CSS 调整**

删除 `.account-security-panel` 的 `max-width: 1120px` 与 `margin-inline: auto`，改为 `width: 100%; max-width: none;`。保留双栏、窄屏单栏、输入区最大宽度和会话底栏，不改变密码/MFA API 或文案。

- [ ] **Step 3: 浏览器验证对齐**

启动 Mock 前端：`$env:VITE_USE_MOCK='true'; npm.cmd run dev -- --host 127.0.0.1 --port 5174`。使用 Playwright 检查 1920px、1024px、768px：账号安全卡片左/右边界与威胁情报卡片一致，输入框不超过内部列宽，`document.documentElement.scrollWidth === clientWidth`。

---

### Task 4: 修改完整安装包的首次密码行为并保留升级兼容

**Files:**
- Modify: `packaging/build-server-packages.sh`（删除 full 包 postinst 随机密码生成）
- Modify: `packaging/rpm/fwlog.spec`（删除 full 包首次随机密码生成）
- Modify: `scripts/deploy-142-from-release.sh`（首次部署不再强制要求 `ADMIN_PASSWORD`，但已有密码文件继续保留）
- Modify: `internal/server/service_test.go`（更新打包断言）
- Modify: `packaging/build_server_packages_test.go`（产物安装 smoke 断言）

- [ ] **Step 1: 先更新失败的打包断言**

将 `TestServerPackagesGenerateInitialAdminPasswordOnce` 改为 `TestServerPackagesDoNotGenerateInitialAdminPassword`，断言 full 包安装脚本不包含 `od -An -N24`、`/etc/fwlog/admin-password` 的新建逻辑；同时断言服务仍保留 `ADMIN_PASSWORD_FILE` 以兼容旧部署，升级脚本不会删除或覆盖已有文件。

- [ ] **Step 2: 删除新安装随机密码生成**

完整 DEB/RPM postinst 不再创建 `/etc/fwlog/admin-password`；`deploy-142-from-release.sh` 删除 `ADMIN_PASSWORD` 必填分支，仅在目标已有密码文件时不动它。不要删除服务环境变量或后端 `loadAdminPassword`，以保持历史部署兼容。

- [ ] **Step 3: 运行打包静态和脚本语法测试**

运行：`go test ./internal/server ./packaging -run 'TestServerPackages|TestSystemdServices' -count=1` 与 `bash -n packaging/build-server-packages.sh scripts/deploy-142-from-release.sh`（Linux CI 环境执行 Bash 命令）。预期：安装脚本断言通过，full/upgrade 包边界保持不变。

---

### Task 5: 集成验证、报告和测试机兼容检查

**Files:**
- Modify: `docs/superpowers/reports/fix-report-v2.1.2-2026-09-09-task-security-layout.md`（追加首次初始化范围，或新建同版本报告）

- [ ] **Step 1: 运行完整前端验证**

在 `web/` 执行：`npm.cmd test`、`npm.cmd run build`。预期所有前端测试通过，构建产物写入 `internal/server/web/dist/`。

- [ ] **Step 2: 运行完整 Go 验证**

执行：`go test -race -count=1 ./...`、`go build ./...`。预期无数据竞争、无编译错误。

- [ ] **Step 3: 浏览器验证首次初始化和既有登录**

使用隔离临时后端/Mock 验证：未初始化显示 SetupPage，创建后显示登录页，刷新不再显示 SetupPage；输入不一致密码停留在页面并显示校验错误。测试机不得删除 `/etc/fwlog/admin-password`，只验证已有账号登录和账号安全页面对齐。

- [ ] **Step 4: 交叉编译并部署测试机**

PowerShell 执行：

```powershell
$env:GOOS='linux'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'
go build -trimpath -o dist/fwlog_linux_amd64 ./cmd/fwlog
```

通过 Paramiko 上传 `/opt/fwlog/fwlog.upload`，先备份 `/opt/fwlog/fwlog`，原子替换并重启 `fwlog.service`。检查 `systemctl is-active fwlog.service`、`curl -k -I https://127.0.0.1:8080/`，确认已有账号密码未改变。

- [ ] **Step 5: 更新报告并记录回滚点**

报告记录改动文件、65 项以上前端测试、Go 测试、浏览器截图、测试机备份路径和未初始化流程只在隔离环境验证的边界。版本保持 `v2.1.2`，不因本轮页面和首次初始化改动单独升版。

---

## 自检结果

- 设计覆盖：首次状态、初始化唯一性、密码持久化、前端入口、升级兼容、账号安全对齐、打包和测试机回滚均有对应任务。
- 占位符检查：无未完成标记或未定义任务引用。
- 类型一致性：接口字段统一为 `required`、`username`、`password`、`confirm_password`；后端复用现有 `passwordHash`、`settingsSaver` 和 `validateNewAdminPassword`。
