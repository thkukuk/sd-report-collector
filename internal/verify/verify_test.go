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
	"testing"
)

func TestCheckUnsigned(t *testing.T) {
	body := []byte(`{"mediaType":"application/vnd.io.systemd.report","metrics":[]}`)
	result := Check(body)
	if result.Signed {
		t.Fatalf("Signed = true, want false")
	}
	if result.Err != nil {
		t.Fatalf("Err = %v, want nil", result.Err)
	}
}

func TestCheckEmptyAndWhitespace(t *testing.T) {
	for _, body := range [][]byte{nil, []byte(""), []byte("   \n")} {
		if result := Check(body); result.Signed {
			t.Errorf("Check(%q).Signed = true, want false", body)
		}
	}
}

func TestCheckReportOnlyRecordIsUnsigned(t *testing.T) {
	// A JSON-SEQ stream with just the report record and no signature
	// trailer is not meaningfully "signed".
	body := []byte("\x1e{\"mediaType\":\"application/vnd.io.systemd.report\"}\n")
	result := Check(body)
	if result.Signed {
		t.Fatalf("Signed = true, want false")
	}
}

func TestCheckRealFixture(t *testing.T) {
	body, err := os.ReadFile("../../test-data/signed-report.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	result := Check(body)
	if !result.Signed {
		t.Fatalf("Signed = false, want true")
	}
	if result.Err != nil {
		t.Fatalf("Err = %v, want nil", result.Err)
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

	result := Check(tampered)
	if !result.Signed {
		t.Fatalf("Signed = false, want true")
	}
	if result.Err == nil {
		t.Fatalf("Err = nil, want a verification failure")
	}
}

// buildSigned constructs a minimal signed JSON-SEQ stream using a freshly
// generated Ed25519 key, mirroring exactly what
// systemd-report-sign-plain(8) emits.
func buildSigned(t *testing.T, reportJSON string, mutate func(sig *signaturePayload)) []byte {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	reportRecord := append([]byte{recordSeparator}, append([]byte(reportJSON), '\n')...)
	digest := sha256.Sum256(reportRecord)

	sigBytes := ed25519.Sign(priv, digest[:])

	spki, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshaling public key: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: spki})

	payload := signaturePayload{
		mechanism: mechanismPlain,
		sha256:    hex.EncodeToString(digest[:]),
		signature: base64.StdEncoding.EncodeToString(sigBytes),
		key:       string(pubPEM),
	}
	if mutate != nil {
		mutate(&payload)
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

type signaturePayload struct {
	mechanism string
	sha256    string
	signature string
	key       string
}

func TestCheckSyntheticValid(t *testing.T) {
	body := buildSigned(t, `{"mediaType":"application/vnd.io.systemd.report","metrics":[]}`, nil)
	result := Check(body)
	if !result.Signed || result.Err != nil {
		t.Fatalf("Signed = %v, Err = %v, want Signed=true, Err=nil", result.Signed, result.Err)
	}
}

func TestCheckSyntheticTamperedDigest(t *testing.T) {
	body := buildSigned(t, `{"mediaType":"application/vnd.io.systemd.report","metrics":[]}`, func(sig *signaturePayload) {
		sig.sha256 = "00" + sig.sha256[2:]
	})
	result := Check(body)
	if !result.Signed {
		t.Fatalf("Signed = false, want true")
	}
	if result.Err == nil {
		t.Fatalf("Err = nil, want digest mismatch error")
	}
}

func TestCheckSyntheticBadSignature(t *testing.T) {
	body := buildSigned(t, `{"mediaType":"application/vnd.io.systemd.report","metrics":[]}`, func(sig *signaturePayload) {
		raw, _ := base64.StdEncoding.DecodeString(sig.signature)
		raw[0] ^= 0xFF
		sig.signature = base64.StdEncoding.EncodeToString(raw)
	})
	result := Check(body)
	if !result.Signed {
		t.Fatalf("Signed = false, want true")
	}
	if result.Err == nil {
		t.Fatalf("Err = nil, want signature verification error")
	}
}

func TestCheckSyntheticUnsupportedMechanism(t *testing.T) {
	body := buildSigned(t, `{"mediaType":"application/vnd.io.systemd.report","metrics":[]}`, func(sig *signaturePayload) {
		sig.mechanism = "tpm2"
	})
	result := Check(body)
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

	result := Check(out.Bytes())
	if !result.Signed {
		t.Fatalf("Signed = false, want true")
	}
	if result.Err == nil {
		t.Fatalf("Err = nil, want parse error")
	}
}
