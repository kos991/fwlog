# 响应式布局与默认日志来源修复报告

## 1. 问题与范围

修复维护页在窄桌面和移动端的横向溢出、网格裁切及二级菜单弹层空白；明确新安装实例的默认日志来源行为。

## 2. 改动文件

- [`web/src/styles.css`](../../web/src/styles.css)
- [`web/tests/maintenanceLayout.test.ts`](../../web/tests/maintenanceLayout.test.ts)
- [`internal/server/server.go`](../../internal/server/server.go)
- [`internal/server/service_test.go`](../../internal/server/service_test.go)

## 3. 关键修复

- ProLayout 内容容器、工作区和维护卡片增加 `min-width: 0` 与宽度约束，禁止页面级横向溢出。
- 二级菜单弹层改为 `max-content` 自适应宽度，仅保留菜单文字和左右内边距，避免固定宽度造成空白。
- 二级菜单“入库进度 / 日志来源 / CIDR 别名 / 日志入库”与对应页面主标题保持一致，不再统一显示含义过宽的“日志配置”。
- 901-1200px 桌面窗口将入库、升级和扫描网格切换为两列；移动端统一单列并允许按钮换行。
- `defaultSettings` 首次初始化时写入一个启用的默认文件日志来源，来源目录和标签沿用配置值；管理员明确保存 `log_sources: []` 后仍保持为空。

## 4. 验收方式

- `npm.cmd run build`：通过。
- `npm.cmd test`：62 项通过。
- `go test ./internal/server/...`：通过。

## 5. 版本同步

版本保持 `v2.1.2`，未修改版本号及发布文件。

## 6. 风险与备注

已有实例若数据库中持久化了 `log_sources: []`，不会被自动覆盖；需要在日志来源页手动添加来源。已有历史入库记录与当前来源配置是两个独立维度。

## 7. 结论

窄屏布局约束和默认日志来源初始化已完成，并通过本轮前后端专项测试。
