// Package verify checks the cryptographic signature systemd-report attaches
// to a report when run with --sign=. A signed report is emitted as a
// JSON-SEQ stream (RFC 7464): the report object first, followed by one or
// more signature objects, each covering the exact bytes of the report
// record (its leading 0x1E separator and trailing newline included).
//
// Verifying a signature only proves the report bytes have not been altered
// since signing; the "plain" mechanism's key is a bare, self-generated host
// key with no CA or enrollment behind it, so this is an integrity check,
// not a statement of trust in the sender's identity.
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

// Result describes the outcome of checking a report body for an embedded
// systemd-report signature trailer.
type Result struct {
	// Signed is true if body carried one or more signature records.
	Signed bool
	// Err is non-nil when Signed is true and verification failed: a
	// malformed signature record, a digest mismatch, an invalid Ed25519
	// signature, or a mechanism other than "plain", which this package
	// cannot check and therefore treats as unverified.
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
// signature record found. A plain JSON report (no 0x1E framing) is
// considered unsigned. A signed report is only treated as verified if every
// attached signature record checks out; any single failure fails the whole
// report, since there is no way to tell a genuinely bad signature apart
// from one using a mechanism not supported here.
func Check(body []byte) Result {
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	if len(trimmed) == 0 || trimmed[0] != recordSeparator {
		return Result{}
	}

	records := splitRecords(trimmed)
	if len(records) < 2 {
		return Result{}
	}

	reportRecord := records[0]
	for _, sigRecord := range records[1:] {
		if err := verifyOne(reportRecord, sigRecord); err != nil {
			return Result{Signed: true, Err: err}
		}
	}
	return Result{Signed: true}
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

func verifyOne(reportRecord, sigRecord []byte) error {
	var sig signatureRecord
	if err := json.Unmarshal(sigRecord[1:], &sig); err != nil {
		return fmt.Errorf("parsing signature record: %w", err)
	}

	wantDigest, err := hex.DecodeString(sig.SHA256)
	if err != nil || len(wantDigest) != sha256.Size {
		return errors.New("signature record has an invalid sha256 field")
	}
	gotDigest := sha256.Sum256(reportRecord)
	if !bytes.Equal(gotDigest[:], wantDigest) {
		return errors.New("digest mismatch: report bytes do not match the signed digest")
	}

	if sig.Mechanism != mechanismPlain {
		return fmt.Errorf("unsupported signature mechanism %q", sig.Mechanism)
	}

	block, _ := pem.Decode([]byte(sig.Data.Key))
	if block == nil {
		return errors.New("signature key is not valid PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parsing signature public key: %w", err)
	}
	edKey, ok := pub.(ed25519.PublicKey)
	if !ok {
		return errors.New("signature key is not an Ed25519 key")
	}

	sigBytes, err := base64.StdEncoding.DecodeString(sig.Data.Signature)
	if err != nil {
		return fmt.Errorf("signature is not valid base64: %w", err)
	}

	// The "plain" backend signs the digest bytes directly (Ed25519,
	// unhashed input), not a second hash of them; see
	// systemd-report-sign-plain(8) and digest_and_sign() with md=NULL.
	if !ed25519.Verify(edKey, wantDigest, sigBytes) {
		return errors.New("Ed25519 signature verification failed")
	}
	return nil
}
