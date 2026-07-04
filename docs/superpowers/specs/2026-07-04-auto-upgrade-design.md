# 自动升级功能设计

## 目标

为 fwlog 增加一个可从系统设置页触发的升级链路。当前阶段只实现“检查更新、人工确认升级、查看升级状态”，不做定时自动升级，不发布新版本。

## 范围

- 后端新增升级 API：
  - `GET /api/upgrade/check`
  - `GET /api/upgrade/status`
  - `POST /api/upgrade/run`
- 前端系统设置页新增“自动升级”维护区域。
- 升级源固定为 GitHub Release：`kos991/fwlog`。
- Linux 资产固定使用 `nat-query-service_linux_amd64`。
- 升级动作在后台执行，接口立即返回当前状态。

## 不做的事

- 不自动定时升级。
- 不在本轮发布新 tag 或 GitHub Release。
- 不引入数据库表，升级状态先保存在进程内存。
- 不绕过登录态。升级相关接口必须要求已登录。

## 后端设计

新增 `upgrade_service.go`，负责版本检查和升级执行。发布版本由 `appVersion` 表示，默认值为 `dev`，后续发布工作流可通过 `-ldflags "-X main.appVersion=vX.Y.Z"` 注入。

`GET /api/upgrade/check` 调用 GitHub Release API 获取最新 release，确认是否存在 `nat-query-service_linux_amd64`、`nat-query-service.service`、`deploy-142-from-release.sh` 三个资产，并返回当前版本、最新版本和资产状态。

`POST /api/upgrade/run` 接收明确版本号，例如：

```json
{"version":"v1.1.0"}
```

服务端只允许一个升级任务运行。任务步骤：

1. 下载 `nat-query-service_linux_amd64` 到临时文件。
2. 校验文件非空。
3. 备份当前 `/opt/nat-query/nat-query-service`。
4. 替换目标二进制并设置 `0755`。
5. 执行 `systemctl restart nat-query-service`。
6. 更新内存状态为 `succeeded` 或 `failed`。

升级失败时记录错误和备份路径，但不自动回滚，避免在原因不明时扩大影响。

## 前端设计

系统设置页“维护”标签内新增升级卡片：

- 显示当前版本、最新版本、升级状态、错误信息。
- “检查更新”按钮调用 `/api/upgrade/check`。
- 输入目标版本号后点击“升级”调用 `/api/upgrade/run`。
- 升级中轮询 `/api/upgrade/status`。

## 错误处理

- 未登录返回 `401 unauthenticated`。
- 版本号为空或格式不合法返回 `400 invalid_version`。
- 已有升级任务运行返回 `409 upgrade_running`。
- GitHub Release 缺失 Linux 资产返回 `release_asset_missing`。
- 下载、备份、替换或重启失败会写入升级状态 `failed`。

## 测试

- 后端单元测试覆盖 release 解析、资产校验、版本校验、并发保护和路由注册。
- 前端构建验证 `npm.cmd run build --prefix web`。
- 后端验证 `go test -count=1 ./...`。

