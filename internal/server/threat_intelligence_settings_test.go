package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fwlog/internal/threatintel"
)

func TestThreatIntelligenceSettingsAreProtectedFromGenericSettings(t *testing.T) {
	app := NewApp(LoadConfig())
	app.settings["threat_intelligence.threatbook.credential"] = "v1:ciphertext"
	if _, ok := app.getSettings()["threat_intelligence.threatbook.credential"]; ok {
		t.Fatal("credential leaked")
	}

	updates, err := app.normalizeSettingsPayload(map[string]any{
		"threat_intelligence.threatbook.credential": "plaintext",
		"log_tag": "kept",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := updates["threat_intelligence.threatbook.credential"]; ok {
		t.Fatal("protected key accepted")
	}
	if updates["log_tag"] != "kept" {
		t.Fatal("ordinary setting lost")
	}

	saved := map[string]string{}
	app.settingsSaver = func(_ context.Context, settings map[string]string) error {
		for key, value := range settings {
			saved[key] = value
		}
		return nil
	}
	app.settings["log_tag"] = "persisted"
	if err := app.saveSettings(context.Background(), map[string]any{
		"threat_intelligence.threatbook.credential": "plaintext",
		"log_tag": "ignored",
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := saved["threat_intelligence.threatbook.credential"]; ok {
		t.Fatal("protected key saved")
	}
	if saved["log_tag"] != "persisted" {
		t.Fatalf("ordinary setting not saved: %#v", saved)
	}
}

func TestThreatIntelligenceSettingsConfigStoreEncryptsFirstCredential(t *testing.T) {
	app, saved := newThreatIntelSettingsTestApp(t)
	store := newThreatIntelSettingsTestStore(app)
	credential := "secret-token"

	status, err := store.Update(context.Background(), threatintel.ProviderThreatBook, threatintel.ProviderConfigUpdate{
		Enabled:    true,
		Credential: &credential,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || !status.Configured || status.CredentialError != "" {
		t.Fatalf("status = %#v", status)
	}

	ciphertext := app.settings[threatIntelSettingKey(threatintel.ProviderThreatBook, "credential")]
	if !strings.HasPrefix(ciphertext, "v1:") || strings.Contains(ciphertext, credential) {
		t.Fatalf("unsafe in-memory credential = %q", ciphertext)
	}
	persisted := (*saved)[threatIntelSettingKey(threatintel.ProviderThreatBook, "credential")]
	if persisted != ciphertext || strings.Contains(persisted, credential) {
		t.Fatalf("unsafe persisted credential = %q, want encrypted %q", persisted, ciphertext)
	}

	encodedStatus, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedStatus), credential) || strings.Contains(string(encodedStatus), ciphertext) {
		t.Fatalf("status leaked credential: %s", encodedStatus)
	}

	cfg, err := store.Config(context.Background(), threatintel.ProviderThreatBook)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Enabled || cfg.Credential != credential {
		t.Fatalf("config = %#v", cfg)
	}
}

func TestThreatIntelligenceSettingsConfigStoreRetainsOverwritesAndClearsCredential(t *testing.T) {
	app, _ := newThreatIntelSettingsTestApp(t)
	store := newThreatIntelSettingsTestStore(app)
	credential := "old-secret"
	if _, err := store.Update(context.Background(), threatintel.ProviderThreatBook, threatintel.ProviderConfigUpdate{
		Enabled:    true,
		Credential: &credential,
	}); err != nil {
		t.Fatal(err)
	}
	oldCiphertext := app.settings[threatIntelSettingKey(threatintel.ProviderThreatBook, "credential")]

	if _, err := store.Update(context.Background(), threatintel.ProviderThreatBook, threatintel.ProviderConfigUpdate{
		Enabled: false,
	}); err != nil {
		t.Fatal(err)
	}
	if got := app.settings[threatIntelSettingKey(threatintel.ProviderThreatBook, "credential")]; got != oldCiphertext {
		t.Fatalf("nil credential changed ciphertext: %q", got)
	}

	blank := " \t\n"
	if _, err := store.Update(context.Background(), threatintel.ProviderThreatBook, threatintel.ProviderConfigUpdate{
		Enabled:    true,
		Credential: &blank,
	}); err != nil {
		t.Fatal(err)
	}
	if got := app.settings[threatIntelSettingKey(threatintel.ProviderThreatBook, "credential")]; got != oldCiphertext {
		t.Fatalf("blank credential changed ciphertext: %q", got)
	}

	newCredential := "new-secret"
	if _, err := store.Update(context.Background(), threatintel.ProviderThreatBook, threatintel.ProviderConfigUpdate{
		Enabled:    true,
		Credential: &newCredential,
	}); err != nil {
		t.Fatal(err)
	}
	newCiphertext := app.settings[threatIntelSettingKey(threatintel.ProviderThreatBook, "credential")]
	if newCiphertext == "" || newCiphertext == oldCiphertext || strings.Contains(newCiphertext, newCredential) {
		t.Fatalf("credential should be overwritten with safe ciphertext: %q", newCiphertext)
	}
	cfg, err := store.Config(context.Background(), threatintel.ProviderThreatBook)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Credential != newCredential {
		t.Fatalf("config credential = %q, want %q", cfg.Credential, newCredential)
	}

	conflictingCredential := "conflicting-secret"
	if _, err := store.Update(context.Background(), threatintel.ProviderThreatBook, threatintel.ProviderConfigUpdate{
		Enabled:         true,
		Credential:      &conflictingCredential,
		ClearCredential: true,
	}); err == nil {
		t.Fatal("simultaneous credential and clear_credential should be rejected")
	}
	if got := app.settings[threatIntelSettingKey(threatintel.ProviderThreatBook, "credential")]; got != newCiphertext {
		t.Fatalf("rejected update changed ciphertext: %q", got)
	}

	status, err := store.Update(context.Background(), threatintel.ProviderThreatBook, threatintel.ProviderConfigUpdate{
		Enabled:         true,
		ClearCredential: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Configured || app.settings[threatIntelSettingKey(threatintel.ProviderThreatBook, "credential")] != "" {
		t.Fatalf("credential should be cleared, status = %#v, value = %q", status, app.settings[threatIntelSettingKey(threatintel.ProviderThreatBook, "credential")])
	}
}

func TestThreatIntelligenceSettingsConfigStoreDoesNotApplyWhenPersistenceFails(t *testing.T) {
	cfg := LoadConfig()
	cfg.ThreatIntelligenceKeyFile = filepath.Join(t.TempDir(), "threat-intelligence.key")
	app := NewApp(cfg)
	app.settings[threatIntelSettingKey(threatintel.ProviderThreatBook, "enabled")] = "false"
	app.settingsSaver = func(context.Context, map[string]string) error {
		return errors.New("write failed")
	}
	store := newThreatIntelSettingsTestStore(app)
	credential := "secret-token"

	_, err := store.Update(context.Background(), threatintel.ProviderThreatBook, threatintel.ProviderConfigUpdate{
		Enabled:    true,
		Credential: &credential,
	})
	if err == nil {
		t.Fatal("save failure should be returned")
	}
	if got := app.settings[threatIntelSettingKey(threatintel.ProviderThreatBook, "enabled")]; got != "false" {
		t.Fatalf("failed save changed enabled setting: %q", got)
	}
	if got := app.settings[threatIntelSettingKey(threatintel.ProviderThreatBook, "credential")]; got != "" {
		t.Fatalf("failed save stored credential: %q", got)
	}
}

func TestThreatIntelligenceSettingsConfigStoreStatusesUseFixedProviderOrderAndRecordTests(t *testing.T) {
	app, _ := newThreatIntelSettingsTestApp(t)
	store := newThreatIntelSettingsTestStore(app)
	testedAt := time.Date(2026, 8, 31, 10, 20, 30, 123, time.UTC)
	if err := store.RecordTest(context.Background(), threatintel.ProviderQianxin, threatintel.ProviderTestStatus{
		Status:   "success",
		Message:  "连接成功",
		TestedAt: testedAt,
	}); err != nil {
		t.Fatal(err)
	}

	statuses, err := store.Statuses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantProviders := []threatintel.Provider{
		threatintel.ProviderThreatBook,
		threatintel.ProviderNSFocus,
		threatintel.ProviderQianxin,
		threatintel.ProviderTencent,
	}
	wantNames := []string{"微步在线", "绿盟科技", "奇安信", "腾讯安全"}
	if len(statuses) != len(wantProviders) {
		t.Fatalf("statuses = %#v", statuses)
	}
	for i, want := range wantProviders {
		if statuses[i].Provider != want || statuses[i].Name != wantNames[i] {
			t.Fatalf("status[%d] = %#v, want provider %s name %s", i, statuses[i], want, wantNames[i])
		}
	}
	qianxin := statuses[2]
	if qianxin.LastTestStatus != "success" || qianxin.LastTestMessage != "连接成功" || qianxin.LastTestedAt == nil || !qianxin.LastTestedAt.Equal(testedAt) {
		t.Fatalf("qianxin test status = %#v", qianxin)
	}
}

func TestThreatIntelligenceSettingsConfigStoreReportsCredentialErrorWithoutLeakingOrDeletingState(t *testing.T) {
	app, _ := newThreatIntelSettingsTestApp(t)
	store := newThreatIntelSettingsTestStore(app)
	credential := "secret-token"
	if _, err := store.Update(context.Background(), threatintel.ProviderThreatBook, threatintel.ProviderConfigUpdate{
		Enabled:    true,
		Credential: &credential,
	}); err != nil {
		t.Fatal(err)
	}
	ciphertext := app.settings[threatIntelSettingKey(threatintel.ProviderThreatBook, "credential")]
	testedAt := time.Date(2026, 8, 31, 10, 20, 30, 0, time.UTC)
	if err := store.RecordTest(context.Background(), threatintel.ProviderThreatBook, threatintel.ProviderTestStatus{
		Status:   "failed",
		Message:  "凭据无效",
		TestedAt: testedAt,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(app.cfg.ThreatIntelligenceKeyFile, []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}

	statuses, err := store.Statuses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	status := statuses[0]
	if !status.Configured || status.CredentialError != "credential_error" {
		t.Fatalf("status = %#v", status)
	}
	if status.LastTestStatus != "failed" || status.LastTestMessage != "凭据无效" || status.LastTestedAt == nil || !status.LastTestedAt.Equal(testedAt) {
		t.Fatalf("historical test status lost: %#v", status)
	}
	if got := app.settings[threatIntelSettingKey(threatintel.ProviderThreatBook, "credential")]; got != ciphertext {
		t.Fatalf("credential ciphertext changed: %q", got)
	}

	encodedStatus, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedStatus), credential) || strings.Contains(string(encodedStatus), ciphertext) {
		t.Fatalf("status leaked credential: %s", encodedStatus)
	}
	cfg, err := store.Config(context.Background(), threatintel.ProviderThreatBook)
	if err == nil {
		t.Fatalf("Config should fail when key is damaged, config = %#v", cfg)
	}
	if cfg.Credential != "" {
		t.Fatalf("failed Config leaked credential: %#v", cfg)
	}
}

func TestThreatIntelligenceSettingsConfigStoreReportsMissingCredentialKeyWithoutDeletingState(t *testing.T) {
	app, _ := newThreatIntelSettingsTestApp(t)
	store := newThreatIntelSettingsTestStore(app)
	credential := "secret-token"
	if _, err := store.Update(context.Background(), threatintel.ProviderThreatBook, threatintel.ProviderConfigUpdate{
		Enabled:    true,
		Credential: &credential,
	}); err != nil {
		t.Fatal(err)
	}
	ciphertext := app.settings[threatIntelSettingKey(threatintel.ProviderThreatBook, "credential")]
	if err := os.Remove(app.cfg.ThreatIntelligenceKeyFile); err != nil {
		t.Fatal(err)
	}

	statuses, err := store.Statuses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	status := statuses[0]
	if !status.Configured || status.CredentialError != "credential_error" {
		t.Fatalf("status = %#v", status)
	}
	if got := app.settings[threatIntelSettingKey(threatintel.ProviderThreatBook, "credential")]; got != ciphertext {
		t.Fatalf("credential ciphertext changed: %q", got)
	}
}

func newThreatIntelSettingsTestApp(t *testing.T) (*App, *map[string]string) {
	t.Helper()

	cfg := LoadConfig()
	cfg.ThreatIntelligenceKeyFile = filepath.Join(t.TempDir(), "threat-intelligence.key")
	app := NewApp(cfg)
	saved := map[string]string{}
	app.settingsSaver = func(_ context.Context, settings map[string]string) error {
		for key, value := range settings {
			saved[key] = value
		}
		return nil
	}
	return app, &saved
}

func newThreatIntelSettingsTestStore(app *App) *appThreatIntelligenceConfigStore {
	return &appThreatIntelligenceConfigStore{
		app:    app,
		cipher: threatintel.NewCredentialCipher(app.cfg.ThreatIntelligenceKeyFile),
	}
}
