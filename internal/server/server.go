// Package server implements the mTLS-authenticated HTTP endpoint that
// receives systemd-report uploads.
package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"sd-report-collector/internal/config"
)

// Server is the mTLS report-receiving daemon.
type Server struct {
	cfg     *config.Config
	httpSrv *http.Server
	plugins sync.WaitGroup
}

// New builds a Server from cfg, loading the server certificate/key and the
// client CA bundle used to require and verify client certificates (mTLS).
func New(cfg *config.Config) (*Server, error) {
	tlsConfig, err := buildTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	s := &Server{cfg: cfg}

	mux := http.NewServeMux()
	mux.HandleFunc(cfg.ListenPath, s.handleReport)

	s.httpSrv = &http.Server{
		Addr:      cfg.ListenAddress,
		Handler:   mux,
		TLSConfig: tlsConfig,
	}
	return s, nil
}

func buildTLSConfig(cfg *config.Config) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(cfg.ServerCertificateFile, cfg.ServerKeyFile)
	if err != nil {
		return nil, fmt.Errorf("loading server certificate/key: %w", err)
	}

	caPEM, err := os.ReadFile(cfg.ClientCAFile)
	if err != nil {
		return nil, fmt.Errorf("reading client CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("no certificates found in %s", cfg.ClientCAFile)
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
	}, nil
}

// ListenAndServe starts accepting requests; it blocks until the server is
// shut down.
func (s *Server) ListenAndServe() error {
	return s.httpSrv.ListenAndServeTLS("", "")
}

// Shutdown gracefully stops accepting new requests and waits (up to timeout)
// for any in-flight plugin executions to finish.
func (s *Server) Shutdown(ctx context.Context) error {
	err := s.httpSrv.Shutdown(ctx)

	done := make(chan struct{})
	go func() {
		s.plugins.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
	}
	return err
}
