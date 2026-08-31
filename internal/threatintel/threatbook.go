package threatintel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const threatBookDefaultEndpoint = "https://api.threatbook.cn/v3/scene/ip_reputation"

type threatBookAdapter struct {
	client   *http.Client
	endpoint string
}

func NewThreatBookAdapter(client *http.Client, endpoint string) Adapter {
	if client == nil {
		client = http.DefaultClient
	}
	if endpoint == "" {
		endpoint = threatBookDefaultEndpoint
	}
	return threatBookAdapter{client: client, endpoint: endpoint}
}

func (a threatBookAdapter) Provider() Provider { return ProviderThreatBook }

func (a threatBookAdapter) Analyze(ctx context.Context, credential, ip string) (Result, error) {
	endpoint, err := url.Parse(a.endpoint)
	if err != nil {
		return Result{}, newServiceError(ErrorInternal, "微步接口配置无效", errors.New("invalid ThreatBook endpoint"))
	}
	query := endpoint.Query()
	query.Set("apikey", credential)
	query.Set("resource", ip)
	query.Set("lang", "zh")
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return Result{}, newServiceError(ErrorInternal, "创建微步请求失败", errors.New("invalid ThreatBook request"))
	}
	raw, err := doProviderJSON(a.client, request)
	if err != nil {
		return Result{}, err
	}
	return mapThreatBookResponse(ip, raw)
}

type threatBookResponse struct {
	ResponseCode *int                              `json:"response_code"`
	Data         map[string]threatBookIPReputation `json:"data"`
}

type threatBookIPReputation struct {
	IsMalicious     *bool    `json:"is_malicious"`
	ConfidenceLevel string   `json:"confidence_level"`
	Severity        string   `json:"severity"`
	Judgments       []string `json:"judgments"`
	UpdateTime      string   `json:"update_time"`
}

func mapThreatBookResponse(ip string, raw json.RawMessage) (Result, error) {
	var response threatBookResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return Result{}, newServiceError(ErrorInvalidResponse, "微步返回数据结构无效", errors.New("invalid ThreatBook response structure"))
	}
	if response.ResponseCode == nil {
		return Result{}, newServiceError(ErrorInvalidResponse, "微步返回数据结构无效", errors.New("missing ThreatBook response code"))
	}
	if *response.ResponseCode != 0 {
		return Result{}, newServiceError(ErrorInvalidResponse, "微步返回业务错误", fmt.Errorf("ThreatBook response code %d", *response.ResponseCode))
	}
	if response.Data == nil {
		return Result{}, newServiceError(ErrorInvalidResponse, "微步返回数据结构无效", errors.New("missing ThreatBook data"))
	}

	reputation, ok := response.Data[ip]
	if !ok {
		return Result{
			Provider:    ProviderThreatBook,
			IP:          ip,
			Verdict:     "unknown",
			RiskLevel:   "unknown",
			AnalyzedAt:  time.Now(),
			Summary:     fmt.Sprintf("微步在线未返回 %s 的明确威胁结论", ip),
			RawResponse: raw,
		}, nil
	}

	verdict := "benign"
	if reputation.IsMalicious == nil {
		return Result{}, newServiceError(ErrorInvalidResponse, "微步返回数据结构无效", errors.New("missing ThreatBook malicious flag"))
	}
	if *reputation.IsMalicious {
		verdict = "malicious"
	}
	tags := uniqueProviderTags(reputation.Judgments)
	sourceUpdatedAt, err := parseThreatBookTime(reputation.UpdateTime)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Provider:        ProviderThreatBook,
		IP:              ip,
		Verdict:         verdict,
		RiskLevel:       reputation.Severity,
		ConfidenceLevel: reputation.ConfidenceLevel,
		Tags:            tags,
		SourceUpdatedAt: sourceUpdatedAt,
		AnalyzedAt:      time.Now(),
		Summary:         summarizeThreatBookResult(ip, verdict, reputation.Severity, reputation.ConfidenceLevel, tags),
		RawResponse:     raw,
	}, nil
}

func parseThreatBookTime(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", raw, time.Local)
	if err != nil {
		return nil, newServiceError(ErrorInvalidResponse, "微步返回时间格式无效", errors.New("invalid ThreatBook update time"))
	}
	return &parsed, nil
}

func summarizeThreatBookResult(ip, verdict, riskLevel, confidenceLevel string, tags []string) string {
	parts := []string{fmt.Sprintf("微步在线判定 %s 为 %s", ip, verdict)}
	if riskLevel != "" {
		parts = append(parts, "风险 "+riskLevel)
	}
	if confidenceLevel != "" {
		parts = append(parts, "置信度 "+confidenceLevel)
	}
	if len(tags) > 0 {
		parts = append(parts, "标签："+strings.Join(tags, "、"))
	}
	return strings.Join(parts, "，")
}
