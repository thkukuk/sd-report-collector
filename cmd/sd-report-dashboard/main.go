// Command sd-report-dashboard is an sd-report-collector plugin that renders
// a saved systemd-report as a static HTML dashboard. sd-report-collector
// invokes it as "sd-report-dashboard <full-path-to-saved-report>" after
// every report it stores.
package main

import (
	"log/slog"
	"os"

	"sd-report-collector/internal/config"
	"sd-report-collector/internal/dashboard"
)

func main() {
	if len(os.Args) != 2 {
		slog.Error("Usage: sd-report-dashboard <path-to-saved-report>")
		os.Exit(2)
	}
	reportPath := os.Args[1]

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

	slog.Debug("sd-report-dashboard", "report", reportPath)

	outPath, metrics, err := dashboard.Generate(cfg, reportPath)
	if err != nil {
		slog.Error("Failed to render dashboard", "report", reportPath, "error", err)
		os.Exit(1)
	}

	slog.Info("Wrote dashboard", "path", outPath, "metrics", metrics)
}
