package verify

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TrustStore locates the Ed25519 public key a given host's signed reports
// should be verified against, implementing the three trust tiers documented
// on Origin. Each host has at most one key file per directory, named
// "<hostname>.pem" and holding a single PEM-encoded PKIX public key.
//
// TrustedDir is meant to be populated by the administrator, out of band from
// the report traffic itself (e.g. by copying the key off the client machine
// over a trusted channel): a key found there authenticates the sender. Leave
// it empty to disable that tier.
//
// KnownDir is sd-report-collector's own cache of keys learned from earlier
// reports (trust-on-first-use); it is created and written to automatically.
// Leave it empty to disable first-use learning: hosts without a TrustedDir
// entry then fail verification instead.
type TrustStore struct {
	TrustedDir string
	KnownDir   string
}

// resolve returns the key candidate (the report's own embedded/attached
// key, already confirmed internally consistent by the caller) should be
// verified against for hostname, and which trust tier that key came from.
// If TrustedDir or KnownDir already have a key on file for hostname,
// candidate must match it exactly or resolve fails: a mismatch means either
// the host's key was rotated without updating the trust store, or someone
// else is signing reports claiming to be this host. If neither directory
// has a key yet, candidate is accepted trust-on-first-use and stored under
// KnownDir.
func (t TrustStore) resolve(hostname string, candidate ed25519.PublicKey) (ed25519.PublicKey, Origin, error) {
	hostname, err := sanitizeHostname(hostname)
	if err != nil {
		return nil, "", err
	}

	if key, ok, err := readKeyFile(t.TrustedDir, hostname); err != nil {
		return nil, "", fmt.Errorf("reading trusted key for host %q: %w", hostname, err)
	} else if ok {
		if !key.Equal(candidate) {
			return nil, "", fmt.Errorf("report key for host %q does not match the administrator-provisioned key in %s", hostname, t.TrustedDir)
		}
		return key, OriginTrusted, nil
	}

	if key, ok, err := readKeyFile(t.KnownDir, hostname); err != nil {
		return nil, "", fmt.Errorf("reading known key for host %q: %w", hostname, err)
	} else if ok {
		if !key.Equal(candidate) {
			return nil, "", fmt.Errorf("report key for host %q does not match the previously learned key in %s (key rotation or a spoofed sender)", hostname, t.KnownDir)
		}
		return key, OriginKnown, nil
	}

	if err := writeKeyFile(t.KnownDir, hostname, candidate); err != nil {
		return nil, "", fmt.Errorf("storing first-seen key for host %q: %w", hostname, err)
	}
	return candidate, OriginFirstSeen, nil
}

// sanitizeHostname rejects hostnames that are unsafe to use as a path
// component, mirroring internal/store's rules.
func sanitizeHostname(hostname string) (string, error) {
	if hostname == "" || hostname == "." || hostname == ".." {
		return "", errors.New("invalid hostname")
	}
	if strings.ContainsAny(hostname, "/\x00") {
		return "", errors.New("invalid hostname")
	}
	return hostname, nil
}

// readKeyFile reads and parses "<dir>/<hostname>.pem". A missing dir or file
// is reported as (nil, false, nil), not an error: the caller tries the next
// tier.
func readKeyFile(dir, hostname string) (ed25519.PublicKey, bool, error) {
	if dir == "" {
		return nil, false, nil
	}
	path := filepath.Join(dir, hostname+".pem")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	key, err := parseEd25519PEM(string(data))
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", path, err)
	}
	return key, true, nil
}

// writeKeyFile PEM-encodes key and writes it to "<dir>/<hostname>.pem",
// creating dir if needed. The write is atomic (temp file + rename) so a
// concurrent reader never observes a partial file.
func writeKeyFile(dir, hostname string, key ed25519.PublicKey) error {
	if dir == "" {
		return errors.New("no directory configured to learn first-seen keys in")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	spki, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return fmt.Errorf("marshaling public key: %w", err)
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: spki})

	path := filepath.Join(dir, hostname+".pem")
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once successfully renamed

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming %s to %s: %w", tmpPath, path, err)
	}
	return nil
}
