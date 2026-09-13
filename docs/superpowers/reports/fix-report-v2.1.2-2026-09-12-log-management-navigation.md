# 日志管理导航与接口下载修复报告

## 1. 问题与范围

- 日志接收、日志整理曾通过三级菜单选择具体功能，操作路径重复且容易产生弹出菜单错位。
- API 与 MCP 下载入口的控件样式不一致。
- 本轮只收口日志管理导航和接口下载区域，不调整登录页及其他业务页面。

## 2. 改动文件

- [web/src/layout/AppLayout.tsx](../../../web/src/layout/AppLayout.tsx#L16)：日志管理只保留“日志接收”和“日志整理”两个入口。
- [web/src/main.tsx](../../../web/src/main.tsx#L99)：两个入口直接渲染各自页面。
- [web/src/pages/LogReceptionPage.tsx](../../../web/src/pages/LogReceptionPage.tsx#L24)：接收进度与日志来源改为页面内标签。
- [web/src/pages/SystemMaintenancePage.tsx](../../../web/src/pages/SystemMaintenancePage.tsx#L1177)：API 与 MCP 下载入口统一为紧凑按钮。
- [web/src/styles.css](../../../web/src/styles.css#L2236)：补充统一下载按钮样式。
- [web/tests/maintenanceLayout.test.ts](../../../web/tests/maintenanceLayout.test.ts#L65)：增加导航层级与下载入口回归断言。

## 3. 关键修复

- 点击“日志接收”直接进入日志接收页，使用“接收进度 / 日志来源”标签切换。
- 点击“日志整理”直接进入日志整理页，使用“日志重建 / 扫描计划 / 网段别称”标签切换。
- 侧边栏不再为上述两个入口提供第三级功能菜单。
- Chrome 插件与 MCP 配置下载使用相同的尺寸、边框、字体和交互颜色。

## 4. 验证

- `npm.cmd test`：77 项通过，0 项失败。
- `npm.cmd run build`：TypeScript 与 Vite 生产构建成功。
- Linux amd64 二进制交叉编译成功，并部署到 `192.168.0.130:/opt/fwlog/fwlog`。
- 远端 `fwlog` 服务状态为 `active`，`ExecMainStatus=0`。
- 远端 `/api/session` 返回 HTTP 200，Chrome 插件下载地址返回 HTTP 200。

## 5. 版本同步

- 本轮继续使用 `v2.1.2`，未修改版本号。
- 未修改 release note、安装包版本和运行时版本文件。

## 6. 风险与回滚

- 仅改变前端导航状态与展示层级，不改变后端接口协议和日志数据。
- 测试机原二进制备份：`/opt/fwlog/backups/fwlog-20260912151002`。

## 7. 结论

日志管理已收口为两级侧边栏入口，具体功能统一在页面内切换；接口下载区样式已统一，并已完成测试机部署验证。
