# 目标 IP 威胁情报分析实施计划

> **面向执行代理：** 必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans`，按任务逐项实施；使用本文的 `- [ ]` 复选框跟踪进度。

**目标：** 在日志查询结果中按需调用微步、绿盟、奇安信、腾讯四个平台分析目标公网 IP，并安全保存每个平台、每个 IP 的最新成功结果。

**架构：** 新增独立的 `internal/threatintel` 领域包，平台适配器、凭据加密和分析编排都通过小接口与 `server`、ClickHouse 解耦。前端使用独立的配置面板和查询浮层，浏览器只访问 FWLOG 后端；打开浮层只读本地记录，用户点击后才调用第三方。

**技术栈：** Go 1.21、`net/http`、`net/netip`、AES-256-GCM、`golang.org/x/sync/singleflight`、ClickHouse `ReplacingMergeTree`、React 18、TypeScript、Ant Design 5、Node 22 内置测试运行器、GitHub Actions。

## 全局约束

- 只处理目标 IP 威胁情报功能，不修改现有时间搜索逻辑、升级预览或其他用户改动。
- 当前工作树已有未提交修改，且与 `router.go`、`store.go`、`api.ts`、`LogSearchPage.tsx`、`styles.css` 等目标文件重叠；每次修改前读取现状，禁止回退或覆盖已有改动。
- 四个平台独立配置、独立点击、独立消耗额度；不增加一键查询、自动查询、定时刷新或自动重试。
- 第三方请求总超时固定为 15 秒；同一平台和规范化 IP 的并发分析只执行一次外部请求。
- 只分析公网单播 IP；微步、奇安信、腾讯支持 IPv4/IPv6，绿盟只支持 IPv4；端口永不传给第三方。
- 只有已启用且凭据可用的平台显示查询图标；打开浮层不调用第三方。
- 第三方失败不写结果，不覆盖旧成功记录；无情报的成功响应保存为 `unknown`。
- 凭据只存后端，使用 AES-256-GCM 加密；主密钥默认路径固定为 `/data/fwlog/threat-intelligence.key`，权限固定为 `0600`。
- 所有 `threat_intelligence.*` 设置键必须被通用 `/api/settings` 拒绝读写。
- GitHub Actions 只使用 `httptest` 和脱敏样例，不接触真实账号、密钥或付费接口。
- 不修改版本号；发布节奏继续遵循仓库现有补丁版本策略。

---

### 任务 1：领域类型、错误码与公网 IP 校验

**文件：**
- 新建：`internal/threatintel/types.go`
- 新建：`internal/threatintel/ip.go`
- 新建：`internal/threatintel/ip_test.go`

**接口：**
- 产出：`Provider`、`Result`、`ProviderStatus`、`ProviderConfig`、`ProviderConfigUpdate`、`AnalyzeOutcome`。
- 产出：`Adapter`、`ConfigStore`、`ResultStore` 接口，后续适配器、服务和服务器适配层共同使用。
- 产出：`NormalizePublicIP(raw string) (string, error)` 和 `ErrorCodeOf(error) ErrorCode`。

- [ ] **步骤 1：先写公网 IP 与类型约束测试**

```go
func TestNormalizePublicIP(t *testing.T) {
    tests := []struct{ raw, want string }{
        {"8.8.8.8", "8.8.8.8"},
        {"::ffff:8.8.8.8", "8.8.8.8"},
        {"2001:4860:4860::8888", "2001:4860:4860::8888"},
    }
    for _, tt := range tests {
        got, err := NormalizePublicIP(tt.raw)
        if err != nil || got != tt.want { t.Fatalf("NormalizePublicIP(%q) = %q, %v", tt.raw, got, err) }
    }
}

func TestNormalizePublicIPRejectsNonPublicAddresses(t *testing.T) {
    for _, raw := range []string{"", "not-an-ip", "10.0.0.1", "127.0.0.1", "169.254.1.1", "224.0.0.1", "255.255.255.255", "::"} {
        _, err := NormalizePublicIP(raw)
        if err == nil { t.Fatalf("NormalizePublicIP(%q) should fail", raw) }
    }
}
```

- [ ] **步骤 2：运行测试并确认因包或函数不存在而失败**

运行：`go test ./internal/threatintel -run 'TestNormalizePublicIP' -count=1`

预期：编译失败，提示 `NormalizePublicIP` 或领域类型未定义。

- [ ] **步骤 3：实现稳定领域契约**

```go
type Provider string

const (
    ProviderThreatBook Provider = "threatbook"
    ProviderNSFocus   Provider = "nsfocus"
    ProviderQianxin   Provider = "qianxin"
    ProviderTencent   Provider = "tencent"
)

type Result struct {
    Provider        Provider        `json:"provider"`
    IP              string          `json:"ip"`
    Verdict         string          `json:"verdict"`
    RiskLevel       string          `json:"risk_level"`
    ConfidenceScore *float64        `json:"confidence_score"`
    ConfidenceLevel string          `json:"confidence_level"`
    Tags            []string        `json:"tags"`
    FirstSeen       *time.Time      `json:"first_seen"`
    LastSeen        *time.Time      `json:"last_seen"`
    SourceUpdatedAt *time.Time      `json:"source_updated_at"`
    AnalyzedAt      time.Time       `json:"analyzed_at"`
    Summary         string          `json:"summary"`
    RawResponse     json.RawMessage `json:"raw_response"`
}

type ProviderStatus struct {
    Provider         Provider   `json:"provider"`
    Name             string     `json:"name"`
    Enabled          bool       `json:"enabled"`
    Configured       bool       `json:"configured"`
    CredentialError string     `json:"credential_error,omitempty"`
    LastTestStatus   string     `json:"last_test_status,omitempty"`
    LastTestMessage  string     `json:"last_test_message,omitempty"`
    LastTestedAt     *time.Time `json:"last_tested_at,omitempty"`
}

type ProviderConfig struct {
    Provider   Provider
    Enabled    bool
    Credential string
}

type ProviderConfigUpdate struct {
    Enabled         bool
    Credential      *string
    ClearCredential bool
}

type ProviderTestStatus struct {
    Status   string
    Message  string
    TestedAt time.Time
}

type AnalyzeOutcome struct {
    Result         *Result `json:"result,omitempty"`
    PreviousResult *Result `json:"previous_result,omitempty"`
}

type ErrorCode string

const (
    ErrorInvalidIP             ErrorCode = "invalid_ip"
    ErrorUnsupportedIP         ErrorCode = "unsupported_ip"
    ErrorProviderDisabled      ErrorCode = "provider_disabled"
    ErrorProviderNotConfigured ErrorCode = "provider_not_configured"
    ErrorCredentialUnavailable ErrorCode = "credential_unavailable"
    ErrorInvalidCredential     ErrorCode = "invalid_credential"
    ErrorQuotaExhausted        ErrorCode = "quota_exhausted"
    ErrorRateLimited           ErrorCode = "rate_limited"
    ErrorTimeout               ErrorCode = "timeout"
    ErrorProviderUnavailable   ErrorCode = "provider_unavailable"
    ErrorInvalidResponse       ErrorCode = "invalid_response"
    ErrorInternal              ErrorCode = "internal_error"
)

