package threatintel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	nsfocusDefaultEndpoint = "https://nti.nsfocus.com/api/v2/objects/ioc-ipv4/"
	nsfocusAccept          = "application/nsfocus.nti.spec+json; version=2.0"
)

type nsFocusAdapter struct {
	client   *http.Client
	endpoint string
	now      func() time.Time
}

func NewNSFocusAdapter(client *http.Client, endpoint string, now func() time.Time) Adapter {
	if client == nil {
		client = http.DefaultClient
	}
	if endpoint == "" {
		endpoint = nsfocusDefaultEndpoint
	}
	if now == nil {
		now = time.Now
	}
	return nsFocusAdapter{client: client, endpoint: endpoint, now: now}
}

func (a nsFocusAdapter) Provider() Provider { return ProviderNSFocus }

func (a nsFocusAdapter) Analyze(ctx context.Context, credential, ip string) (Result, error) {
	ipv4, err := normalizeNSFocusIPv4(ip)
	if err != nil {
		return Result{}, err
	}

	endpoint, err := url.Parse(a.endpoint)
	if err != nil {
		return Result{}, newServiceError(ErrorInternal, "绿盟 NTI 接口配置无效", errors.New("invalid NSFocus endpoint"))
	}
	query := endpoint.Query()
	query.Set("query", ipv4)
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return Result{}, newServiceError(ErrorInternal, "创建绿盟 NTI 请求失败", errors.New("invalid NSFocus request"))
	}
	request.Header.Set("Accept", nsfocusAccept)
	request.Header.Set("X-Ns-Nti-Key", credential)

	raw, err := doProviderJSONWithErrorClassifier(a.client, request, func(_ int, raw json.RawMessage) (ErrorCode, string, bool) {
		return classifyNSFocusBusinessError(raw)
	})
	if err != nil {
		return Result{}, err
	}
	return mapNSFocusResponse(ipv4, raw, a.now())
}

type nsfocusResponse struct {
	Count   int             `json:"count"`
	Objects []nsfocusObject `json:"objects"`
}

type nsfocusObject struct {
	Revoked     bool     `json:"revoked"`
	ValidUntil  string   `json:"valid_until"`
	Confidence  *float64 `json:"confidence"`
	ThreatLevel int      `json:"threat_level"`
	Categories  []string `json:"categories"`
	ThreatTypes []string `json:"threat_types"`
	ActTypes    []string `json:"act_types"`
	Tags        []string `json:"tags"`
	Modified    string   `json:"modified"`
	Created     string   `json:"created"`
	FirstSeen   string   `json:"first_seen"`
	LastSeen    string   `json:"last_seen"`
}

func normalizeNSFocusIPv4(raw string) (string, error) {
	ip, err := NormalizePublicIP(raw)
	if err != nil {
		return "", err
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return "", newServiceError(ErrorInvalidIP, "请输入有效的 IP 地址", errors.New("invalid normalized IP"))
	}
	if !addr.Is4() {
		return "", newServiceError(ErrorUnsupportedIP, "绿盟 NTI 仅支持 IPv4 地址", nil)
	}
	return addr.String(), nil
}

func mapNSFocusResponse(ip string, raw json.RawMessage, now time.Time) (Result, error) {
	if code, message, ok := classifyNSFocusBusinessError(raw); ok {
		return Result{}, newServiceError(code, message, errors.New("NSFocus business error"))
	}

	var response nsfocusResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return Result{}, newServiceError(ErrorInvalidResponse, "绿盟 NTI 返回数据结构无效", errors.New("invalid NSFocus response structure"))
	}

	activeObjects := make([]nsfocusObject, 0, len(response.Objects))
	for _, object := range response.Objects {
		validUntil, err := parseNSFocusTime(object.ValidUntil)
		if err != nil {
			return Result{}, err
		}
		if nsfocusObjectActive(object.Revoked, validUntil, now) {
			activeObjects = append(activeObjects, object)
		}
	}
	if len(activeObjects) == 0 {
		return Result{
			Provider:    ProviderNSFocus,
			IP:          ip,
			Verdict:     "unknown",
			RiskLevel:   "unknown",
			AnalyzedAt:  now,
			Summary:     fmt.Sprintf("绿盟 NTI 未返回 %s 的有效威胁对象", ip),
			RawResponse: raw,
		}, nil
	}

	maxThreatLevel := 0
	var maxConfidence *float64
	var firstSeen, lastSeen, sourceUpdatedAt *time.Time
	tags := make([]string, 0)
	for _, object := range activeObjects {
		if object.ThreatLevel > maxThreatLevel {
			maxThreatLevel = object.ThreatLevel
		}
		if object.Confidence != nil {
			confidence := clampNSFocusConfidence(*object.Confidence)
			if maxConfidence == nil || confidence > *maxConfidence {
				maxConfidence = &confidence
			}
		}
		tags = append(tags, object.Categories...)
		tags = append(tags, object.ThreatTypes...)
		tags = append(tags, object.ActTypes...)
		tags = append(tags, object.Tags...)
		firstSeen = earliestNSFocusTime(firstSeen, object.FirstSeen)
		lastSeen = latestNSFocusTime(lastSeen, object.LastSeen)
		sourceUpdatedAt = latestNSFocusTime(sourceUpdatedAt, object.Modified)
		sourceUpdatedAt = latestNSFocusTime(sourceUpdatedAt, object.Created)
	}

	tags = uniqueProviderTags(tags)
	sort.Strings(tags)
	risk := nsfocusRisk(maxThreatLevel)

	return Result{
		Provider:        ProviderNSFocus,
		IP:              ip,
		Verdict:         "malicious",
		RiskLevel:       risk,
		ConfidenceScore: maxConfidence,
		Tags:            tags,
		FirstSeen:       firstSeen,
		LastSeen:        lastSeen,
		SourceUpdatedAt: sourceUpdatedAt,
		AnalyzedAt:      now,
		Summary:         summarizeNSFocusResult(ip, risk, maxConfidence, tags),
		RawResponse:     raw,
	}, nil
}

