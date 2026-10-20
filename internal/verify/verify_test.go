package verify

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const testHost = "testhost"

func TestCheckUnsigned(t *testing.T) {
	body := []byte(`{"mediaType":"application/vnd.io.systemd.report","metrics":[]}`)
	result := Check(body, testHost, TrustStore{})
	if result.Signed {
		t.Fatalf("Signed = true, want false")
	}
	if result.Origin != OriginUnsigned {
		t.Errorf("Origin = %q, want %q", result.Origin, OriginUnsigned)
	}
	if result.Err != nil {
		t.Fatalf("Err = %v, want nil", result.Err)
	}
}

func TestCheckEmptyAndWhitespace(t *testing.T) {
	for _, body := range [][]byte{nil, []byte(""), []byte("   \n")} {
		if result := Check(body, testHost, TrustStore{}); result.Signed {
			t.Errorf("Check(%q).Signed = true, want false", body)
		}
	}
}

func TestCheckReportOnlyRecordIsUnsigned(t *testing.T) {
	// A JSON-SEQ stream with just the report record and no signature
	// trailer is not meaningfully "signed".
	body := []byte("\x1e{\"mediaType\":\"application/vnd.io.systemd.report\"}\n")
	result := Check(body, testHost, TrustStore{})
	if result.Signed {
		t.Fatalf("Signed = true, want false")
	}
}

func TestCheckRealFixtureFirstSeenThenKnown(t *testing.T) {
	body, err := os.ReadFile("../../test-data/signed-report.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	store := TrustStore{KnownDir: t.TempDir()}

	result := Check(body, testHost, store)
	if !result.Signed {
		t.Fatalf("Signed = false, want true")
	}
	if result.Err != nil {
		t.Fatalf("Err = %v, want nil", result.Err)
	}
	if result.Origin != OriginFirstSeen {
		t.Errorf("Origin = %q, want %q", result.Origin, OriginFirstSeen)
	}
	if _, err := os.Stat(filepath.Join(store.KnownDir, testHost+".pem")); err != nil {
		t.Errorf("expected first-seen key to be stored: %v", err)
	}

	// A second report from the same host now matches the learned key.
	result = Check(body, testHost, store)
	if result.Err != nil {
		t.Fatalf("second Check: Err = %v, want nil", result.Err)
	}
	if result.Origin != OriginKnown {
		t.Errorf("second Check: Origin = %q, want %q", result.Origin, OriginKnown)
	}
}

func TestCheckRealFixtureTampered(t *testing.T) {
	body, err := os.ReadFile("../../test-data/signed-report.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	// Flip a byte inside the report record, well before the signature
	// trailer, without touching the JSON structure's length.
	i := bytes.IndexByte(body, '{') + 20
	tampered := append([]byte(nil), body...)
	tampered[i] ^= 0xFF

	store := TrustStore{KnownDir: t.TempDir()}
	result := Check(tampered, testHost, store)
	if !result.Signed {
		t.Fatalf("Signed = false, want true")
	}
	if result.Err == nil {
		t.Fatalf("Err = nil, want a verification failure")
	}
}

// signedOpts tunes buildSigned's output for tests that need to break one
// specific part of an otherwise-valid signed report.
type signedOpts struct {
	embeddedKeyOverride string
	attachedKeyOverride string
	mutate              func(sig *signaturePayload)
}

type signaturePayload struct {
	mechanism string
	sha256    string
	signature string
	key       string
}

// buildSigned constructs a minimal signed JSON-SEQ stream using a freshly
// generated Ed25519 key, mirroring exactly what systemd-report-sign-plain(8)
// emits: the report embeds the public key as the
// "io.systemd.Report.SignPlain.PublicKey" metric, and the signature trailer
// attaches the same key alongside the signature.
func buildSigned(t *testing.T, opts signedOpts) []byte {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	spki, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshaling public key: %v", err)
	}
	pubPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: spki}))

	embeddedKey := pubPEM
	if opts.embeddedKeyOverride != "" {
		embeddedKey = opts.embeddedKeyOverride
	}
	reportJSON := fmt.Sprintf(
		`{"mediaType":"application/vnd.io.systemd.report","metrics":[{"name":%q,"value":%q}]}`,
		embeddedKeyMetric, embeddedKey)

	reportRecord := append([]byte{recordSeparator}, append([]byte(reportJSON), '\n')...)
	digest := sha256.Sum256(reportRecord)
	sigBytes := ed25519.Sign(priv, digest[:])

	attachedKey := pubPEM
	if opts.attachedKeyOverride != "" {
		attachedKey = opts.attachedKeyOverride
	}
	payload := signaturePayload{
		mechanism: mechanismPlain,
		sha256:    hex.EncodeToString(digest[:]),
		signature: base64.StdEncoding.EncodeToString(sigBytes),
		key:       attachedKey,
	}
	if opts.mutate != nil {
		opts.mutate(&payload)
	}

	sigJSON := fmt.Sprintf(
		`{"mediaType":"application/vnd.io.systemd.report.signature","mechanism":%q,"sha256":%q,"data":{"signature":%q,"key":%q}}`,
		payload.mechanism, payload.sha256, payload.signature, payload.key)

	var out bytes.Buffer
	out.Write(reportRecord)
	out.WriteByte(recordSeparator)
	out.WriteString(sigJSON)
	out.WriteByte('\n')
	return out.Bytes()
}

