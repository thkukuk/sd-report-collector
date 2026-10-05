// Package store saves incoming reports to disk, zstd-compressed, under a
// per-hostname directory.
package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

// filenameTimestampLayout renders a time.Time filesystem-safely, e.g.
// "20261005T072328Z".
const filenameTimestampLayout = "20060102T150405Z"

// ErrInvalidHostname is returned when a hostname is unsafe to use as a path
// component (path separators, "..", empty, etc).
var ErrInvalidHostname = errors.New("invalid hostname")

func sanitizeHostname(hostname string) (string, error) {
	if hostname == "" || hostname == "." || hostname == ".." {
		return "", ErrInvalidHostname
	}
	if strings.ContainsAny(hostname, "/\x00") {
		return "", ErrInvalidHostname
	}
	return hostname, nil
}

// Save compresses body with zstd and writes it to
// "<baseDir>/<hostname>/<hostname>-<timestamp>.json.zst", returning the full
// absolute path of the written file. If that name is already taken (e.g. two
// reports from the same host within the same second), reportID disambiguates
// it.
func Save(baseDir, hostname string, timestamp time.Time, reportID string, body []byte) (string, error) {
	hostname, err := sanitizeHostname(hostname)
	if err != nil {
		return "", err
	}

	dir := filepath.Join(baseDir, hostname)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}

	compressed, err := compress(body)
	if err != nil {
		return "", fmt.Errorf("compressing report: %w", err)
	}

	ts := timestamp.UTC().Format(filenameTimestampLayout)
	name := fmt.Sprintf("%s-%s.json.zst", hostname, ts)
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		name = fmt.Sprintf("%s-%s-%s.json.zst", hostname, ts, shortID(reportID))
		path = filepath.Join(dir, name)
	}

	if err := writeAtomic(path, compressed); err != nil {
		return "", err
	}
	return path, nil
}

func compress(data []byte) ([]byte, error) {
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		return nil, err
	}
	defer enc.Close()
	return enc.EncodeAll(data, nil), nil
}

// writeAtomic writes data to a temp file alongside path and renames it into
// place, so readers (e.g. a plugin, or a concurrent request) never observe a
// partially written file.
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
	if err := os.Chmod(tmpPath, 0o640); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming %s to %s: %w", tmpPath, path, err)
	}
	return nil
}

// shortID returns a short, filesystem-safe fragment derived from reportID
// (which may contain base64 characters like '/' and '+'), for disambiguating
// filename collisions.
func shortID(reportID string) string {
	var b strings.Builder
	for _, r := range reportID {
		if b.Len() >= 12 {
			break
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return b.String()
}
