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

// RunAll invokes every plugin in order, passing fullPath as its single
// argument. Each plugin gets up to timeout to finish; a failing or slow
// plugin is logged and does not stop the rest from running.
func RunAll(plugins []config.Plugin, timeout time.Duration, fullPath string) {
	for _, p := range plugins {
		runOne(p, timeout, fullPath)
	}
}

func runOne(p config.Plugin, timeout time.Duration, fullPath string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, p.Path, fullPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("plugin failed", "plugin", p.Name, "path", p.Path, "report", fullPath, "error", err, "output", string(output))
		return
	}
	slog.Debug("plugin finished", "plugin", p.Name, "path", p.Path, "report", fullPath)
}
