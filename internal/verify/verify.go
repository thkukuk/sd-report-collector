// Package verify checks the cryptographic signature systemd-report attaches
// to a report when run with --sign=. A signed report is emitted as a
// JSON-SEQ stream (RFC 7464): the report object first, followed by one or
// more signature objects, each covering the exact bytes of the report
// record (its leading 0x1E separator and trailing newline included).
//
// The "plain" mechanism's key is a bare, self-generated host key with no CA
// or enrollment behind it: on its own, a valid signature only proves the
// report bytes were produced by whoever holds that key, not that the key
// belongs to the host it claims to be from (anyone can mint a new keypair
// and sign a forged report with it). To turn that into a real identity
// check, this package pins each host to a key via [TrustStore] using a
// three-tier model: an administrator-provisioned key under TrustedDir always
// wins; failing that, a key already learned from a previous report under
// KnownDir; failing that, the key is trusted on first use and persisted to
// KnownDir for next time. See trust.go and the package README for details.
package verify

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
)

const recordSeparator = 0x1E

// mechanismPlain is the only signature mechanism this package can verify;
// see systemd-report-sign-plain(8).
const mechanismPlain = "plain"

// embeddedKeyMetric is the name of the metric systemd-report embeds in the
// report body itself, carrying the same public key used by mechanismPlain's
// signature record. Checking the two against each other is what lets Check
// detect a report whose signature trailer was swapped for a different
// keypair than the one the report claims to be signed with.
const embeddedKeyMetric = "io.systemd.Report.SignPlain.PublicKey"

// Origin describes where the key a signed report was verified against came
// from, in decreasing order of trust.
type Origin string

const (
	// OriginUnsigned means the report carried no signature at all.
	OriginUnsigned Origin = "unsigned"
	// OriginTrusted means the report was verified against a key the
	// administrator placed under TrustStore.TrustedDir out of band. This is
	// the only tier that authenticates the sender; the others merely prove
	// self-consistency (see the package doc comment).
	OriginTrusted Origin = "trusted"
	// OriginKnown means the report was verified against a key learned from
	// a previous report from this host and stored under
	// TrustStore.KnownDir.
	OriginKnown Origin = "known"
	// OriginFirstSeen means no key was known for this host yet; the report
	// was accepted trust-on-first-use and its key was just stored under
	// TrustStore.KnownDir.
	OriginFirstSeen Origin = "first-seen"
)

// Result describes the outcome of checking a report body for an embedded
// systemd-report signature trailer.
type Result struct {
	// Signed is true if body carried one or more signature records.
	Signed bool
	// Origin says where the verification key came from. It is always
	// OriginUnsigned when Signed is false, and is otherwise only
	// meaningful when Err is nil.
	Origin Origin
	// Err is non-nil when Signed is true and verification failed: a
	// malformed signature record, a digest mismatch, an invalid Ed25519
	// signature, a mechanism other than "plain" (which this package cannot
	// check and therefore treats as unverified), a mismatch between the
	// report's embedded key and its attached signature key, or a mismatch
	// against a trusted/known key on file for the host.
	Err error
}

type signatureRecord struct {
	Mechanism string `json:"mechanism"`
	SHA256    string `json:"sha256"`
	Data      struct {
		Signature string `json:"signature"`
		Key       string `json:"key"`
	} `json:"data"`
}

// Check inspects body for a JSON-SEQ signature trailer and verifies every
// signature record found against hostname's pinned key in store. A plain
// JSON report (no 0x1E framing) is considered unsigned. A signed report is
// only treated as verified if every attached signature record checks out;
// any single failure fails the whole report, since there is no way to tell
// a genuinely bad signature apart from one using a mechanism not supported
// here.
func Check(body []byte, hostname string, store TrustStore) Result {
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	if len(trimmed) == 0 || trimmed[0] != recordSeparator {
		return Result{Origin: OriginUnsigned}
	}

	records := splitRecords(trimmed)
	if len(records) < 2 {
		return Result{Origin: OriginUnsigned}
	}

	reportRecord := records[0]
	var origin Origin
	for _, sigRecord := range records[1:] {
		o, err := verifyOne(reportRecord, sigRecord, hostname, store)
		if err != nil {
			return Result{Signed: true, Err: err}
		}
		origin = o
	}
	return Result{Signed: true, Origin: origin}
}

