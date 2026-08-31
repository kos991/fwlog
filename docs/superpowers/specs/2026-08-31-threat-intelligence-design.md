# 目标 IP 威胁情报分析设计

## 背景

FWLOG 的日志查询结果已经展示目标 IP，但目前无法直接判断该 IP 是否具有恶意风险。新功能在“目标 IP / 端口”列接入微步、绿盟、奇安信和腾讯四个平台，让用户按需查看本地历史结果，并在明确点击后调用对应平台的接口。

各平台账号、套餐和额度相互独立，因此本功能不要求用户同时拥有四个平台的账号，也不做自动查询或四平台一键查询。未配置的平台不会影响其他平台使用。

## 目标与成功标准

- 用户可以从日志查询结果中的目标 IP 进入任一已配置平台的分析浮层。
- 打开浮层只读取 FWLOG 本地记录，不消耗第三方额度。
- 用户点击“开始分析”或“重新分析”后，才调用所选平台一次。
- 每个平台、每个 IP 对外只保留并展示最新一次成功结果。
- 调用失败不覆盖旧结果，不把失败误判为无风险或无情报。
- 平台凭据不下发浏览器，不出现在普通设置接口、应用日志和 GitHub Actions 中。
- 未购买或未配置的平台保持不可用状态，不阻塞其他平台和版本发布。

## 范围

本次包含：

- 微步、绿盟、奇安信、腾讯四个平台的独立配置、连接测试和 IP 分析。
- 日志查询结果中的平台图标、结果浮层和主动分析操作。
- 统一结果模型、最新成功结果存储、原始响应留存和错误提示。
- 凭据加密、日志脱敏、并发防重和自动化测试。

本次不包含：

- 四个平台的一键批量分析、后台自动分析或定时刷新。
- 域名、URL、文件哈希、IP 加端口等其他情报对象。
- 平台账号注册、套餐购买、充值和额度统计。
- 多平台结果加权评分或自动处置恶意 IP。
- 历史版本列表；界面只读取最新成功结果。

## 用户交互

### 查询结果

“目标 IP / 端口”列在现有内容旁显示平台图标。每个平台使用独立图标并带悬停提示和无障碍名称。只有同时满足“已启用”和“凭据已配置”的平台才显示图标；图标资源随前端静态资源发布，不依赖第三方站点加载。

点击图标打开锚定浮层，浮层显示平台名称和当前目标 IP：

- 无本地记录：显示“暂无分析记录”和“开始分析”。
- 有本地记录：显示统一判定、风险等级、置信度、标签、来源更新时间、分析时间、摘要和可展开的原始详情，并提供“重新分析”。
- 分析中：按钮禁用并显示加载状态，浮层尺寸保持稳定。
- 分析失败且有旧记录：继续显示旧记录，并单独显示“本次分析失败”及明确原因。
- 分析失败且无旧记录：显示错误原因和可再次执行的按钮。

关闭浮层或切换平台不会触发分析。分析成功后只刷新当前平台、当前 IP 的浮层内容。

### 系统维护

“系统维护”页面新增“威胁情报平台”配置区，分别配置四个平台。每个平台包含启用开关、凭据输入、连接测试和最后一次测试状态。

- 已保存凭据只显示固定脱敏状态，不显示可推断长度的掩码。
- 编辑时凭据留空表示保留；输入新值表示覆盖；提供明确的“清除凭据”操作。
- 未保存的凭据可以参与本次保存，但连接测试只使用后端已保存凭据，避免通过测试接口传递明文。
- 绿盟配置项提示其授权可能绑定 FWLOG 服务器公网 IP。
- 四个平台均无独立的免费鉴权探测接口，因此连接测试使用固定公网 IP `1.1.1.1` 发起一次查询，只在用户点击时执行。界面明确提示“连接测试可能消耗 1 次接口额度”；测试结果不写入分析结果表。

## 总体架构

前端只访问 FWLOG 后端。后端增加威胁情报服务层，包含以下边界：

1. 配置服务负责平台启用状态、凭据加密保存和脱敏状态。
2. 分析服务负责 IP 校验、平台选择、并发防重、超时控制和结果持久化。
3. 四个平台适配器分别负责请求构造、鉴权、响应解析和错误码映射。
4. 存储接口负责读取及写入最新成功结果，业务层不依赖 ClickHouse SQL 细节。

适配器统一接受规范化后的 IP，返回统一结果或分类错误。平台专属字段仅保留在原始响应中，不泄漏到公共业务逻辑。

## 平台接口

### 微步

```text
GET https://api.threatbook.cn/v3/scene/ip_reputation
```

请求参数为 `apikey`、`resource` 和 `lang=zh`，支持 IPv4 和 IPv6。主要使用 `is_malicious`、`confidence_level`、`severity`、`judgments` 和 `update_time`。由于 API Key 位于 URL 参数中，任何请求日志都不得记录完整 URL 或查询串。

### 绿盟

```text
GET https://nti.nsfocus.com/api/v2/objects/ioc-ipv4/?query=<IPv4>
Accept: application/nsfocus.nti.spec+json; version=2.0
X-Ns-Nti-Key: <key>
```

