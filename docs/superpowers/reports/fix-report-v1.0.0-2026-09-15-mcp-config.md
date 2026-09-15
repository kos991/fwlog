# MCP 接入配置格式修复

## 问题与范围

下载配置最初使用完整客户端配置文件的 `mcpServers.fwlog` 包装层。用户当前使用的“新增 MCP”页面已经单独管理 MCP 名称，只读取粘贴 JSON 的顶层 `type`，因此即使内层存在 `type`，仍提示缺少该字段。

## 改动文件

- [`internal/security/mfa.go`](../../../internal/security/mfa.go)：JWT 支持独立作用域和有效期。
- [`internal/server/interface_access.go`](../../../internal/server/interface_access.go)：管理员下载时签发 30 天 MCP 专用令牌；该令牌只能调用 MCP 接口。
- [`web/src/pages/SystemMaintenancePage.tsx`](../../../web/src/pages/SystemMaintenancePage.tsx)：下载内容改为单实例顶层配置，并把实时签发的 Bearer JWT 直接写入请求头。
- [`web/tests/maintenanceLayout.test.ts`](../../../web/tests/maintenanceLayout.test.ts)：约束 `type` 位于顶层，并禁止重新加入 `mcpServers.fwlog` 包装层。

## 验收与测试

- `npm.cmd test`：79 项通过。
- `npm.cmd run build`：通过。
- `go test -count=1 ./...`：通过。

## 版本同步

本轮未修改版本号，沿用 `v1.0.0`。

## 结论

重新下载 MCP 接入配置后，可直接粘贴到“新增 MCP”的配置输入框。配置包含真实的 30 天 MCP 专用 JWT，无需手动替换占位符，且不能用于访问系统管理接口。管理员退出网页不会导致服务接入令牌立即失效。
