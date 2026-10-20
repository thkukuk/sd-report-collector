// Command sd-report-dashboard is an sd-report-collector plugin that renders
// a saved systemd-report as a static HTML dashboard. sd-report-collector
// invokes it as "sd-report-dashboard <sign-status> <full-path-to-saved-report>"
// after every report it stores. sign-status is one of "unsigned", "trusted",
// "known", or "first-seen"; see internal/verify.Origin.
package main

import (
	"log/slog"
	"os"

	"sd-report-collector/internal/config"
	"sd-report-collector/internal/dashboard"
)

func main() {
	if len(os.Args) != 3 {
		slog.Error("Usage: sd-report-dashboard <sign-status> <path-to-saved-report>")
		os.Exit(2)
	}
	signStatus := os.Args[1]
	reportPath := os.Args[2]

	cfg, err := config.LoadDashboard()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	level, err := config.ParseLogLevel(cfg.LogLevel)
	if err != nil {
		slog.Error("Invalid LogLevel", "value", cfg.LogLevel, "error", err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	slog.Debug("sd-report-dashboard", "report", reportPath, "sign_status", signStatus)

	outPath, metrics, err := dashboard.Generate(cfg, signStatus, reportPath)
	if err != nil {
		slog.Error("Failed to render dashboard", "report", reportPath, "error", err)
		os.Exit(1)
	}

	slog.Info("Wrote dashboard", "path", outPath, "metrics", metrics)
}
