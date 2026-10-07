// Package dashboard renders a saved systemd-report as a static, self
// contained HTML dashboard, for use as an sd-report-collector plugin.
package dashboard

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/klauspost/compress/zstd"

	"sd-report-collector/internal/config"
)

//go:embed dashboard_template.html
var defaultTemplate []byte

// reportMeta is the subset of a report's fields needed to report how many
// metrics it contained.
type reportMeta struct {
	Metrics []json.RawMessage `json:"metrics"`
}

// Generate renders reportPath (the full path to a saved, zstd-compressed
// systemd-report, as passed to every sd-report-collector plugin) into an
// HTML dashboard under cfg.OutputDirectory, named after the report's
// hostname (taken from reportPath's parent directory, matching how
// internal/store lays out saved reports). It returns the path written and
// the number of metrics the report contained.
func Generate(cfg *config.DashboardConfig, reportPath string) (string, int, error) {
	compressed, err := os.ReadFile(reportPath)
	if err != nil {
		return "", 0, fmt.Errorf("reading %s: %w", reportPath, err)
	}

	reportJSON, err := decompress(compressed)
	if err != nil {
		return "", 0, fmt.Errorf("decompressing %s: %w", reportPath, err)
	}

	var meta reportMeta
	if err := json.Unmarshal(reportJSON, &meta); err != nil {
		return "", 0, fmt.Errorf("parsing report JSON: %w", err)
	}

	describe := map[string]DescribeEntry{}
	if cfg.DescribeFile != "" {
		describe, err = ParseDescribe(cfg.DescribeFile)
		if err != nil {
			return "", 0, fmt.Errorf("reading describe file %s: %w", cfg.DescribeFile, err)
		}
	}

	tmpl, err := loadTemplate(cfg.TemplateFile)
	if err != nil {
		return "", 0, err
	}

	hostname := filepath.Base(filepath.Dir(reportPath))
	if hostname == "." || hostname == string(filepath.Separator) || hostname == "" {
		hostname = "unknown"
	}

	html, err := render(tmpl, reportJSON, describe, hostList(cfg.Hosts, hostname), hostname)
	if err != nil {
		return "", 0, err
	}

	if err := os.MkdirAll(cfg.OutputDirectory, 0o750); err != nil {
		return "", 0, fmt.Errorf("creating %s: %w", cfg.OutputDirectory, err)
	}
	outPath := filepath.Join(cfg.OutputDirectory, hostname+".html")
	if err := writeAtomic(outPath, html); err != nil {
		return "", 0, err
	}

	return outPath, len(meta.Metrics), nil
}

func decompress(data []byte) ([]byte, error) {
	dec, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	return dec.DecodeAll(data, nil)
}

func loadTemplate(path string) ([]byte, error) {
	if path == "" {
		return defaultTemplate, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading template %s: %w", path, err)
	}
	return data, nil
}

// hostList returns the configured hosts plus current (deduplicated and
// sorted), so the dashboard's host switcher always offers at least the host
// it was just generated for, even if it's missing from the configuration.
func hostList(configured []string, current string) []string {
	set := map[string]struct{}{current: {}}
	for _, h := range configured {
		set[h] = struct{}{}
	}
	hosts := make([]string, 0, len(set))
	for h := range set {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	return hosts
}

// render substitutes the template's JSON placeholders. reportJSON is
// compacted in place (not re-marshaled) so large integer timestamps survive
// byte-for-byte; describe, the errno table and the host list are small and
// built by us, so marshaling them fresh is fine.
func render(tmpl []byte, reportJSON []byte, describe map[string]DescribeEntry, hosts []string, currentHost string) ([]byte, error) {
	var compactReport bytes.Buffer
	if err := json.Compact(&compactReport, reportJSON); err != nil {
		return nil, fmt.Errorf("compacting report JSON: %w", err)
	}

	descJSON, err := json.Marshal(describe)
	if err != nil {
		return nil, fmt.Errorf("marshaling describe data: %w", err)
	}
	errnoJSON, err := json.Marshal(linuxErrnoNames)
	if err != nil {
		return nil, fmt.Errorf("marshaling errno table: %w", err)
	}
	hostsJSON, err := json.Marshal(hosts)
	if err != nil {
		return nil, fmt.Errorf("marshaling host list: %w", err)
	}
	currentHostJSON, err := json.Marshal(currentHost)
	if err != nil {
		return nil, fmt.Errorf("marshaling current host: %w", err)
	}

	html := string(tmpl)
	html = strings.Replace(html, "__REPORT_JSON__", escapeScript(compactReport.Bytes()), 1)
	html = strings.Replace(html, "__DESCRIBE_JSON__", escapeScript(descJSON), 1)
	html = strings.Replace(html, "__ERRNO_JSON__", escapeScript(errnoJSON), 1)
	html = strings.Replace(html, "__HOSTS_JSON__", escapeScript(hostsJSON), 1)
	html = strings.Replace(html, "__CURRENT_HOST_JSON__", escapeScript(currentHostJSON), 1)
	return []byte(html), nil
}

// escapeScript prevents a literal "</script>" inside embedded JSON from
// closing the surrounding <script> tag early.
func escapeScript(data []byte) string {
	return strings.ReplaceAll(string(data), "</", "<\\/")
}

// writeAtomic writes data to a temp file alongside path and renames it into
// place, so a concurrent reader never observes a partially written
// dashboard.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once successfully renamed

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", tmpPath, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("syncing %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming %s to %s: %w", tmpPath, path, err)
	}
	return nil
}
