package influxdb

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

func writeCompressedReport(t *testing.T, dir, hostname string, body []byte) string {
	t.Helper()

	enc, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatalf("creating zstd writer: %v", err)
	}
	defer enc.Close()
	compressed := enc.EncodeAll(body, nil)

	hostDir := filepath.Join(dir, hostname)
	if err := os.MkdirAll(hostDir, 0o755); err != nil {
		t.Fatalf("creating host dir: %v", err)
	}
	path := filepath.Join(hostDir, hostname+"-20261005T072328Z.json.zst")
	if err := os.WriteFile(path, compressed, 0o644); err != nil {
		t.Fatalf("writing fixture report: %v", err)
	}
	return path
}

func TestBuildPointsFromFixtureReport(t *testing.T) {
	body, err := os.ReadFile("../../test-data/report.json")
	if err != nil {
		t.Fatalf("reading fixture report: %v", err)
	}

	reportsDir := t.TempDir()
	reportPath := writeCompressedReport(t, reportsDir, "myhost", body)

	hostname, points, err := buildPoints(reportPath, nil)
	if err != nil {
		t.Fatalf("buildPoints: %v", err)
	}
	if hostname != "myhost" {
		t.Errorf("hostname = %q, want %q", hostname, "myhost")
	}
	if len(points) != 3232 {
		t.Errorf("len(points) = %d, want 3232", len(points))
	}

	wantTime, err := time.Parse(time.RFC3339, "2026-10-05T07:23:28Z")
	if err != nil {
		t.Fatalf("parsing expected time: %v", err)
	}
	for _, p := range points {
		if !p.Time().Equal(wantTime) {
			t.Fatalf("point %s timestamp = %v, want %v", p.Name(), p.Time(), wantTime)
		}
	}
}

func TestBuildPointsTagsAndFields(t *testing.T) {
	body := []byte(`{
		"timestamp": "Mon 2026-10-05 07:23:28 UTC",
		"metrics": [
			{"name": "io.systemd.Basic.LoadAverage1Min", "value": 0.125},
			{"name": "io.systemd.Manager.UnitActiveState", "object": "sshd.service", "value": "active"},
			{"name": "io.systemd.DiskSpace.FreeBytes", "object": "/", "value": 123, "fields": {"source": "/dev/vda2", "fileSystemType": "btrfs"}}
		]
	}`)

	reportsDir := t.TempDir()
	reportPath := writeCompressedReport(t, reportsDir, "otherhost", body)

	_, points, err := buildPoints(reportPath, nil)
	if err != nil {
		t.Fatalf("buildPoints: %v", err)
	}
	if len(points) != 3 {
		t.Fatalf("len(points) = %d, want 3", len(points))
	}

	diskPoint := points[2]
	if diskPoint.Name() != "io.systemd.DiskSpace.FreeBytes" {
		t.Errorf("measurement = %q", diskPoint.Name())
	}
	tags := map[string]string{}
	for _, tag := range diskPoint.TagList() {
		tags[tag.Key] = tag.Value
	}
	want := map[string]string{
		"host":           "otherhost",
		"object":         "/",
		"source":         "/dev/vda2",
		"fileSystemType": "btrfs",
	}
	for k, v := range want {
		if tags[k] != v {
			t.Errorf("tag %q = %q, want %q", k, tags[k], v)
		}
	}

	fields := diskPoint.FieldList()
	if len(fields) != 1 || fields[0].Key != "value" {
		t.Fatalf("fields = %+v, want a single %q field", fields, "value")
	}
	if fields[0].Value.(float64) != 123 {
		t.Errorf("value field = %v, want 123", fields[0].Value)
	}
}

func TestBuildPointsSkipsUnnamedMetrics(t *testing.T) {
	body := []byte(`{"metrics": [{"value": 1}, {"name": "x", "value": 1}]}`)

	reportsDir := t.TempDir()
	reportPath := writeCompressedReport(t, reportsDir, "host", body)

	_, points, err := buildPoints(reportPath, nil)
	if err != nil {
		t.Fatalf("buildPoints: %v", err)
	}
	if len(points) != 1 {
		t.Errorf("len(points) = %d, want 1", len(points))
	}
}

func TestBuildPointsExcludesMatchingMetrics(t *testing.T) {
	body := []byte(`{"metrics": [
		{"name": "io.systemd.Manager.UnitsTotal", "value": 1},
		{"name": "io.systemd.Manager.SystemState", "value": "running"},
		{"name": "io.systemd.Basic.CPUsOnline", "value": 4},
		{"name": "io.systemd.Basic.LoadAverage1Min", "value": 0.1}
	]}`)

	reportsDir := t.TempDir()
	reportPath := writeCompressedReport(t, reportsDir, "host", body)

	exclude := []string{"io.systemd.Manager.*", "io.systemd.Basic.CPUsOnline"}
	_, points, err := buildPoints(reportPath, exclude)
	if err != nil {
		t.Fatalf("buildPoints: %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("len(points) = %d, want 1", len(points))
	}
	if points[0].Name() != "io.systemd.Basic.LoadAverage1Min" {
		t.Errorf("surviving metric = %q, want %q", points[0].Name(), "io.systemd.Basic.LoadAverage1Min")
	}
}

func TestMatchesAny(t *testing.T) {
	patterns := []string{"io.systemd.Manager.*", "io.systemd.Basic.CPUsOnline"}

	cases := map[string]bool{
		"io.systemd.Manager.UnitsTotal":    true,
		"io.systemd.Manager.":              true,
		"io.systemd.Basic.CPUsOnline":      true,
		"io.systemd.Basic.CPUsOnline2":     false,
		"io.systemd.Basic.LoadAverage1Min": false,
		"org.openSUSE.rebootmgr.Version":   false,
	}
	for name, want := range cases {
		if got := matchesAny(name, patterns); got != want {
			t.Errorf("matchesAny(%q) = %v, want %v", name, got, want)
		}
	}
}
