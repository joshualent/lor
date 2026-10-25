// Package ca is Lor's SSH certificate authority. It loads or generates the CA
// key and signs short-lived user certificates. SignUserCert refuses anything
// that breaks the rules in plan section 6; every rule has a unit test.
package ca

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// DefaultAbsoluteMax is the default bug guard on certificate lifetime.
const DefaultAbsoluteMax = 30 * 24 * time.Hour

// KeyFile is the CA private key's file name inside the CA directory.
const KeyFile = "lor_ca"

// principalRe matches the only principals Lor ever signs: a host name, or a
// host name with ":sudo".
var principalRe = regexp.MustCompile(`^[a-z0-9-]+(:sudo)?$`)

// HostNameRe matches valid host names (the principal base).
var HostNameRe = regexp.MustCompile(`^[a-z0-9-]{1,63}$`)

// PermitPTY is the only extension Lor certificates carry.
const PermitPTY = "permit-pty"

// UserCertRequest describes one certificate to sign.
type UserCertRequest struct {
	PublicKey   ssh.PublicKey
	Serial      uint64       // certificates.id
	KeyID       string       // "lor:g42:c57:agent=claude:host=nas:level=sudo"
	Principal   string       // "nas" or "nas:sudo"
	ValidAfter  time.Time    // now - 2m (clock skew)
	ValidBefore time.Time    // grant's current end time
	SourceAddrs []netip.Addr // agent's registered LAN IP(s)
}

// ErrRefused wraps every refusal so callers can tell refusals from I/O errors.
var ErrRefused = errors.New("ca: refused to sign")

func refuse(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrRefused, fmt.Sprintf(format, args...))
}

// CA signs user certificates with an ed25519 key.
type CA struct {
	signer      ssh.Signer
	absoluteMax time.Duration
	now         func() time.Time
}

// Option configures a CA.
type Option func(*CA)

// WithAbsoluteMax sets the maximum certificate lifetime measured from now.
func WithAbsoluteMax(d time.Duration) Option { return func(c *CA) { c.absoluteMax = d } }

// WithClock injects a clock (tests).
func WithClock(now func() time.Time) Option { return func(c *CA) { c.now = now } }

// New wraps an existing signer. The signer must be ed25519.
func New(signer ssh.Signer, opts ...Option) (*CA, error) {
	if signer.PublicKey().Type() != ssh.KeyAlgoED25519 {
		return nil, fmt.Errorf("ca: key must be ed25519, got %s", signer.PublicKey().Type())
	}
	c := &CA{signer: signer, absoluteMax: DefaultAbsoluteMax, now: time.Now}
	for _, o := range opts {
		o(c)
	}
	if c.absoluteMax <= 0 {
		return nil, errors.New("ca: absolute max duration must be positive")
	}
	return c, nil
}

// LoadOrGenerate loads <dir>/lor_ca, generating a new ed25519 key (mode 0600)
// if it doesn't exist. created reports whether a new key was generated.
func LoadOrGenerate(dir string, opts ...Option) (c *CA, created bool, err error) {
	path := filepath.Join(dir, KeyFile)
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := checkKeyFileMode(path); err != nil {
			return nil, false, err
		}
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			return nil, false, fmt.Errorf("ca: parse %s: %w", path, err)
		}
		c, err := New(signer, opts...)
		return c, false, err
	case errors.Is(err, os.ErrNotExist):
	default:
		return nil, false, fmt.Errorf("ca: read %s: %w", path, err)
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, false, fmt.Errorf("ca: create dir: %w", err)
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, false, err
	}
	block, err := ssh.MarshalPrivateKey(priv, "lor-ca")
	if err != nil {
		return nil, false, err
	}
	// O_EXCL so two processes can't both generate a key.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("ca: create key: %w", err)
	}
	if _, err := f.Write(pem.EncodeToMemory(block)); err != nil {
		f.Close()
		return nil, false, err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return nil, false, err
	}
	if err := f.Close(); err != nil {
		return nil, false, err
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, false, err
	}
	c, err = New(signer, opts...)
	if err != nil {
		return nil, false, err
	}
	if err := os.WriteFile(path+".pub", []byte(c.AuthorizedKey()+"\n"), 0o644); err != nil {
		return nil, false, err
	}
	return c, true, nil
}

func checkKeyFileMode(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("ca: %s must not be accessible by group or others (mode %o)", path, fi.Mode().Perm())
	}
	return nil
}

// PublicKey returns the CA public key.
func (c *CA) PublicKey() ssh.PublicKey { return c.signer.PublicKey() }

// AuthorizedKey returns the CA public key in authorized_keys format, as written
// to /etc/ssh/lor/ca.pub on hosts.
func (c *CA) AuthorizedKey() string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(c.signer.PublicKey()))) + " lor-ca"
}

// Fingerprint returns the SHA256 fingerprint of the CA public key.
func (c *CA) Fingerprint() string { return ssh.FingerprintSHA256(c.signer.PublicKey()) }

