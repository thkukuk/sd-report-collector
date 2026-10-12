package config

import (
	"fmt"
	"log/slog"
	"strings"
)

// LevelCritical is a slog level above LevelError, reserved for events an
// operator must act on immediately, such as a report failing signature
// verification. It is not selectable via LogLevel: anything at LogLevel
// "error" or below already lets it through.
const LevelCritical = slog.LevelError + 4

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
