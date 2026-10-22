# 接收侧扫描计划调整记录

## 1. 目标

- 移除登录流程图节点左侧不需要的黑色圆点。
- 将“扫描计划”从“数据处理”中拆出，放到日志管理的接收侧独立页面。

## 2. 改动

- [`web/src/components/LoginDataFlowScene.tsx`](../../web/src/components/LoginDataFlowScene.tsx)：移除节点标记圆点。
- [`web/src/layout/AppLayout.tsx`](../../web/src/layout/AppLayout.tsx)：新增“扫描计划”二级入口。
- [`web/src/main.tsx`](../../web/src/main.tsx)：接入 `/progress/schedule` 页面。
- [`web/src/pages/SystemMaintenancePage.tsx`](../../web/src/pages/SystemMaintenancePage.tsx)：扫描计划单独展示；数据处理页只保留手动处理和历史重建。
- [`web/tests/maintenanceLayout.test.ts`](../../web/tests/maintenanceLayout.test.ts)：更新页面归属和数量断言。
- [`web/tests/threatIntelligenceSettings.test.ts`](../../web/tests/threatIntelligenceSettings.test.ts)：同步维护页面板数量断言。

## 3. 页面结构

```text
日志管理
├── 日志接收
│   ├── 接收进度
│   ├── 扫描计划
│   └── 日志来源
└── 日志整理
    ├── 数据处理
    └── 网段名称
```

## 4. 验证

```powershell
cd web
npm.cmd test
npm.cmd run build
```

结果：77 项测试全部通过，前端构建通过。

## 5. 测试机部署

- 目标：`root@192.168.0.130:/opt/fwlog`
- 构建方式：`GOOS=linux GOARCH=amd64 CGO_ENABLED=0`
- 远端备份：`/opt/fwlog/backup-20260911-234637`
- 远端服务：`fwlog active`，`ExecMainStatus=0`
- 远端进程：`MainPID=284627`
- 远端二进制 SHA256：`19925fb7a230910cc70c697a515bf21f27746945258cb91016204f38f3802467`
- HTTPS 检查：`/api/session` 返回 `{"authenticated":false}`

## 6. 菜单分组调整

接收侧三个页面统一归入“日志接收”菜单，“日志整理”保持独立入口并继续使用页内页签。

## 7. 结论

黑色节点圆点已移除，扫描计划已成为日志管理下独立的接收侧页面，测试机已运行新版本。
