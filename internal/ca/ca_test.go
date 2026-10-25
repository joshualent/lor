package ca

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

var testNow = time.Date(2026, 10, 25, 14, 0, 0, 0, time.UTC)

func newTestCA(t *testing.T) *CA {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(s, WithClock(func() time.Time { return testNow }))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func newKey(t *testing.T) (ssh.PublicKey, ssh.Signer) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pk, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return pk, s
}

func validRequest(t *testing.T) UserCertRequest {
	pk, _ := newKey(t)
	return UserCertRequest{
		PublicKey:   pk,
		Serial:      57,
		KeyID:       KeyID(42, 57, "claude", "nas", "sudo"),
		Principal:   "nas:sudo",
		ValidAfter:  testNow.Add(-2 * time.Minute),
		ValidBefore: testNow.Add(15 * time.Minute),
		SourceAddrs: []netip.Addr{netip.MustParseAddr("192.168.1.50")},
	}
}

func TestSignValid(t *testing.T) {
	c := newTestCA(t)
	req := validRequest(t)
	cert, err := c.SignUserCert(req)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if cert.CertType != ssh.UserCert {
		t.Errorf("cert type = %d", cert.CertType)
	}
	if got := cert.ValidPrincipals; len(got) != 1 || got[0] != "nas:sudo" {
		t.Errorf("principals = %v", got)
	}
	if got := cert.CriticalOptions["source-address"]; got != "192.168.1.50/32" {
		t.Errorf("source-address = %q", got)
	}
	if len(cert.Extensions) != 1 {
		t.Errorf("extensions = %v", cert.Extensions)
	}
	if cert.KeyId != "lor:g42:c57:agent=claude:host=nas:level=sudo" || cert.Serial != 57 {
		t.Errorf("keyid/serial = %q/%d", cert.KeyId, cert.Serial)
	}
}

// Every refusal rule from plan section 6 gets its own case.
func TestSignRefusals(t *testing.T) {
	c := newTestCA(t)
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	rsaPub, _ := ssh.NewPublicKey(&rsaKey.PublicKey)
	ecKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ecPub, _ := ssh.NewPublicKey(&ecKey.PublicKey)

	cases := []struct {
		name   string
		modify func(r *UserCertRequest)
	}{
		{"missing public key", func(r *UserCertRequest) { r.PublicKey = nil }},
		{"rsa public key", func(r *UserCertRequest) { r.PublicKey = rsaPub }},
		{"ecdsa public key", func(r *UserCertRequest) { r.PublicKey = ecPub }},
		{"certificate as public key", func(r *UserCertRequest) {
			inner, _ := c.SignUserCert(validRequest(t))
			r.PublicKey = inner
		}},
		{"CA key as public key", func(r *UserCertRequest) { r.PublicKey = c.PublicKey() }},
		{"zero serial", func(r *UserCertRequest) { r.Serial = 0 }},
		{"empty key ID", func(r *UserCertRequest) { r.KeyID = "" }},
		{"empty principal", func(r *UserCertRequest) { r.Principal = "" }},
		{"principal root", func(r *UserCertRequest) { r.Principal = "root" + ":admin" }},
		{"principal uppercase", func(r *UserCertRequest) { r.Principal = "NAS" }},
		{"principal with comma (two principals)", func(r *UserCertRequest) { r.Principal = "nas,pihole" }},
		{"principal with space", func(r *UserCertRequest) { r.Principal = "nas pihole" }},
		{"principal bad suffix", func(r *UserCertRequest) { r.Principal = "nas:root" }},
		{"principal double sudo", func(r *UserCertRequest) { r.Principal = "nas:sudo:sudo" }},
		{"principal wildcard", func(r *UserCertRequest) { r.Principal = "*" }},
		{"zero valid-before", func(r *UserCertRequest) { r.ValidBefore = time.Time{} }},
		{"valid-before in the past", func(r *UserCertRequest) { r.ValidBefore = testNow.Add(-time.Second) }},
		{"valid-before now", func(r *UserCertRequest) { r.ValidBefore = testNow }},
		{"valid-before beyond absolute max", func(r *UserCertRequest) {
			r.ValidBefore = testNow.Add(DefaultAbsoluteMax + time.Second)
		}},
		{"valid-before effectively infinite", func(r *UserCertRequest) {
			r.ValidBefore = time.Unix(1<<62, 0)
		}},
		{"zero valid-after", func(r *UserCertRequest) { r.ValidAfter = time.Time{} }},
		{"valid-after after valid-before", func(r *UserCertRequest) { r.ValidAfter = r.ValidBefore.Add(time.Second) }},
		{"no source address", func(r *UserCertRequest) { r.SourceAddrs = nil }},
		{"invalid source address", func(r *UserCertRequest) { r.SourceAddrs = []netip.Addr{{}} }},
		{"unspecified source address", func(r *UserCertRequest) {
			r.SourceAddrs = []netip.Addr{netip.MustParseAddr("0.0.0.0")}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validRequest(t)
			tc.modify(&req)
			cert, err := c.SignUserCert(req)
			if err == nil {
				t.Fatalf("expected refusal, got cert %v", cert.ValidPrincipals)
			}
			if !errors.Is(err, ErrRefused) {
				t.Fatalf("error %v is not ErrRefused", err)
			}
		})
	}
}