func TestCheckSyntheticValid(t *testing.T) {
	body := buildSigned(t, signedOpts{})
	result := Check(body, testHost, TrustStore{KnownDir: t.TempDir()})
	if !result.Signed || result.Err != nil {
		t.Fatalf("Signed = %v, Err = %v, want Signed=true, Err=nil", result.Signed, result.Err)
	}
	if result.Origin != OriginFirstSeen {
		t.Errorf("Origin = %q, want %q", result.Origin, OriginFirstSeen)
	}
}

func TestCheckSyntheticTamperedDigest(t *testing.T) {
	body := buildSigned(t, signedOpts{mutate: func(sig *signaturePayload) {
		sig.sha256 = "00" + sig.sha256[2:]
	}})
	result := Check(body, testHost, TrustStore{KnownDir: t.TempDir()})
	if !result.Signed {
		t.Fatalf("Signed = false, want true")
	}
	if result.Err == nil {
		t.Fatalf("Err = nil, want digest mismatch error")
	}
}

func TestCheckSyntheticBadSignature(t *testing.T) {
	body := buildSigned(t, signedOpts{mutate: func(sig *signaturePayload) {
		raw, _ := base64.StdEncoding.DecodeString(sig.signature)
		raw[0] ^= 0xFF
		sig.signature = base64.StdEncoding.EncodeToString(raw)
	}})
	result := Check(body, testHost, TrustStore{KnownDir: t.TempDir()})
	if !result.Signed {
		t.Fatalf("Signed = false, want true")
	}
	if result.Err == nil {
		t.Fatalf("Err = nil, want signature verification error")
	}
}

func TestCheckSyntheticUnsupportedMechanism(t *testing.T) {
	body := buildSigned(t, signedOpts{mutate: func(sig *signaturePayload) {
		sig.mechanism = "tpm2"
	}})
	result := Check(body, testHost, TrustStore{KnownDir: t.TempDir()})
	if !result.Signed {
		t.Fatalf("Signed = false, want true")
	}
	if result.Err == nil {
		t.Fatalf("Err = nil, want unsupported mechanism error")
	}
}

func TestCheckSyntheticMalformedSignatureRecord(t *testing.T) {
	reportJSON := `{"mediaType":"application/vnd.io.systemd.report","metrics":[]}`
	reportRecord := append([]byte{recordSeparator}, append([]byte(reportJSON), '\n')...)

	var out bytes.Buffer
	out.Write(reportRecord)
	out.WriteByte(recordSeparator)
	out.WriteString("not json")
	out.WriteByte('\n')

	result := Check(out.Bytes(), testHost, TrustStore{KnownDir: t.TempDir()})
	if !result.Signed {
		t.Fatalf("Signed = false, want true")
	}
	if result.Err == nil {
		t.Fatalf("Err = nil, want parse error")
	}
}

