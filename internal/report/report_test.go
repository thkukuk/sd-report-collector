package report

import (
	"testing"
	"time"
)

func TestExtractMetaPlainJSON(t *testing.T) {
	body := []byte(`{"mediaType":"application/vnd.io.systemd.report","reportID":"abc123/+==","timestamp":"Mon 2026-10-05 07:23:28 UTC","metrics":[]}`)

	meta := ExtractMeta(body)
	if meta.ReportID != "abc123/+==" {
		t.Errorf("ReportID = %q, want %q", meta.ReportID, "abc123/+==")
	}
	want := time.Date(2026, 10, 5, 7, 23, 28, 0, time.UTC)
	if !meta.Timestamp.Equal(want) {
		t.Errorf("Timestamp = %v, want %v", meta.Timestamp, want)
	}
}

func TestExtractMetaJSONSeq(t *testing.T) {
	report := `{"reportID":"sig-test","timestamp":"Mon 2026-10-05 07:23:28 UTC","metrics":[]}`
	signature := `{"alg":"ed25519","sig":"deadbeef"}`
	body := []byte("\x1e" + report + "\n\x1e" + signature + "\n")

	meta := ExtractMeta(body)
	if meta.ReportID != "sig-test" {
		t.Errorf("ReportID = %q, want %q", meta.ReportID, "sig-test")
	}
	want := time.Date(2026, 10, 5, 7, 23, 28, 0, time.UTC)
	if !meta.Timestamp.Equal(want) {
		t.Errorf("Timestamp = %v, want %v", meta.Timestamp, want)
	}
}

func TestExtractMetaFallback(t *testing.T) {
	cases := [][]byte{
		nil,
		[]byte(""),
		[]byte("not json"),
		[]byte(`{"timestamp":"not a valid timestamp"}`),
	}
	for _, body := range cases {
		meta := ExtractMeta(body)
		if !meta.Timestamp.IsZero() {
			t.Errorf("ExtractMeta(%q).Timestamp = %v, want zero value", body, meta.Timestamp)
		}
	}
}
