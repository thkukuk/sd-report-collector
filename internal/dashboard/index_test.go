package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"sd-report-collector/internal/config"
)

func TestGenerateWritesIndexForEachHost(t *testing.T) {
	body, err := os.ReadFile("../../test-data/report.json")
	if err != nil {
		t.Fatalf("reading fixture report: %v", err)
	}

	reportsDir := t.TempDir()
	outDir := t.TempDir()
	cfg := &config.DashboardConfig{OutputDirectory: outDir, StaleMinutes: 30}

	for _, host := range []string{"alpha", "beta"} {
		reportPath := writeCompressedReport(t, reportsDir, host, body)
		if _, _, err := Generate(cfg, "trusted", reportPath); err != nil {
			t.Fatalf("Generate(%s): %v", host, err)
		}
	}

	indexPath := filepath.Join(outDir, "index.html")
	html, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("reading index.html: %v", err)
	}
	content := string(html)

	for _, want := range []string{`"hostname":"alpha"`, `"hostname":"beta"`, `"systemState":"degraded"`, `"signStatus":"trusted"`} {
		if !strings.Contains(content, want) {
			t.Errorf("expected %q embedded in index.html", want)
		}
	}
	if !strings.Contains(content, `"staleMinutes":30`) {
		t.Errorf("expected stale minutes embedded in index.html")
	}
	if strings.Contains(content, "__INDEX_DATA_JSON__") {
		t.Errorf("placeholder left unsubstituted in index.html")
	}

	statePath := filepath.Join(outDir, indexStateFile)
	if _, err := os.Stat(statePath); err != nil {
		t.Errorf("expected index state file at %s: %v", statePath, err)
	}

	dataPath := filepath.Join(outDir, indexDataFile)
	data, err := os.ReadFile(dataPath)
	if err != nil {
		t.Fatalf("reading %s: %v", dataPath, err)
	}
	dataContent := string(data)
	for _, want := range []string{`"hostname":"alpha"`, `"hostname":"beta"`, `"staleMinutes":30`} {
		if !strings.Contains(dataContent, want) {
			t.Errorf("expected %q in %s", want, indexDataFile)
		}
	}
}

func TestGenerateIndexSurvivesMissingState(t *testing.T) {
	body, err := os.ReadFile("../../test-data/report.json")
	if err != nil {
		t.Fatalf("reading fixture report: %v", err)
	}

	reportsDir := t.TempDir()
	outDir := t.TempDir()
	reportPath := writeCompressedReport(t, reportsDir, "solo", body)
	cfg := &config.DashboardConfig{OutputDirectory: outDir}

	if _, _, err := Generate(cfg, "unsigned", reportPath); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "index.html")); err != nil {
		t.Errorf("expected index.html to be written: %v", err)
	}
}

func TestGenerateIndexUnderConcurrentHosts(t *testing.T) {
	body, err := os.ReadFile("../../test-data/report.json")
	if err != nil {
		t.Fatalf("reading fixture report: %v", err)
	}

	reportsDir := t.TempDir()
	outDir := t.TempDir()
	cfg := &config.DashboardConfig{OutputDirectory: outDir, StaleMinutes: 30}

	const n = 20
	hosts := make([]string, n)
	for i := range hosts {
		hosts[i] = fmt.Sprintf("host%02d", i)
	}

	var wg sync.WaitGroup
	for _, host := range hosts {
		reportPath := writeCompressedReport(t, reportsDir, host, body)
		wg.Add(1)
		go func(host, reportPath string) {
			defer wg.Done()
			if _, _, err := Generate(cfg, "trusted", reportPath); err != nil {
				t.Errorf("Generate(%s): %v", host, err)
			}
		}(host, reportPath)
	}
	wg.Wait()

	states, err := loadIndexState(filepath.Join(outDir, indexStateFile))
	if err != nil {
		t.Fatalf("loadIndexState: %v", err)
	}
	if len(states) != n {
		t.Errorf("got %d hosts in index state, want %d (lost update under concurrent writers)", len(states), n)
	}
}
