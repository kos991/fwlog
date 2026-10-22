# 登录流程图视觉优化修复报告

## 1. 问题与范围

现有登录页流程图存在卡片感过强、元素偏大、动效缺少业务含义的问题。本轮保留原有登录卡片比例和认证流程，将左侧改为低存在感的 NAT 日志信号流：日志源、接收、存储、检索与归档以路径和节点标记表达，数据包沿真实路径运行，不改变认证接口或响应式交互契约。

## 2. 改动文件

- [`web/src/components/LoginDataFlowScene.tsx`](../../web/src/components/LoginDataFlowScene.tsx)
- [`web/src/animations/useLoginSceneMotion.ts`](../../web/src/animations/useLoginSceneMotion.ts)
- [`web/src/styles.css`](../../web/src/styles.css)
- [`web/package.json`](../../web/package.json)
- [`web/package-lock.json`](../../web/package-lock.json)
- [`web/tests/loginAnimation.test.ts`](../../web/tests/loginAnimation.test.ts)

## 3. 关键修复

- SVG 画布保留 `geometricPrecision`，移除大面积节点卡片，使用标记、图标和路线标签表达日志流，降低左侧视觉存在感。
- 使用 GSAP `useGSAP` scoped context 与 `MotionPathPlugin`，实现路径逐段绘制、蓝色主流程数据包、青绿色归档数据包、节点图标脉冲和 Logo 通道粒子流。
- 登录成功时暂停循环并让数据包汇聚后淡出；登录失败只轻微反馈登录卡片，不抖动流程图。
- 登录卡片边框恢复为普通浅色细边，仅在顶部增加蓝青渐变流光线，避免整圈渐变喧宾夺主。
- 采用 C 方案收紧登录卡片：最大宽度 340px、桌面内边距 20px 22px、区块间距 14px，并同步压缩移动端留白和表单保留高度。
- 采用 A 方案扩展左侧日志流：增加上下两组低透明度入口/归档辅助轨迹和节点，与中央主流程汇聚，填充画布留白但不增加背景框。
- 按目标图恢复节点信息卡片和底部“日志处理流程”进度条，主路径粒子与节点图标使用 GSAP 持续滚动/脉冲；登录卡片宽度上限校准为 424px。
- 保留页面隐藏暂停和 `prefers-reduced-motion` 静态降级；移除不再使用的 Anime.js 依赖及底部说明性 telemetry 文案。

## 4. 验收方式

- `npm.cmd run build`
- `npm.cmd test`
- 浏览器截图验证因当前环境无法下载 Playwright Chromium（`cdn.playwright.dev` DNS 不可用）未完成，已保留静态构建与测试证据。

本轮构建通过，前端测试 56 项全部通过。

## 5. 版本同步

本轮为登录页局部视觉优化，版本保持 `v2.1.2`，未修改版本文件、发布说明或构建产物版本。

## 6. 风险与备注

改动仅作用于登录页装饰性 SVG、动画 Hook、样式和前端依赖；移动端仍按项目现有约定隐藏流程图，认证接口和错误处理不受影响。GSAP 会增加前端包体积，构建已出现现有的 chunk size warning，未引入新的运行时错误。

## 7. 结论

正式登录页已回到项目既有浅色登录卡片方向，并完成克制的 NAT 日志信号流动效；`npm.cmd run build` 与 56 项前端测试均通过。
