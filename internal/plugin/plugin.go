// Package plugin runs the configured list of external programs against a
// freshly saved report file.
package plugin

import (
	"context"
	"log/slog"
	"os/exec"
	"time"

	"sd-report-collector/internal/config"
)

// RunAll invokes every plugin in order, as "<path> <signStatus> <fullPath>".
// signStatus tells the plugin whether the report was signed and, if so, how
// much that signature is worth trusting: "unsigned", "trusted" (verified
// against an administrator-provisioned key), "known" (verified against a
// key learned from an earlier report), or "first-seen" (verified against
// the report's own key, seen for the first time) — see
// internal/verify.Origin. Each plugin gets up to timeout to finish; a
// failing or slow plugin is logged and does not stop the rest from running.
func RunAll(plugins []config.Plugin, timeout time.Duration, signStatus, fullPath string) {
	for _, p := range plugins {
		runOne(p, timeout, signStatus, fullPath)
	}
}

func runOne(p config.Plugin, timeout time.Duration, signStatus, fullPath string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	slog.Debug("Plugin started", "plugin", p.Name, "path", p.Path, "report", fullPath, "sign_status", signStatus)
	cmd := exec.CommandContext(ctx, p.Path, signStatus, fullPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("Plugin failed", "plugin", p.Name, "path", p.Path, "report", fullPath, "sign_status", signStatus, "error", err, "output", string(output))
		return
	}
	slog.Debug("Plugin finished", "plugin", p.Name, "path", p.Path, "report", fullPath, "sign_status", signStatus)
}
