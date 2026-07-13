package storage

import "context"

type SettingsStore interface {
	LoadSettings(context.Context) (map[string]string, error)
	SaveSettings(context.Context, map[string]string) error
}
