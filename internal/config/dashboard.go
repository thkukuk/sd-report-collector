package config

import "errors"

const (
	dashboardProjectName = "sd-report-dashboard"
	dashboardConfigName  = "sd-report-dashboard"
)

// DashboardConfig holds the runtime configuration for the sd-report-dashboard
// plugin, as read from sd-report-dashboard.conf (and its drop-ins) following
// the UAPI.6 spec.
type DashboardConfig struct {
	// OutputDirectory is where "<hostname>.html" dashboards are written.
	OutputDirectory string
	// TemplateFile, if set, overrides the compiled-in dashboard template.
	TemplateFile string
	// DescribeFile, if set, points at a "systemd-report describe" dump used
	// to annotate metrics with their type and description.
	DescribeFile string
}

func dashboardDefaults() DashboardConfig {
	return DashboardConfig{
		OutputDirectory: "/var/lib/sd-report-collector/dashboards",
	}
}

// LoadDashboard reads the merged sd-report-dashboard configuration from the
// standard UAPI.6 search path (/usr/lib, /run, /etc, with .d drop-ins).
func LoadDashboard() (*DashboardConfig, error) {
	kf, err := readConfig(dashboardProjectName, usrSubdir, dashboardConfigName, configSuffix)
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

	return &cfg, nil
}
