package config

import "errors"

// InfluxDBConfig configures the sd-report-influxdb plugin.
type InfluxDBConfig struct {
	// Server is the InfluxDB server hostname or IP.
	Server string
	// Port is the InfluxDB server port.
	Port string
	// TLS selects https instead of http when connecting to Server.
	TLS bool
	// Bucket is the InfluxDB v2 bucket (or v1 database) metrics are written
	// into.
	Bucket string
	// Organization owns Bucket. Required for InfluxDB v2, ignored by v1.
	Organization string
	// Token authenticates against InfluxDB v2 ("username:password" for v1).
	// Overridden by the INFLUXDB_TOKEN environment variable, if set.
	Token string
	// ExcludeMetrics lists metric family name patterns excluded from export,
	// read from the comma-separated "ExcludeMetrics" key. A pattern ending in
	// "*" excludes every family with that prefix (e.g. "io.systemd.Manager.*");
	// any other pattern must match a family name exactly.
	ExcludeMetrics []string
	// LogLevel controls slog's minimum level: debug, info, warn, or error.
	LogLevel string
}

func influxDBDefaults() InfluxDBConfig {
	return InfluxDBConfig{
		Server:   "localhost",
		Port:     "8086",
		Bucket:   "sd-report-collector",
		LogLevel: "info",
	}
}

// LoadInfluxDB reads the merged sd-report-collector configuration's
// [InfluxDB] section from the standard UAPI.6 search path.
func LoadInfluxDB() (*InfluxDBConfig, error) {
	kf, err := readConfig(projectName, usrSubdir, configName, configSuffix)
	if errors.Is(err, ErrNoConfigFile) {
		cfg := influxDBDefaults()
		return &cfg, nil
	}
	if err != nil {
		return nil, err
	}
	defer kf.Close()

	cfg := influxDBDefaults()

	if cfg.Server, err = kf.getString("InfluxDB", "Server", cfg.Server); err != nil {
		return nil, err
	}
	if cfg.Port, err = kf.getString("InfluxDB", "Port", cfg.Port); err != nil {
		return nil, err
	}
	if cfg.TLS, err = kf.getBool("InfluxDB", "TLS", cfg.TLS); err != nil {
		return nil, err
	}
	if cfg.Bucket, err = kf.getString("InfluxDB", "Bucket", cfg.Bucket); err != nil {
		return nil, err
	}
	if cfg.Organization, err = kf.getString("InfluxDB", "Organization", cfg.Organization); err != nil {
		return nil, err
	}
	if cfg.Token, err = kf.getString("InfluxDB", "Token", cfg.Token); err != nil {
		return nil, err
	}
	excludeCSV, err := kf.getString("InfluxDB", "ExcludeMetrics", "")
	if err != nil {
		return nil, err
	}
	cfg.ExcludeMetrics = splitCSV(excludeCSV)
	if cfg.LogLevel, err = kf.getStringFallback([]string{"Global", "InfluxDB"}, "LogLevel", cfg.LogLevel); err != nil {
		return nil, err
	}

	return &cfg, nil
}
