package config

import (
	"errors"
	"strings"
)

type DashboardConfig struct {
	// OutputDirectory is where "<hostname>.html" dashboards are written.
	OutputDirectory string
	// TemplateFile, if set, overrides the compiled-in dashboard template.
	TemplateFile string
	// DescribeFile, if set, points at a "systemd-report describe" dump used
	// to annotate metrics with their type and description.
	DescribeFile string
	// Hosts lists the hostnames offered by the dashboard's host switcher,
	// read from the comma-separated "Hosts" key.
	Hosts []string
	// LogLevel controls slog's minimum level: debug, info, warn, or error.
	LogLevel string
}

func dashboardDefaults() DashboardConfig {
	return DashboardConfig{
		OutputDirectory: "/var/lib/sd-report-collector/dashboards",
		LogLevel:        "info",
	}
}

func LoadDashboard() (*DashboardConfig, error) {
	kf, err := readConfig(projectName, usrSubdir, configName, configSuffix)
	if errors.Is(err, ErrNoConfigFile) {
		cfg := dashboardDefaults()
		return &cfg, nil
	}
	if err != nil {
		return nil, err
	}
	defer kf.Close()

	cfg := dashboardDefaults()

	if cfg.OutputDirectory, err = kf.getString("Dashboard", "OutputDirectory", cfg.OutputDirectory); err != nil {
		return nil, err
	}
	if cfg.TemplateFile, err = kf.getString("Dashboard", "TemplateFile", cfg.TemplateFile); err != nil {
		return nil, err
	}
	if cfg.DescribeFile, err = kf.getString("Dashboard", "DescribeFile", cfg.DescribeFile); err != nil {
		return nil, err
	}
	hostsCSV, err := kf.getString("Dashboard", "Hosts", "")
	if err != nil {
		return nil, err
	}
	cfg.Hosts = splitCSV(hostsCSV)
	if cfg.LogLevel, err = kf.getStringFallback([]string{"Global", "Dashboard"}, "LogLevel", cfg.LogLevel); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// splitCSV splits a comma-separated list, trimming whitespace and dropping
// empty entries.
func splitCSV(s string) []string {
	var result []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
