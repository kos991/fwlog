# 管理员 MFA 与 JWT 登录修复报告

## 1. 问题与范围

为 FWLOG 管理员登录增加 TOTP 双因子认证和密码可见性切换，并将登录成功后的会话令牌升级为带过期时间和全局失效版本的 JWT。未启用 MFA 时保留原有密码登录兼容行为。

## 2. 改动文件

- [`internal/security/mfa.go`](../../../internal/security/mfa.go)
- [`internal/security/mfa_test.go`](../../../internal/security/mfa_test.go)
- [`internal/security/session.go`](../../../internal/security/session.go)
- [`internal/server/session_auth.go`](../../../internal/server/session_auth.go)
- [`internal/server/mfa_controller.go`](../../../internal/server/mfa_controller.go)
- [`internal/server/jwt_key.go`](../../../internal/server/jwt_key.go)
- [`internal/server/server.go`](../../../internal/server/server.go)
- [`internal/server/router.go`](../../../internal/server/router.go)
- [`internal/server/settings_controller.go`](../../../internal/server/settings_controller.go)
- [`internal/config/config.go`](../../../internal/config/config.go)
- [`internal/config/types.go`](../../../internal/config/types.go)
- [`web/src/pages/LoginPage.tsx`](../../../web/src/pages/LoginPage.tsx)
- [`web/src/api.ts`](../../../web/src/api.ts)
- [`web/src/pages/SystemMaintenancePage.tsx`](../../../web/src/pages/SystemMaintenancePage.tsx)
- [`web/src/styles.css`](../../../web/src/styles.css)
- [`web/tests/loginMFA.test.ts`](../../../web/tests/loginMFA.test.ts)
- [`web/package.json`](../../../web/package.json)
- [`web/package-lock.json`](../../../web/package-lock.json)

## 3. 关键修复内容

- 使用 TOTP 6 位动态验证码，允许一个时间窗口的时钟偏差。
- MFA 配置通过受保护接口完成：生成密钥、验证启用、关闭 MFA 和重新生成恢复码。
- TOTP 密钥复用现有加密密钥文件加密保存；恢复码只保存带随机盐的哈希。
- 登录页提供两条明确入口：密码登录直接签发密码认证 JWT；MFA 登录仅提交 TOTP 或一次性恢复码并签发 MFA 认证 JWT。未传登录模式的旧客户端仍保留密码后 MFA 挑战兼容流程。
- JWT 使用持久化 32 字节密钥，默认路径为 `/data/fwlog/jwt.key`，非 Windows 权限要求 `0600`。
- JWT 放入 `HttpOnly`、`SameSite=Lax` Cookie；令牌包含 subject、签发时间、过期时间、jti、认证方式和会话版本。
- 修改密码、退出登录或关闭 MFA 时递增会话版本并清除当前令牌，使旧 JWT 失效。
- JWT 结构令牌解析失败时不再回退到旧内存令牌比较，避免过期 JWT 被兼容分支重新接受。
- 登录页增加密码可见性切换、验证码输入和恢复码切换；移动端仍隐藏装饰性流程图。
- 登录表单为密码态和 MFA 态保留一致的内容高度；窄屏取消垂直偏移并保持卡片宽度约束，避免键盘弹出或验证码切换时错位。
- MFA 登录失败的 401 仅返回接口错误文案，不触发全局会话失效回调；增加“返回密码登录”操作，验证码方式切换时清空旧输入。
- 登录页增加“密码登录 / MFA 登录”两种入口切换；MFA 入口仅提交 MFA 凭据，密码入口仅提交管理员密码，避免两种方式重复输入。
- 应用启动不再阻塞等待会话接口；登录页立即渲染，JWT 会话在后台恢复，有效会话恢复后自动进入控制台。
- MFA 设置页使用 `qrcode` 根据 `otpauth_uri` 生成可直接扫描的身份验证器二维码，并在启用或取消时清理二维码状态；设置界面仅保留二维码和备用密钥，移除重复的 URI 长文本；窄屏自动改为纵向布局。

## 4. 验收方式

已执行：

- `go test -count=1 ./...`：通过。
- `go test ./internal/server ./internal/security`：协议修正后通过。
- `npm.cmd run build`：通过。
- `npm.cmd test`：60 项全部通过。

未执行：

- `go test -race -count=1 ./...`：当前 Windows 环境 `CGO_ENABLED=0`，Go race detector 要求启用 cgo，因此命令无法启动。
- Playwright 浏览器截图：当前环境无法解析 `cdn.playwright.dev`，Chromium 下载失败。

## 5. 版本同步

版本保持 `v2.1.2`，未修改版本号、发布说明或安装包版本。

## 6. 风险与备注

- JWT 签名密钥丢失会使现有 JWT 全部失效，但不会泄露密码或 TOTP 密钥；管理员需要重新登录。
- 当前实现使用 15 分钟 JWT 有效期，未加入独立 refresh token；过期后需重新完成登录和 MFA。
- 依赖 `github.com/golang-jwt/jwt/v5` 与 `github.com/pquerna/otp`，前端不保存 JWT 或 MFA 秘密。
- 前端新增 `qrcode` 与 `@types/qrcode` 依赖；二维码仅在受保护的 MFA 设置流程中由浏览器本地生成，不新增服务端密钥传输接口。

## 7. 结论

管理员登录已支持 TOTP MFA、一次性恢复码、JWT Cookie 会话和密码可见性切换；MFA 设置页可扫码绑定身份验证器；后端全量测试与前端构建测试通过。