func classifyNSFocusBusinessError(raw json.RawMessage) (ErrorCode, string, bool) {
	var response nsfocusErrorResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return "", "", false
	}
	body := strings.ToLower(strings.Join(response.texts(), " "))
	switch {
	case strings.Contains(body, "invalid nti key"):
		return ErrorInvalidCredential, "绿盟 NTI 凭据无效", true
	case strings.Contains(body, "authorization expired"):
		return ErrorInvalidCredential, "绿盟 NTI 授权已过期，请更新凭据", true
	case strings.Contains(body, "over limit"):
		return ErrorQuotaExhausted, "绿盟 NTI 查询额度已用尽", true
	case strings.Contains(body, "ip mismatch") || strings.Contains(body, "ip not allowed") || strings.Contains(body, "whitelist"):
		return ErrorInvalidCredential, "绿盟 NTI 授权 IP 不匹配，请检查白名单配置", true
	case strings.Contains(body, "forbidden"):
		return ErrorInvalidCredential, "绿盟 NTI 访问被拒绝，请检查凭据和授权范围", true
	default:
		return "", "", false
	}
}

type nsfocusErrorResponse struct {
	Message json.RawMessage `json:"message"`
	Detail  json.RawMessage `json:"detail"`
	Error   json.RawMessage `json:"error"`
}

func (r nsfocusErrorResponse) texts() []string {
	texts := make([]string, 0, 3)
	for _, field := range []json.RawMessage{r.Message, r.Detail, r.Error} {
		texts = append(texts, nsfocusErrorFieldTexts(field)...)
	}
	return texts
}

func nsfocusErrorFieldTexts(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return []string{text}
	}
	var nested nsfocusErrorResponse
	if err := json.Unmarshal(raw, &nested); err != nil {
		return nil
	}
	return nested.texts()
}

func nsfocusRisk(level int) string {
	switch level {
	case 5:
		return "high"
	case 3:
		return "medium"
	case 1:
		return "low"
	default:
		return "unknown"
	}
}

func nsfocusObjectActive(revoked bool, validUntil *time.Time, now time.Time) bool {
	return !revoked && (validUntil == nil || !validUntil.Before(now))
}

func parseNSFocusTime(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, newServiceError(ErrorInvalidResponse, "绿盟 NTI 返回时间格式无效", errors.New("invalid NSFocus time"))
	}
	return &parsed, nil
}

func earliestNSFocusTime(current *time.Time, raw string) *time.Time {
	parsed, err := parseNSFocusTime(raw)
	if err != nil || parsed == nil {
		return current
	}
	if current == nil || parsed.Before(*current) {
		return parsed
	}
	return current
}

func latestNSFocusTime(current *time.Time, raw string) *time.Time {
	parsed, err := parseNSFocusTime(raw)
	if err != nil || parsed == nil {
		return current
	}
	if current == nil || parsed.After(*current) {
		return parsed
	}
	return current
}

func clampNSFocusConfidence(confidence float64) float64 {
	switch {
	case confidence < 0:
		return 0
	case confidence > 100:
		return 100
	default:
		return confidence
	}
}

func summarizeNSFocusResult(ip, risk string, confidence *float64, tags []string) string {
	parts := []string{fmt.Sprintf("绿盟 NTI 判定 %s 为 malicious", ip)}
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