// SignUserCert validates req against the rules and returns a signed user
// certificate. It never signs anything that fails validation.
func (c *CA) SignUserCert(req UserCertRequest) (*ssh.Certificate, error) {
	now := c.now()
	if req.PublicKey == nil {
		return nil, refuse("missing public key")
	}
	if _, isCert := req.PublicKey.(*ssh.Certificate); isCert {
		return nil, refuse("public key must be a plain key, not a certificate")
	}
	if req.PublicKey.Type() != ssh.KeyAlgoED25519 {
		return nil, refuse("public key must be ed25519, got %s", req.PublicKey.Type())
	}
	if string(req.PublicKey.Marshal()) == string(c.signer.PublicKey().Marshal()) {
		return nil, refuse("public key is the CA key")
	}
	if req.Serial == 0 {
		return nil, refuse("serial must be non-zero")
	}
	if req.KeyID == "" {
		return nil, refuse("key ID must not be empty")
	}
	if !principalRe.MatchString(req.Principal) {
		return nil, refuse("invalid principal %q", req.Principal)
	}
	if req.ValidBefore.IsZero() {
		return nil, refuse("valid-before must be set")
	}
	if !req.ValidBefore.After(now) {
		return nil, refuse("valid-before must be in the future")
	}
	if req.ValidBefore.Sub(now) > c.absoluteMax {
		return nil, refuse("valid-before is more than %s from now", c.absoluteMax)
	}
	if req.ValidAfter.IsZero() {
		return nil, refuse("valid-after must be set")
	}
	if !req.ValidAfter.Before(req.ValidBefore) {
		return nil, refuse("valid-after must be before valid-before")
	}
	sourceAddress, err := formatSourceAddrs(req.SourceAddrs)
	if err != nil {
		return nil, err
	}

	cert := &ssh.Certificate{
		Key:             req.PublicKey,
		Serial:          req.Serial,
		CertType:        ssh.UserCert,
		KeyId:           req.KeyID,
		ValidPrincipals: []string{req.Principal},
		ValidAfter:      uint64(req.ValidAfter.Unix()),
		ValidBefore:     uint64(req.ValidBefore.Unix()),
		Permissions: ssh.Permissions{
			CriticalOptions: map[string]string{"source-address": sourceAddress},
			Extensions:      map[string]string{PermitPTY: ""},
		},
	}
	if err := CheckCert(cert, now, c.absoluteMax); err != nil {
		return nil, err
	}
	if err := cert.SignCert(rand.Reader, c.signer); err != nil {
		return nil, fmt.Errorf("ca: sign: %w", err)
	}
	return cert, nil
}

func formatSourceAddrs(addrs []netip.Addr) (string, error) {
	if len(addrs) == 0 {
		return "", refuse("source-address must not be empty")
	}
	parts := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if !a.IsValid() || a.IsUnspecified() {
			return "", refuse("invalid source address %q", a)
		}
		a = a.Unmap()
		parts = append(parts, netip.PrefixFrom(a, a.BitLen()).String())
	}
	return strings.Join(parts, ","), nil
}

// CheckCert is the final structural check applied to every certificate before
// it is signed. It is exported so the rules can be tested on crafted
// certificates that SignUserCert itself would never build.
func CheckCert(cert *ssh.Certificate, now time.Time, absoluteMax time.Duration) error {
	if cert.CertType != ssh.UserCert {
		return refuse("only user certificates are signed")
	}
	if cert.Key == nil || cert.Key.Type() != ssh.KeyAlgoED25519 {
		return refuse("certified key must be ed25519")
	}
	if len(cert.ValidPrincipals) != 1 {
		return refuse("exactly one principal required, got %d", len(cert.ValidPrincipals))
	}
	if !principalRe.MatchString(cert.ValidPrincipals[0]) {
		return refuse("invalid principal %q", cert.ValidPrincipals[0])
	}
	if cert.ValidBefore == ssh.CertTimeInfinity || cert.ValidBefore == 0 {
		return refuse("valid-before must be finite")
	}
	vb := time.Unix(int64(cert.ValidBefore), 0)
	if !vb.After(now) {
		return refuse("valid-before must be in the future")
	}
	if vb.Sub(now) > absoluteMax {
		return refuse("valid-before is more than %s from now", absoluteMax)
	}
	if cert.ValidAfter >= cert.ValidBefore {
		return refuse("valid-after must be before valid-before")
	}
	if len(cert.CriticalOptions) != 1 || cert.CriticalOptions["source-address"] == "" {
		return refuse("exactly one critical option, a non-empty source-address, is required")
	}
	for _, p := range strings.Split(cert.CriticalOptions["source-address"], ",") {
		pfx, err := netip.ParsePrefix(p)
		if err != nil || pfx.Bits() != pfx.Addr().BitLen() || pfx.Addr().IsUnspecified() {
			return refuse("source-address must list single host addresses, got %q", p)
		}
	}
	if len(cert.Extensions) != 1 {
		return refuse("only the permit-pty extension is allowed")
	}
	if _, ok := cert.Extensions[PermitPTY]; !ok {
		return refuse("only the permit-pty extension is allowed")
	}
	return nil
}

// Principal returns the principal for a host and level ("user" or "sudo").
func Principal(host, level string) string {
	if level == "sudo" {
		return host + ":sudo"
	}
	return host
}

// KeyID builds the key ID recorded in each host's sshd auth log.
func KeyID(grantID, certID int64, agent, host, level string) string {
	return fmt.Sprintf("lor:g%d:c%d:agent=%s:host=%s:level=%s", grantID, certID, agent, host, level)
}

// ParsePublicKey parses an authorized_keys-format public key and requires a
// plain ed25519 key.
func ParsePublicKey(s string) (ssh.PublicKey, error) {
	pk, _, _, _, err := ssh.ParseAuthorizedKey([]byte(s))
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}
	if _, isCert := pk.(*ssh.Certificate); isCert {
		return nil, errors.New("public key must not be a certificate")
	}
	if pk.Type() != ssh.KeyAlgoED25519 {
		return nil, fmt.Errorf("public key must be ed25519, got %s", pk.Type())
	}
	return pk, nil
}
