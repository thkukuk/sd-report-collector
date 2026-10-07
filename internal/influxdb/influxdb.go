// Package influxdb renders a saved systemd-report into InfluxDB points and
// writes them to a configured server, for use as an sd-report-collector
// plugin feeding a Grafana dashboard.
package influxdb

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/klauspost/compress/zstd"

	"sd-report-collector/internal/config"
	"sd-report-collector/internal/report"
)

// reportDoc is the subset of a report's fields needed to build InfluxDB
// points from its metrics.
type reportDoc struct {
	Metrics []reportMetric `json:"metrics"`
}

// reportMetric mirrors one entry of a report's "metrics" array: a metric
// family name, an optional object it applies to (a unit, mount point, device,
// ...), optional label fields, and its value (a JSON number, string, or
// bool).
type reportMetric struct {
	Name   string            `json:"name"`
	Object string            `json:"object,omitempty"`
	Fields map[string]string `json:"fields,omitempty"`
	Value  interface{}       `json:"value"`
}

// Generate renders reportPath (the full path to a saved, zstd-compressed
// systemd-report, as passed to every sd-report-collector plugin) into
// InfluxDB points and writes them to cfg's server, tagged with the report's
// hostname (taken from reportPath's parent directory, matching how
// internal/store lays out saved reports). It returns the hostname and the
// number of points written.
func Generate(cfg *config.InfluxDBConfig, reportPath string) (string, int, error) {
	hostname, points, err := buildPoints(reportPath, cfg.ExcludeMetrics)
	if err != nil {
		return "", 0, err
	}
	if len(points) == 0 {
		return hostname, 0, nil
	}

	client, err := connect(cfg)
	if err != nil {
		return "", 0, err
	}
	defer client.Close()

	writeAPI := client.WriteAPIBlocking(cfg.Organization, cfg.Bucket)
	if err := writeAPI.WritePoint(context.Background(), points...); err != nil {
		return "", 0, fmt.Errorf("writing points to InfluxDB: %w", err)
	}

	return hostname, len(points), nil
}

// buildPoints decompresses and parses reportPath, returning its hostname and
// one InfluxDB point per metric not matched by exclude, all timestamped with
// the report's own timestamp (falling back to the current time if the report
// has none). See matchesAny for exclude's pattern syntax.
func buildPoints(reportPath string, exclude []string) (string, []*write.Point, error) {
	compressed, err := os.ReadFile(reportPath)
	if err != nil {
		return "", nil, fmt.Errorf("reading %s: %w", reportPath, err)
	}

	reportJSON, err := decompress(compressed)
	if err != nil {
		return "", nil, fmt.Errorf("decompressing %s: %w", reportPath, err)
	}

	var doc reportDoc
	if err := json.Unmarshal(reportJSON, &doc); err != nil {
		return "", nil, fmt.Errorf("parsing report JSON: %w", err)
	}

	hostname := filepath.Base(filepath.Dir(reportPath))
	if hostname == "." || hostname == string(filepath.Separator) || hostname == "" {
		hostname = "unknown"
	}

	timestamp := report.ExtractMeta(reportJSON).Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	points := make([]*write.Point, 0, len(doc.Metrics))
	for _, m := range doc.Metrics {
		if m.Name == "" || matchesAny(m.Name, exclude) {
			continue
		}
		points = append(points, metricPoint(m, hostname, timestamp))
	}
	return hostname, points, nil
}

// matchesAny reports whether name matches any of patterns: a pattern ending
// in "*" matches any family name with that prefix (e.g. "io.systemd.Manager.*"
// excludes the whole io.systemd.Manager family); any other pattern must match
// name exactly.
func matchesAny(name string, patterns []string) bool {
	for _, p := range patterns {
		if prefix, ok := strings.CutSuffix(p, "*"); ok {
			if strings.HasPrefix(name, prefix) {
				return true
			}
		} else if name == p {
			return true
		}
	}
	return false
}

// metricPoint turns one report metric into an InfluxDB point: the metric
// family name as measurement, "host" (and "object", if any) plus the
// metric's own label fields as tags, and the value under a single "value"
// field. The report's reportID is deliberately not stored anywhere here: it
// is unique per upload, and tagging points with it would explode series
// cardinality for no benefit.
func metricPoint(m reportMetric, hostname string, timestamp time.Time) *write.Point {
	tags := make(map[string]string, len(m.Fields)+2)
	tags["host"] = hostname
	if m.Object != "" {
		tags["object"] = m.Object
	}
	for k, v := range m.Fields {
		tags[k] = v
	}

	fields := map[string]interface{}{"value": m.Value}
	return influxdb2.NewPoint(m.Name, tags, fields, timestamp)
}

func decompress(data []byte) ([]byte, error) {
	dec, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	return dec.DecodeAll(data, nil)
}

// connect opens a client against cfg's InfluxDB server and verifies it is
// reachable. INFLUXDB_TOKEN, if set, overrides cfg.Token.
func connect(cfg *config.InfluxDBConfig) (influxdb2.Client, error) {
	token := cfg.Token
	if envToken := os.Getenv("INFLUXDB_TOKEN"); envToken != "" {
		token = envToken
	}

	protocol := "http"
	if cfg.TLS {
		protocol = "https"
	}
	serverURL := fmt.Sprintf("%s://%s:%s", protocol, cfg.Server, cfg.Port)

	client := influxdb2.NewClient(serverURL, token)
	ok, err := client.Ping(context.Background())
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("connecting to InfluxDB at %s: %w", serverURL, err)
	}
	if !ok {
		client.Close()
		return nil, fmt.Errorf("InfluxDB at %s did not respond to ping", serverURL)
	}
	return client, nil
}