type ServiceError struct {
    Code    ErrorCode
    Message string
    Cause   error
}

func (e *ServiceError) Error() string { return e.Message }
func (e *ServiceError) Unwrap() error { return e.Cause }
func newServiceError(code ErrorCode, message string, cause error) error {
    return &ServiceError{Code: code, Message: message, Cause: cause}
}

type Adapter interface {
    Provider() Provider
    Analyze(context.Context, string, string) (Result, error)
}

type ConfigStore interface {
    Statuses(context.Context) ([]ProviderStatus, error)
    Config(context.Context, Provider) (ProviderConfig, error)
    Update(context.Context, Provider, ProviderConfigUpdate) (ProviderStatus, error)
    RecordTest(context.Context, Provider, ProviderTestStatus) error
}

type ResultStore interface {
    LatestResult(context.Context, Provider, string) (Result, bool, error)
    SaveResult(context.Context, Result) error
}
```

同时实现 `ParseProvider`、`ProviderName` 和 `ErrorCodeOf`。`NormalizePublicIP` 使用 `netip.ParseAddr`、`Unmap`、`IsGlobalUnicast`、`IsPrivate`、`IsLoopback`、`IsLinkLocalUnicast`、`IsMulticast`、`IsUnspecified`，并单独拒绝 `255.255.255.255`。`internal_error` 只用于 FWLOG 存储或配置故障；对外中文消息不能包含底层响应体。

- [ ] **步骤 4：运行领域测试**

运行：`go test ./internal/threatintel -count=1`

预期：通过。

- [ ] **步骤 5：提交领域契约**

```bash
git add internal/threatintel/types.go internal/threatintel/ip.go internal/threatintel/ip_test.go
git commit -m "feat(threat-intel): add domain contracts and IP validation"
```

---

### 任务 2：凭据主密钥、AES-GCM 加密与运行配置

**文件：**
- 新建：`internal/threatintel/credentials.go`
- 新建：`internal/threatintel/credentials_test.go`
- 修改：`internal/config/types.go:3-21`
- 修改：`internal/config/config.go:11-35`
- 修改：`internal/config/config_test.go:9-58`
- 修改：`fwlog.service:12-27`
- 修改：`packaging/systemd/fwlog.service:12-27`
- 修改：`packaging/build_server_packages_test.go`

**接口：**
- 产出：`NewCredentialCipher(path string) *CredentialCipher`。
- 产出：`Encrypt(plaintext string) (string, error)` 和 `Decrypt(ciphertext string) (string, error)`。
- 产出：`Config.ThreatIntelligenceKeyFile`，环境变量名为 `THREAT_INTELLIGENCE_KEY_FILE`。

- [ ] **步骤 1：写加密与配置失败测试**

```go
func TestCredentialCipherRoundTrip(t *testing.T) {
    path := filepath.Join(t.TempDir(), "threat-intelligence.key")
    cipher := NewCredentialCipher(path)
    encrypted, err := cipher.Encrypt("test-secret-value")
    if err != nil { t.Fatal(err) }
    if strings.Contains(encrypted, "test-secret-value") || !strings.HasPrefix(encrypted, "v1:") { t.Fatalf("unsafe ciphertext %q", encrypted) }
    plain, err := cipher.Decrypt(encrypted)
    if err != nil || plain != "test-secret-value" { t.Fatalf("Decrypt = %q, %v", plain, err) }
    if runtime.GOOS != "windows" {
        info, _ := os.Stat(path)
        if info.Mode().Perm() != 0o600 { t.Fatalf("key mode = %o", info.Mode().Perm()) }
    }
}

func TestCredentialCipherRejectsTampering(t *testing.T) {
    cipher := NewCredentialCipher(filepath.Join(t.TempDir(), "key"))
    encrypted, _ := cipher.Encrypt("secret")
    _, err := cipher.Decrypt(encrypted[:len(encrypted)-2] + "AA")
    if err == nil { t.Fatal("tampered ciphertext should fail") }
}
```

在 `config_test.go` 增加：清空环境变量时默认路径为 `/data/fwlog/threat-intelligence.key`；设置环境变量时读取自定义绝对路径；两个 systemd 模板都包含同名环境变量。在打包测试增加回归断言：升级安装脚本不得执行删除 `/data/fwlog/threat-intelligence.key` 的命令，生成的 systemd unit 必须保留该路径配置。

```go
func TestPackagesPreserveThreatIntelligenceKey(t *testing.T) {
    for _, path := range []string{"../fwlog.service", "systemd/fwlog.service"} {
        data, err := os.ReadFile(path)
        if err != nil { t.Fatal(err) }
        if !strings.Contains(string(data), `THREAT_INTELLIGENCE_KEY_FILE=/data/fwlog/threat-intelligence.key`) { t.Fatalf("%s misses key path", path) }
    }
    for _, path := range []string{"build-server-packages.sh", "rpm/fwlog.spec"} {
        data, err := os.ReadFile(path)
        if err != nil { t.Fatal(err) }
        if strings.Contains(string(data), "rm -f /data/fwlog/threat-intelligence.key") || strings.Contains(string(data), "rm -rf /data/fwlog") { t.Fatalf("%s deletes persistent key data", path) }
    }
}
```

- [ ] **步骤 2：运行并确认失败**

运行：`go test ./internal/threatintel ./internal/config -run 'Credential|ThreatIntelligence' -count=1`

预期：编译失败，提示 `CredentialCipher` 或 `ThreatIntelligenceKeyFile` 不存在。

- [ ] **步骤 3：实现密钥文件和 AES-256-GCM**

```go
const credentialPrefix = "v1:"

func (c *CredentialCipher) Encrypt(plaintext string) (string, error) {
    key, err := c.loadOrCreateKey()
    if err != nil { return "", err }
    block, err := aes.NewCipher(key)
    if err != nil { return "", err }
    gcm, err := cipher.NewGCM(block)
    if err != nil { return "", err }
    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil { return "", err }
    sealed := gcm.Seal(nonce, nonce, []byte(plaintext), []byte("fwlog-threat-intelligence-v1"))
    return credentialPrefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}
