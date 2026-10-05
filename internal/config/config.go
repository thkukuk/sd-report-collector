package config

import "errors"

const (
	projectName  = "sd-report-collector"
	usrSubdir    = "/usr/lib"
	configName   = "sd-report-collector"
	configSuffix = "conf"
)

// Plugin is a named external program invoked with the full path of a saved
// report after it has been written to disk.
type Plugin struct {
	Name string
	Path string
}

// Config holds sd-report-collector's runtime configuration, as read from
// sd-report-collector.conf (and its drop-ins) following the UAPI.6 spec.
type Config struct {
	ListenAddress         string
	ListenPath            string
	BaseDirectory         string
	ServerCertificateFile string
	ServerKeyFile         string
	ClientCAFile          string
	MaxReportSizeBytes    int64
	PluginTimeoutSec      int64
	Plugins               []Plugin
}

func defaults() Config {
	return Config{
		ListenAddress:         "0.0.0.0:8443",
		ListenPath:            "/report",
		BaseDirectory:         "/var/lib/sd-report-collector/reports",
		ServerCertificateFile: "/etc/ssl/certs/sd-report-collector.pem",
		ServerKeyFile:         "/etc/ssl/private/sd-report-collector.pem",
		ClientCAFile:          "/etc/ssl/sd-report-collector/ca.pem",
		MaxReportSizeBytes:    10 * 1024 * 1024,
		PluginTimeoutSec:      30,
	}
}

// Load reads the merged sd-report-collector configuration from the standard
// UAPI.6 search path (/usr/lib, /run, /etc, with .d drop-ins).
func Load() (*Config, error) {
	kf, err := readConfig(projectName, usrSubdir, configName, configSuffix)
	if errors.Is(err, ErrNoConfigFile) {
		cfg := defaults()
		return &cfg, nil
	}
	if err != nil {
		return nil, err
	}
	defer kf.Close()

	cfg := defaults()

	if cfg.ListenAddress, err = kf.getString("Server", "ListenAddress", cfg.ListenAddress); err != nil {
		return nil, err
	}
	if cfg.ListenPath, err = kf.getString("Server", "ListenPath", cfg.ListenPath); err != nil {
		return nil, err
	}
	if cfg.BaseDirectory, err = kf.getString("Server", "BaseDirectory", cfg.BaseDirectory); err != nil {
		return nil, err
	}
	if cfg.ServerCertificateFile, err = kf.getString("Server", "ServerCertificateFile", cfg.ServerCertificateFile); err != nil {
		return nil, err
	}
	if cfg.ServerKeyFile, err = kf.getString("Server", "ServerKeyFile", cfg.ServerKeyFile); err != nil {
		return nil, err
	}
	if cfg.ClientCAFile, err = kf.getString("Server", "ClientCAFile", cfg.ClientCAFile); err != nil {
		return nil, err
	}
	if cfg.MaxReportSizeBytes, err = kf.getInt("Server", "MaxReportSizeBytes", cfg.MaxReportSizeBytes); err != nil {
		return nil, err
	}
	if cfg.PluginTimeoutSec, err = kf.getInt("Server", "PluginTimeoutSec", cfg.PluginTimeoutSec); err != nil {
		return nil, err
	}

	names, err := kf.getKeys("Plugins")
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		path, err := kf.getString("Plugins", name, "")
		if err != nil {
			return nil, err
		}
		if path == "" {
			continue
		}
		cfg.Plugins = append(cfg.Plugins, Plugin{Name: name, Path: path})
	}

	return &cfg, nil
}