func TestAbsoluteMaxConfigurable(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	s, _ := ssh.NewSignerFromKey(priv)
	c, err := New(s, WithClock(func() time.Time { return testNow }), WithAbsoluteMax(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	req := validRequest(t)
	req.ValidBefore = testNow.Add(time.Hour + time.Second)
	if _, err := c.SignUserCert(req); !errors.Is(err, ErrRefused) {
		t.Fatalf("expected refusal beyond 1h, got %v", err)
	}
	req.ValidBefore = testNow.Add(time.Hour)
	if _, err := c.SignUserCert(req); err != nil {
		t.Fatalf("1h should be allowed: %v", err)
	}
}

// CheckCert rules on crafted certificates SignUserCert itself never builds.
func TestCheckCertRefusals(t *testing.T) {
	pk, _ := newKey(t)
	base := func() *ssh.Certificate {
		return &ssh.Certificate{
			Key:             pk,
			Serial:          1,
			CertType:        ssh.UserCert,
			KeyId:           "x",
			ValidPrincipals: []string{"nas"},
			ValidAfter:      uint64(testNow.Add(-time.Minute).Unix()),
			ValidBefore:     uint64(testNow.Add(time.Hour).Unix()),
			Permissions: ssh.Permissions{
				CriticalOptions: map[string]string{"source-address": "192.168.1.50/32"},
				Extensions:      map[string]string{PermitPTY: ""},
			},
		}
	}
	if err := CheckCert(base(), testNow, DefaultAbsoluteMax); err != nil {
		t.Fatalf("base cert should pass: %v", err)
	}
	cases := []struct {
		name   string
		modify func(c *ssh.Certificate)
	}{
		{"host certificate", func(c *ssh.Certificate) { c.CertType = ssh.HostCert }},
		{"no principals", func(c *ssh.Certificate) { c.ValidPrincipals = nil }},
		{"two principals", func(c *ssh.Certificate) { c.ValidPrincipals = []string{"nas", "nas:sudo"} }},
		{"infinite validity", func(c *ssh.Certificate) { c.ValidBefore = ssh.CertTimeInfinity }},
		{"zero valid-before", func(c *ssh.Certificate) { c.ValidBefore = 0 }},
		{"missing source-address", func(c *ssh.Certificate) { c.CriticalOptions = nil }},
		{"empty source-address", func(c *ssh.Certificate) { c.CriticalOptions["source-address"] = "" }},
		{"wide source-address", func(c *ssh.Certificate) { c.CriticalOptions["source-address"] = "192.168.1.0/24" }},
		{"any source-address", func(c *ssh.Certificate) { c.CriticalOptions["source-address"] = "0.0.0.0/0" }},
		{"force-command added", func(c *ssh.Certificate) { c.CriticalOptions["force-command"] = "/bin/true" }},
		{"port forwarding", func(c *ssh.Certificate) { c.Extensions["permit-port-forwarding"] = "" }},
		{"agent forwarding", func(c *ssh.Certificate) { c.Extensions["permit-agent-forwarding"] = "" }},
		{"x11 forwarding", func(c *ssh.Certificate) { c.Extensions["permit-X11-forwarding"] = "" }},
		{"user rc", func(c *ssh.Certificate) { c.Extensions["permit-user-rc"] = "" }},
		{"no extensions", func(c *ssh.Certificate) { c.Extensions = nil }},
		{"other single extension", func(c *ssh.Certificate) {
			c.Extensions = map[string]string{"permit-user-rc": ""}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cert := base()
			tc.modify(cert)
			if err := CheckCert(cert, testNow, DefaultAbsoluteMax); !errors.Is(err, ErrRefused) {
				t.Fatalf("expected refusal, got %v", err)
			}
		})
	}
}

func TestSignThenVerifyWithCertChecker(t *testing.T) {
	c := newTestCA(t)
	req := validRequest(t)
	req.Principal = "nas"
	cert, err := c.SignUserCert(req)
	if err != nil {
		t.Fatal(err)
	}
	checker := &ssh.CertChecker{
		IsUserAuthority: func(auth ssh.PublicKey) bool {
			return bytes.Equal(auth.Marshal(), c.PublicKey().Marshal())
		},
		Clock:                    func() time.Time { return testNow },
		SupportedCriticalOptions: []string{"source-address"},
	}
	if err := checker.CheckCert("nas", cert); err != nil {
		t.Fatalf("CheckCert nas: %v", err)
	}
	if err := checker.CheckCert("pihole", cert); err == nil {
		t.Fatal("cert for nas must not be valid for pihole")
	}
	if err := checker.CheckCert("nas:sudo", cert); err == nil {
		t.Fatal("user cert must not be valid for nas:sudo")
	}

	// source-address is enforced by the SSH server, see TestSourceAddressEnforced.

	expired := &ssh.CertChecker{
		IsUserAuthority:          checker.IsUserAuthority,
		Clock:                    func() time.Time { return testNow.Add(16 * time.Minute) },
		SupportedCriticalOptions: []string{"source-address"},
	}
	if err := expired.CheckCert("nas", cert); err == nil {
		t.Fatal("cert must be rejected after valid-before")
	}
}

func TestCertParsesWithSSHKeygen(t *testing.T) {
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen not installed")
	}
	c := newTestCA(t)
	cert, err := c.SignUserCert(validRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "id_ed25519-cert.pub")
	if err := os.WriteFile(path, ssh.MarshalAuthorizedKey(cert), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(keygen, "-L", "-f", path).CombinedOutput()
	if err != nil {
		t.Fatalf("ssh-keygen -L: %v\n%s", err, out)
	}
	s := string(out)
	for _, want := range []string{
		"Type: ssh-ed25519-cert-v01@openssh.com user certificate",
		`Key ID: "lor:g42:c57:agent=claude:host=nas:level=sudo"`,
		"Serial: 57",
		"nas:sudo",
		"source-address 192.168.1.50/32",
		"permit-pty",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("ssh-keygen output missing %q:\n%s", want, s)
		}
	}
	for _, bad := range []string{"permit-port-forwarding", "permit-user-rc", "permit-agent-forwarding", "forever"} {
		if strings.Contains(s, bad) {
			t.Errorf("ssh-keygen output contains %q:\n%s", bad, s)
		}
	}
}

func TestLoadOrGenerate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ca")
	c1, created, err := LoadOrGenerate(dir)
	if err != nil || !created {
		t.Fatalf("generate: created=%v err=%v", created, err)
	}
	fi, err := os.Stat(filepath.Join(dir, KeyFile))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("key mode = %o, want 600", fi.Mode().Perm())
	}
	c2, created, err := LoadOrGenerate(dir)
	if err != nil || created {
		t.Fatalf("load: created=%v err=%v", created, err)
	}
	if c1.Fingerprint() != c2.Fingerprint() {
		t.Fatal("reloaded CA has a different key")
	}
	pub, err := os.ReadFile(filepath.Join(dir, KeyFile+".pub"))
	if err != nil || strings.TrimSpace(string(pub)) != c1.AuthorizedKey() {
		t.Fatalf("pub file = %q err=%v", pub, err)
	}

	// A world-readable key is refused.
	if err := os.Chmod(filepath.Join(dir, KeyFile), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadOrGenerate(dir); err == nil {
		t.Fatal("expected refusal of a 0644 key")
	}
}