```

`loadOrCreateKey` 必须创建父目录为 `0700`、通过 `os.OpenFile(c.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)` 原子创建 32 字节随机密钥；解密时不自动重建丢失的旧密钥。Linux 上读取已存在密钥前验证权限不宽于 `0600`。

在两个 systemd 文件加入：

```ini
Environment="THREAT_INTELLIGENCE_KEY_FILE=/data/fwlog/threat-intelligence.key"
```

- [ ] **步骤 4：运行加密、配置和竞态测试**

运行：`go test -race ./internal/threatintel ./internal/config ./packaging -count=1`

预期：通过，临时密钥文件权限为 `0600`。

- [ ] **步骤 5：提交凭据基础设施**

```bash
git add internal/threatintel/credentials.go internal/threatintel/credentials_test.go internal/config/types.go internal/config/config.go internal/config/config_test.go fwlog.service packaging/systemd/fwlog.service packaging/build_server_packages_test.go
git commit -m "feat(threat-intel): encrypt provider credentials"
```

---

### 任务 3：平台配置仓库与通用设置隔离

**文件：**
- 新建：`internal/server/threat_intelligence_settings.go`
- 新建：`internal/server/threat_intelligence_settings_test.go`
- 修改：`internal/server/settings_controller.go:84-142,184-203`

**接口：**
- 消费：任务 1 的 `ConfigStore`、`ProviderConfigUpdate`、`ProviderStatus`。
- 消费：任务 2 的 `CredentialCipher`。
- 产出：`appThreatIntelligenceConfigStore`，供分析服务读取明文凭据、保存配置和记录连接测试状态。

- [ ] **步骤 1：写保护、保留、覆盖和清除测试**

```go
func TestThreatIntelligenceSettingsAreProtectedFromGenericSettings(t *testing.T) {
    app := NewApp(LoadConfig())
    app.settings["threat_intelligence.threatbook.credential"] = "v1:ciphertext"
    if _, ok := app.getSettings()["threat_intelligence.threatbook.credential"]; ok { t.Fatal("credential leaked") }

    updates, err := app.normalizeSettingsPayload(map[string]any{
        "threat_intelligence.threatbook.credential": "plaintext",
        "log_tag": "kept",
    })
    if err != nil { t.Fatal(err) }
    if _, ok := updates["threat_intelligence.threatbook.credential"]; ok { t.Fatal("protected key accepted") }
    if updates["log_tag"] != "kept" { t.Fatal("ordinary setting lost") }
}
```

配置仓库测试还必须断言：首次保存得到 `configured=true` 且内存/持久化值不含明文；`credential=nil` 或只含空白字符时保留旧密文；只有 `clear_credential=true` 才清空；密钥文件损坏时状态返回 `credential_error` 且不返回明文；四个平台顺序固定为微步、绿盟、奇安信、腾讯。

- [ ] **步骤 2：运行并确认失败**

运行：`go test ./internal/server -run 'ThreatIntelligenceSettings' -count=1`

预期：测试失败，通用设置仍暴露或接受 `threat_intelligence.*`。

- [ ] **步骤 3：实现前缀保护和配置仓库**

```go
func isProtectedSettingsKey(key string) bool {
    return protectedSettingsKeys[key] || strings.HasPrefix(key, "threat_intelligence.")
}

func threatIntelSettingKey(provider threatintel.Provider, field string) string {
    return "threat_intelligence." + string(provider) + "." + field
}
```

把 `getSettings`、`normalizeSettingsPayload`、`saveSettings` 中对 `protectedSettingsKeys[key]` 的直接判断全部改为 `isProtectedSettingsKey(key)`。专用配置仓库通过 `saveNormalizedSettings` 持久化成功后再调用 `applyNormalizedSettings`，不得先改内存再保存。连接测试状态保存 `last_test_status`、`last_test_message`、`last_tested_at`，凭据字段只保存 `v1:` 密文。

- [ ] **步骤 4：运行设置测试和既有路由测试**

运行：`go test ./internal/server -run 'ThreatIntelligenceSettings|Settings' -count=1`

预期：通过，既有 `/api/settings` 行为不回归。

- [ ] **步骤 5：提交配置隔离**

```bash
git add internal/server/threat_intelligence_settings.go internal/server/threat_intelligence_settings_test.go internal/server/settings_controller.go
git commit -m "feat(threat-intel): isolate provider settings"
```

---

### 任务 4：ClickHouse 最新成功结果存储

**文件：**
- 新建：`internal/storage/clickhouse/threat_intelligence_store.go`
- 新建：`internal/storage/clickhouse/threat_intelligence_store_test.go`
- 修改：`internal/storage/clickhouse/store.go:78-99,350-365,480-560`
- 修改：`internal/storage/clickhouse/store_test.go:10-30`

**接口：**
- 消费：任务 1 的 `threatintel.Result` 和 `ResultStore`。
- 产出：`LatestResult(context.Context, Provider, string)` 与 `SaveResult(context.Context, Result)`。

- [ ] **步骤 1：写 DDL、读取和写入 SQL 测试**

```go
func TestThreatIntelligenceDDLStoresLatestSuccessfulResult(t *testing.T) {
    ddl := strings.Join(ClickHouseDDL(), "\n")
    for _, want := range []string{
        "CREATE TABLE IF NOT EXISTS threat_intelligence_results",
        "confidence_score Nullable(Float64)",
        "confidence_level LowCardinality(String)",
        "ENGINE = ReplacingMergeTree(analyzed_at)",
        "ORDER BY (provider, ip)",
    } {
        if !strings.Contains(ddl, want) { t.Fatalf("DDL missing %q", want) }
    }
    if strings.Contains(ddl[strings.Index(ddl, "threat_intelligence_results"):], "TTL ") { t.Fatal("result table must be permanent") }
}

