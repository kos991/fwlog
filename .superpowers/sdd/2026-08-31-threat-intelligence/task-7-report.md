# Task 7 报告：奇安信信誉适配器

## 结果

- 状态：DONE
- 提交：见最终回复哈希

## 实现范围

- 新增 `internal/threatintel/qianxin.go`，产出 `NewQianxinAdapter(client *http.Client, endpoint string) Adapter`。
- 默认端点固定为 `https://webapi.ti.qianxin.com/ip/v3/reputation`，测试可注入 endpoint。
- 请求使用 GET，query 固定为 `param=<IP>&mode=0`，凭据仅放入 `Api-Key` header。
- IPv4/IPv6 均复用 `NormalizePublicIP`，支持公网地址。
- 业务 `status=10000` 才映射成功；非 10000 返回稳定 `ErrorInvalidResponse`，不透传上游 `message`。
- `summary_info.reputation` 映射 `malicious/suspicious/benign/unknown`，未知值映射为 `unknown`。
- 成功但缺少 `summary_info` 时返回 `unknown` 成功。
- 标签仅取 `malicious_label`，不从 verdict 反推风险等级、分值或置信度。
- `latest_reputation_time` 解析为 `SourceUpdatedAt`。
- `normal_info`、`geo` 仅保留在脱敏后的 `RawResponse`。
- 公共 RawResponse 脱敏字段补充 `api-key`，覆盖奇安信 `Api-Key` 回显。

## TDD 记录

- RED：先新增 `qianxin_test.go`。
- RED 命令：`go test ./internal/threatintel -run Qianxin -count=1`
- RED 结果：编译失败，`undefined: NewQianxinAdapter`。
- GREEN：新增奇安信适配器，并复用公共 bounded HTTP 与 RawResponse 脱敏。

## 测试

- `go test ./internal/threatintel -run Qianxin -count=1`：通过。
- `go test ./internal/threatintel -count=1`：通过。
- `git diff --check`：通过；仅输出 Git 的 LF/CRLF 工作区提示。

## 自审

- 已确认凭据不进入 URL query。
- 已确认非成功业务状态不保存为 `unknown`，且错误链不包含凭据或上游 message。
- 已确认 RawResponse 中 `Api-Key` 字段会被公共脱敏机制替换为 `[redacted]`。
- 已确认风险等级、置信分值、置信等级不会由 verdict 反推。

## 顾虑

- 无。