// splitRecords splits a JSON-SEQ stream (data must start with the record
// separator) into its individual records. Each returned record keeps its
// leading separator and trailing newline intact, matching the exact bytes
// systemd-report hashed when signing.
func splitRecords(data []byte) [][]byte {
	var records [][]byte
	for len(data) > 0 {
		rest := data[1:] // drop the leading separator
		end := bytes.IndexByte(rest, recordSeparator)
		var content []byte
		if end < 0 {
			content, data = rest, nil
		} else {
			content, data = rest[:end], rest[end:]
		}
		record := make([]byte, 0, len(content)+1)
		record = append(record, recordSeparator)
		record = append(record, content...)
		records = append(records, record)
	}
	return records
}

func verifyOne(reportRecord, sigRecord []byte, hostname string, store TrustStore) (Origin, error) {
	var sig signatureRecord
	if err := json.Unmarshal(sigRecord[1:], &sig); err != nil {
		return "", fmt.Errorf("parsing signature record: %w", err)
	}

	wantDigest, err := hex.DecodeString(sig.SHA256)
	if err != nil || len(wantDigest) != sha256.Size {
		return "", errors.New("signature record has an invalid sha256 field")
	}
	gotDigest := sha256.Sum256(reportRecord)
	if !bytes.Equal(gotDigest[:], wantDigest) {
		return "", errors.New("digest mismatch: report bytes do not match the signed digest")
	}

	if sig.Mechanism != mechanismPlain {
		return "", fmt.Errorf("unsupported signature mechanism %q", sig.Mechanism)
	}

	embeddedPEM, err := embeddedPublicKeyPEM(reportRecord)
	if err != nil {
		return "", err
	}
	embeddedKey, err := parseEd25519PEM(embeddedPEM)
	if err != nil {
		return "", fmt.Errorf("report's embedded public key: %w", err)
	}
	attachedKey, err := parseEd25519PEM(sig.Data.Key)
	if err != nil {
		return "", fmt.Errorf("signature's attached public key: %w", err)
	}
	if !embeddedKey.Equal(attachedKey) {
		return "", errors.New("report's embedded public key and the signature's attached public key do not match")
	}

	trustedKey, origin, err := store.resolve(hostname, embeddedKey)
	if err != nil {
		return "", err
	}

	// The "plain" backend signs the digest bytes directly (Ed25519,
	// unhashed input), not a second hash of them; see
	// systemd-report-sign-plain(8) and digest_and_sign() with md=NULL.
	sigBytes, err := base64.StdEncoding.DecodeString(sig.Data.Signature)
	if err != nil {
		return "", fmt.Errorf("signature is not valid base64: %w", err)
	}
	if !ed25519.Verify(trustedKey, wantDigest, sigBytes) {
		return "", errors.New("Ed25519 signature verification failed")
	}
	return origin, nil
}

// embeddedPublicKeyPEM extracts the PEM-encoded public key systemd-report
// embeds in the report body as the embeddedKeyMetric metric. reportRecord
// keeps the JSON-SEQ framing splitRecords produces (leading separator,
// trailing newline).
func embeddedPublicKeyPEM(reportRecord []byte) (string, error) {
	body := bytes.TrimRight(reportRecord[1:], "\n")

	var report struct {
		Metrics []struct {
			Name  string          `json:"name"`
			Value json.RawMessage `json:"value"`
		} `json:"metrics"`
	}
	if err := json.Unmarshal(body, &report); err != nil {
		return "", fmt.Errorf("parsing report record: %w", err)
	}

	for _, m := range report.Metrics {
		if m.Name != embeddedKeyMetric {
			continue
		}
		var key string
		if err := json.Unmarshal(m.Value, &key); err != nil {
			return "", fmt.Errorf("parsing %s metric: %w", embeddedKeyMetric, err)
		}
		return key, nil
	}
	return "", fmt.Errorf("report does not embed a %s metric", embeddedKeyMetric)
}

// parseEd25519PEM decodes an Ed25519 public key from a PEM-encoded
// PKIX/SubjectPublicKeyInfo block, the format systemd-report-sign-plain(8)
// uses for both the embedded report metric and the signature's "key" field.
func parseEd25519PEM(s string) (ed25519.PublicKey, error) {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return nil, errors.New("not valid PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing public key: %w", err)
	}
	edKey, ok := pub.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("not an Ed25519 key")
	}
	return edKey, nil
}
