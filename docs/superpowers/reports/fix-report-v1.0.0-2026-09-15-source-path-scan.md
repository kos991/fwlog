# 来源配置变更避免无关扫描

## 问题与范围

编辑日志来源的设备序列号、来源标识、显示名称或启用状态时，不应重新触发归档处理或文件扫描。只有日志目录、接收目录或归档目录发生变化时，才需要启动归档处理。

## 改动文件

- [`internal/server/settings_controller.go`](../../../internal/server/settings_controller.go)：保存 `log_sources` 前后比较目录字段，仅目录变化时调用归档触发器。
- [`internal/server/log_source_settings_test.go`](../../../internal/server/log_source_settings_test.go)：增加元数据变更、目录变更和来源顺序变化测试。

## 验收与测试

- `go test ./internal/server/...`：通过。
- `go test -count=1 ./...`：通过。
- `npm.cmd test`：79 项通过。
- `npm.cmd run build`：通过。
- `go test -race -count=1 ./...`：当前 Windows 环境缺少 C 编译器 `gcc`，无法执行 race 构建；已使用非 race 全量测试替代验证。

## 版本同步

本轮未修改版本号，沿用当前 `v1.0.0`。

## 风险与备注

保存来源配置仍会更新接收器，使新增或修改后的接收参数立即生效；不会因此启动文件归档或导入任务。目录变化的归档处理仍由既有 RSyslog 归档逻辑执行。

## 结论

已将扫描触发条件收窄到目录字段变化，命名和启用状态调整不会再触发该处理。
