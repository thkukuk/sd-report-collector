// Package report does the minimal parsing needed of an incoming
// systemd-report payload: pulling out the report's own timestamp and ID so
// the server can name the file it stores, without caring about the rest of
// the report's structure.
package report

import (
	"bytes"
	"encoding/json"
	"time"
)

// recordSeparator is RFC 7464's JSON-SEQ record separator, used by
// systemd-report when a report is cryptographically signed: the report
// object followed by one or more signature objects.
const recordSeparator = 0x1E

// timestampLayout matches the format systemd-report emits, e.g.
// "Mon 2026-10-05 07:23:28 UTC".
const timestampLayout = "Mon 2006-01-02 15:04:05 MST"

// Meta is the subset of a report's fields the server needs.
type Meta struct {
	Timestamp time.Time
	ReportID  string
}

// firstRecord returns the first JSON record in body, stripping the JSON-SEQ
// framing if present.
func firstRecord(body []byte) []byte {
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	if len(trimmed) == 0 || trimmed[0] != recordSeparator {
		return trimmed
	}
	rest := trimmed[1:]
	if end := bytes.IndexByte(rest, recordSeparator); end >= 0 {
		rest = rest[:end]
	}
	return bytes.TrimRight(rest, "\n")
}

// ExtractMeta parses just enough of body (a plain JSON report object, or a
// JSON-SEQ stream when signed) to recover the report's own timestamp and ID.
// It never fails: on any parse problem it returns the zero Meta, and the
// caller is expected to fall back to its own receive time.
func ExtractMeta(body []byte) Meta {
	var fields struct {
		Timestamp string `json:"timestamp"`
		ReportID  string `json:"reportID"`
	}
	if err := json.Unmarshal(firstRecord(body), &fields); err != nil {
		return Meta{}
	}

	meta := Meta{ReportID: fields.ReportID}
	if fields.Timestamp != "" {
		if ts, err := time.Parse(timestampLayout, fields.Timestamp); err == nil {
			meta.Timestamp = ts
		}
	}
	return meta
}
