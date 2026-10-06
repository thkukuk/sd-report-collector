package config

import "errors"

type DashboardConfig struct {
	// OutputDirectory is where "<hostname>.html" dashboards are written.
	OutputDirectory string
	// TemplateFile, if set, overrides the compiled-in dashboard template.
	TemplateFile string
	// DescribeFile, if set, points at a "systemd-report describe" dump used
	// to annotate metrics with their type and description.
	DescribeFile string
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
	if cfg.LogLevel, err = kf.getStringFallback([]string{"Global", "Dashboard"}, "LogLevel", cfg.LogLevel); err != nil {
		return nil, err
	}

	return &cfg, nil
}
