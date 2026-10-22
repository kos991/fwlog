# 登录地球预览与插件登录统一报告

## 1. 问题与范围

网页登录和 Chrome 插件登录页需要统一视觉语言，并加入不影响登录操作的旋转点阵地球；上一轮“日志管理”页面合并也需要在同一份预览构建中保留。

本轮调整登录入口的视觉层和本地预览入口，不改变密码登录、MFA 登录、查询接口或日志处理协议。

## 2. 改动文件

- `web/src/components/LoginGlobeScene.tsx`（v1.0.0 死代码清理中删除，见 [fix-report-v1.0.0-2026-10-14-dead-code-cleanup.md](fix-report-v1.0.0-2026-10-14-dead-code-cleanup.md)）：新增本地点阵地球、弧线和流动点动画。
- `web/src/assets/globe-land-points.json`（同上）：离线陆地点阵资产。
- [`web/src/pages/LoginPage.tsx`](../../web/src/pages/LoginPage.tsx)：将登录页左侧视觉替换为点阵地球。
- [`web/src/animations/useLoginSceneMotion.ts`](../../web/src/animations/useLoginSceneMotion.ts)：移除旧流程图动画，保留登录框 GSAP 动效。
- [`web/src/main.tsx`](../../web/src/main.tsx)：增加 `?preview=login` 直达预览入口。
- [`web/src/styles.css`](../../web/src/styles.css)：加入响应式地球布局，窄屏隐藏装饰地球。
- [`web/package.json`](../../web/package.json)、[`web/package-lock.json`](../../web/package-lock.json)：加入 `three` 与 `@types/three`（v1.0.0 死代码清理中随点阵地球一起移除，见 [fix-report-v1.0.0-2026-10-14-dead-code-cleanup.md](fix-report-v1.0.0-2026-10-14-dead-code-cleanup.md)）。
- `chrome-extension/login-globe.js`（v1.0.0 死代码清理中删除，同上）：插件紧凑点阵地球动画，支持页面隐藏暂停和 reduced-motion。
- `chrome-extension/globe-land-points.json`（同上）：插件本地点阵资产。
- `chrome-extension/vendor/three.module.min.js`（同上）：本地 Three.js 模块。
- [`chrome-extension/popup.html`](../../chrome-extension/popup.html)、[`chrome-extension/popup.css`](../../chrome-extension/popup.css)：加入插件登录视觉区和无 WebGL SVG 回退。

上一轮日志菜单合并仍由 [`fix-report-v2.1.2-2026-09-11-log-management-navigation.md`](fix-report-v2.1.2-2026-09-11-log-management-navigation.md) 记录，当前预览自动包含该改动。

## 3. 关键行为

- 网页登录页保留原登录框尺寸与位置，桌面端左侧显示约 420px 点阵地球。
- 插件登录页保持 360px 宽，顶部视觉区显示约 112px 紧凑地球。
- 地球资源全部本地加载，不依赖外部 CDN；动画约 36 秒旋转一周。
- 登录表单立即渲染，地球异步加载失败不阻塞登录；浏览器不支持 WebGL 时显示轻量 SVG 回退。
- `prefers-reduced-motion` 下停止旋转。
- 日志管理菜单保持“日志管理 -> 日志接收 / 日志整理”，上一轮页面合并功能不回退。

## 4. 验证情况

在 Windows PowerShell、本地 Node 22 环境执行：

```powershell
cd web
npm.cmd test
npm.cmd run build
cd ..\chrome-extension
node --check login-globe.js
```

结果：前端 77 项测试全部通过，TypeScript 检查和 Vite 生产构建通过，插件脚本语法检查通过。

本地预览：

- 网页：`http://127.0.0.1:5181/?preview=login`
- 插件：`http://127.0.0.1:5182/popup.html`

截图证据：`.tmp-preview/screens/login-page.png`、`.tmp-preview/screens/plugin-login-final.png`。

## 5. 版本与部署

版本保持 `v2.1.2-test`，未修改 release tag、安装包版本或运行时版本文件。本轮已将后端与既有功能改动部署到 `192.168.0.130`。根据反馈，网页登录最终恢复为原有日志流程图视觉，插件页面保持原状态；由于服务采用嵌入式前端，部署二进制会携带当前已构建的界面资源。

部署过程中首次误用了 Windows 构建产物，远端出现 `203/EXEC`；已立即从 `/opt/fwlog/backup-20260911-152051` 恢复旧版本，随后使用 `GOOS=linux GOARCH=amd64 CGO_ENABLED=0` 重新编译并部署成功。最终回滚备份位于 `/opt/fwlog/backup-20260911-164500`。

## 6. 风险与回滚

新增前端依赖会使主 Web 包出现已有的大分块提示；不影响功能。远端保留 `/opt/fwlog/backup-20260911-164500` 可回滚目录；日志数据和后端接口不受影响。

## 7. 结论

网页登录已恢复为原有日志流程图登录态，后端与日志管理合并改动已部署到测试机；插件页面保持原状态。
