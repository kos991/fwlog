package app

import (
	"context"
	"time"
)

type LogSource struct {
	SourceID  string    `json:"source_id"`
	LogDir    string    `json:"log_dir"`
	LogTag    string    `json:"log_tag"`
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AppSettings map[string]string

func AppSettingsSQL() string {
	return `SELECT key, value FROM app_settings FINAL`
}

func EnabledLogSourcesSQL() string {
	return `SELECT source_id, log_dir, log_tag, enabled, updated_at FROM log_sources FINAL WHERE enabled = 1`
}

func SettingsUpsertSQL() string {
	return `INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, now())`
}

func (s *ClickHouseStore) LoadSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.conn.Query(ctx, AppSettingsSQL())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key string
		var value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		settings[key] = value
	}
	return settings, rows.Err()
}

func (s *ClickHouseStore) SaveSettings(ctx context.Context, settings map[string]string) error {
	for key, value := range settings {
		if err := s.conn.Exec(ctx, SettingsUpsertSQL(), key, value); err != nil {
			return err
		}
	}
	return nil
}
