package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

func decompress(t *testing.T, path string) []byte {
	t.Helper()
	compressed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	dec, err := zstd.NewReader(nil)
	if err != nil {
		t.Fatalf("creating zstd reader: %v", err)
	}
	defer dec.Close()
	out, err := dec.DecodeAll(compressed, nil)
	if err != nil {
		t.Fatalf("decompressing %s: %v", path, err)
	}
	return out
}

func TestSaveRoundTrip(t *testing.T) {
	baseDir := t.TempDir()
	body, err := os.ReadFile("../../test-data/report.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	ts := time.Date(2026, 10, 5, 7, 23, 28, 0, time.UTC)

	path, err := Save(baseDir, "myhost", ts, "report-id-1", body)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	wantPath := filepath.Join(baseDir, "myhost", "myhost-20261005T072328Z.json.zst")
	if path != wantPath {
		t.Errorf("path = %q, want %q", path, wantPath)
	}

	got := decompress(t, path)
	if !bytes.Equal(got, body) {
		t.Errorf("decompressed content does not match original fixture")
	}
}

func TestSaveCollision(t *testing.T) {
	baseDir := t.TempDir()
	ts := time.Date(2026, 10, 5, 7, 23, 28, 0, time.UTC)

	path1, err := Save(baseDir, "myhost", ts, "report-id-1", []byte(`{"n":1}`))
	if err != nil {
		t.Fatalf("Save 1: %v", err)
	}
	path2, err := Save(baseDir, "myhost", ts, "report-id-2", []byte(`{"n":2}`))
	if err != nil {
		t.Fatalf("Save 2: %v", err)
	}
	if path1 == path2 {
		t.Fatalf("expected distinct paths for colliding timestamps, got %q twice", path1)
	}

	got1 := decompress(t, path1)
	got2 := decompress(t, path2)
	if string(got1) != `{"n":1}` || string(got2) != `{"n":2}` {
		t.Errorf("content mismatch: %q, %q", got1, got2)
	}
}

func TestSaveInvalidHostname(t *testing.T) {
	baseDir := t.TempDir()
	ts := time.Now()
	for _, h := range []string{"", ".", "..", "../etc", "a/b", "a\x00b"} {
		if _, err := Save(baseDir, h, ts, "id", []byte("{}")); err == nil {
			t.Errorf("Save with hostname %q: expected error, got nil", h)
		}
	}
}

func TestPruneKeepsNewestFiles(t *testing.T) {
	baseDir := t.TempDir()
	base := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)

	var paths []string
	for i := range 5 {
		ts := base.Add(time.Duration(i) * 5 * time.Minute)
		path, err := Save(baseDir, "myhost", ts, fmt.Sprintf("id-%d", i), []byte("{}"))
		if err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
		paths = append(paths, path)
	}

	if err := Prune(baseDir, "myhost", 2); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	for i, path := range paths {
		_, err := os.Stat(path)
		switch {
		case i < 3 && !os.IsNotExist(err):
			t.Errorf("expected %s to be pruned, stat err = %v", path, err)
		case i >= 3 && err != nil:
			t.Errorf("expected %s to survive, stat err = %v", path, err)
		}
	}
}

func TestPruneDisabled(t *testing.T) {
	baseDir := t.TempDir()
	ts := time.Now()
	path, err := Save(baseDir, "myhost", ts, "id", []byte("{}"))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := Prune(baseDir, "myhost", 0); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected %s to survive with maxFiles=0, stat err = %v", path, err)
	}
}

func TestPruneMissingDirectory(t *testing.T) {
	baseDir := t.TempDir()
	if err := Prune(baseDir, "nohost", 7); err != nil {
		t.Errorf("Prune on missing directory: %v", err)
	}
}
