// Package influxdb renders a saved systemd-report into InfluxDB points and
// writes them to a configured server, for use as an sd-report-collector
// plugin feeding a Grafana dashboard.
package influxdb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
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
	Name   string                 `json:"name"`
	Object string                 `json:"object,omitempty"`
	Fields map[string]interface{} `json:"fields,omitempty"`
	Value  interface{}            `json:"value"`
}

// Generate renders reportPath (the full path to a saved systemd-report, as
// passed to every sd-report-collector plugin, zstd-compressed or not) into
// InfluxDB points and writes them to cfg's server, tagged with the report's
// hostname (taken from reportPath's parent directory, matching how
// internal/store lays out saved reports) and signStatus, the sign-status
// sd-report-collector passed as this plugin's first argument ("unsigned",
// "trusted", "known", or "first-seen"; see internal/verify.Origin). It
// returns the hostname and the number of points written.
func Generate(cfg *config.InfluxDBConfig, signStatus, reportPath string) (string, int, error) {
	hostname, points, err := buildPoints(reportPath, signStatus, cfg.ExcludeMetrics)
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
// has none) and tagged with signStatus. See matchesAny for exclude's pattern
// syntax.
func buildPoints(reportPath, signStatus string, exclude []string) (string, []*write.Point, error) {
	compressed, err := os.ReadFile(reportPath)
	if err != nil {
		return "", nil, fmt.Errorf("reading %s: %w", reportPath, err)
	}

	reportJSON, err := decompress(compressed)
	if err != nil {
		return "", nil, fmt.Errorf("decompressing %s: %w", reportPath, err)
	}
	reportJSON = report.FirstRecord(reportJSON)

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
		if m.Name == "" || matchesAny(m.Name, exclude) || !scalarValue(m.Value) {
			continue
		}
		points = append(points, metricPoint(m, hostname, signStatus, timestamp))
	}
	return hostname, points, nil
}

// scalarValue reports whether v (a decoded JSON metric value) can be written
// as a single InfluxDB field. Object- and array-valued metrics (e.g.
// io.systemd.Journal.HighPriorityMessage, whose value is a full journal
// entry) can't be flattened into one field, and including them would make
// the whole batch write fail instead of just that point.
func scalarValue(v interface{}) bool {
	switch v.(type) {
	case map[string]interface{}, []interface{}:
		return false
	default:
		return true
	}
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
// family name as measurement, "host", "sign_status" (and "object", if any)
// plus the metric's own label fields as tags, and the value under a single
// "value" field. The report's reportID is deliberately not stored anywhere
// here: it is unique per upload, and tagging points with it would explode
// series cardinality for no benefit.
func metricPoint(m reportMetric, hostname, signStatus string, timestamp time.Time) *write.Point {
	tags := make(map[string]string, len(m.Fields)+3)
	tags["host"] = hostname
	if signStatus != "" {
		tags["sign_status"] = signStatus
	}
	if m.Object != "" {
		tags["object"] = m.Object
	}
	for k, v := range m.Fields {
		tags[k] = fieldTagValue(v)
	}

	fields := map[string]interface{}{"value": m.Value}
	return influxdb2.NewPoint(m.Name, tags, fields, timestamp)
}

// fieldTagValue renders a metric's "fields" entry (decoded from JSON, so a
// string, bool, or float64) as the string InfluxDB tags require. JSON numbers
// that are whole, like an interface's "ifindex", are rendered without a
// trailing ".0".
func fieldTagValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == math.Trunc(val) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'g', -1, 64)
	default:
		return fmt.Sprint(val)
	}
}

// zstdMagic is the 4-byte magic number at the start of every zstd frame
// (RFC 8878), used to tell a compressed report apart from one passed in
// uncompressed, e.g. when this plugin is invoked manually on a raw report.
var zstdMagic = []byte{0x28, 0xB5, 0x2F, 0xFD}

func decompress(data []byte) ([]byte, error) {
	if !bytes.HasPrefix(data, zstdMagic) {
		return data, nil
	}
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