func TestThreatIntelligenceLatestQueryUsesFinal(t *testing.T) {
    sql := ThreatIntelligenceLatestSQL()
    for _, want := range []string{"FROM threat_intelligence_results FINAL", "WHERE provider = ? AND ip = ?", "LIMIT 1"} {
        if !strings.Contains(sql, want) { t.Fatalf("query missing %q", want) }
    }
}
```

同时在现有磁盘统计测试中断言 `ClickHouseDiskUsageSQL()` 包含 `threat_intelligence_results`，避免系统健康页漏算新增结果表。

- [ ] **步骤 2：运行并确认失败**

运行：`go test ./internal/storage/clickhouse -run 'ThreatIntelligence' -count=1`

预期：失败，DDL 和 SQL helper 尚不存在。

- [ ] **步骤 3：实现表和存储方法**

DDL 字段必须与规格一致：`provider`、`ip`、`verdict`、`risk_level`、可空分值、置信等级、标签数组、三个 UTC 可空时间、UTC `analyzed_at`、摘要和原始 JSON。写入只执行参数化 `INSERT`；读取使用 `FINAL` 并把空 `raw_response` 规范为 `{}`。把新表加入 `ClickHouseDiskUsageSQL()` 的表名白名单。

```go
func ThreatIntelligenceLatestSQL() string {
    return `SELECT provider, ip, verdict, risk_level, confidence_score, confidence_level,
tags, first_seen, last_seen, source_updated_at, analyzed_at, summary, raw_response
FROM threat_intelligence_results FINAL
WHERE provider = ? AND ip = ?
LIMIT 1`
}
```

- [ ] **步骤 4：运行 ClickHouse 存储测试**

运行：`go test ./internal/storage/clickhouse -run 'ThreatIntelligence|ClickHouseDDL' -count=1`

预期：通过，既有表定义仍在。

- [ ] **步骤 5：提交结果存储**

```bash
git add internal/storage/clickhouse/threat_intelligence_store.go internal/storage/clickhouse/threat_intelligence_store_test.go internal/storage/clickhouse/store.go internal/storage/clickhouse/store_test.go
git commit -m "feat(threat-intel): store latest successful results"
```

---

### 任务 5：公共 HTTP 解析与微步适配器

**文件：**
- 新建：`internal/threatintel/provider_http.go`
- 新建：`internal/threatintel/provider_http_test.go`
- 新建：`internal/threatintel/threatbook.go`
- 新建：`internal/threatintel/threatbook_test.go`

**接口：**
- 产出：`NewThreatBookAdapter(client *http.Client, endpoint string) Adapter`。
- 产出：供其余适配器使用的 HTTP 状态分类、JSON 读取、标签去重和原始响应凭据字段清理函数。

- [ ] **步骤 1：写请求、映射和脱敏测试**

```go
func TestThreatBookAdapterBuildsRequestAndMapsResult(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet || r.URL.Query().Get("resource") != "8.8.8.8" || r.URL.Query().Get("lang") != "zh" { t.Fatalf("request = %s %s", r.Method, r.URL.String()) }
        if r.URL.Query().Get("apikey") != "test-key" { t.Fatal("missing apikey") }
        io.WriteString(w, `{"response_code":0,"data":{"8.8.8.8":{"is_malicious":true,"confidence_level":"high","severity":"critical","judgments":["僵尸网络"],"update_time":"2026-08-30 10:20:30"}}}`)
    }))
    defer server.Close()

    result, err := NewThreatBookAdapter(server.Client(), server.URL).Analyze(context.Background(), "test-key", "8.8.8.8")
    if err != nil { t.Fatal(err) }
    if result.Verdict != "malicious" || result.RiskLevel != "critical" || result.ConfidenceLevel != "high" { t.Fatalf("result = %#v", result) }
}
```

公共测试使用响应 `{"apikey":"echoed-secret","data":{}}`，断言保存的 `RawResponse` 不含 `echoed-secret`；HTTP `401/403/429/500` 分别映射到稳定错误，不把响应体直接作为用户消息。

- [ ] **步骤 2：运行并确认失败**

运行：`go test ./internal/threatintel -run 'ProviderHTTP|ThreatBook' -count=1`

预期：编译失败，适配器构造函数不存在。

- [ ] **步骤 3：实现固定接口和结果映射**

生产默认端点固定为 `https://api.threatbook.cn/v3/scene/ip_reputation`。测试构造函数允许注入本地端点，但 HTTP API 不接受端点参数。请求日志不能记录 `r.URL.String()`。`is_malicious=false` 映射 `benign`；响应成功但目标键不存在时返回 `unknown` 成功结果。

```go
func (a *threatBookAdapter) Analyze(ctx context.Context, credential, ip string) (Result, error) {
    endpoint, err := url.Parse(a.endpoint)
    if err != nil { return Result{}, newServiceError(ErrorInternal, "微步接口配置无效", err) }
    query := endpoint.Query()
    query.Set("apikey", credential)
    query.Set("resource", ip)
    query.Set("lang", "zh")
    endpoint.RawQuery = query.Encode()
    request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
    if err != nil { return Result{}, newServiceError(ErrorInternal, "创建微步请求失败", err) }
    raw, err := doProviderJSON(a.client, request)
    if err != nil { return Result{}, err }
    return mapThreatBookResponse(ip, raw)
}
```

- [ ] **步骤 4：运行适配器测试**

运行：`go test ./internal/threatintel -run 'ProviderHTTP|ThreatBook' -count=1`

预期：通过。

- [ ] **步骤 5：提交微步适配器**

```bash
git add internal/threatintel/provider_http.go internal/threatintel/provider_http_test.go internal/threatintel/threatbook.go internal/threatintel/threatbook_test.go
git commit -m "feat(threat-intel): add ThreatBook adapter"
```

---

### 任务 6：绿盟 IPv4 适配器

**文件：**
- 新建：`internal/threatintel/nsfocus.go`
- 新建：`internal/threatintel/nsfocus_test.go`

**接口：**
- 产出：`NewNSFocusAdapter(client *http.Client, endpoint string, now func() time.Time) Adapter`。

- [ ] **步骤 1：写鉴权、有效对象、过期对象和 IPv6 测试**

```go
func TestNSFocusAdapterUsesHeaderAndHighestActiveThreat(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Header.Get("X-Ns-Nti-Key") != "test-key" { t.Fatal("missing X-Ns-Nti-Key") }
        if r.Header.Get("Accept") != "application/nsfocus.nti.spec+json; version=2.0" { t.Fatal("wrong Accept") }
        if r.URL.Query().Get("query") != "8.8.8.8" { t.Fatal("wrong query") }
        io.WriteString(w, `{"count":2,"objects":[{"revoked":false,"valid_until":"2026-09-30T00:00:00Z","confidence":88,"threat_level":5,"categories":["botnet"],"threat_types":["c2"],"tags":["active"],"modified":"2026-08-30T00:00:00Z"},{"revoked":true,"threat_level":5}]}`)
    }))
    defer server.Close()
    adapter := NewNSFocusAdapter(server.Client(), server.URL, func() time.Time { return time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC) })
    result, err := adapter.Analyze(context.Background(), "test-key", "8.8.8.8")
    if err != nil || result.Verdict != "malicious" || result.RiskLevel != "high" { t.Fatalf("result = %#v, %v", result, err) }
}
```

另写测试：只有撤销/过期对象时为 `unknown`；IPv6 不启动测试服务器请求并返回 `unsupported_ip`；`Invalid NTI key`、`Forbidden`、`Over limit`、`Authorization expired` 映射为对应中文稳定错误。

- [ ] **步骤 2：运行并确认失败**

运行：`go test ./internal/threatintel -run 'NSFocus' -count=1`

预期：编译失败，`NewNSFocusAdapter` 不存在。

- [ ] **步骤 3：实现绿盟适配器**

默认端点固定为 `https://nti.nsfocus.com/api/v2/objects/ioc-ipv4/`。只聚合未撤销且 `valid_until` 未过期的对象；`threat_level` 的 `1/3/5` 映射 `low/medium/high`；置信分值保持 `0-100`；标签由 `categories`、`threat_types`、`act_types`、`tags` 去重并稳定排序。

```go
func nsfocusRisk(level int) string {
    switch level {
    case 5: return "high"
    case 3: return "medium"
    case 1: return "low"
    default: return "unknown"
    }
}

func nsfocusObjectActive(revoked bool, validUntil *time.Time, now time.Time) bool {
    return !revoked && (validUntil == nil || !validUntil.Before(now))
}
```

- [ ] **步骤 4：运行绿盟测试**

运行：`go test ./internal/threatintel -run 'NSFocus' -count=1`

预期：通过。

- [ ] **步骤 5：提交绿盟适配器**

```bash
git add internal/threatintel/nsfocus.go internal/threatintel/nsfocus_test.go
git commit -m "feat(threat-intel): add NSFocus adapter"
```

---

### 任务 7：奇安信信誉适配器

**文件：**
- 新建：`internal/threatintel/qianxin.go`
- 新建：`internal/threatintel/qianxin_test.go`

**接口：**
- 产出：`NewQianxinAdapter(client *http.Client, endpoint string) Adapter`。

