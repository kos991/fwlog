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

const qianxinDefaultEndpoint = "https://webapi.ti.qianxin.com/ip/v3/reputation"

type qianxinAdapter struct {
	client   *http.Client
	endpoint string
}

func NewQianxinAdapter(client *http.Client, endpoint string) Adapter {
	if client == nil {
		client = http.DefaultClient
	}
	if endpoint == "" {
		endpoint = qianxinDefaultEndpoint
	}
	return qianxinAdapter{client: client, endpoint: endpoint}
}

func (a qianxinAdapter) Provider() Provider { return ProviderQianxin }

func (a qianxinAdapter) Analyze(ctx context.Context, credential, ip string) (Result, error) {
	normalizedIP, err := NormalizePublicIP(ip)
	if err != nil {
		return Result{}, err
	}

	endpoint, err := url.Parse(a.endpoint)
	if err != nil {
		return Result{}, newServiceError(ErrorInternal, "奇安信 TI 接口配置无效", errors.New("invalid Qianxin endpoint"))
	}
	query := endpoint.Query()
	query.Set("param", normalizedIP)
	query.Set("mode", "0")
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return Result{}, newServiceError(ErrorInternal, "创建奇安信 TI 请求失败", errors.New("invalid Qianxin request"))
	}
	request.Header.Set("Api-Key", credential)

	raw, err := doProviderJSON(a.client, request)
	if err != nil {
		return Result{}, err
	}
	return mapQianxinResponse(normalizedIP, raw)
}

type qianxinResponse struct {
	Status *int        `json:"status"`
	Data   qianxinData `json:"data"`
}

type qianxinData struct {
	SummaryInfo *qianxinSummaryInfo `json:"summary_info"`
}

type qianxinSummaryInfo struct {
	Reputation           string   `json:"reputation"`
	LatestReputationTime string   `json:"latest_reputation_time"`
	MaliciousLabel       []string `json:"malicious_label"`
}

func mapQianxinResponse(ip string, raw json.RawMessage) (Result, error) {
	var response qianxinResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return Result{}, newServiceError(ErrorInvalidResponse, "奇安信 TI 返回数据结构无效", errors.New("invalid Qianxin response structure"))
	}
	if response.Status == nil {
		return Result{}, newServiceError(ErrorInvalidResponse, "奇安信 TI 返回数据结构无效", errors.New("missing Qianxin status"))
	}
	if *response.Status != 10000 {
		return Result{}, newServiceError(ErrorInvalidResponse, "奇安信 TI 返回业务错误", fmt.Errorf("Qianxin business status %d", *response.Status))
	}

	verdict := "unknown"
	var tags []string
	var sourceUpdatedAt *time.Time
	if response.Data.SummaryInfo != nil {
		verdict = qianxinVerdict(response.Data.SummaryInfo.Reputation)
		tags = uniqueProviderTags(response.Data.SummaryInfo.MaliciousLabel)
		parsed, err := parseQianxinTime(response.Data.SummaryInfo.LatestReputationTime)
		if err != nil {
			return Result{}, err
		}
		sourceUpdatedAt = parsed
	}

	return Result{
		Provider:        ProviderQianxin,
		IP:              ip,
		Verdict:         verdict,
		RiskLevel:       "unknown",
		ConfidenceLevel: "unknown",
		Tags:            tags,
		SourceUpdatedAt: sourceUpdatedAt,
		AnalyzedAt:      time.Now(),
		Summary:         summarizeQianxinResult(ip, verdict, tags),
		RawResponse:     raw,
	}, nil
}

func qianxinVerdict(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "malicious", "suspicious", "benign":
		return normalized
	default:
		return "unknown"
	}
}

func parseQianxinTime(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", raw, time.Local)
	if err != nil {
		return nil, newServiceError(ErrorInvalidResponse, "奇安信 TI 返回时间格式无效", errors.New("invalid Qianxin reputation time"))
	}
	return &parsed, nil
}

func summarizeQianxinResult(ip, verdict string, tags []string) string {
	parts := []string{fmt.Sprintf("奇安信 TI 判定 %s 为 %s", ip, verdict)}
	if len(tags) > 0 {
		parts = append(parts, "标签："+strings.Join(tags, "、"))
	}
	return strings.Join(parts, "，")
}
