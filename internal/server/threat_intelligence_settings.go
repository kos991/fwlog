package server

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"fwlog/internal/threatintel"
)

const credentialStatusError = "credential_error"

var orderedThreatIntelProviders = []threatintel.Provider{
	threatintel.ProviderThreatBook,
	threatintel.ProviderNSFocus,
	threatintel.ProviderQianxin,
	threatintel.ProviderTencent,
}

var _ threatintel.ConfigStore = (*appThreatIntelligenceConfigStore)(nil)

type appThreatIntelligenceConfigStore struct {
	app    *App
	cipher *threatintel.CredentialCipher
}

func newAppThreatIntelligenceConfigStore(app *App) *appThreatIntelligenceConfigStore {
	return &appThreatIntelligenceConfigStore{
		app:    app,
		cipher: threatintel.NewCredentialCipher(app.cfg.ThreatIntelligenceKeyFile),
	}
}

func isProtectedSettingsKey(key string) bool {
	return protectedSettingsKeys[key] || strings.HasPrefix(key, "threat_intelligence.")
}

func threatIntelSettingKey(provider threatintel.Provider, field string) string {
	return "threat_intelligence." + string(provider) + "." + field
}

func (s *appThreatIntelligenceConfigStore) Statuses(context.Context) ([]threatintel.ProviderStatus, error) {
	settings := s.settingsSnapshot()
	statuses := make([]threatintel.ProviderStatus, 0, len(orderedThreatIntelProviders))
	for _, provider := range orderedThreatIntelProviders {
		statuses = append(statuses, s.statusFromSettings(provider, settings))
	}
	return statuses, nil
}

func (s *appThreatIntelligenceConfigStore) Config(_ context.Context, provider threatintel.Provider) (threatintel.ProviderConfig, error) {
	if err := validateThreatIntelProvider(provider); err != nil {
		return threatintel.ProviderConfig{}, err
	}

	settings := s.settingsSnapshot()
	cfg := threatintel.ProviderConfig{
		Provider: provider,
		Enabled:  settingBoolOrFallback(settings, threatIntelSettingKey(provider, "enabled"), false),
	}
	ciphertext := strings.TrimSpace(settings[threatIntelSettingKey(provider, "credential")])
	if ciphertext == "" {
		return cfg, nil
	}
	credential, err := s.credentialCipher().Decrypt(ciphertext)
	if err != nil {
		return cfg, err
	}
	cfg.Credential = credential
	return cfg, nil
}

func (s *appThreatIntelligenceConfigStore) Update(ctx context.Context, provider threatintel.Provider, update threatintel.ProviderConfigUpdate) (threatintel.ProviderStatus, error) {
	if err := validateThreatIntelProvider(provider); err != nil {
		return threatintel.ProviderStatus{}, err
	}

	credential := ""
	if update.Credential != nil {
		credential = strings.TrimSpace(*update.Credential)
	}
	if credential != "" && update.ClearCredential {
		return s.currentStatus(provider), errors.New("不能同时提交凭据和清除凭据标记")
	}

	settings := map[string]string{
		threatIntelSettingKey(provider, "enabled"): strconv.FormatBool(update.Enabled),
	}
	switch {
	case update.ClearCredential:
		settings[threatIntelSettingKey(provider, "credential")] = ""
	case credential != "":
		encrypted, err := s.credentialCipher().Encrypt(credential)
		if err != nil {
			return s.currentStatus(provider), err
		}
		settings[threatIntelSettingKey(provider, "credential")] = encrypted
	}

	if err := s.app.saveNormalizedSettings(ctx, settings); err != nil {
		return s.currentStatus(provider), err
	}
	s.app.applyNormalizedSettings(settings)
	return s.currentStatus(provider), nil
}

func (s *appThreatIntelligenceConfigStore) RecordTest(ctx context.Context, provider threatintel.Provider, status threatintel.ProviderTestStatus) error {
	if err := validateThreatIntelProvider(provider); err != nil {
		return err
	}

	settings := map[string]string{
		threatIntelSettingKey(provider, "last_test_status"):  status.Status,
		threatIntelSettingKey(provider, "last_test_message"): status.Message,
		threatIntelSettingKey(provider, "last_tested_at"):    status.TestedAt.UTC().Format(time.RFC3339Nano),
	}
	if err := s.app.saveNormalizedSettings(ctx, settings); err != nil {
		return err
	}
	s.app.applyNormalizedSettings(settings)
	return nil
}

func (s *appThreatIntelligenceConfigStore) currentStatus(provider threatintel.Provider) threatintel.ProviderStatus {
	return s.statusFromSettings(provider, s.settingsSnapshot())
}

func (s *appThreatIntelligenceConfigStore) statusFromSettings(provider threatintel.Provider, settings map[string]string) threatintel.ProviderStatus {
	status := threatintel.ProviderStatus{
		Provider:        provider,
		Name:            threatintel.ProviderName(provider),
		Enabled:         settingBoolOrFallback(settings, threatIntelSettingKey(provider, "enabled"), false),
		LastTestStatus:  settings[threatIntelSettingKey(provider, "last_test_status")],
		LastTestMessage: settings[threatIntelSettingKey(provider, "last_test_message")],
	}
	if testedAt, err := time.Parse(time.RFC3339Nano, settings[threatIntelSettingKey(provider, "last_tested_at")]); err == nil {
		status.LastTestedAt = &testedAt
	}

	ciphertext := strings.TrimSpace(settings[threatIntelSettingKey(provider, "credential")])
	status.Configured = ciphertext != ""
	if status.Configured {
		if _, err := s.credentialCipher().Decrypt(ciphertext); err != nil {
			status.CredentialError = credentialStatusError
		}
	}
	return status
}

func (s *appThreatIntelligenceConfigStore) settingsSnapshot() map[string]string {
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()

	settings := make(map[string]string, len(s.app.settings))
	for key, value := range s.app.settings {
		settings[key] = value
	}
	return settings
}

func (s *appThreatIntelligenceConfigStore) credentialCipher() *threatintel.CredentialCipher {
	if s.cipher != nil {
		return s.cipher
	}
	return threatintel.NewCredentialCipher(s.app.cfg.ThreatIntelligenceKeyFile)
}

func validateThreatIntelProvider(provider threatintel.Provider) error {
	if _, ok := threatintel.ParseProvider(string(provider)); !ok {
		return errors.New("未知威胁情报平台")
	}
	return nil
}
