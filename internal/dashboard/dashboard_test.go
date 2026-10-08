package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"

	"sd-report-collector/internal/config"
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

func TestGenerateWritesDashboardNamedAfterHostname(t *testing.T) {
	body, err := os.ReadFile("../../test-data/report.json")
	if err != nil {
		t.Fatalf("reading fixture report: %v", err)
	}

	reportsDir := t.TempDir()
	reportPath := writeCompressedReport(t, reportsDir, "myhost", body)

	outDir := t.TempDir()
	cfg := &config.DashboardConfig{
		OutputDirectory: outDir,
		DescribeFile:    "../../test-data/report.describe",
	}

	outPath, metrics, err := Generate(cfg, reportPath)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	wantPath := filepath.Join(outDir, "myhost.html")
	if outPath != wantPath {
		t.Errorf("outPath = %q, want %q", outPath, wantPath)
	}
	if metrics != 3749 {
		t.Errorf("metrics = %d, want 3749", metrics)
	}

	html, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading generated dashboard: %v", err)
	}
	content := string(html)

	if strings.Contains(content, "__REPORT_JSON__") ||
		strings.Contains(content, "__DESCRIBE_JSON__") ||
		strings.Contains(content, "__ERRNO_JSON__") {
		t.Errorf("placeholder left unsubstituted in output")
	}
	if !strings.Contains(content, `"org.openSUSE.rebootmgr.RebootStatus"`) {
		t.Errorf("expected report metric name in output")
	}
	if !strings.Contains(content, "Overall system state") {
		t.Errorf("expected describe-file description in output")
	}
	if !strings.Contains(content, `"EPERM"`) {
		t.Errorf("expected errno table in output")
	}
}

func TestGenerateWithoutDescribeFile(t *testing.T) {
	body, err := os.ReadFile("../../test-data/report.json")
	if err != nil {
		t.Fatalf("reading fixture report: %v", err)
	}

	reportsDir := t.TempDir()
	reportPath := writeCompressedReport(t, reportsDir, "otherhost", body)

	outDir := t.TempDir()
	cfg := &config.DashboardConfig{OutputDirectory: outDir}

	outPath, _, err := Generate(cfg, reportPath)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if outPath != filepath.Join(outDir, "otherhost.html") {
		t.Errorf("outPath = %q", outPath)
	}
}

func TestGenerateWithCustomTemplate(t *testing.T) {
	body := []byte(`{"metrics":[{"name":"x","value":1}]}`)

	reportsDir := t.TempDir()
	reportPath := writeCompressedReport(t, reportsDir, "tmplhost", body)

	tmplPath := filepath.Join(t.TempDir(), "custom.html")
	custom := "<html>__REPORT_JSON__|__DESCRIBE_JSON__|__ERRNO_JSON__</html>"
	if err := os.WriteFile(tmplPath, []byte(custom), 0o644); err != nil {
		t.Fatalf("writing custom template: %v", err)
	}

	outDir := t.TempDir()
	cfg := &config.DashboardConfig{OutputDirectory: outDir, TemplateFile: tmplPath}

	outPath, metrics, err := Generate(cfg, reportPath)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if metrics != 1 {
		t.Errorf("metrics = %d, want 1", metrics)
	}

	html, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading generated dashboard: %v", err)
	}
	if !strings.Contains(string(html), `{"metrics":[{"name":"x","value":1}]}`) {
		t.Errorf("custom template not used, got: %s", html)
	}
}