- [ ] **步骤 1：写 IPv4/IPv6 请求和信誉映射测试**

```go
func TestQianxinAdapterRequestsFullReputation(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Header.Get("Api-Key") != "test-key" || r.URL.Query().Get("param") != "2001:4860:4860::8888" || r.URL.Query().Get("mode") != "0" { t.Fatalf("bad request %s", r.URL.String()) }
        io.WriteString(w, `{"status":10000,"message":"success","data":{"summary_info":{"reputation":"suspicious","latest_reputation_time":"2026-08-30 08:00:00","malicious_label":["扫描"]},"normal_info":{},"geo":{}}}`)
    }))
    defer server.Close()
    result, err := NewQianxinAdapter(server.Client(), server.URL).Analyze(context.Background(), "test-key", "2001:4860:4860::8888")
    if err != nil || result.Verdict != "suspicious" { t.Fatalf("result = %#v, %v", result, err) }
}
```

再以表驱动测试验证 `malicious/suspicious/benign/unknown` 四种值直接映射；成功但缺少 `summary_info` 时保存 `unknown`；非成功业务状态不写成 `unknown`。

- [ ] **步骤 2：运行并确认失败**

运行：`go test ./internal/threatintel -run 'Qianxin' -count=1`

预期：编译失败，构造函数不存在。

- [ ] **步骤 3：实现奇安信适配器**

默认端点固定为 `https://webapi.ti.qianxin.com/ip/v3/reputation`，固定 `mode=0`。标签来源固定为 `malicious_label`；奇安信未返回统一风险等级或置信度时保留 `unknown`/空值，不根据信誉反推。

```go
func qianxinVerdict(value string) string {
    switch strings.ToLower(strings.TrimSpace(value)) {
    case "malicious", "suspicious", "benign":
        return strings.ToLower(strings.TrimSpace(value))
    default:
        return "unknown"
    }
}
```

- [ ] **步骤 4：运行奇安信测试**

运行：`go test ./internal/threatintel -run 'Qianxin' -count=1`

预期：通过。

- [ ] **步骤 5：提交奇安信适配器**

```bash
git add internal/threatintel/qianxin.go internal/threatintel/qianxin_test.go
git commit -m "feat(threat-intel): add Qianxin adapter"
```

---

### 任务 8：腾讯 IPAnalysis 适配器与默认注册表

**文件：**
- 新建：`internal/threatintel/tencent.go`
- 新建：`internal/threatintel/tencent_test.go`
- 新建：`internal/threatintel/providers.go`

**接口：**
- 产出：`NewTencentAdapter(client *http.Client, endpoint string) Adapter`。
- 产出：`DefaultAdapters(client *http.Client) map[Provider]Adapter`，四个平台均使用官方固定端点。

- [ ] **步骤 1：写 POST 请求体、判定和额度错误测试**

```go
func TestTencentAdapterPostsIPAnalysis(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        var body map[string]any
        json.NewDecoder(r.Body).Decode(&body)
        if r.Method != http.MethodPost || body["c_action"] != "IPAnalysis" || body["c_appkey"] != "test-key" || body["key"] != "8.8.8.8" || body["type"] != "ip" || body["option"] != float64(0) { t.Fatalf("body = %#v", body) }
        io.WriteString(w, `{"return_code":0,"return_msg":"success","result":"black","threat_level":5,"confidence":96,"tags":["C2"],"first_seen":"2026-08-01T00:00:00Z","last_seen":"2026-08-30T00:00:00Z"}`)
    }))
    defer server.Close()
    result, err := NewTencentAdapter(server.Client(), server.URL).Analyze(context.Background(), "test-key", "8.8.8.8")
    if err != nil || result.Verdict != "malicious" || result.RiskLevel != "critical" { t.Fatalf("result = %#v, %v", result, err) }
}
```

表驱动断言：`black/suspicious/white/info` 映射为 `malicious/suspicious/benign/unknown`；风险 `0..5` 映射为 `unknown/info/low/medium/high/critical`；`1004` 为 `quota_exhausted`，`1005` 为 `rate_limited`。

- [ ] **步骤 2：运行并确认失败**

运行：`go test ./internal/threatintel -run 'Tencent|DefaultAdapters' -count=1`

预期：编译失败，腾讯适配器或注册表不存在。

- [ ] **步骤 3：实现腾讯适配器和固定注册表**

请求体严格固定为：

```go
map[string]any{
    "c_version": "3.0", "c_action": "IPAnalysis", "c_appkey": credential,
    "c_lang": "zh", "type": "ip", "key": ip, "option": 0,
}
```

默认端点固定为 `https://xti.qq.com/api/v3/ti`。`DefaultAdapters` 创建四个适配器，不允许调用方覆盖生产端点。

- [ ] **步骤 4：运行全部适配器测试**

运行：`go test ./internal/threatintel -run 'ProviderHTTP|ThreatBook|NSFocus|Qianxin|Tencent|DefaultAdapters' -count=1`

预期：通过。

- [ ] **步骤 5：提交腾讯适配器**

```bash
git add internal/threatintel/tencent.go internal/threatintel/tencent_test.go internal/threatintel/providers.go
git commit -m "feat(threat-intel): add Tencent adapter registry"
```

---

### 任务 9：分析服务、15 秒超时和并发合并

**文件：**
- 新建：`internal/threatintel/service.go`
- 新建：`internal/threatintel/service_test.go`
- 修改：`go.mod`
- 修改：`go.sum`
- 修改：`internal/server/server.go:19-49,65-93`
- 新建：`internal/server/threat_intelligence_adapter.go`

**接口：**
- 消费：任务 1 的三个接口、任务 3 的配置仓库、任务 4 的结果存储、任务 8 的默认适配器。
- 产出：`NewService(config ConfigStore, results ResultStore, adapters map[Provider]Adapter, timeout time.Duration) *Service`。
- 产出：`Providers`、`UpdateProvider`、`TestProvider`、`Result`、`Analyze` 五个服务方法。

- [ ] **步骤 1：添加依赖并写服务失败测试**

运行：`go get golang.org/x/sync@v0.8.0`

并写测试：

```go
func TestServiceCoalescesConcurrentAnalysisAndSavesOnce(t *testing.T) {
    adapter := &fakeAdapter{provider: ProviderThreatBook, result: Result{Verdict: "malicious", RawResponse: json.RawMessage(`{}`)}}
    store := &fakeResultStore{}
    service := NewService(configuredStore(ProviderThreatBook), store, map[Provider]Adapter{ProviderThreatBook: adapter}, 15*time.Second)
    var wg sync.WaitGroup
    for i := 0; i < 8; i++ {
        wg.Add(1)
        go func() { defer wg.Done(); _, _ = service.Analyze(context.Background(), ProviderThreatBook, "8.8.8.8") }()
    }
    wg.Wait()
    if adapter.calls.Load() != 1 || store.saveCalls != 1 { t.Fatalf("calls = %d, saves = %d", adapter.calls.Load(), store.saveCalls) }
}
```

