package hosts

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/joshualent/lor/internal/apierr"
	"github.com/joshualent/lor/internal/ca"
	"github.com/joshualent/lor/internal/store"
)

func newKey() ssh.PublicKey {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	k, _ := ssh.NewPublicKey(pub)
	return k
}

type env struct {
	ctx  context.Context
	db   *store.DB
	svc  *Service
	now  time.Time
	keys map[string]ssh.PublicKey
}

func newEnv(t *testing.T) *env {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "lor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	authority, _, err := ca.LoadOrGenerate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := &env{ctx: ctx, db: db, now: time.Now(), keys: map[string]ssh.PublicKey{}}
	scan := func(_ context.Context, addr string) (ssh.PublicKey, error) {
		if k, ok := e.keys[addr]; ok {
			return k, nil
		}
		return nil, errors.New("connection refused")
	}
	e.svc, err = New(ctx, db, authority, "https://lor.example.com", scan, func() time.Time { return e.now })
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestValidateAddress(t *testing.T) {
	good := map[string]string{
		"192.168.1.20":        "192.168.1.20",
		" 192.168.1.20 ":      "192.168.1.20",
		"192.168.1.20:22":     "192.168.1.20",
		"192.168.1.20:2222":   "192.168.1.20:2222",
		"fd00::20":            "fd00::20",
		"[fd00::20]:2222":     "[fd00::20]:2222",
		"::ffff:192.168.1.20": "192.168.1.20",
	}
	for in, want := range good {
		got, err := ValidateAddress(in)
		if err != nil || got != want {
			t.Errorf("ValidateAddress(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"nas", "nas.lan:22", "0.0.0.0", "192.168.1.20:0", "", "http://192.168.1.20"} {
		if _, err := ValidateAddress(bad); err == nil {
			t.Errorf("ValidateAddress(%q) should fail", bad)
		}
	}
	if ip, port := SplitAddress("192.168.1.20:2222"); ip != "192.168.1.20" || port != 2222 {
		t.Fatal("split with port")
	}
	if ip, port := SplitAddress("192.168.1.20"); ip != "192.168.1.20" || port != 22 {
		t.Fatal("split default")
	}
}

func TestCreateAndRescan(t *testing.T) {
	e := newEnv(t)
	k1 := newKey()
	e.keys["192.168.1.20"] = k1
	h, scanErr, err := e.svc.Create(e.ctx, "admin:x", Input{Name: "nas", Address: "192.168.1.20", MaxLevel: "sudo", LorInfra: true})
	if err != nil || scanErr != "" {
		t.Fatalf("create: %v %q", err, scanErr)
	}
	if h.HostKey != MarshalKey(k1) {
		t.Fatal("first scan should store the key")
	}
	if _, _, err := e.svc.Create(e.ctx, "admin:x", Input{Name: "nas", Address: "192.168.1.21", MaxLevel: "sudo"}); apierr.Code(err) != "host_exists" {
		t.Fatalf("duplicate: %v", err)
	}
	if _, _, err := e.svc.Create(e.ctx, "admin:x", Input{Name: "NAS!", Address: "192.168.1.21", MaxLevel: "sudo"}); apierr.Code(err) != "invalid_name" {
		t.Fatalf("bad name: %v", err)
	}

	// Same key: unchanged.
	res, err := e.svc.Rescan(e.ctx, "admin:x", "nas")
	if err != nil || res.Changed {
		t.Fatalf("rescan same: %+v %v", res, err)
	}
	// Changed key: reported, not stored.
	k2 := newKey()
	e.keys["192.168.1.20"] = k2
	res, err = e.svc.Rescan(e.ctx, "admin:x", "nas")
	if err != nil || !res.Changed || res.PreviousFP != ssh.FingerprintSHA256(k1) {
		t.Fatalf("rescan changed: %+v %v", res, err)
	}
	stored, _ := e.db.GetHostByName(e.ctx, "nas")
	if stored.HostKey != MarshalKey(k1) {
		t.Fatal("a changed key must not be stored without step-up")
	}
	// Changing the address keeps the old key (no TOFU bypass).
	addr := "192.168.1.99"
	e.keys[addr] = newKey()
	if _, err := e.svc.Edit(e.ctx, "admin:x", "nas", Update{Address: &addr}); err != nil {
		t.Fatal(err)
	}
	res, _ = e.svc.Rescan(e.ctx, "admin:x", "nas")
	if !res.Changed {
		t.Fatal("new address with a different key must be reported as changed")
	}
	// Accept (the step-up path).
	if _, err := e.svc.AcceptKey(e.ctx, "admin:x", stored.ID, res.Key); err != nil {
		t.Fatal(err)
	}
	stored, _ = e.db.GetHostByName(e.ctx, "nas")
	if stored.HostKey != res.Key {
		t.Fatal("accepted key not stored")
	}
	if _, _, err := e.svc.CheckAcceptKey(e.ctx, stored.ID, res.Key); apierr.Code(err) != "key_unchanged" {
		t.Fatalf("re-accept: %v", err)
	}
}

func TestCreateScanFailure(t *testing.T) {
	e := newEnv(t)
	h, scanErr, err := e.svc.Create(e.ctx, "admin:x", Input{Name: "pihole", Address: "192.168.1.30", MaxLevel: "user"})
	if err != nil || scanErr == "" || h.HostKey != "" {
		t.Fatalf("create: %v %q %+v", err, scanErr, h)
	}
}

func TestBootstrapToken(t *testing.T) {
	e := newEnv(t)
	tok, exp := e.svc.BootstrapToken("nas")
	if exp.Sub(e.now) != 15*time.Minute {
		t.Fatal("ttl")
	}
	if !e.svc.VerifyBootstrapToken("nas", tok) {
		t.Fatal("fresh token must verify")
	}
	if e.svc.VerifyBootstrapToken("pihole", tok) {
		t.Fatal("token is bound to the host")
	}
	if e.svc.VerifyBootstrapToken("nas", tok[:len(tok)-2]+"AA") || e.svc.VerifyBootstrapToken("nas", "garbage") {
		t.Fatal("tampered token must fail")
	}
	e.now = e.now.Add(15 * time.Minute)
	if e.svc.VerifyBootstrapToken("nas", tok) {
		t.Fatal("token must stop working after 15 minutes")
	}
}

func TestBootstrapLinksAndRender(t *testing.T) {
	e := newEnv(t)
	h := &store.Host{Name: "pihole", MaxLevel: "user"}
	b := e.svc.BootstrapLinks(h)
	if !strings.HasPrefix(b.Install, "curl -fsSL 'https://lor.example.com/bootstrap/pihole.sh?t=") ||
		!strings.HasSuffix(b.Install, "| ssh pihole sudo sh -s -- --no-sudo") {
		t.Fatalf("install %q", b.Install)
	}
	if !strings.HasSuffix(b.Uninstall, "sudo sh -s -- --uninstall") {
		t.Fatalf("uninstall %q", b.Uninstall)
	}
	h.MaxLevel = "sudo"
	if b := e.svc.BootstrapLinks(h); !strings.HasSuffix(b.Install, "| ssh pihole sudo sh") {
		t.Fatalf("sudo install %q", b.Install)
	}
	script, err := e.svc.RenderBootstrap(h)
	if err != nil {
		t.Fatal(err)
	}
	s := string(script)
	if !strings.Contains(s, "LOR_HOST='pihole'") || !strings.Contains(s, "LOR_CA_PUB='"+e.svc.ca.AuthorizedKey()+"'") {
		t.Fatal("script missing host or CA key")
	}
	if strings.Contains(s, "{{") {
		t.Fatal("unrendered template")
	}
	if sh, err := exec.LookPath("sh"); err == nil {
		p := filepath.Join(t.TempDir(), "b.sh")
		os.WriteFile(p, script, 0o600)
		if out, err := exec.Command(sh, "-n", p).CombinedOutput(); err != nil {
			t.Fatalf("sh -n: %v %s", err, out)
		}
	}
	if sc, err := exec.LookPath("shellcheck"); err == nil {
		p := filepath.Join(t.TempDir(), "b.sh")
		os.WriteFile(p, script, 0o600)
		if out, err := exec.Command(sc, "-s", "sh", p).CombinedOutput(); err != nil {
			t.Fatalf("shellcheck: %v\n%s", err, out)
		}
	}
}

// TestScanRealSSHServer scans an x/crypto SSH server on loopback.
func TestScanRealSSHServer(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	cfg := &ssh.ServerConfig{NoClientAuth: false, PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
		return nil, errors.New("no")
	}}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { ssh.NewServerConn(c, cfg); c.Close() }()
		}
	}()
	k, err := Scan(context.Background(), ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if MarshalKey(k) != MarshalKey(signer.PublicKey()) {
		t.Fatal("wrong key")
	}
	if _, err := Scan(context.Background(), "127.0.0.1:1"); err == nil {
		t.Fatal("closed port should fail")
	}
}
