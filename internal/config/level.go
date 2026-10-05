package config

import (
	"fmt"
	"log/slog"
	"strings"
)

// ParseLogLevel maps a LogLevel config value ("debug", "info", "warn", or
// "error", case-insensitive) to the corresponding slog.Level.
func ParseLogLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("invalid log level %q", s)
	}
}