另写测试：打开本地结果只读存储；禁用/未配置平台不调用适配器；失败返回旧结果且不保存；成功 `unknown` 会保存；测试连接固定查询 `1.1.1.1` 且不保存结果；超时返回 `timeout`；绿盟 IPv6 返回 `unsupported_ip`；客户端取消后已开始的共享请求仍在 15 秒内完成保存。

- [ ] **步骤 2：运行并确认失败**

运行：`go test ./internal/threatintel -run 'Service' -count=1`

预期：编译失败，`NewService` 不存在。

- [ ] **步骤 3：实现服务与 App 存储适配器**

```go
type Service struct {
    config   ConfigStore
    results  ResultStore
    adapters map[Provider]Adapter
    timeout  time.Duration
    flights  singleflight.Group
}

func (s *Service) Analyze(ctx context.Context, provider Provider, rawIP string) (AnalyzeOutcome, error) {
    ip, err := NormalizePublicIP(rawIP)
    if err != nil { return AnalyzeOutcome{}, err }
    value, err, _ := s.flights.Do(string(provider)+"\x00"+ip, func() (any, error) {
        callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.timeout)
        defer cancel()
        return s.analyzeOnce(callCtx, provider, ip)
    })
    outcome, _ := value.(AnalyzeOutcome)
    return outcome, err
}
```

`analyzeOnce` 先读取旧结果，再验证启用状态和凭据，再调用适配器；只有适配器成功且 `SaveResult` 成功才返回新结果。`TestProvider` 允许平台处于停用状态，但必须已配置凭据；它调用固定 IP 并记录测试状态，不调用 `SaveResult`。

在 `App` 增加可注入的 `threatIntelligenceService` 字段。`NewApp` 构造配置仓库、ClickHouse 转发适配器、默认 HTTP Client 和 15 秒服务；不得在启动时创建密钥或调用外部接口。

- [ ] **步骤 4：运行服务竞态测试**

运行：`go test -race ./internal/threatintel ./internal/server -run 'Service|ThreatIntelligence' -count=1`

预期：通过，无数据竞争，同键只调用一次。

- [ ] **步骤 5：提交分析服务**

```bash
git add go.mod go.sum internal/threatintel/service.go internal/threatintel/service_test.go internal/server/server.go internal/server/threat_intelligence_adapter.go
git commit -m "feat(threat-intel): orchestrate on-demand analysis"
```

---

### 任务 10：鉴权 HTTP 接口与稳定错误响应

**文件：**
- 新建：`internal/server/threat_intelligence_controller.go`
- 新建：`internal/server/threat_intelligence_controller_test.go`
- 修改：`internal/server/router.go:10-34`
- 修改：`internal/server/routes_test.go:20-57`

**接口：**
- 消费：任务 9 的服务方法。
- 产出：设计规格中的五类 URL：平台列表、配置保存、连接测试、本地结果、主动分析。

- [ ] **步骤 1：写路径、鉴权、方法和响应测试**

```go
func TestThreatIntelligenceResultDoesNotAnalyze(t *testing.T) {
    service := &fakeThreatIntelligenceService{localResult: &threatintel.Result{Provider: threatintel.ProviderThreatBook, IP: "8.8.8.8", Verdict: "benign"}}
    app := NewApp(LoadConfig())
    app.threatIntelligenceService = service
    req := httptest.NewRequest(http.MethodGet, "/api/threat-intelligence/providers/threatbook/results?ip=8.8.8.8", nil)
    res := httptest.NewRecorder()
    threatIntelligenceHandler(app).ServeHTTP(res, req)
    if res.Code != http.StatusOK || service.analyzeCalls != 0 { t.Fatalf("status=%d analyze=%d", res.Code, service.analyzeCalls) }
}
```

路由测试覆盖：未登录访问全部新接口得到 `401`；未知平台 `404`；错误方法 `405` 并带 `Allow`；无效 JSON/IP `400`；同时提交非空 `credential` 与 `clear_credential=true` 返回 `400`；额度/频率 `429`；第三方不可用 `502`；超时 `504`；内部存储故障 `500`。配置响应和 `/api/settings` 均不得包含 `credential` 明文或密文。

- [ ] **步骤 2：运行并确认失败**

运行：`go test ./internal/server -run 'ThreatIntelligence|RouterRegistersAPIRoutes' -count=1`

预期：新路由返回 `404` 或处理器未定义。

- [ ] **步骤 3：实现动态路径分发**

Go 版本为 1.21，使用两个 ServeMux 注册项覆盖根路径和子路径：

```go
mux.Handle("/api/threat-intelligence/providers", a.requireAuth(threatIntelligenceHandler(a)))
mux.Handle("/api/threat-intelligence/providers/", a.requireAuth(threatIntelligenceHandler(a)))
```

配置请求体固定为：

```go
type providerUpdateRequest struct {
    Enabled         bool    `json:"enabled"`
    Credential      *string `json:"credential"`
    ClearCredential bool    `json:"clear_credential"`
}
```

本地无结果返回 `200` 和 `{"result":null}`。分析失败时响应包含 `error`、中文 `message` 和可空 `previous_result`。错误响应不得包含第三方原始响应、请求 URL 或底层 error 字符串。

平台列表响应固定为 `{"providers":[]}` 这一对象结构，数组中元素类型为 `ProviderStatus`。第三方凭据无效、凭据文件不可用、平台停用或未配置统一返回 `422`，不得返回 `401`，避免前端把第三方鉴权失败误判为 FWLOG 登录失效。只有 `requireAuth` 可以为这些路由返回 `401`。

- [ ] **步骤 4：运行服务器测试**

运行：`go test ./internal/server -run 'ThreatIntelligence|RouterRegistersAPIRoutes|Settings' -count=1`

预期：通过。

- [ ] **步骤 5：提交 HTTP 接口**

```bash
git add internal/server/threat_intelligence_controller.go internal/server/threat_intelligence_controller_test.go internal/server/router.go internal/server/routes_test.go
git commit -m "feat(threat-intel): expose authenticated provider APIs"
```

---

### 任务 11：前端类型、展示映射与本地 Mock

**文件：**
- 新建：`web/src/threatIntelligence.ts`
- 新建：`web/tests/threatIntelligencePresentation.test.ts`
- 修改：`web/src/api.ts:1-50,60-320`
- 修改：`web/tests/apiMockCopy.test.ts`

**接口：**
- 产出：`ThreatProvider`、`ThreatProviderStatus`、`ThreatIntelligenceResult`、`ThreatAnalysisResponse`。
- 产出：`THREAT_PROVIDER_META`、`visibleThreatProviders`、`verdictText`、`riskText`、`riskColor`、`formatConfidence`。
- 产出：开发 Mock 的平台列表、配置、测试、本地结果和分析响应。

- [ ] **步骤 1：写纯函数和 Mock 路径测试**

