// Command sd-report-dashboard is an sd-report-collector plugin that renders
// a saved systemd-report as a static HTML dashboard. sd-report-collector
// invokes it as "sd-report-dashboard <full-path-to-saved-report>" after
// every report it stores.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"sd-report-collector/internal/config"
	"sd-report-collector/internal/dashboard"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <path-to-saved-report>\n", os.Args[0])
		os.Exit(2)
	}
	reportPath := os.Args[1]

	cfg, err := config.LoadDashboard()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	outPath, metrics, err := dashboard.Generate(cfg, reportPath)
	if err != nil {
		slog.Error("Failed to render dashboard", "report", reportPath, "error", err)
		os.Exit(1)
	}

	fmt.Printf("Wrote %s (%d metrics)\n", outPath, metrics)
}