该接口只用于 IPv4。解析有效、未撤销且未过期的 `objects`，使用 `confidence`、`threat_level`、`categories`、`threat_types`、`act_types`、`tags` 和 `modified`。鉴权还可能受调用服务器公网 IP 限制。

### 奇安信

```text
GET https://webapi.ti.qianxin.com/ip/v3/reputation?param=<IP>&mode=0
Api-Key: <key>
```

支持 IPv4 和 IPv6。使用 `summary_info.reputation`、`latest_reputation_time`、`malicious_label`、`normal_info` 和 `geo`。使用 `mode=0` 获取完整响应，供原始详情留存。

### 腾讯

```text
POST https://xti.qq.com/api/v3/ti
Content-Type: application/json
```

请求体固定使用 `c_version=3.0`、`c_action=IPAnalysis`、`c_lang=zh`、`type=ip` 和 `option=0`，凭据写入 `c_appkey`，查询值只传 IP，不传端口。解析 `return_code`、`return_msg`、`result`、`threat_level`、`confidence`、`tags`、`first_seen`、`last_seen` 及其他详情字段。

## 统一判定

统一结果字段如下：

```text
provider
ip
verdict: malicious | suspicious | benign | unknown
risk_level: critical | high | medium | low | info | unknown
confidence_score
confidence_level: high | medium | low | unknown
tags
first_seen
last_seen
source_updated_at
analyzed_at
summary
raw_response
```

映射规则：

- 微步：`is_malicious=true` 为 `malicious`，`false` 为 `benign`；风险等级使用 `severity`，`confidence_level` 直接映射，无数据为 `unknown`。
- 绿盟：存在有效、未撤销且未过期的对象时为 `malicious`，风险取对象中最高 `threat_level`，其中 `1/3/5` 分别映射为 `low/medium/high`；没有有效对象为 `unknown`，不推断为 `benign`。
- 奇安信：`malicious`、`suspicious`、`benign`、`unknown` 直接映射。
- 腾讯：`black` 为 `malicious`，`suspicious` 为 `suspicious`，`white` 为 `benign`，`info` 或无数据为 `unknown`；`threat_level` 的 `0/1/2/3/4/5` 分别映射为 `unknown/info/low/medium/high/critical`。

平台返回数值置信度时归一为 `0-100` 并写入 `confidence_score`，返回高、中、低等级时写入 `confidence_level`；不根据风险等级反推置信度。平台没有给出某个字段时保留为空，不使用伪造默认值。`summary` 由适配器根据平台标签和判定字段确定性生成，不调用生成式服务。`raw_response` 保存成功响应的完整 JSON；持久化前执行凭据字段检查，防止第三方异常回显密钥。

## 凭据与安全

平台配置使用专用接口，不通过现有通用 `/api/settings` 返回。所有 `threat_intelligence.*` 设置键加入通用设置接口的保护列表，既不允许读取，也不允许通过该接口写入。

凭据使用 AES-256-GCM 加密后存入现有 `app_settings`。主密钥首次需要保存凭据时由系统安全随机生成，默认保存在 `/data/fwlog/threat-intelligence.key`，文件权限为 `0600`；该文件不进入 ClickHouse、日志、升级包或导出数据。密钥文件丢失或损坏时，现有凭据标记为不可用，用户重新输入即可恢复，不删除历史分析结果。

后端日志只记录平台、规范化 IP、耗时、结果类型和脱敏错误分类。禁止记录请求头、请求体、完整查询串、凭据、原始响应和加密文本。

所有新增接口沿用现有 FWLOG 登录鉴权。平台主机名和路径在代码中固定，不接受前端传入第三方 URL，避免形成任意外部请求能力。

## 数据存储

新增 ClickHouse 表 `threat_intelligence_results`：

```text
provider LowCardinality(String)
ip String
verdict LowCardinality(String)
risk_level LowCardinality(String)
confidence_score Nullable(Float64)
confidence_level LowCardinality(String)
tags Array(String)
first_seen Nullable(DateTime64(3, 'UTC'))
last_seen Nullable(DateTime64(3, 'UTC'))
source_updated_at Nullable(DateTime64(3, 'UTC'))
analyzed_at DateTime64(3, 'UTC')
summary String
raw_response String
```

表使用 `ReplacingMergeTree(analyzed_at)`，排序键为 `(provider, ip)`，不设置 TTL。读取时按 `(provider, ip)` 取得最新版本；ClickHouse 后台合并前可能暂存旧物理行，但接口始终只返回最新成功结果。只有完成解析和统一映射的成功响应才能写入该表。

新增独立存储接口，提供“按平台和 IP 读取最新结果”和“写入成功结果”两个操作。表创建属于向前兼容的增量变更，旧版本程序会忽略该表；回滚应用不需要删除历史结果。

## 后端接口

所有接口均需要登录鉴权。

### 平台状态

```text
GET /api/threat-intelligence/providers
```

返回四个平台的 `provider`、`name`、`enabled`、`configured`、`credential_error` 和最后连接测试状态，不返回凭据、密文或密钥长度。

```text
POST /api/threat-intelligence/providers/{provider}
```

