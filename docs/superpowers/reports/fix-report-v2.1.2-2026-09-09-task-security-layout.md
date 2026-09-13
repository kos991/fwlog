# 任务调度与账号安全布局修复报告

## 1. 问题与范围

- 原“日志入库”页面实际同时承担自动扫描计划和手动导入任务，页面名称与职责不完全匹配。
- 账号安全页密码输入区过宽，当前会话侧栏存在大面积空白，页面级“保存”与“修改密码”操作产生歧义。

## 2. 改动文件

- [`web/src/layout/AppLayout.tsx`](../../../web/src/layout/AppLayout.tsx)
- [`web/src/pages/SystemMaintenancePage.tsx`](../../../web/src/pages/SystemMaintenancePage.tsx)
- [`web/src/styles.css`](../../../web/src/styles.css)
- [`web/tests/maintenanceLayout.test.ts`](../../../web/tests/maintenanceLayout.test.ts)
- [`web/tests/threatIntelligenceSettings.test.ts`](../../../web/tests/threatIntelligenceSettings.test.ts)

## 3. 关键修复

- 二级菜单、维护页签和页面标题统一由“日志入库”调整为“任务调度”；具体导入动作仍保留“开始导入”等准确文案。
- 后续按业务语义进一步统一为“处理概览 / 数据处理 / 新增日志处理 / 历史数据重建”，并将网段配置命名为“网段名称”；保留后端 `cidr_aliases` 字段兼容。
- 账号安全页改为密码与 MFA 双栏布局，面板最大宽度为 1120px，密码输入区最大宽度为 520px。
- 当前会话和退出登录合并为底部横向操作栏，移除空洞的竖向会话侧栏。
- 账号安全页隐藏页面级“保存”，密码更新只保留明确的“修改密码”操作。
- 900px 以下改为单栏，避免窄屏横向溢出。

## 4. 验收

- `npm.cmd test`：65 项通过。
- `npm.cmd run build`：通过。
- 1920px 浏览器检查：面板宽 1120px，密码区约 555px，无横向溢出，页面级“保存”不可见。
- 1024px 浏览器检查：双栏保持正常，页面无横向溢出。
- 二级菜单浏览器检查：显示“进度概览 / 日志来源 / CIDR 别名 / 任务调度”。
- 测试机 `fwlog.service` 为 `active`，`https://127.0.0.1:8080/` 返回 200。

截图：

- [账号安全桌面布局](../../../.tmp-preview/account-security-desktop.png)
- [账号安全中等宽度布局](../../../.tmp-preview/account-security-medium.png)
- [任务调度二级菜单](../../../.tmp-preview/task-scheduling-menu.png)
- [任务调度页面](../../../.tmp-preview/task-scheduling-page.png)

## 5. 版本同步

版本保持 `v2.1.2`，本轮属于当前补丁版本内的页面命名和布局修复，不单独升版。

## 6. 风险与备注

- 后端密码和 MFA 接口契约未改变。
- 关闭 MFA 仍要求当前密码和验证码，界面已明确提示当前密码位于左侧密码区域。
- 测试机替换前备份为 `/opt/fwlog/fwlog.backup-20260909104511`。

## 7. 结论

任务页面命名已与调度职责一致，账号安全页的操作层级、宽度和会话区域已重新整理并部署到测试机。
