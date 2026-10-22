# 登录流程图与接收页面调整记录

## 1. 本轮目标

- 恢复浅色登录页的数据流程图，并增强连接线和移动粒子的可见性。
- 将日志管理中的“接收进度”和“日志来源”拆成两个独立页面。

## 2. 改动文件

- [`web/src/components/LoginDataFlowScene.tsx`](../../web/src/components/LoginDataFlowScene.tsx)
- [`web/src/animations/useLoginSceneMotion.ts`](../../web/src/animations/useLoginSceneMotion.ts)
- [`web/src/styles.css`](../../web/src/styles.css)
- [`web/src/layout/AppLayout.tsx`](../../web/src/layout/AppLayout.tsx)
- [`web/src/main.tsx`](../../web/src/main.tsx)
- [`web/src/pages/IncrementalProgressPage.tsx`](../../web/src/pages/IncrementalProgressPage.tsx)
- [`web/src/pages/SystemMaintenancePage.tsx`](../../web/src/pages/SystemMaintenancePage.tsx)
- [`web/tests/loginAnimation.test.ts`](../../web/tests/loginAnimation.test.ts)
- [`web/tests/maintenanceLayout.test.ts`](../../web/tests/maintenanceLayout.test.ts)

## 3. 关键变化

登录页保留原有浅色布局，流程线增加柔和光晕，移动粒子增大并增加白色描边和阴影，GSAP 动画提高粒子移动速度与不透明度。

日志管理现在提供三个二级入口：

```text
日志管理
├── 接收进度
├── 日志来源
└── 日志整理
```

“接收进度”独立展示轮询进度和按日期状态；“日志来源”独立管理文件目录源与 RSyslog 接收源。原有后端接口和保存逻辑不变。

## 4. 验证

在 Windows PowerShell、Node 22 环境执行：

```powershell
cd web
npm.cmd test
npm.cmd run build
```

结果：77 项测试全部通过，TypeScript 检查和 Vite 构建通过。

## 5. 版本与部署

本轮继续使用 `v2.1.2-test`，未修改版本文件、release tag 或后端接口。仅完成本地前端代码与构建产物验证，未在本轮重复部署测试机。

## 6. 结论

登录流程图的连接线和移动粒子已增强；接收进度与日志来源已拆为独立页面入口，可通过本地预览地址检查实际布局。