保存 `enabled`、可选的新凭据和明确的清除标记。平台仅允许 `threatbook`、`nsfocus`、`qianxin`、`tencent`，其他值返回 `404`。

```text
POST /api/threat-intelligence/providers/{provider}/test
```

使用已保存凭据执行连接测试，返回成功状态或统一错误，不写分析结果。

### 本地结果与分析

```text
GET /api/threat-intelligence/providers/{provider}/results?ip=<IP>
```

校验并规范化 IP 后读取本地最新成功结果。没有记录时返回 `200` 和 `result: null`，使“无记录”与接口失败保持可区分。

```text
POST /api/threat-intelligence/providers/{provider}/analyze
Content-Type: application/json

{"ip":"203.0.113.50"}
```

校验平台状态和 IP，调用单个平台并在成功后保存统一结果。失败响应包含稳定的 `error` 代码和中文 `message`；如果存在旧结果，可同时返回 `previous_result` 供前端继续展示。

## 数据流

1. 查询页面加载时获取一次平台状态，据此决定显示哪些图标。
2. 用户点击某个平台图标，前端请求该平台和 IP 的本地结果。
3. 用户点击“开始分析”或“重新分析”，前端提交规范化前的目标 IP。
4. 后端解析 IP，拒绝不支持的地址类型，检查平台已启用且凭据可用。
5. 后端以 `(provider, normalized_ip)` 为键合并正在执行的相同请求，并调用对应适配器。
6. 适配器在超时范围内完成请求、错误分类和结果映射。
7. 成功结果写入 ClickHouse 后返回；失败不写入，并保留旧结果。

## 超时、并发与错误处理

- 每次第三方调用总超时为 15 秒，不做自动重试，避免重复消耗额度。
- 前端提交后禁用当前按钮；后端使用进程内 `singleflight` 合并同一平台、同一 IP 的并发分析。
- 不同平台或不同 IP 可以并行执行，互不阻塞。
- 错误统一为：`invalid_ip`、`unsupported_ip`、`provider_disabled`、`provider_not_configured`、`credential_unavailable`、`invalid_credential`、`quota_exhausted`、`rate_limited`、`timeout`、`provider_unavailable` 和 `invalid_response`。
- 腾讯 `1004` 映射为额度耗尽，`1005` 映射为频率限制；绿盟的无效 Key、禁止访问、超限、授权过期和授权 IP 不匹配分别映射为对应稳定错误。
- HTTP 非成功状态、业务错误码和无法解析的成功响应都不得写入结果表。
- 第三方成功返回“没有情报”时保存 `unknown`；网络或业务调用失败不保存为 `unknown`。

## IP 处理

后端使用标准 IP 解析库校验并输出规范形式。只允许公网单播地址：私网、环回、链路本地、组播、广播和未指定地址直接返回 `unsupported_ip`，不调用第三方。

微步、奇安信和腾讯支持 IPv4、IPv6；绿盟收到 IPv6 时直接返回“该平台暂不支持 IPv6 分析”。日志查询中的端口不进入分析请求，也不参与结果存储键。

## 测试与验收

GitHub Actions 不调用真实第三方接口。测试通过可注入的 HTTP Client 或本地 `httptest` 服务，使用按官方字段裁剪并脱敏的响应样例。

后端测试覆盖：

- 四个适配器的请求方法、固定地址、鉴权位置、参数和响应映射。
- 恶意、可疑、良性、无情报、额度不足、频率限制、鉴权失败、超时和异常响应。
- IPv4、IPv6、私网地址以及绿盟拒绝 IPv6。
- 结果首次写入、重新分析取得最新成功结果、失败不覆盖旧结果。
- 同一平台和 IP 的并发请求只产生一次外部调用。
- 普通 `/api/settings` 不能读取或覆盖凭据，接口和日志不泄露密钥。
- 新路由的鉴权、方法限制、参数校验和稳定错误结构。

前端测试覆盖：

- 仅为已启用且已配置的平台显示图标。
- 点击图标只读取本地结果，不触发分析。
- 无记录、有记录、分析中、成功、失败并保留旧结果等浮层状态。
- 开始分析和重新分析只提交一次，按钮在请求期间禁用。
- 平台名称、图标悬停提示和无障碍名称完整。

提交前执行：

```text
go test ./...
npm --prefix web test
npm --prefix web run build
```

真实平台验收按用户已拥有的账号逐个平台进行。没有账号的平台保持未配置，不阻塞发布；其模拟测试必须通过。真实验收确认连接测试、一次公网 IP 分析、结果持久化、重新分析和错误提示，不使用真实恶意流量执行测试。

## 发布与回滚

本功能包含新增表和新增接口，不改变现有日志表及查询接口。应用回滚时旧版本忽略新增表和受保护配置；重新升级后可继续读取已有结果。凭据主密钥文件和结果表不得在普通升级过程中删除。

版本发布遵循 FWLOG 现有策略：累计约五个已合并问题修复后统一发布补丁版本；若实现过程中发现凭据泄露、结果覆盖或升级阻断问题，则按安全或数据完整性问题单独处理。
