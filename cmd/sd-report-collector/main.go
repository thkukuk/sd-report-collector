// Command sd-report-collector receives systemd-report uploads over mTLS, stores
// them zstd-compressed under a per-hostname directory, and hands each saved
// report off to configured plugins.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sd-report-collector/internal/config"
	"sd-report-collector/internal/server"
)

// replaceLevelAttr renders config.LevelCritical as "CRITICAL" instead of
// slog's default "ERROR+4", so quarantined-report log lines stand out.
func replaceLevelAttr(groups []string, a slog.Attr) slog.Attr {
	if a.Key == slog.LevelKey {
		if lvl, ok := a.Value.Any().(slog.Level); ok && lvl == config.LevelCritical {
			a.Value = slog.StringValue("CRITICAL")
		}
	}
	return a
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	level, err := config.ParseLogLevel(cfg.LogLevel)
	if err != nil {
		slog.Error("Invalid LogLevel", "value", cfg.LogLevel, "error", err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: replaceLevelAttr,
	})))

	srv, err := server.New(cfg)
	if err != nil {
		slog.Error("Failed to initialize server", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("Listening", "address", cfg.ListenAddress, "path", cfg.ListenPath)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		slog.Info("Shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("Error during shutdown", "error", err)
			os.Exit(1)
		}
	}
}