```ts
test('shows only enabled providers with usable credentials', () => {
  const statuses = [
    { provider: 'threatbook', name: '微步', enabled: true, configured: true },
    { provider: 'nsfocus', name: '绿盟', enabled: false, configured: true },
    { provider: 'qianxin', name: '奇安信', enabled: true, configured: false },
    { provider: 'tencent', name: '腾讯', enabled: true, configured: true, last_test_status: 'failed' },
  ] satisfies ThreatProviderStatus[];
  assert.deepEqual(visibleThreatProviders(statuses).map((item) => item.provider), ['threatbook']);
});

test('renders stable Chinese verdict and risk copy', () => {
  assert.equal(verdictText('malicious'), '恶意');
  assert.equal(verdictText('unknown'), '未知');
  assert.equal(riskText('critical'), '严重');
  assert.equal(formatConfidence(96, 'unknown'), '96');
  assert.equal(formatConfidence(null, 'high'), '高');
});
```

`apiMockCopy.test.ts` 读取 `api.ts`，断言存在 `/api/threat-intelligence/providers`、`/results`、`/analyze`、`/test`，并断言 Mock 凭据仅使用 `test-key`，不出现真实域名请求。

- [ ] **步骤 2：运行并确认失败**

运行：`npm --prefix web test`

预期：模块或导出不存在。

- [ ] **步骤 3：实现类型、映射和 Mock 状态**

```ts
export const THREAT_PROVIDER_META = {
  threatbook: { name: '微步' },
  nsfocus: { name: '绿盟' },
  qianxin: { name: '奇安信' },
  tencent: { name: '腾讯' },
} as const;

export type ThreatProvider = keyof typeof THREAT_PROVIDER_META;

export type ThreatProviderStatus = {
  provider: ThreatProvider;
  name: string;
  enabled: boolean;
  configured: boolean;
  credential_error?: string;
  last_test_status?: 'success' | 'failed' | '';
  last_test_message?: string;
  last_tested_at?: string;
};

export type ThreatIntelligenceResult = {
  provider: ThreatProvider;
  ip: string;
  verdict: 'malicious' | 'suspicious' | 'benign' | 'unknown';
  risk_level: 'critical' | 'high' | 'medium' | 'low' | 'info' | 'unknown';
  confidence_score: number | null;
  confidence_level: 'high' | 'medium' | 'low' | 'unknown';
  tags: string[];
  first_seen: string | null;
  last_seen: string | null;
  source_updated_at: string | null;
  analyzed_at: string;
  summary: string;
  raw_response: unknown;
};

export type ThreatProviderListResponse = { providers: ThreatProviderStatus[] };
export type ThreatResultResponse = { result: ThreatIntelligenceResult | null };
export type ThreatAnalysisResponse = { result?: ThreatIntelligenceResult; previous_result?: ThreatIntelligenceResult };

export function visibleThreatProviders(statuses: ThreatProviderStatus[]) {
  return statuses.filter((item) => item.enabled && item.configured && !item.credential_error && item.last_test_status !== 'failed');
}
```

Mock 初始状态只启用微步和腾讯，以便同时验证“显示”和“隐藏”。读取结果返回固定的 `8.8.8.8` 良性记录；分析返回新的成功记录；连接测试返回 `last_test_status: "success"`。Mock 保存配置时只保存 `configured` 布尔状态，不保留输入凭据。

- [ ] **步骤 4：运行前端测试与类型构建**

运行：`npm --prefix web test`

运行：`npm --prefix web run build`

预期：全部通过。

- [ ] **步骤 5：提交前端契约**

```bash
git add web/src/threatIntelligence.ts web/tests/threatIntelligencePresentation.test.ts web/src/api.ts web/tests/apiMockCopy.test.ts
git commit -m "feat(threat-intel): add frontend data contracts"
```

---

### 任务 12：系统维护平台配置面板

**文件：**
- 新建：`web/src/components/ThreatIntelligenceSettingsPanel.tsx`
- 新建：`web/tests/threatIntelligenceSettings.test.ts`
- 修改：`web/src/pages/SystemMaintenancePage.tsx:1-23,599-1090`
- 修改：`web/tests/maintenanceLayout.test.ts:6-62`
- 修改：`web/src/styles.css`

**接口：**
- 消费：任务 11 的平台类型和 `apiGet/apiPost`。
- 产出：`ThreatIntelligenceSettingsPanel`，自行加载和保存专用配置，不进入维护页通用 Form 数据。

- [ ] **步骤 1：写配置面板结构与安全文案测试**

```ts
test('maintenance page exposes independent threat intelligence settings', () => {
  const page = fs.readFileSync(path.resolve('src/pages/SystemMaintenancePage.tsx'), 'utf8');
  const panel = fs.readFileSync(path.resolve('src/components/ThreatIntelligenceSettingsPanel.tsx'), 'utf8');
  assert.match(page, /key: 'threat-intelligence'/);
  assert.match(page, /威胁情报/);
  for (const name of ['微步', '绿盟', '奇安信', '腾讯']) assert.match(panel, new RegExp(name));
  assert.match(panel, /连接测试可能消耗 1 次接口额度/);
  assert.match(panel, /clear_credential: true/);
  assert.doesNotMatch(panel, /value=\{provider\.credential\}/);
});
```

更新维护布局测试：面板 section 数量从 5 调整为 6；新增页签名称和面板 note；现有五个页签断言保持。

- [ ] **步骤 2：运行并确认失败**

运行：`npm --prefix web test`

预期：组件文件或页签不存在。

- [ ] **步骤 3：实现独立配置面板**

每个平台重复项只使用一层 `.threat-provider-row`，包含平台名称、启用开关、`Input.Password`、状态 Tag、保存按钮、连接测试按钮和清除凭据图标按钮。输入框初始值永远为空，保存成功后立即清空局部输入。连接测试使用 `Popconfirm` 明示额度影响；绿盟额外显示公网 IP 授权提示。

```tsx
<Popconfirm
  title="确认测试连接？"
  description="连接测试可能消耗 1 次接口额度"
  okText="开始测试"
  cancelText="取消"
  onConfirm={() => void testProvider(provider.provider)}
>
  <Button icon={<ApiOutlined />}>连接测试</Button>
</Popconfirm>
```

维护页新增带 `SafetyCertificateOutlined` 或现有安全类图标的“威胁情报”页签，children 只渲染该组件；顶部通用“保存”按钮不提交此面板。

- [ ] **步骤 4：运行维护页测试和构建**

运行：`npm --prefix web test`

运行：`npm --prefix web run build`

预期：通过，无嵌套 Form 警告对应代码结构。

- [ ] **步骤 5：提交配置面板**

```bash
git add web/src/components/ThreatIntelligenceSettingsPanel.tsx web/tests/threatIntelligenceSettings.test.ts web/src/pages/SystemMaintenancePage.tsx web/tests/maintenanceLayout.test.ts web/src/styles.css
git commit -m "feat(threat-intel): add provider settings panel"
```

---

### 任务 13：查询结果平台图标与按需分析浮层

**文件：**
- 新建：`web/src/components/ThreatIntelligencePopover.tsx`
- 新建：`web/tests/threatIntelligencePopover.test.ts`
- 新建：`web/src/assets/threat-intelligence/threatbook.ico`
- 新建：`web/src/assets/threat-intelligence/nsfocus.ico`
- 新建：`web/src/assets/threat-intelligence/qianxin.ico`
- 新建：`web/src/assets/threat-intelligence/tencent.ico`
- 修改：`web/src/pages/LogSearchPage.tsx:1-10,206-321,336-344`
- 修改：`web/tests/searchResultPresentation.test.ts`
- 修改：`web/src/styles.css`

