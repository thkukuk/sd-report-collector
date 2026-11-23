package certgen

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestInitAndClientProduceUsableMTLSMaterial(t *testing.T) {
	baseDir := t.TempDir()

	if err := Init(baseDir, nil, false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	for _, f := range []string{"ca.pem", "ca.key", "server.pem", "server.key", "sd-report-collector.conf.d/certgen.conf"} {
		if _, err := os.Stat(filepath.Join(baseDir, f)); err != nil {
			t.Fatalf("expected %s to exist: %v", f, err)
		}
	}

	dropin, err := os.ReadFile(filepath.Join(baseDir, "sd-report-collector.conf.d", "certgen.conf"))
	if err != nil {
		t.Fatalf("reading config drop-in: %v", err)
	}
	for _, want := range []string{
		"ServerCertificateFile=" + filepath.Join(baseDir, "server.pem"),
		"ServerKeyFile=" + filepath.Join(baseDir, "server.key"),
		"ClientCAFile=" + filepath.Join(baseDir, "ca.pem"),
	} {
		if !strings.Contains(string(dropin), want) {
			t.Errorf("config drop-in missing %q; got:\n%s", want, dropin)
		}
	}

	if err := Client(baseDir, "myhost.example.com", false); err != nil {
		t.Fatalf("Client: %v", err)
	}
	clientCertPath := filepath.Join(baseDir, "clients", "myhost.example.com.pem")
	clientKeyPath := filepath.Join(baseDir, "clients", "myhost.example.com.key")

	serverCert, err := tls.LoadX509KeyPair(filepath.Join(baseDir, "server.pem"), filepath.Join(baseDir, "server.key"))
	if err != nil {
		t.Fatalf("loading server keypair: %v", err)
	}
	clientCert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
	if err != nil {
		t.Fatalf("loading client keypair: %v", err)
	}

	caPEM, err := os.ReadFile(filepath.Join(baseDir, "ca.pem"))
	if err != nil {
		t.Fatalf("reading CA cert: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("failed to add CA cert to pool")
	}

	verifyLeaf := func(t *testing.T, name string, raw [][]byte, keyUsage x509.ExtKeyUsage) {
		t.Helper()
		leaf, err := x509.ParseCertificate(raw[0])
		if err != nil {
			t.Fatalf("parsing %s leaf: %v", name, err)
		}
		if _, err := leaf.Verify(x509.VerifyOptions{
			Roots:     pool,
			KeyUsages: []x509.ExtKeyUsage{keyUsage},
		}); err != nil {
			t.Fatalf("verifying %s cert against CA: %v", name, err)
		}
	}

	verifyLeaf(t, "server", serverCert.Certificate, x509.ExtKeyUsageServerAuth)
	verifyLeaf(t, "client", clientCert.Certificate, x509.ExtKeyUsageClientAuth)
}

func TestInitRefusesToOverwriteWithoutForce(t *testing.T) {
	baseDir := t.TempDir()

	if err := Init(baseDir, nil, false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := Init(baseDir, nil, false); err == nil {
		t.Fatal("expected second Init without --force to fail")
	}
	if err := Init(baseDir, nil, true); err != nil {
		t.Fatalf("Init with --force: %v", err)
	}
}

func TestInitAddsExtraSANsToServerCertificate(t *testing.T) {
	baseDir := t.TempDir()

	if err := Init(baseDir, []string{"report.example.com", "192.168.3.16"}, false); err != nil {
		t.Fatalf("Init: %v", err)
	}

	certPEM, err := os.ReadFile(filepath.Join(baseDir, "server.pem"))
	if err != nil {
		t.Fatalf("reading server certificate: %v", err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("failed to decode server certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parsing server certificate: %v", err)
	}

	if !slices.Contains(cert.DNSNames, "report.example.com") {
		t.Errorf("expected DNSNames to contain %q, got %v", "report.example.com", cert.DNSNames)
	}
	wantIP := net.ParseIP("192.168.3.16")
	found := false
	for _, ip := range cert.IPAddresses {
		if ip.Equal(wantIP) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected IPAddresses to contain %v, got %v", wantIP, cert.IPAddresses)
	}
}

func TestClientRequiresInitFirst(t *testing.T) {
	baseDir := t.TempDir()

	if err := Client(baseDir, "somehost", false); err == nil {
		t.Fatal("expected Client to fail before Init has been run")
	}
}

func TestClientRefusesToOverwriteWithoutForce(t *testing.T) {
	baseDir := t.TempDir()

	if err := Init(baseDir, nil, false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := Client(baseDir, "somehost", false); err != nil {
		t.Fatalf("Client: %v", err)
	}
	if err := Client(baseDir, "somehost", false); err == nil {
		t.Fatal("expected second Client without --force to fail")
	}
	if err := Client(baseDir, "somehost", true); err != nil {
		t.Fatalf("Client with --force: %v", err)
	}
}

func TestClientRejectsInvalidNames(t *testing.T) {
	baseDir := t.TempDir()
	if err := Init(baseDir, nil, false); err != nil {
		t.Fatalf("Init: %v", err)
	}

	for _, name := range []string{"", ".", "..", "foo/bar", "..\\bar"} {
		if err := Client(baseDir, name, false); err == nil {
			t.Fatalf("expected Client(%q) to fail", name)
		}
	}
}
