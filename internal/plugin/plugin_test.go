package plugin

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"sd-report-collector/internal/config"
)

func TestRunAllInvokesPluginWithFullPath(t *testing.T) {
	dir := t.TempDir()
	recordFile := filepath.Join(dir, "recorded-arg")
	script := filepath.Join(dir, "stub.sh")
	scriptBody := "#!/bin/sh\necho -n \"$1\" > " + recordFile + "\n"
	if err := os.WriteFile(script, []byte(scriptBody), 0o755); err != nil {
		t.Fatalf("writing stub script: %v", err)
	}

	reportPath := filepath.Join(dir, "myhost-20261005T072328Z.json.zst")
	plugins := []config.Plugin{{Name: "stub", Path: script}}

	RunAll(plugins, 5*time.Second, reportPath)

	got, err := os.ReadFile(recordFile)
	if err != nil {
		t.Fatalf("reading recorded arg: %v", err)
	}
	if string(got) != reportPath {
		t.Errorf("plugin received argv[1] = %q, want %q", got, reportPath)
	}
}

func TestRunAllContinuesAfterFailure(t *testing.T) {
	dir := t.TempDir()
	okFile := filepath.Join(dir, "ok-ran")
	failing := filepath.Join(dir, "failing.sh")
	ok := filepath.Join(dir, "ok.sh")

	if err := os.WriteFile(failing, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("writing failing script: %v", err)
	}
	if err := os.WriteFile(ok, []byte("#!/bin/sh\ntouch "+okFile+"\n"), 0o755); err != nil {
		t.Fatalf("writing ok script: %v", err)
	}

	plugins := []config.Plugin{
		{Name: "failing", Path: failing},
		{Name: "ok", Path: ok},
	}
	RunAll(plugins, 5*time.Second, filepath.Join(dir, "report.json.zst"))

	if _, err := os.Stat(okFile); err != nil {
		t.Errorf("expected ok plugin to run despite earlier failure: %v", err)
	}
}
