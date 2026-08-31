package threatintel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxProviderJSONResponseBytes = 4 << 20

var providerCredentialFields = map[string]struct{}{
	"apikey":        {},
	"key":           {},
	"api_key":       {},
	"token":         {},
	"secret":        {},
	"credential":    {},
	"access_token":  {},
	"refresh_token": {},
	"auth_token":    {},
	"authorization": {},
	"password":      {},
	"passwd":        {},
	"pwd":           {},
	"client_secret": {},
}

func doProviderJSON(client *http.Client, request *http.Request) (json.RawMessage, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if request == nil {
		return nil, newServiceError(ErrorInternal, "情报服务请求无效", errors.New("nil request"))
	}

	response, err := client.Do(request)
	if err != nil {
		code := ErrorProviderUnavailable
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(request.Context().Err(), context.DeadlineExceeded) {
			code = ErrorTimeout
		}
		return nil, newServiceError(code, "情报服务暂时不可用", errors.New("provider request failed"))
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		code, message := classifyProviderHTTPStatus(response.StatusCode)
		return nil, newServiceError(code, message, fmt.Errorf("provider status %d", response.StatusCode))
	}

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxProviderJSONResponseBytes+1))
	if err != nil {
		return nil, newServiceError(ErrorInvalidResponse, "情报服务返回数据读取失败", errors.New("provider response read failed"))
	}
	if len(raw) > maxProviderJSONResponseBytes {
		return nil, newServiceError(ErrorInvalidResponse, "情报服务返回数据过大", errors.New("provider response exceeds size limit"))
	}
	cleaned, err := sanitizeProviderRawResponse(raw)
	if err != nil {
		return nil, err
	}
	return cleaned, nil
}

func classifyProviderHTTPStatus(status int) (ErrorCode, string) {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return ErrorInvalidCredential, "情报服务凭据无效"
	case status == http.StatusTooManyRequests:
		return ErrorRateLimited, "情报服务请求过于频繁"
	case status >= http.StatusInternalServerError:
		return ErrorProviderUnavailable, "情报服务暂时不可用"
	default:
		return ErrorInvalidResponse, "情报服务返回状态异常"
	}
}

func sanitizeProviderRawResponse(raw []byte) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, newServiceError(ErrorInvalidResponse, "情报服务返回数据格式无效", errors.New("provider response is not valid JSON"))
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, newServiceError(ErrorInvalidResponse, "情报服务返回数据格式无效", errors.New("provider response contains trailing data"))
	}

	sanitizeProviderValue(value)
	cleaned, err := json.Marshal(value)
	if err != nil {
		return nil, newServiceError(ErrorInvalidResponse, "情报服务返回数据清理失败", errors.New("provider response sanitize failed"))
	}
	return json.RawMessage(cleaned), nil
}

func sanitizeProviderValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isProviderCredentialField(key) {
				typed[key] = "[redacted]"
				continue
			}
			sanitizeProviderValue(child)
		}
	case []any:
		for _, child := range typed {
			sanitizeProviderValue(child)
		}
	}
}

func isProviderCredentialField(key string) bool {
	_, ok := providerCredentialFields[strings.ToLower(key)]
	return ok
}

func uniqueProviderTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	return result
}
