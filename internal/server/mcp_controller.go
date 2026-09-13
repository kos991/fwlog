package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const mcpProtocolVersion = "2025-06-18"

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpToolCallParams struct {
	Name      string            `json:"name"`
	Arguments mcpQueryArguments `json:"arguments"`
}

type mcpQueryArguments struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	SourceID string `json:"source_id"`
	IP       string `json:"ip"`
	SrcIP    string `json:"src_ip"`
	DstIP    string `json:"dst_ip"`
	NATIP    string `json:"nat_ip"`
	Port     uint16 `json:"port"`
	SrcPort  uint16 `json:"src_port"`
	DstPort  uint16 `json:"dst_port"`
	NATPort  uint16 `json:"nat_port"`
	Protocol string `json:"protocol"`
	Action   string `json:"action"`
	LogTag   string `json:"log_tag"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Cursor   string `json:"cursor"`
}

func (a *App) mcpHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request mcpRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeMCPError(w, nil, -32700, "JSON 请求格式无效")
			return
		}
		if request.JSONRPC != "2.0" || strings.TrimSpace(request.Method) == "" {
			writeMCPError(w, request.ID, -32600, "MCP 请求格式无效")
			return
		}

		switch request.Method {
		case "initialize":
			writeMCPResult(w, request.ID, map[string]any{
				"protocolVersion": mcpProtocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]string{"name": "fwlog", "version": appVersion},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeMCPResult(w, request.ID, map[string]any{"tools": []any{mcpQueryToolDefinition()}})
		case "tools/call":
			a.handleMCPToolCall(w, r, request)
		default:
			writeMCPError(w, request.ID, -32601, "不支持的 MCP 方法")
		}
	})
}

func (a *App) handleMCPToolCall(w http.ResponseWriter, r *http.Request, request mcpRequest) {
	var params mcpToolCallParams
	if err := decodeMCPParams(request.Params, &params); err != nil {
		writeMCPError(w, request.ID, -32602, "工具参数格式无效")
		return
	}
	if params.Name != "query_logs" {
		writeMCPError(w, request.ID, -32602, "不支持的 MCP 工具")
		return
	}

	queryRequest := buildMCPQueryRequest(r, params.Arguments)
	response, err := (appQueryService{app: a}).Query(queryRequest)
	if err != nil {
		writeMCPResult(w, request.ID, map[string]any{
			"content": []map[string]string{{"type": "text", "text": err.Error()}},
			"isError": true,
		})
		return
	}
	payload, err := json.Marshal(response)
	if err != nil {
		writeMCPError(w, request.ID, -32603, "查询结果编码失败")
		return
	}
	writeMCPResult(w, request.ID, map[string]any{
		"content":           []map[string]string{{"type": "text", "text": string(payload)}},
		"structuredContent": response,
		"isError":           false,
	})
}

func decodeMCPParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return errors.New("missing params")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func buildMCPQueryRequest(r *http.Request, args mcpQueryArguments) *http.Request {
	values := url.Values{}
	setMCPQueryValue(values, "start", args.Start)
	setMCPQueryValue(values, "end", args.End)
	setMCPQueryValue(values, "source_id", args.SourceID)
	setMCPQueryValue(values, "ip", args.IP)
	setMCPQueryValue(values, "src_ip", args.SrcIP)
	setMCPQueryValue(values, "dst_ip", args.DstIP)
	setMCPQueryValue(values, "nat_ip", args.NATIP)
	setMCPQueryValue(values, "protocol", args.Protocol)
	setMCPQueryValue(values, "action", args.Action)
	setMCPQueryValue(values, "log_tag", args.LogTag)
	setMCPQueryValue(values, "cursor", args.Cursor)
	setMCPQueryNumber(values, "port", int(args.Port))
	setMCPQueryNumber(values, "src_port", int(args.SrcPort))
	setMCPQueryNumber(values, "dst_port", int(args.DstPort))
	setMCPQueryNumber(values, "nat_port", int(args.NATPort))
	setMCPQueryNumber(values, "page", args.Page)
	setMCPQueryNumber(values, "page_size", args.PageSize)

	queryRequest := r.Clone(r.Context())
	queryURL := *r.URL
	queryURL.Path = "/api/query"
	queryURL.RawQuery = values.Encode()
	queryRequest.Method = http.MethodGet
	queryRequest.URL = &queryURL
	return queryRequest
}

func setMCPQueryValue(values url.Values, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		values.Set(key, value)
	}
}

func setMCPQueryNumber(values url.Values, key string, value int) {
	if value > 0 {
		values.Set(key, strconv.Itoa(value))
	}
}

func mcpQueryToolDefinition() map[string]any {
	return map[string]any{
		"name":        "query_logs",
		"description": "按时间、地址、端口、协议和日志来源查询 NAT 日志，只返回查询结果。",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"start":     map[string]string{"type": "string", "description": "开始时间，格式为 YYYY-MM-DD HH:mm:ss 或 RFC3339"},
				"end":       map[string]string{"type": "string", "description": "结束时间，格式为 YYYY-MM-DD HH:mm:ss 或 RFC3339"},
				"source_id": map[string]string{"type": "string"},
				"ip":        map[string]string{"type": "string"},
				"src_ip":    map[string]string{"type": "string"},
				"dst_ip":    map[string]string{"type": "string"},
				"nat_ip":    map[string]string{"type": "string"},
				"port":      map[string]string{"type": "integer"},
				"src_port":  map[string]string{"type": "integer"},
				"dst_port":  map[string]string{"type": "integer"},
				"nat_port":  map[string]string{"type": "integer"},
				"protocol":  map[string]string{"type": "string"},
				"action":    map[string]string{"type": "string"},
				"log_tag":   map[string]string{"type": "string"},
				"page":      map[string]string{"type": "integer"},
				"page_size": map[string]string{"type": "integer"},
				"cursor":    map[string]string{"type": "string"},
			},
			"additionalProperties": false,
		},
	}
}

func writeMCPResult(w http.ResponseWriter, id json.RawMessage, result any) {
	writeJSON(w, map[string]any{"jsonrpc": "2.0", "id": mcpResponseID(id), "result": result})
}

func writeMCPError(w http.ResponseWriter, id json.RawMessage, code int, message string) {
	writeJSON(w, map[string]any{
		"jsonrpc": "2.0",
		"id":      mcpResponseID(id),
		"error":   map[string]any{"code": code, "message": message},
	})
}

func mcpResponseID(id json.RawMessage) any {
	if len(id) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(id, &value); err != nil {
		return nil
	}
	return value
}
