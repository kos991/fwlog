package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"fwlog/internal/threatintel"
)

const threatIntelligenceProvidersPath = "/api/threat-intelligence/providers"

type providerUpdateRequest struct {
	Enabled         bool    `json:"enabled"`
	Credential      *string `json:"credential"`
	ClearCredential bool    `json:"clear_credential"`
}

type analyzeRequest struct {
	IP string `json:"ip"`
}

func threatIntelligenceHandler(app *App) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, threatIntelligenceProvidersPath)
		if path == "" || path == "/" {
			if r.Method != http.MethodGet {
				writeMethodNotAllowed(w, http.MethodGet)
				return
			}
			statuses, err := app.threatIntelligenceService.Providers(r.Context())
			if err != nil {
				writeThreatIntelligenceError(w, err)
				return
			}
			writeJSON(w, map[string]any{"providers": statuses})
			return
		}

		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) < 1 || parts[0] == "" {
			writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "威胁情报接口不存在"})
			return
		}
		provider, ok := threatintel.ParseProvider(parts[0])
		if !ok {
			writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "provider_not_found", "message": "威胁情报平台不存在"})
			return
		}

		operation := "update"
		if len(parts) == 2 {
			operation = parts[1]
		}
		if len(parts) > 2 {
			writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "威胁情报接口不存在"})
			return
		}

		switch operation {
		case "update":
			if len(parts) != 1 {
				writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "威胁情报接口不存在"})
				return
			}
			if r.Method != http.MethodPost {
				writeMethodNotAllowed(w, http.MethodPost)
				return
			}
			app.updateThreatIntelligenceProvider(w, r, provider)
		case "test":
			if r.Method != http.MethodPost {
				writeMethodNotAllowed(w, http.MethodPost)
				return
			}
			status, err := app.threatIntelligenceService.TestProvider(r.Context(), provider)
			if err != nil {
				writeThreatIntelligenceError(w, err)
				return
			}
			writeJSON(w, status)
		case "results":
			if r.Method != http.MethodGet {
				writeMethodNotAllowed(w, http.MethodGet)
				return
			}
			result, err := app.threatIntelligenceService.Result(r.Context(), provider, r.URL.Query().Get("ip"))
			if err != nil {
				writeThreatIntelligenceError(w, err)
				return
			}
			writeJSON(w, map[string]any{"result": result})
		case "analyze":
			if r.Method != http.MethodPost {
				writeMethodNotAllowed(w, http.MethodPost)
				return
			}
			var payload analyzeRequest
			if err := decodeJSONBody(r, &payload); err != nil || strings.TrimSpace(payload.IP) == "" {
				if err == nil {
					err = errors.New("ip 不能为空")
				}
				writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid_request", "message": err.Error()})
				return
			}
			outcome, err := app.threatIntelligenceService.Analyze(r.Context(), provider, payload.IP)
			if err != nil {
				writeThreatIntelligenceAnalysisError(w, outcome, err)
				return
			}
			writeJSON(w, outcome)
		default:
			writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "威胁情报接口不存在"})
		}
	})
}

func (a *App) updateThreatIntelligenceProvider(w http.ResponseWriter, r *http.Request, provider threatintel.Provider) {
	var payload providerUpdateRequest
	if err := decodeJSONBody(r, &payload); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid_request", "message": err.Error()})
		return
	}
	if payload.Credential != nil && strings.TrimSpace(*payload.Credential) != "" && payload.ClearCredential {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid_request", "message": "不能同时提交凭据和清除凭据标记"})
		return
	}
	status, err := a.threatIntelligenceService.UpdateProvider(r.Context(), provider, threatintel.ProviderConfigUpdate{
		Enabled: payload.Enabled, Credential: payload.Credential, ClearCredential: payload.ClearCredential,
	})
	if err != nil {
		writeThreatIntelligenceError(w, err)
		return
	}
	writeJSON(w, status)
}

func writeMethodNotAllowed(w http.ResponseWriter, method string) {
	w.Header().Set("Allow", method)
	writeJSONStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed", "message": fmt.Sprintf("仅支持 %s 请求", method)})
}

func writeThreatIntelligenceAnalysisError(w http.ResponseWriter, outcome threatintel.AnalyzeOutcome, err error) {
	payload := map[string]any{"error": string(threatintel.ErrorCodeOf(err)), "message": err.Error()}
	if outcome.PreviousResult != nil {
		payload["previous_result"] = outcome.PreviousResult
	}
	writeJSONStatus(w, threatIntelligenceErrorStatus(err), payload)
}

func writeThreatIntelligenceError(w http.ResponseWriter, err error) {
	writeJSONStatus(w, threatIntelligenceErrorStatus(err), map[string]string{
		"error": string(threatintel.ErrorCodeOf(err)), "message": err.Error(),
	})
}

func threatIntelligenceErrorStatus(err error) int {
	switch threatintel.ErrorCodeOf(err) {
	case threatintel.ErrorInvalidIP:
		return http.StatusBadRequest
	case threatintel.ErrorProviderDisabled, threatintel.ErrorProviderNotConfigured, threatintel.ErrorCredentialUnavailable, threatintel.ErrorInvalidCredential:
		return http.StatusUnprocessableEntity
	case threatintel.ErrorQuotaExhausted, threatintel.ErrorRateLimited:
		return http.StatusTooManyRequests
	case threatintel.ErrorTimeout:
		return http.StatusGatewayTimeout
	case threatintel.ErrorProviderUnavailable, threatintel.ErrorInvalidResponse:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}
