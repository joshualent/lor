// Package tlscert generates Lor's self-signed TLS certificate (shared by the
// admin and agent listeners) and computes the fingerprint the CLI pins.
package tlscert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// File names inside <data_dir>/tls.
const (
	CertFile = "cert.pem"
	KeyFile  = "key.pem"
)

// Fingerprint returns "SHA256:<base64>" of a DER certificate.
func Fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// LoadOrGenerate loads <dir>/cert.pem and key.pem, generating a 10-year
// self-signed ECDSA P-256 certificate with the given SANs (IPs or DNS names)
// if they don't exist. It returns the certificate, its fingerprint and
// whether it was created.
func LoadOrGenerate(dir string, sans []string) (tls.Certificate, string, bool, error) {
	certPath, keyPath := filepath.Join(dir, CertFile), filepath.Join(dir, KeyFile)
	if _, err := os.Stat(certPath); err == nil {
		c, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return tls.Certificate{}, "", false, fmt.Errorf("tls: load %s: %w", certPath, err)
		}
		return c, Fingerprint(c.Certificate[0]), false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return tls.Certificate{}, "", false, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return tls.Certificate{}, "", false, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, "", false, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return tls.Certificate{}, "", false, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "lor"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true, // self-signed: it is its own trust anchor in Caddy's pool
	}
	seen := map[string]bool{}
	for _, s := range append(sans, "localhost", "127.0.0.1") {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		if ip := net.ParseIP(s); ip != nil {
			if !ip.IsUnspecified() {
				tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
			}
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, s)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, "", false, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return tls.Certificate{}, "", false, err
	}
	if err := writeExcl(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return tls.Certificate{}, "", false, err
	}
	if err := writeExcl(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return tls.Certificate{}, "", false, err
	}
	c, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, "", false, err
	}
	return c, Fingerprint(der), true, nil
}

func writeExcl(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tls-tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// MissingSANs reports which of the wanted IPs aren't in the certificate.
func MissingSANs(c tls.Certificate, want []string) []string {
	leaf, err := x509.ParseCertificate(c.Certificate[0])
	if err != nil {
		return want
	}
	var missing []string
	for _, w := range want {
		ip := net.ParseIP(w)
		if ip == nil || ip.IsUnspecified() {
			continue
		}
		found := false
		for _, have := range leaf.IPAddresses {
			if have.Equal(ip) {
				found = true
			}
		}
		if !found {
			missing = append(missing, w)
		}
	}
	return missing
}

// PinnedConfig returns a client TLS config that accepts exactly the server
// certificate with the given fingerprint, and nothing else.
func PinnedConfig(fingerprint string) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, // replaced by the pin check below
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("no server certificate")
			}
			got := Fingerprint(cs.PeerCertificates[0].Raw)
			if got != fingerprint {
				return fmt.Errorf("server certificate fingerprint %s doesn't match the pinned %s", got, fingerprint)
			}
			return nil
		},
	}
}
