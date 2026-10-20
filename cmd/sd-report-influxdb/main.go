// Command sd-report-influxdb is an sd-report-collector plugin that exports a
// saved systemd-report's metrics to InfluxDB, for display on a Grafana
// dashboard. sd-report-collector invokes it as
// "sd-report-influxdb <sign-status> <full-path-to-saved-report>" after every
// report it stores. sign-status is one of "unsigned", "trusted", "known", or
// "first-seen"; see internal/verify.Origin.
package main

import (
	"log/slog"
	"os"

	"sd-report-collector/internal/config"
	"sd-report-collector/internal/influxdb"
)

func main() {
	if len(os.Args) != 3 {
		slog.Error("Usage: sd-report-influxdb <sign-status> <path-to-saved-report>")
		os.Exit(2)
	}
	signStatus := os.Args[1]
	reportPath := os.Args[2]

	cfg, err := config.LoadInfluxDB()
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

	slog.Debug("sd-report-influxdb", "report", reportPath, "sign_status", signStatus)

	hostname, points, err := influxdb.Generate(cfg, signStatus, reportPath)
	if err != nil {
		slog.Error("Failed to export report to InfluxDB", "report", reportPath, "error", err)
		os.Exit(1)
	}

	slog.Info("Exported report to InfluxDB", "hostname", hostname, "points", points)
}
