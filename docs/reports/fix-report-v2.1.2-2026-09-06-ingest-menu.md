# 入库进度二级菜单调整报告

## 1. 问题与范围

将系统设置中的“日志来源”“CIDR 别名”“日志入库”调整到“入库进度”一级菜单下，使用 ProLayout 嵌套菜单提供二级弹出入口。

## 2. 改动文件

- [`web/src/layout/AppLayout.tsx`](../../web/src/layout/AppLayout.tsx)
- [`web/src/main.tsx`](../../web/src/main.tsx)
- [`web/src/pages/SystemMaintenancePage.tsx`](../../web/src/pages/SystemMaintenancePage.tsx)
- [`web/tests/maintenanceLayout.test.ts`](../../web/tests/maintenanceLayout.test.ts)

## 3. 关键修复内容

- “入库进度”新增二级菜单：进度概览、日志来源、CIDR 别名、日志入库。
- 二级菜单使用 `/progress/sources`、`/progress/cidr`、`/progress/ingest` 路由状态，并复用现有配置页面逻辑。
- 系统设置仅保留程序升级、威胁情报、账号安全。
- 折叠侧栏下二级菜单由 ProLayout 以弹出菜单呈现；配置页仍保持独立内容和保存逻辑。
- 入库配置页隐藏页内重复 Tabs，仅保留左侧二级菜单作为页面切换入口；主工作区增加最小宽度和横向溢出约束，避免来源列表撑宽页面。
- 二级弹出菜单宽度收窄为 150px，减少右侧空白。
- 侧栏固定为 64px 紧凑模式并移除展开按钮，彻底避免展开侧栏形成大面积空白；入库二级菜单始终使用悬浮弹层。
- 二级弹出菜单容器和菜单项进一步收窄为 136px，避免短标签右侧出现大块空白。
- 明确保存为空的 `log_sources: []` 不再在前端伪造默认日志来源；仅在完全缺少该字段的旧配置上显示兼容性默认来源。

## 4. 验收方式

已执行：

- `npm.cmd run build`：通过。
- `go test ./internal/server`：通过。
- `npm.cmd test`：61 项全部通过。
- `npm.cmd run build`：通过。
- 远端 `systemctl is-active fwlog.service`：`active`。
- 远端 `https://127.0.0.1:8080/`：HTTP `200`。

## 5. 版本同步

版本保持 `v2.1.2`，未修改版本号、发布说明或安装包版本。

## 6. 远端回滚边界

本次仅替换嵌入前端资源的 `/opt/fwlog/fwlog`，远端备份为 `/opt/fwlog/fwlog.backup-20260906233103`。

## 7. 结论

入库相关配置已从系统设置移动到入库进度的二级弹出菜单，系统设置页面范围收窄，服务运行正常。
