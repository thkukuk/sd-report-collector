// Command sd-report-influxdb is an sd-report-collector plugin that exports a
// saved systemd-report's metrics to InfluxDB, for display on a Grafana
// dashboard. sd-report-collector invokes it as
// "sd-report-influxdb <full-path-to-saved-report>" after every report it
// stores.
package main

import (
	"log/slog"
	"os"

	"sd-report-collector/internal/config"
	"sd-report-collector/internal/influxdb"
)

func main() {
	if len(os.Args) != 2 {
		slog.Error("Usage: sd-report-influxdb <path-to-saved-report>")
		os.Exit(2)
	}
	reportPath := os.Args[1]

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

	slog.Debug("sd-report-influxdb", "report", reportPath)

	hostname, points, err := influxdb.Generate(cfg, reportPath)
	if err != nil {
		slog.Error("Failed to export report to InfluxDB", "report", reportPath, "error", err)
		os.Exit(1)
	}

	slog.Info("Exported report to InfluxDB", "hostname", hostname, "points", points)
}
