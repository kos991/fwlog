package main

import "time"

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
