# v1.0.0 发布准备报告

## 1. 目标与范围

本轮为 fwlog V2 主线准备首个正式版本。版本策略从 `v1.0.0` 开始，保留本地开发默认值 `dev`，由 GitHub Release tag 在正式构建时注入程序版本。

## 2. 改动文件

- [.github/workflows/release-build.yml](../../../.github/workflows/release-build.yml)：正式发布版本校验改为 `v1.0.0` 起步，允许四段版本作为测试预发布版本；更新手动发布示例。
- [README.md](../../../README.md)：补充首个正式版本和本地 `dev` 版本说明。
- [release-notes/v1.0.0.md](../../../release-notes/v1.0.0.md)：新增首发版本能力、交付物和升级说明。

## 3. 版本策略

- 正式版本：`v1.0.0` 及更高的三段版本，例如 `v1.0.1`、`v2.0.0`。
- 测试预发布版本：允许四段版本，例如 `v1.0.0.1`，工作流会标记为 prerelease。
- `internal/server/upgrade_service.go` 中的 `appVersion = "dev"` 保留不变，避免本地构建伪装成正式版本。
- 安装包内部版本由构建参数写入 `/opt/fwlog/VERSION`；包文件名使用去掉前缀 `v` 的版本号。

## 4. 发布前检查清单

- [ ] 创建并推送 Git tag `v1.0.0`。
- [ ] 确认 GitHub Actions 的 `Release Build` 成功完成。
- [ ] 确认 `checksums.txt`、`latest.json` 与全部 amd64 交付物已上传。
- [ ] 在干净的 Linux 环境验证完整安装包和升级包。
- [ ] 在测试机验证服务启动、登录、日志接收、查询 API 和 MCP 接口。
- [ ] 发布前保留 `/opt/fwlog` 与 ClickHouse 数据备份。

## 5. 验证方式

本报告对应的代码验证命令：

```text
cd web
npm.cmd test
npm.cmd run build
cd ..
go test -race -count=1 ./...
```

本轮实测结果：

- `cd web && npm.cmd test`：77 项通过，0 项失败。
- `cd web && npm.cmd run build`：TypeScript 检查和 Vite 生产构建通过，产物生成到 `internal/server/web/dist/`。
- `go test -count=1 ./...`：全部 Go 包通过。
- `go test -race -count=1 ./...`：本机未安装 `gcc`，Windows 环境无法启用 cgo 竞态检测；该命令已在 CI 的 Ubuntu 24.04 环境中配置执行。
- GitHub Actions 发布测试运行 [34731407327](https://github.com/kos991/fwlog/actions/runs/34731407327)：完成且结论为 `success`。
- 测试 Release [v1.0.0.1](https://github.com/kos991/fwlog/releases/tag/v1.0.0.1)：预发布状态，已上传 10 个资产。
- 已删除历史 Release `v2.1.0`、`v2.1.1`、`v2.1.2` 及其全部资产；远端旧 tag 已复核为空。

发布工作流还会在 Ubuntu 24.04 中执行 Go/Web 测试、Linux amd64 编译、DEB/RPM 完整安装和升级事务检查。

## 6. 风险与回滚

- 本轮未将 `appVersion` 硬编码为 `1.0.0`，因此未通过 Release tag 构建的二进制仍会显示 `dev`，这是预期行为。
- 未删除历史 `v2.1.x` 发布说明，历史版本仍可用于升级兼容和问题追溯。
- 若首发包验证失败，可不推送 `v1.0.0` tag；若已创建 Release，可删除或重新运行同一 tag 的 Release Build。

## 7. 结论

版本入口、首发说明和发布检查项已准备完成。代码级验证和一次远端预发布构建已通过；正式发布前仍需确认待发布代码已合入 `main`，再创建 `v1.0.0` tag 触发正式 Release Build。
