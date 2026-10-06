package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Rooting a real /usr, /run, /etc hierarchy to exercise readConfig's UAPI.6
// search order isn't practical in a unit test (covered instead by the manual
// end-to-end test); this exercises the same getters against a single
// hand-written file via econf_readFile.
func TestGetStringAndKeysAgainstHandWrittenFile(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, "sd-report-collector.conf")
	content := `[Server]
ListenAddress=127.0.0.1:9443
MaxReportSizeBytes=12345

[Plugins]
archive=/bin/true
notify=/bin/false
`
	if err := os.WriteFile(confPath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing fixture config: %v", err)
	}

	kf, err := readFile(confPath)
	if err != nil {
		t.Fatalf("readFileForTest: %v", err)
	}
	defer kf.Close()

	addr, err := kf.getString("Server", "ListenAddress", "default")
	if err != nil {
		t.Fatalf("getString: %v", err)
	}
	if addr != "127.0.0.1:9443" {
		t.Errorf("ListenAddress = %q, want %q", addr, "127.0.0.1:9443")
	}

	missing, err := kf.getString("Server", "DoesNotExist", "fallback")
	if err != nil {
		t.Fatalf("getString missing key: %v", err)
	}
	if missing != "fallback" {
		t.Errorf("missing key = %q, want %q", missing, "fallback")
	}

	size, err := kf.getInt("Server", "MaxReportSizeBytes", 0)
	if err != nil {
		t.Fatalf("getInt: %v", err)
	}
	if size != 12345 {
		t.Errorf("MaxReportSizeBytes = %d, want 12345", size)
	}

	keys, err := kf.getKeys("Plugins")
	if err != nil {
		t.Fatalf("getKeys: %v", err)
	}
	if len(keys) != 2 || keys[0] != "archive" || keys[1] != "notify" {
		t.Errorf("getKeys(Plugins) = %v, want sorted [archive notify]", keys)
	}

	empty, err := kf.getKeys("NoSuchGroup")
	if err != nil {
		t.Fatalf("getKeys on absent group: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("getKeys(NoSuchGroup) = %v, want empty", empty)
	}
}

func TestGetStringFallback(t *testing.T) {
	dir := t.TempDir()

	write := func(name, content string) *keyFile {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("writing fixture config: %v", err)
		}
		kf, err := readFile(path)
		if err != nil {
			t.Fatalf("readFile: %v", err)
		}
		t.Cleanup(kf.Close)
		return kf
	}

	t.Run("prefers Global over Server", func(t *testing.T) {
		kf := write("global-wins.conf", "[Global]\nLogLevel=debug\n\n[Server]\nLogLevel=warn\n")
		got, err := kf.getStringFallback([]string{"Global", "Server"}, "LogLevel", "info")
		if err != nil {
			t.Fatalf("getStringFallback: %v", err)
		}
		if got != "debug" {
			t.Errorf("LogLevel = %q, want %q", got, "debug")
		}
	})

	t.Run("falls back to Server when Global unset", func(t *testing.T) {
		kf := write("server-only.conf", "[Server]\nLogLevel=warn\n")
		got, err := kf.getStringFallback([]string{"Global", "Server"}, "LogLevel", "info")
		if err != nil {
			t.Fatalf("getStringFallback: %v", err)
		}
		if got != "warn" {
			t.Errorf("LogLevel = %q, want %q", got, "warn")
		}
	})

	t.Run("uses default when neither set", func(t *testing.T) {
		kf := write("neither.conf", "[Server]\nListenAddress=127.0.0.1:9443\n")
		got, err := kf.getStringFallback([]string{"Global", "Server"}, "LogLevel", "info")
		if err != nil {
			t.Fatalf("getStringFallback: %v", err)
		}
		if got != "info" {
			t.Errorf("LogLevel = %q, want %q", got, "info")
		}
	})
}
