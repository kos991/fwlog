# 日志管理页面合并报告

## 1. 问题与范围

原“入库进度”菜单下分散为“处理概览、日志来源、网段名称、数据处理”四个入口，用户需要在多个页面之间来回切换，菜单名称也不能直接说明要完成的事情。

本轮仅调整前端菜单、页面归属和相关文案，不修改后端接口、日志处理逻辑、保存协议或数据结构。

## 2. 改动文件

- [`web/src/layout/AppLayout.tsx`](../../web/src/layout/AppLayout.tsx)：将四个二级入口收敛为“日志接收”和“日志整理”。
- [`web/src/main.tsx`](../../web/src/main.tsx)：接入两个新页面状态，并让概览、查询页的进度入口跳转到“日志接收”。
- [`web/src/pages/IncrementalProgressPage.tsx`](../../web/src/pages/IncrementalProgressPage.tsx)：增加嵌入模式，保留独立刷新、完成记录开关和原轮询逻辑。
- [`web/src/pages/SystemMaintenancePage.tsx`](../../web/src/pages/SystemMaintenancePage.tsx)：将处理进度与日志来源合并；将处理设置与网段名称放入“日志整理”的页内切换。
- [`web/src/styles.css`](../../web/src/styles.css)：收紧嵌入式进度工具栏间距。
- [`web/tests/maintenanceLayout.test.ts`](../../web/tests/maintenanceLayout.test.ts)：增加菜单数量、页面归属、页内切换和紧凑间距的回归断言。

## 3. 关键改动

菜单调整为：

```text
日志管理
├── 日志接收
└── 日志整理
```

“日志接收”同屏展示当前处理进度、按日期处理结果和日志来源列表。进度请求仍独立轮询：处理中每 5 秒刷新，其他状态每 30 秒刷新；日志来源仍按原逻辑即时保存。

“日志整理”默认显示“处理设置”，并可在页面内切换到“网段名称”。历史数据重建仍与普通设置分开，并保留原有确认流程。

## 4. 验证情况

在 Windows PowerShell、本地 Node 22 环境执行：

```powershell
cd web
npm.cmd test
npm.cmd run build
```

验证结果：前端 75 项测试全部通过；TypeScript 检查和 Vite 生产构建通过。Vite 仍报告已有的大文件分块提示，本轮未增加依赖，也未扩大该问题范围。

未部署到测试机，未连接真实后端进行浏览器手测。

## 5. 版本同步清单

版本保持 `v2.1.2-test`。本轮属于当前补丁版本内的前端信息结构调整，未修改 release tag、[`release-notes/v2.1.2.md`](../../release-notes/v2.1.2.md)、安装包版本或运行时版本文件。

## 6. 风险与回滚

后端接口和数据没有变化。回滚时只需恢复上述前端源码与测试文件；已保存的日志来源、网段名称和处理设置不会受影响。

## 7. 结论

原四个分散入口已合并为两个直白入口，页面内容按“查看接收情况”和“配置日志整理”重新归类，原有进度刷新、来源管理、处理设置和历史重建功能均保留。
