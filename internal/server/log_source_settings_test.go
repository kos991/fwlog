package server

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeRSyslogSourceUsesArchiveDirectoryAsLogDirectory(t *testing.T) {
	sources := normalizeLogSourcePayloads([]logSourcePayload{{
		SourceID:     " fw-a ",
		SerialNumber: " SN-FW-A-001 ",
		LogTag:       " 核心防火墙 ",
		SourceType:   "rsyslog",
		ClientIP:     " 192.168.10.20 ",
		SpoolDir:     "/data/fwlog/received/fw-a",
		ArchiveDir:   "/data/fwlog/archive/fw-a",
	}}, false)

	if len(sources) != 1 {
		t.Fatalf("sources = %#v", sources)
	}
	source := sources[0]
	if source.SourceID != "fw-a" || source.SerialNumber != "SN-FW-A-001" || source.LogTag != "核心防火墙" || source.ClientIP != "192.168.10.20" {
		t.Fatalf("normalized identity = %#v", source)
	}
	if source.LogDir != "/data/fwlog/archive/fw-a" || source.ArchiveDir != "/data/fwlog/archive/fw-a" {
		t.Fatalf("archive directory should be the scan directory: %#v", source)
	}
	if source.ArchiveRetentionDays != 0 {
		t.Fatalf("archive retention = %d, want permanent retention", source.ArchiveRetentionDays)
	}
}

func TestNormalizeRSyslogSourceKeepsCompressedArchivesInSpoolByDefault(t *testing.T) {
	sources := normalizeLogSourcePayloads([]logSourcePayload{{
		SourceID:   "fw-a",
		SourceType: "rsyslog",
		ClientIP:   "10.0.0.0/24",
		SpoolDir:   "/data/fwlog/received/fw-a",
	}}, false)

	if len(sources) != 1 || sources[0].LogDir != "/data/fwlog/received/fw-a" || sources[0].ArchiveDir != "" {
		t.Fatalf("archive should remain in spool by default: %#v", sources)
	}
}

func TestValidateLogSourcesRejectsDuplicateEndpointRoute(t *testing.T) {
	err := validateLogSources([]LogSource{
		{SourceID: "a", SourceType: "rsyslog", ListenProtocol: "udp", ListenHost: "0.0.0.0", ListenPort: 5514, ClientIP: "10.0.0.0/24", SpoolDir: "/data/a"},
		{SourceID: "b", SourceType: "rsyslog", ListenProtocol: "udp", ListenHost: "0.0.0.0", ListenPort: 5514, ClientIP: "10.0.0.0/24", SpoolDir: "/data/b"},
	})
	if err == nil || !strings.Contains(err.Error(), "重复的客户端 IP") {
		t.Fatalf("validateLogSources error = %v", err)
	}
}

func TestValidateLogSourcesRejectsInvalidClientNetwork(t *testing.T) {
	err := validateLogSources([]LogSource{{
		SourceID: "a", SourceType: "rsyslog", ListenProtocol: "tcp", ListenHost: "0.0.0.0", ListenPort: 5514,
		ClientIP: "192.168.1.999", SpoolDir: "/data/a",
	}})
	if err == nil || !strings.Contains(err.Error(), "无效") {
		t.Fatalf("validateLogSources error = %v", err)
	}
}

func TestValidateLogSourcesRejectsRelativeArchiveDirectory(t *testing.T) {
	err := validateLogSources([]LogSource{{
		SourceID: "a", SourceType: "rsyslog", ListenProtocol: "udp", ListenHost: "0.0.0.0", ListenPort: 5514,
		ClientIP: "192.168.1.10", SpoolDir: "/data/a", ArchiveDir: "archive/a",
	}})
	if err == nil || !strings.Contains(err.Error(), "绝对路径") {
		t.Fatalf("validateLogSources error = %v", err)
	}
}

func TestSettingsHandlerRejectsInvalidLogSourcesWithoutChangingSettings(t *testing.T) {
	app := NewApp(LoadConfig())
	original := `[{"source_id":"fw-a","log_dir":"/data/fw-a","log_tag":"A","enabled":true,"source_type":"file"}]`
	app.settings["log_sources"] = original

	body := `{"log_sources":[{"source_id":"fw-b","log_tag":"B","source_type":"rsyslog","client_ip":"not-an-ip","listen_protocol":"udp","listen_host":"0.0.0.0","listen_port":5514,"spool_dir":"/data/fw-b","enabled":true}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	settingsHandler(app).ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if got := app.getSettings()["log_sources"]; got != original {
		t.Fatalf("invalid settings changed log_sources: %q", got)
	}
}

func TestSettingsHandlerKeepsOldSettingsWhenPersistenceFails(t *testing.T) {
	app := NewApp(LoadConfig())
	original := `[{"source_id":"fw-a","log_dir":"/data/fw-a","log_tag":"A","enabled":true,"source_type":"file"}]`
	app.settings["log_sources"] = original
	app.settingsSaver = func(context.Context, map[string]string) error {
		return errors.New("write failed")
	}

	body := `{"log_sources":[{"source_id":"fw-b","log_dir":"/data/fw-b","log_tag":"B","source_type":"file","enabled":true}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	settingsHandler(app).ServeHTTP(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if got := app.getSettings()["log_sources"]; got != original {
		t.Fatalf("failed persistence changed log_sources: %q", got)
	}
}