func TestParsePublicKey(t *testing.T) {
	pk, _ := newKey(t)
	if _, err := ParsePublicKey(string(ssh.MarshalAuthorizedKey(pk))); err != nil {
		t.Fatalf("ed25519: %v", err)
	}
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	rsaPub, _ := ssh.NewPublicKey(&rsaKey.PublicKey)
	if _, err := ParsePublicKey(string(ssh.MarshalAuthorizedKey(rsaPub))); err == nil {
		t.Fatal("rsa key must be refused")
	}
	if _, err := ParsePublicKey("garbage"); err == nil {
		t.Fatal("garbage must be refused")
	}
}

func TestPrincipal(t *testing.T) {
	if Principal("nas", "sudo") != "nas:sudo" || Principal("nas", "user") != "nas" {
		t.Fatal("Principal mapping wrong")
	}
}

// TestSourceAddressEnforced runs an x/crypto SSH server on loopback and checks
// that a certificate is only accepted from its source-address.
func TestSourceAddressEnforced(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	s, _ := ssh.NewSignerFromKey(priv)
	c, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	_, hostPriv, _ := ed25519.GenerateKey(rand.Reader)
	hostSigner, _ := ssh.NewSignerFromKey(hostPriv)
	checker := &ssh.CertChecker{
		IsUserAuthority: func(auth ssh.PublicKey) bool {
			return bytes.Equal(auth.Marshal(), c.PublicKey().Marshal())
		},
		SupportedCriticalOptions: []string{"source-address"},
	}
	cfg := &ssh.ServerConfig{PublicKeyCallback: checker.Authenticate}
	cfg.AddHostKey(hostSigner)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				sc, chans, reqs, err := ssh.NewServerConn(conn, cfg)
				if err != nil {
					conn.Close()
					return
				}
				go ssh.DiscardRequests(reqs)
				for ch := range chans {
					ch.Reject(ssh.Prohibited, "no")
				}
				sc.Close()
			}()
		}
	}()

	dial := func(src string) error {
		pk, signer := newKey(t)
		now := time.Now()
		cert, err := c.SignUserCert(UserCertRequest{
			PublicKey: pk, Serial: 1, KeyID: "k", Principal: "nas",
			ValidAfter: now.Add(-2 * time.Minute), ValidBefore: now.Add(time.Hour),
			SourceAddrs: []netip.Addr{netip.MustParseAddr(src)},
		})
		if err != nil {
			t.Fatal(err)
		}
		certSigner, err := ssh.NewCertSigner(cert, signer)
		if err != nil {
			t.Fatal(err)
		}
		client, err := ssh.Dial("tcp", ln.Addr().String(), &ssh.ClientConfig{
			User:            "nas",
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(certSigner)},
			HostKeyCallback: ssh.FixedHostKey(hostSigner.PublicKey()),
			Timeout:         5 * time.Second,
		})
		if err == nil {
			client.Close()
		}
		return err
	}
	if err := dial("127.0.0.1"); err != nil {
		t.Fatalf("cert for 127.0.0.1 should log in from 127.0.0.1: %v", err)
	}
	if err := dial("192.168.1.50"); err == nil {
		t.Fatal("cert for 192.168.1.50 must be rejected from 127.0.0.1")
	}
}
