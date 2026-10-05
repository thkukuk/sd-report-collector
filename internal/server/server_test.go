package server

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"sd-report-collector/internal/config"
)

type pemPair struct {
	cert []byte
	key  []byte
}

func mustEncode(t *testing.T, der []byte, key *ecdsa.PrivateKey) pemPair {
	t.Helper()
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshaling key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	return pemPair{cert: certPEM, key: keyPEM}
}

func generateCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey, pemPair) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating CA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating CA certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parsing CA certificate: %v", err)
	}
	return cert, key, mustEncode(t, der, key)
}

func issueCert(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, cn string, serial int64) pemPair {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key for %s: %v", cn, err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("creating certificate for %s: %v", cn, err)
	}
	return mustEncode(t, der, key)
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// TestHandleReportEndToEnd exercises the full stack: mTLS handshake with a
// client certificate, the POST handler, zstd-compressed storage under the
// client's CommonName, and asynchronous plugin invocation.
func TestHandleReportEndToEnd(t *testing.T) {
	dir := t.TempDir()
	ca, caKey, caPEM := generateCA(t)
	serverPEM := issueCert(t, ca, caKey, "localhost", 2)
	clientPEM := issueCert(t, ca, caKey, "myhost.example.com", 3)

	writeFile(t, filepath.Join(dir, "ca.pem"), caPEM.cert)
	writeFile(t, filepath.Join(dir, "server.pem"), serverPEM.cert)
	writeFile(t, filepath.Join(dir, "server.key"), serverPEM.key)

	baseDir := filepath.Join(dir, "reports")
	pluginLog := filepath.Join(dir, "plugin.log")
	pluginScript := filepath.Join(dir, "plugin.sh")
	writeFile(t, pluginScript, []byte("#!/bin/sh\necho \"$1\" >> "+pluginLog+"\n"))
	if err := os.Chmod(pluginScript, 0o755); err != nil {
		t.Fatalf("chmod plugin script: %v", err)
	}

	cfg := &config.Config{
		ListenAddress:         "127.0.0.1:0",
		ListenPath:            "/report",
		BaseDirectory:         baseDir,
		ServerCertificateFile: filepath.Join(dir, "server.pem"),
		ServerKeyFile:         filepath.Join(dir, "server.key"),
		ClientCAFile:          filepath.Join(dir, "ca.pem"),
		MaxReportSizeBytes:    1 << 20,
		PluginTimeoutSec:      5,
		Plugins:               []config.Plugin{{Name: "stub", Path: pluginScript}},
	}

	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	tlsLn := tls.NewListener(ln, srv.httpSrv.TLSConfig)
	go srv.httpSrv.Serve(tlsLn)
	defer srv.httpSrv.Close()

	clientCert, err := tls.X509KeyPair(clientPEM.cert, clientPEM.key)
	if err != nil {
		t.Fatalf("loading client keypair: %v", err)
	}
	rootPool := x509.NewCertPool()
	rootPool.AppendCertsFromPEM(caPEM.cert)

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{clientCert},
				RootCAs:      rootPool,
			},
		},
	}

	body, err := os.ReadFile("../../test-data/report.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	port := ln.Addr().(*net.TCPAddr).Port
	url := "https://localhost:" + strconv.Itoa(port) + "/report"
	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	hostDir := filepath.Join(baseDir, "myhost.example.com")
	entries, err := os.ReadDir(hostDir)
	if err != nil {
		t.Fatalf("reading host dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one saved report, got %d", len(entries))
	}
	if !strings.HasSuffix(entries[0].Name(), ".json.zst") {
		t.Errorf("saved file name = %q, want *.json.zst", entries[0].Name())
	}

	deadline := time.Now().Add(2 * time.Second)
	var logContent []byte
	for time.Now().Before(deadline) {
		logContent, err = os.ReadFile(pluginLog)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("plugin did not run in time: %v", err)
	}
	wantPath := filepath.Join(hostDir, entries[0].Name())
	if !strings.Contains(string(logContent), wantPath) {
		t.Errorf("plugin log = %q, want to contain %q", logContent, wantPath)
	}
}
