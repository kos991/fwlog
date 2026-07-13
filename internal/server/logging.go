package server

import (
	"log/slog"
	"os"
)

func initLogger() *slog.Logger {
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.String("time", a.Value.Time().Format("2006-01-02T15:04:05.000Z07:00"))
			}
			return a
		},
	})

	return slog.New(handler)
}
