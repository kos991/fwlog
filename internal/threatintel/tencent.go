package threatintel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const tencentDefaultEndpoint = "https://xti.qq.com/api/v3/ti"

type tencentAdapter struct {
	client   *http.Client
	endpoint string
}

func NewTencentAdapter(client *http.Client, endpoint string) Adapter {
	if client == nil {
		client = http.DefaultClient
	}
	if endpoint == "" {
		endpoint = tencentDefaultEndpoint
	}
	return tencentAdapter{client: client, endpoint: endpoint}
}

func (a tencentAdapter) Provider() Provider { return ProviderTencent }

func (a tencentAdapter) Analyze(ctx context.Context, credential, ip string) (Result, error) {
	normalizedIP, err := NormalizePublicIP(ip)
	if err != nil {
		return Result{}, err
	}

	endpoint, err := url.Parse(a.endpoint)
	if err != nil {
		return Result{}, newServiceError(ErrorInternal, "腾讯安全接口配置无效", errors.New("invalid Tencent endpoint"))
	}
	body, err := json.Marshal(map[string]any{
		"c_version": "3.0",
		"c_action":  "IPAnalysis",
		"c_appkey":  credential,
		"c_lang":    "zh",
		"type":      "ip",
		"key":       normalizedIP,
		"option":    0,
	})
	if err != nil {
		return Result{}, newServiceError(ErrorInternal, "创建腾讯安全请求失败", errors.New("invalid Tencent request body"))
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return Result{}, newServiceError(ErrorInternal, "创建腾讯安全请求失败", errors.New("invalid Tencent request"))
	}
	request.Header.Set("Content-Type", "application/json")

	raw, err := doProviderJSON(a.client, request)
	if err != nil {
		return Result{}, err
	}
	return mapTencentResponse(normalizedIP, raw)
}

type tencentResponse struct {
	ReturnCode  *int     `json:"return_code"`
	Result      string   `json:"result"`
	ThreatLevel *int     `json:"threat_level"`
	Confidence  *float64 `json:"confidence"`
	Tags        []string `json:"tags"`
	FirstSeen   string   `json:"first_seen"`
	LastSeen    string   `json:"last_seen"`
}

func mapTencentResponse(ip string, raw json.RawMessage) (Result, error) {
	var response tencentResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return Result{}, newServiceError(ErrorInvalidResponse, "腾讯安全返回数据结构无效", errors.New("invalid Tencent response structure"))
	}
	if response.ReturnCode == nil {
		return Result{}, newServiceError(ErrorInvalidResponse, "腾讯安全返回数据结构无效", errors.New("missing Tencent return code"))
	}
	if *response.ReturnCode != 0 {
		code, message := classifyTencentBusinessError(*response.ReturnCode)
		return Result{}, newServiceError(code, message, fmt.Errorf("Tencent business return_code %d", *response.ReturnCode))
	}

	firstSeen, err := parseTencentTime(response.FirstSeen)
	if err != nil {
		return Result{}, err
	}
	lastSeen, err := parseTencentTime(response.LastSeen)
	if err != nil {
		return Result{}, err
	}
	confidence := normalizeTencentConfidence(response.Confidence)
	tags := uniqueProviderTags(response.Tags)
	verdict := tencentVerdict(response.Result)
	risk := tencentRisk(response.ThreatLevel)

	return Result{
		Provider:        ProviderTencent,
		IP:              ip,
		Verdict:         verdict,
		RiskLevel:       risk,
		ConfidenceScore: confidence,
		Tags:            tags,
		FirstSeen:       firstSeen,
		LastSeen:        lastSeen,
		AnalyzedAt:      time.Now(),
		Summary:         summarizeTencentResult(ip, verdict, risk, confidence, tags),
		RawResponse:     raw,
	}, nil
}

func classifyTencentBusinessError(returnCode int) (ErrorCode, string) {
	switch returnCode {
	case 1004:
		return ErrorQuotaExhausted, "腾讯安全查询额度已用尽"
	case 1005:
		return ErrorRateLimited, "腾讯安全请求过于频繁"
	default:
		return ErrorInvalidResponse, "腾讯安全返回业务错误"
	}
}

func tencentVerdict(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "black":
		return "malicious"
	case "suspicious":
		return "suspicious"
	case "white":
		return "benign"
	case "info":
		return "unknown"
	default:
		return "unknown"
	}
}

func tencentRisk(level *int) string {
	if level == nil {
		return "unknown"
	}
	switch *level {
	case 0:
		return "unknown"
	case 1:
		return "info"
	case 2:
		return "low"
	case 3:
		return "medium"
	case 4:
		return "high"
	case 5:
		return "critical"
	default:
		return "unknown"
	}
}

func normalizeTencentConfidence(confidence *float64) *float64 {
	if confidence == nil {
		return nil
	}
	value := *confidence
	if value > 0 && value <= 1 {
		value *= 100
	}
	switch {
	case value < 0:
		value = 0
	case value > 100:
		value = 100
	}
	return &value
}

func parseTencentTime(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, newServiceError(ErrorInvalidResponse, "腾讯安全返回时间格式无效", errors.New("invalid Tencent time"))
	}
	return &parsed, nil
}

func summarizeTencentResult(ip, verdict, risk string, confidence *float64, tags []string) string {
	parts := []string{fmt.Sprintf("腾讯安全判定 %s 为 %s", ip, verdict)}
	if risk != "" {
		parts = append(parts, "风险 "+risk)
	}
	if confidence != nil {
		parts = append(parts, fmt.Sprintf("置信分 %.0f", *confidence))
	}
	if len(tags) > 0 {
		parts = append(parts, "标签："+strings.Join(tags, "、"))
	}
	return strings.Join(parts, "，")
}
