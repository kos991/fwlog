package threatintel

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type Provider string

const (
	ProviderThreatBook Provider = "threatbook"
	ProviderNSFocus    Provider = "nsfocus"
	ProviderQianxin    Provider = "qianxin"
	ProviderTencent    Provider = "tencent"
)

func ParseProvider(raw string) (Provider, bool) {
	switch Provider(raw) {
	case ProviderThreatBook:
		return ProviderThreatBook, true
	case ProviderNSFocus:
		return ProviderNSFocus, true
	case ProviderQianxin:
		return ProviderQianxin, true
	case ProviderTencent:
		return ProviderTencent, true
	default:
		return "", false
	}
}

func ProviderName(provider Provider) string {
	switch provider {
	case ProviderThreatBook:
		return "微步在线"
	case ProviderNSFocus:
		return "绿盟科技"
	case ProviderQianxin:
		return "奇安信"
	case ProviderTencent:
		return "腾讯安全"
	default:
		return string(provider)
	}
}

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
	Provider        Provider   `json:"provider"`
	Name            string     `json:"name"`
	Enabled         bool       `json:"enabled"`
	Configured      bool       `json:"configured"`
	CredentialError string     `json:"credential_error,omitempty"`
	LastTestStatus  string     `json:"last_test_status,omitempty"`
	LastTestMessage string     `json:"last_test_message,omitempty"`
	LastTestedAt    *time.Time `json:"last_tested_at,omitempty"`
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

func ErrorCodeOf(err error) ErrorCode {
	var serviceErr *ServiceError
	if errors.As(err, &serviceErr) {
		return serviceErr.Code
	}
	return ErrorInternal
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