**接口：**
- 消费：任务 11 的平台状态、结果类型和展示映射。
- 产出：`ThreatIntelligenceActions({ ip, providers })`，目标 IP 单元格直接使用。

- [ ] **步骤 1：下载并固定官方站点图标资源**

在 PowerShell 执行：

```powershell
New-Item -ItemType Directory -Force web/src/assets/threat-intelligence
Invoke-WebRequest https://x.threatbook.com/favicon.ico -OutFile web/src/assets/threat-intelligence/threatbook.ico
Invoke-WebRequest https://ti.nsfocus.com/favicon.ico -OutFile web/src/assets/threat-intelligence/nsfocus.ico
Invoke-WebRequest https://ti.qianxin.com/container/favicon.ico -OutFile web/src/assets/threat-intelligence/qianxin.ico
Invoke-WebRequest https://tix.qq.com/favicon.ico -OutFile web/src/assets/threat-intelligence/tencent.ico
```

逐个确认文件非空且能被浏览器解码；应用运行时只引用本地资源。

- [ ] **步骤 2：写“打开只读、点击才分析”的源码与展示测试**

```ts
test('popover loads local history before explicit analysis', () => {
  const component = fs.readFileSync(path.resolve('src/components/ThreatIntelligencePopover.tsx'), 'utf8');
  assert.match(component, /\/results\?ip=/);
  assert.match(component, /开始分析/);
  assert.match(component, /重新分析/);
  assert.match(component, /onClick=\{\(\) => void analyze\(\)\}/);
  assert.doesNotMatch(component, /onOpenChange=\{[^}]*analyze/s);
  assert.match(component, /本次分析失败/);
  assert.match(component, /原始详情/);
});
```

`searchResultPresentation.test.ts` 增加断言：初始加载独立请求平台状态；目标列渲染 `ThreatIntelligenceActions`；传入的是 `row.dst_ip`，不包含 `row.dst_port`；平台状态加载失败只隐藏图标，不阻断日志状态加载。

- [ ] **步骤 3：运行并确认失败**

运行：`npm --prefix web test`

预期：组件不存在或目标列未接入。

- [ ] **步骤 4：实现图标和固定尺寸浮层**

```tsx
<Tooltip title={meta.name}>
  <Button
    type="text"
    shape="circle"
    className="threat-provider-trigger"
    aria-label={`${meta.name}威胁情报`}
    icon={<img src={PROVIDER_ICONS[provider.provider]} alt="" />}
  />
</Tooltip>
```

组件顶部固定本地图标映射，禁止运行时拼接远程 URL：

```tsx
import threatbookIcon from '../assets/threat-intelligence/threatbook.ico';
import nsfocusIcon from '../assets/threat-intelligence/nsfocus.ico';
import qianxinIcon from '../assets/threat-intelligence/qianxin.ico';
import tencentIcon from '../assets/threat-intelligence/tencent.ico';

const PROVIDER_ICONS: Record<ThreatProvider, string> = {
  threatbook: threatbookIcon,
  nsfocus: nsfocusIcon,
  qianxin: qianxinIcon,
  tencent: tencentIcon,
};
```

每个平台使用独立 `Popover`。首次打开调用 `GET /api/threat-intelligence/providers/{provider}/results?ip={ip}`；“开始分析/重新分析”调用 `POST /api/threat-intelligence/providers/{provider}/analyze`。浮层宽度固定 `380px`、移动端最大宽度 `calc(100vw - 24px)`；加载、空、成功、失败保留旧结果四种状态不改变触发器尺寸。结果使用 `Tag`、紧凑键值布局和 `Collapse` 展示格式化原始 JSON，不渲染 HTML。

查询页平台状态使用单独 `useEffect` 加载，失败时 `setThreatProviders([])`，不得复用会触发“加载入库状态失败”的现有 `try/catch`。目标列宽度按图标数量设置为稳定的 `240`，IP/端口和图标允许换行但不重叠。

- [ ] **步骤 5：运行前端全量测试和构建**

运行：`npm --prefix web test`

运行：`npm --prefix web run build`

预期：通过，Vite 正确打包四个本地图标。

- [ ] **步骤 6：提交查询交互**

```bash
git add web/src/components/ThreatIntelligencePopover.tsx web/tests/threatIntelligencePopover.test.ts web/src/assets/threat-intelligence web/src/pages/LogSearchPage.tsx web/tests/searchResultPresentation.test.ts web/src/styles.css
git commit -m "feat(threat-intel): add on-demand IP analysis popovers"
```

---

## 最终验证

- [ ] **步骤 1：格式和静态差异检查**

运行：`gofmt -w (Get-ChildItem -Path internal/threatintel/*.go,internal/server/threat_intelligence*.go,internal/storage/clickhouse/threat_intelligence*.go | Select-Object -ExpandProperty FullName)`

运行：`git diff --check`

预期：无输出。

- [ ] **步骤 2：前端测试和生产构建**

运行：`npm --prefix web test`

运行：`npm --prefix web run build`

预期：全部测试通过，TypeScript 和 Vite 构建成功。

- [ ] **步骤 3：Go 全量竞态测试和构建**

运行：`go test -race -count=1 ./...`

运行：`go build -o "$env:TEMP\fwlog-threat-intel-check.exe" ./cmd/fwlog`

预期：全部通过；测试日志中没有真实第三方请求。

- [ ] **步骤 4：GitHub Actions 配置核对**

确认 `.github/workflows/ci.yml` 仍依次运行 `npm run build`、`npm test`、`go test -race -count=1 ./...`，且未添加任何平台凭据或真实接口调用。无需修改工作流文件。

- [ ] **步骤 5：Mock 浏览器验收**

在 PowerShell 启动：

```powershell
$env:VITE_USE_MOCK='true'
npm --prefix web run dev -- --host 0.0.0.0 --port 4173
```

访问 `http://127.0.0.1:4173`，分别以 `1440x900` 和 `390x844` 验证：

1. 日志查询目标列只显示 Mock 中已启用且已配置的平台图标。
2. 悬停图标显示平台名称；点击后先显示本地记录，Network 中没有 `/analyze`。
3. 点击“重新分析”后只出现一次 `/analyze`，按钮在请求期间不可重复点击。
4. 系统维护的威胁情报页签显示四个平台、脱敏状态、额度提示和绿盟公网 IP 授权提示。
5. 桌面和移动端没有文本溢出、浮层越界、按钮抖动或元素重叠。

- [ ] **步骤 6：可用凭据的单平台真实验收**

只对用户已经拥有账号的平台执行：保存凭据、连接测试、分析一个普通公网 IP、刷新页面读取历史结果、重新分析。未配置平台保持隐藏，不影响验收结论；不得为验收申请付费账号或把凭据写入命令、截图、日志、测试文件。
