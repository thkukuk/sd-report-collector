package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"sd-report-collector/internal/config"
	"sd-report-collector/internal/plugin"
	"sd-report-collector/internal/report"
	"sd-report-collector/internal/store"
	"sd-report-collector/internal/verify"
)

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hostname := clientHostname(r)
	if hostname == "" {
		http.Error(w, "no usable client certificate identity", http.StatusForbidden)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxReportSizeBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Warn("Rejecting report: body read failed", "hostname", hostname, "error", err)
		http.Error(w, "request body too large or unreadable", http.StatusRequestEntityTooLarge)
		return
	}

	meta := report.ExtractMeta(body)
	timestamp := meta.Timestamp
	if timestamp.IsZero() {
		slog.Warn("Report has no usable timestamp field, using receive time", "hostname", hostname)
		timestamp = time.Now()
	}

	result := verify.Check(body, hostname, s.trustStore)
	if result.Signed && result.Err != nil {
		path, qerr := store.Save(s.cfg.QuarantineDirectory, hostname, timestamp, meta.ReportID, body)
		if qerr != nil {
			slog.Log(r.Context(), config.LevelCritical, "Signed report failed verification and could not be quarantined",
				"hostname", hostname, "error", result.Err, "quarantine_error", qerr)
		} else {
			slog.Log(r.Context(), config.LevelCritical, "Signed report failed verification; quarantined",
				"hostname", hostname, "path", path, "error", result.Err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	path, err := store.Save(s.cfg.BaseDirectory, hostname, timestamp, meta.ReportID, body)
	if err != nil {
		slog.Error("Failed to save report", "hostname", hostname, "error", err)
		http.Error(w, "failed to store report", http.StatusInternalServerError)
		return
	}
	slog.Info("Report saved", "hostname", hostname, "path", path, "signed", result.Signed, "origin", result.Origin)

	if err := store.Prune(s.cfg.BaseDirectory, hostname, s.cfg.MaxReportsPerHost); err != nil {
		slog.Warn("Failed to prune old reports", "hostname", hostname, "error", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	if len(s.cfg.Plugins) > 0 {
		s.plugins.Add(1)
		go func() {
			defer s.plugins.Done()
			plugin.RunAll(s.cfg.Plugins, time.Duration(s.cfg.PluginTimeoutSec)*time.Second, string(result.Origin), path)
		}()
	}
}

// clientHostname derives a filesystem-safe host identity from the verified
// mTLS client certificate: its Subject CommonName, falling back to the first
// DNS SAN. The JSON payload's own hostname field is never used for this,
// since it is attacker-controlled.
func clientHostname(r *http.Request) string {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return ""
	}
	cert := r.TLS.PeerCertificates[0]
	if cert.Subject.CommonName != "" {
		return cert.Subject.CommonName
	}
	if len(cert.DNSNames) > 0 {
		return cert.DNSNames[0]
	}
	return ""
}