func TestCheckSyntheticEmbeddedKeyDoesNotMatchAttachedKey(t *testing.T) {
	other, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	spki, err := x509.MarshalPKIXPublicKey(other)
	if err != nil {
		t.Fatalf("marshaling public key: %v", err)
	}
	otherPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: spki}))

	body := buildSigned(t, signedOpts{attachedKeyOverride: otherPEM})
	result := Check(body, testHost, TrustStore{KnownDir: t.TempDir()})
	if !result.Signed {
		t.Fatalf("Signed = false, want true")
	}
	if result.Err == nil {
		t.Fatalf("Err = nil, want embedded/attached key mismatch error")
	}
}

func writeKeyFileForTest(t *testing.T, dir, hostname string, key ed25519.PublicKey) {
	t.Helper()
	if err := writeKeyFile(dir, hostname, key); err != nil {
		t.Fatalf("writeKeyFile: %v", err)
	}
}

func TestCheckTrustedKeyMatches(t *testing.T) {
	body := buildSigned(t, signedOpts{})
	embeddedPEM, err := embeddedPublicKeyPEM(splitRecords(body)[0])
	if err != nil {
		t.Fatalf("embeddedPublicKeyPEM: %v", err)
	}
	key, err := parseEd25519PEM(embeddedPEM)
	if err != nil {
		t.Fatalf("parseEd25519PEM: %v", err)
	}

	trustedDir := t.TempDir()
	writeKeyFileForTest(t, trustedDir, testHost, key)

	result := Check(body, testHost, TrustStore{TrustedDir: trustedDir, KnownDir: t.TempDir()})
	if result.Err != nil {
		t.Fatalf("Err = %v, want nil", result.Err)
	}
	if result.Origin != OriginTrusted {
		t.Errorf("Origin = %q, want %q", result.Origin, OriginTrusted)
	}
}

func TestCheckTrustedKeyMismatch(t *testing.T) {
	body := buildSigned(t, signedOpts{})
	other, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	trustedDir := t.TempDir()
	writeKeyFileForTest(t, trustedDir, testHost, other)

	result := Check(body, testHost, TrustStore{TrustedDir: trustedDir, KnownDir: t.TempDir()})
	if result.Err == nil {
		t.Fatalf("Err = nil, want trusted-key mismatch error")
	}
}

func TestCheckKnownKeyMatches(t *testing.T) {
	body := buildSigned(t, signedOpts{})
	embeddedPEM, err := embeddedPublicKeyPEM(splitRecords(body)[0])
	if err != nil {
		t.Fatalf("embeddedPublicKeyPEM: %v", err)
	}
	key, err := parseEd25519PEM(embeddedPEM)
	if err != nil {
		t.Fatalf("parseEd25519PEM: %v", err)
	}

	knownDir := t.TempDir()
	writeKeyFileForTest(t, knownDir, testHost, key)

	result := Check(body, testHost, TrustStore{KnownDir: knownDir})
	if result.Err != nil {
		t.Fatalf("Err = %v, want nil", result.Err)
	}
	if result.Origin != OriginKnown {
		t.Errorf("Origin = %q, want %q", result.Origin, OriginKnown)
	}
}

func TestCheckKnownKeyMismatch(t *testing.T) {
	body := buildSigned(t, signedOpts{})
	other, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	knownDir := t.TempDir()
	writeKeyFileForTest(t, knownDir, testHost, other)

	result := Check(body, testHost, TrustStore{KnownDir: knownDir})
	if result.Err == nil {
		t.Fatalf("Err = nil, want known-key mismatch error")
	}
}

func TestCheckNoKnownDirConfiguredFailsClosed(t *testing.T) {
	body := buildSigned(t, signedOpts{})
	result := Check(body, testHost, TrustStore{})
	if result.Err == nil {
		t.Fatalf("Err = nil, want failure: no directory configured to learn first-seen keys")
	}
}
