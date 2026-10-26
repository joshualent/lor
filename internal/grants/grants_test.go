package grants

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/joshualent/lor/internal/apierr"
	"github.com/joshualent/lor/internal/ca"
	"github.com/joshualent/lor/internal/store"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type env struct {
	t      *testing.T
	ctx    context.Context
	db     *store.DB
	svc    *Service
	clk    *clock
	agent  *store.Agent
	other  *store.Agent
	nas    *store.Host
	pihole *store.Host
}

const admin = "admin:iPhone"

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "lor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	clk := &clock{t: time.Date(2026, 10, 25, 14, 0, 0, 0, time.UTC)}
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	authority, err := ca.New(signer, ca.WithClock(clk.now), ca.WithAbsoluteMax(720*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	svc := New(db, authority, Config{
		PublicURL:           "https://lor.example.com",
		DefaultDuration:     time.Hour,
		AgentRequestMax:     8 * time.Hour,
		AbsoluteMax:         720 * time.Hour,
		ApprovalTimeout:     30 * time.Minute,
		CommandLogRetention: 90 * 24 * time.Hour,
	}, clk.now)
	e := &env{t: t, ctx: ctx, db: db, svc: svc, clk: clk}
	e.agent = e.mkAgent("claude", "192.168.1.50")
	e.other = e.mkAgent("other", "192.168.1.51")
	e.nas = e.mkHost("nas", "sudo", true, false)
	e.pihole = e.mkHost("pihole", "user", false, false)
	return e
}

func (e *env) mkAgent(name, ip string) *store.Agent {
	a := &store.Agent{Name: name, Status: "active", LANAddrs: []string{ip}, CreatedAt: 1}
	if err := e.db.CreateAgent(e.ctx, a); err != nil {
		e.t.Fatal(err)
	}
	return a
}

func (e *env) mkHost(name, max string, infra, hyper bool) *store.Host {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	pk, _ := ssh.NewPublicKey(pub)
	h := &store.Host{Name: name, Address: "192.168.1.20", MaxLevel: max, LorInfra: infra, Hypervisor: hyper,
		HostKey: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pk))), HostKeySeenAt: 1, CreatedAt: 1}
	if err := e.db.CreateHost(e.ctx, h); err != nil {
		e.t.Fatal(err)
	}
	return h
}

func newPub(t *testing.T) string {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	pk, _ := ssh.NewPublicKey(pub)
	return string(ssh.MarshalAuthorizedKey(pk))
}

const reason = "Check the ZFS pool status and SMART data."

func (e *env) request(host, level string, d time.Duration) *RequestResult {
	e.t.Helper()
	res, err := e.svc.Request(e.ctx, e.agent, RequestInput{Host: host, Level: level, Duration: d, Reason: reason, PublicKey: newPub(e.t)})
	if err != nil {
		e.t.Fatalf("request: %v", err)
	}
	return res
}

func (e *env) grant(id int64) *store.Grant {
	g, err := e.db.GetGrant(e.ctx, id)
	if err != nil {
		e.t.Fatal(err)
	}
	return g
}

func (e *env) latestCert(id int64) *ssh.Certificate {
	c, err := e.db.LatestCertificate(e.ctx, id)
	if err != nil {
		e.t.Fatal(err)
	}
	pk, _, _, _, err := ssh.ParseAuthorizedKey([]byte(c.Certificate))
	if err != nil {
		e.t.Fatal(err)
	}
	return pk.(*ssh.Certificate)
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if apierr.Code(err) != code {
		t.Fatalf("want error %q, got %v", code, err)
	}
}

func TestRequestValidation(t *testing.T) {
	e := newEnv(t)
	good := RequestInput{Host: "nas", Level: "sudo", Duration: time.Hour, Reason: reason, PublicKey: newPub(t)}
	cases := []struct {
		name string
		mod  func(*RequestInput)
		code string
	}{
		{"reason too short", func(r *RequestInput) { r.Reason = "fix it" }, "invalid_reason"},
		{"reason 9 chars after trim", func(r *RequestInput) { r.Reason = "  123456789  " }, "invalid_reason"},
		{"reason too long", func(r *RequestInput) { r.Reason = strings.Repeat("a", 401) }, "invalid_reason"},
		{"reason control chars", func(r *RequestInput) { r.Reason = "hello world\x1b[31m" }, "invalid_reason"},
		{"missing reason", func(r *RequestInput) { r.Reason = "" }, "invalid_reason"},
		{"unknown host", func(r *RequestInput) { r.Host = "nope" }, "host_not_found"},
		{"level above host max", func(r *RequestInput) { r.Host = "pihole" }, "level_not_allowed"},
		{"bad level", func(r *RequestInput) { r.Level = "root" }, "invalid_level"},
		{"duration above agent max", func(r *RequestInput) { r.Duration = 8*time.Hour + time.Second }, "invalid_duration"},
		{"duration too short", func(r *RequestInput) { r.Duration = time.Second }, "invalid_duration"},
		{"rsa key", func(r *RequestInput) {
			r.PublicKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7 x"
		}, "invalid_public_key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := good
			tc.mod(&in)
			_, err := e.svc.Request(e.ctx, e.agent, in)
			wantCode(t, err, tc.code)
		})
	}
	// Boundaries are inclusive.
	for _, r := range []string{strings.Repeat("a", 10), strings.Repeat("é", 400)} {
		in := good
		in.Reason = r
		in.PublicKey = newPub(t)
		if _, err := e.svc.Request(e.ctx, e.agent, in); err != nil {
			t.Fatalf("reason of %d runes should be ok: %v", len([]rune(r)), err)
		}
	}
	in := good
	in.Duration = 8 * time.Hour
	if _, err := e.svc.Request(e.ctx, e.agent, in); err != nil {
		t.Fatalf("8h should be allowed: %v", err)
	}
}

func TestRequestDisabledAgentAndHost(t *testing.T) {
	e := newEnv(t)
	e.agent.Status = "disabled"
	_, err := e.svc.Request(e.ctx, e.agent, RequestInput{Host: "nas", Level: "user", Reason: reason, PublicKey: newPub(t)})
	wantCode(t, err, "agent_disabled")
	e.agent.Status = "active"

	e.nas.Disabled = true
	if err := e.db.UpdateHost(e.ctx, e.nas); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.Request(e.ctx, e.agent, RequestInput{Host: "nas", Level: "user", Reason: reason, PublicKey: newPub(t)})
	wantCode(t, err, "host_disabled")
}

func TestDefaultDurationAndLorInfra(t *testing.T) {
	e := newEnv(t)
	r := e.request("nas", "user", 0) // nas is lor_infra
	if r.Grant.RequestedDurationS != 900 {
		t.Fatalf("lor_infra default = %ds, want 900", r.Grant.RequestedDurationS)
	}
	r = e.request("pihole", "user", 0)
	if r.Grant.RequestedDurationS != 3600 {
		t.Fatalf("default = %ds, want 3600", r.Grant.RequestedDurationS)
	}
	if r.Grant.Status != store.StatusPending {
		t.Fatalf("status %s", r.Grant.Status)
	}
	if e.svc.ApprovalURL(r.Grant.ID) != "https://lor.example.com/r/2" {
		t.Fatalf("approval url %s", e.svc.ApprovalURL(r.Grant.ID))
	}
}

func TestMaxPending(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 3; i++ {
		e.request("pihole", "user", time.Hour)
	}
	_, err := e.svc.Request(e.ctx, e.agent, RequestInput{Host: "pihole", Level: "user", Reason: reason, PublicKey: newPub(t)})
	wantCode(t, err, "too_many_pending")
	// Another agent isn't affected.
	if _, err := e.svc.Request(e.ctx, e.other, RequestInput{Host: "pihole", Level: "user", Reason: reason, PublicKey: newPub(t)}); err != nil {
		t.Fatal(err)
	}
	// Timed-out requests stop counting.
	e.clk.add(31 * time.Minute)
	e.request("pihole", "user", time.Hour)
}

func TestAuthorizeWithChanges(t *testing.T) {
	e := newEnv(t)
	r := e.request("nas", "sudo", time.Hour)
	id := r.Grant.ID

	// Sudo needs the confirmation.
	_, err := e.svc.Authorize(e.ctx, admin, AuthorizeInput{GrantID: id, Level: "sudo", Duration: 15 * time.Minute})
	wantCode(t, err, "sudo_not_confirmed")

	// Downgrade to user with any duration (beyond agent max is fine for admin).
	g, err := e.svc.Authorize(e.ctx, admin, AuthorizeInput{GrantID: id, Level: "user", Duration: 3 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if g.Status != store.StatusActive || g.Level != "user" || g.RequestedLevel != "sudo" {
		t.Fatalf("grant = %+v", g)
	}
	if g.ValidBefore != e.clk.now().Add(72*time.Hour).Unix() {
		t.Fatal("valid_before must be now + approved duration")
	}
	cert := e.latestCert(id)
	if cert.ValidPrincipals[0] != "nas" {
		t.Fatalf("principal %v, want nas (downgraded)", cert.ValidPrincipals)
	}
	if cert.CriticalOptions["source-address"] != "192.168.1.50/32" {
		t.Fatalf("source-address %q", cert.CriticalOptions["source-address"])
	}
	if !strings.HasPrefix(cert.KeyId, "lor:g1:c") || !strings.HasSuffix(cert.KeyId, ":agent=claude:host=nas:level=user") {
		t.Fatalf("key id %q", cert.KeyId)
	}
	// A decided request can't be authorized again.
	_, err = e.svc.Authorize(e.ctx, admin, AuthorizeInput{GrantID: id, Level: "user", Duration: time.Hour})
	wantCode(t, err, "not_pending")
}

func TestAuthorizeNeverUpgrades(t *testing.T) {
	e := newEnv(t)
	r := e.request("nas", "user", time.Hour)
	_, err := e.svc.Authorize(e.ctx, admin, AuthorizeInput{GrantID: r.Grant.ID, Level: "sudo", Duration: time.Hour, ConfirmSudo: true})
	wantCode(t, err, "level_raised")
	// Host max lowered after the request: sudo can no longer be authorized.
	r = e.request("nas", "sudo", time.Hour)
	e.nas.MaxLevel = "user"
	e.db.UpdateHost(e.ctx, e.nas)
	_, err = e.svc.Authorize(e.ctx, admin, AuthorizeInput{GrantID: r.Grant.ID, Level: "sudo", Duration: time.Hour, ConfirmSudo: true})
	wantCode(t, err, "level_not_allowed")
	// Beyond the absolute max.
	_, err = e.svc.Authorize(e.ctx, admin, AuthorizeInput{GrantID: r.Grant.ID, Level: "user", Duration: 721 * time.Hour})
	wantCode(t, err, "invalid_duration")
}

func TestDenyAndTimeout(t *testing.T) {
	e := newEnv(t)
	r := e.request("pihole", "user", time.Hour)
	g, err := e.svc.Deny(e.ctx, admin, r.Grant.ID, "not now")
	if err != nil || g.Status != store.StatusDenied || g.DecisionNote != "not now" {
		t.Fatalf("deny: %+v %v", g, err)
	}
	_, err = e.svc.Authorize(e.ctx, admin, AuthorizeInput{GrantID: r.Grant.ID, Level: "user", Duration: time.Hour})
	wantCode(t, err, "not_pending")

	r = e.request("pihole", "user", time.Hour)
	e.clk.add(30 * time.Minute)
	_, err = e.svc.Authorize(e.ctx, admin, AuthorizeInput{GrantID: r.Grant.ID, Level: "user", Duration: time.Hour})
	wantCode(t, err, "not_pending")
	if st := e.grant(r.Grant.ID).Status; st != store.StatusRequestTimeout {
		t.Fatalf("status %s", st)
	}
}

func TestReuse(t *testing.T) {
	e := newEnv(t)
	r := e.request("nas", "sudo", time.Hour)
	if _, err := e.svc.Authorize(e.ctx, admin, AuthorizeInput{GrantID: r.Grant.ID, Level: "sudo", Duration: time.Hour, ConfirmSudo: true}); err != nil {
		t.Fatal(err)
	}
	// A user request is satisfied by the active sudo grant.
	r2 := e.request("nas", "user", time.Hour)
	if !r2.Reused || r2.Grant.ID != r.Grant.ID {
		t.Fatalf("expected reuse of %d, got %+v", r.Grant.ID, r2)
	}
	// Another agent doesn't reuse it.
	r3, err := e.svc.Request(e.ctx, e.other, RequestInput{Host: "nas", Level: "user", Reason: reason, PublicKey: newPub(t)})
	if err != nil || r3.Reused {
		t.Fatalf("other agent: %+v %v", r3, err)
	}
	// After expiry it's not reused.
	e.clk.add(time.Hour)
	r4 := e.request("nas", "user", time.Hour)
	if r4.Reused || r4.Grant.Status != store.StatusPending {
		t.Fatalf("expired grant reused: %+v", r4)
	}

	// A user grant does not satisfy a sudo request.
	e2 := newEnv(t)
	u := e2.request("nas", "user", time.Hour)
	e2.svc.Authorize(e2.ctx, admin, AuthorizeInput{GrantID: u.Grant.ID, Level: "user", Duration: time.Hour})
	s := e2.request("nas", "sudo", time.Hour)
	if s.Reused {
		t.Fatal("user grant must not satisfy a sudo request")
	}
}

func TestOfferClaim(t *testing.T) {
	e := newEnv(t)
	_, _, err := e.svc.CreateOffer(e.ctx, admin, OfferInput{AgentID: e.agent.ID, HostID: e.nas.ID, Level: "sudo", Duration: 30 * time.Minute})
	wantCode(t, err, "sudo_not_confirmed")
	g, msg, err := e.svc.CreateOffer(e.ctx, admin, OfferInput{AgentID: e.agent.ID, HostID: e.nas.ID, Level: "sudo", Duration: 30 * time.Minute, ConfirmSudo: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "`nas` (sudo) until") || !strings.Contains(msg, "lor ssh nas -- <cmd>") {
		t.Fatalf("offer message %q", msg)
	}
	end := e.clk.now().Add(30 * time.Minute).Unix()
	if g.Status != store.StatusOffered || g.ValidBefore != end || g.PublicKey != "" {
		t.Fatalf("offer = %+v", g)
	}
	// No certificate exists yet.
	if _, err := e.db.LatestCertificate(e.ctx, g.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("offer must not have a certificate")
	}

	e.clk.add(10 * time.Minute) // the timer started at creation

	// The wrong agent can't claim it.
	_, err = e.svc.Claim(e.ctx, e.other, g.ID, newPub(t))
	wantCode(t, err, "grant_not_found")

	claimed, err := e.svc.Claim(e.ctx, e.agent, g.ID, newPub(t))
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Status != store.StatusActive || claimed.ValidBefore != end {
		t.Fatalf("claimed = %+v", claimed)
	}
	cert := e.latestCert(g.ID)
	if cert.ValidBefore != uint64(end) || cert.ValidPrincipals[0] != "nas:sudo" {
		t.Fatalf("cert end %d principal %v", cert.ValidBefore, cert.ValidPrincipals)
	}
	// Claimed only once.
	_, err = e.svc.Claim(e.ctx, e.agent, g.ID, newPub(t))
	wantCode(t, err, "not_offered")
}

func TestOfferClaimedByRequest(t *testing.T) {
	e := newEnv(t)
	g, _, err := e.svc.CreateOffer(e.ctx, admin, OfferInput{AgentID: e.agent.ID, HostID: e.nas.ID, Level: "user", Duration: 30 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	// A sudo request doesn't match a user offer.
	r := e.request("nas", "sudo", time.Hour)
	if r.Claimed {
		t.Fatal("sudo request must not claim a user offer")
	}
	r = e.request("nas", "user", time.Hour)
	if !r.Claimed || r.Grant.ID != g.ID || r.Grant.Status != store.StatusActive {
		t.Fatalf("expected claim: %+v", r)
	}
}

func TestOfferExpires(t *testing.T) {
	e := newEnv(t)
	g, _, _ := e.svc.CreateOffer(e.ctx, admin, OfferInput{AgentID: e.agent.ID, HostID: e.nas.ID, Level: "user", Duration: 30 * time.Minute})
	e.clk.add(30 * time.Minute)
	_, err := e.svc.Claim(e.ctx, e.agent, g.ID, newPub(t))
	wantCode(t, err, "not_offered")
	if st := e.grant(g.ID).Status; st != store.StatusOfferExpired {
		t.Fatalf("status %s", st)
	}
}

func (e *env) active(host, level string, d time.Duration) *store.Grant {
	e.t.Helper()
	r := e.request(host, level, d)
	g, err := e.svc.Authorize(e.ctx, admin, AuthorizeInput{GrantID: r.Grant.ID, Level: level, Duration: d, ConfirmSudo: level == "sudo"})
	if err != nil {
		e.t.Fatal(err)
	}
	return g
}

func TestExtensions(t *testing.T) {
	e := newEnv(t)
	g := e.active("nas", "sudo", time.Hour)
	end := g.ValidBefore
	e.clk.add(50 * time.Minute)

	ext, reused, err := e.svc.RequestExtension(e.ctx, e.agent, g.ID, time.Hour, "Scrub still running, need more time.")
	if err != nil || reused {
		t.Fatal(err)
	}
	// A second request returns the pending one.
	ext2, reused, err := e.svc.RequestExtension(e.ctx, e.agent, g.ID, 2*time.Hour, "Scrub still running, need more time.")
	if err != nil || !reused || ext2.ID != ext.ID {
		t.Fatalf("expected reuse: %v %v", reused, err)
	}
	// Admin approves with a different duration; time counts from the current end.
	_, g2, err := e.svc.ApproveExtension(e.ctx, admin, ApproveExtensionInput{ExtensionID: ext.ID, Duration: 30 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if g2.ValidBefore != end+1800 || g2.Level != "sudo" {
		t.Fatalf("valid_before %d want %d, level %s", g2.ValidBefore, end+1800, g2.Level)
	}
	cert := e.latestCert(g.ID)
	if cert.ValidBefore != uint64(end+1800) {
		t.Fatal("new certificate must carry the new end")
	}
	certs, _ := e.db.ListCertificates(e.ctx, g.ID)
	if len(certs) != 2 {
		t.Fatalf("want 2 certificates, got %d", len(certs))
	}
	// Same key, new certificate.
	first, _, _, _, _ := ssh.ParseAuthorizedKey([]byte(certs[0].Certificate))
	if string(first.(*ssh.Certificate).Key.Marshal()) != string(cert.Key.Marshal()) {
		t.Fatal("extension must certify the same key")
	}

	// Admin extend.
	g3, err := e.svc.Extend(e.ctx, admin, ExtendInput{GrantID: g.ID, Duration: time.Hour})
	if err != nil || g3.ValidBefore != end+1800+3600 {
		t.Fatalf("extend: %v %+v", err, g3)
	}

	// Extension of an ended grant is moot.
	ext, _, err = e.svc.RequestExtension(e.ctx, e.agent, g.ID, time.Hour, "Scrub still running, need more time.")
	if err != nil {
		t.Fatal(err)
	}
	e.clk.add(3 * time.Hour)
	_, _, err = e.svc.ApproveExtension(e.ctx, admin, ApproveExtensionInput{ExtensionID: ext.ID, Duration: time.Hour})
	wantCode(t, err, "not_pending")
	got, _ := e.db.GetExtension(e.ctx, ext.ID)
	if got.Status != store.ExtMoot {
		t.Fatalf("extension status %s, want moot", got.Status)
	}
	_, _, err = e.svc.RequestExtension(e.ctx, e.agent, g.ID, time.Hour, "Scrub still running, need more time.")
	wantCode(t, err, "grant_expired")
	_, err = e.svc.Extend(e.ctx, admin, ExtendInput{GrantID: g.ID, Duration: time.Hour})
	wantCode(t, err, "not_active")
}

func TestExtensionMootOnRevoke(t *testing.T) {
	e := newEnv(t)
	g := e.active("pihole", "user", time.Hour)
	ext, _, _ := e.svc.RequestExtension(e.ctx, e.agent, g.ID, time.Hour, "Need a bit longer for logs.")
	if _, err := e.svc.Revoke(e.ctx, admin, g.ID, ""); err != nil {
		t.Fatal(err)
	}
	got, _ := e.db.GetExtension(e.ctx, ext.ID)
	if got.Status != store.ExtMoot {
		t.Fatalf("status %s", got.Status)
	}
}

func TestExtendOffer(t *testing.T) {
	e := newEnv(t)
	g, _, _ := e.svc.CreateOffer(e.ctx, admin, OfferInput{AgentID: e.agent.ID, HostID: e.pihole.ID, Level: "user", Duration: 30 * time.Minute})
	g2, err := e.svc.Extend(e.ctx, admin, ExtendInput{GrantID: g.ID, Duration: time.Hour})
	if err != nil || g2.ValidBefore != g.ValidBefore+3600 || g2.Status != store.StatusOffered {
		t.Fatalf("extend offer: %v %+v", err, g2)
	}
}

func TestExtendAbsoluteMax(t *testing.T) {
	e := newEnv(t)
	g := e.active("pihole", "user", time.Hour)
	if _, err := e.svc.Extend(e.ctx, admin, ExtendInput{GrantID: g.ID, Duration: 719 * time.Hour}); err != nil {
		t.Fatalf("extend to exactly the absolute max: %v", err)
	}
	_, err := e.svc.Extend(e.ctx, admin, ExtendInput{GrantID: g.ID, Duration: time.Hour})
	wantCode(t, err, "duration_too_long")
}

func TestConnect(t *testing.T) {
	e := newEnv(t)
	g := e.active("pihole", "user", time.Hour)
	_, cert, err := e.svc.Connect(e.ctx, e.agent, g.ID, []string{"zpool", "status"})
	if err != nil || cert == nil || cert.Certificate == "" {
		t.Fatalf("connect: %v", err)
	}
	cmds, _ := e.db.ListCommands(e.ctx, g.ID, 0)
	if len(cmds) != 1 || strings.Join(cmds[0].Argv, " ") != "zpool status" || cmds[0].CertificateID != cert.ID {
		t.Fatalf("commands %+v", cmds)
	}
	// Wrong agent.
	_, _, err = e.svc.Connect(e.ctx, e.other, g.ID, []string{"id"})
	wantCode(t, err, "grant_not_found")

	// Revoked: fails and logs nothing.
	e.svc.Revoke(e.ctx, admin, g.ID, "")
	_, _, err = e.svc.Connect(e.ctx, e.agent, g.ID, []string{"id"})
	wantCode(t, err, "grant_revoked")

	// Expired: fails and logs nothing, even before the sweeper runs.
	g2 := e.active("nas", "user", time.Hour)
	e.clk.add(time.Hour)
	_, _, err = e.svc.Connect(e.ctx, e.agent, g2.ID, []string{"id"})
	wantCode(t, err, "grant_expired")

	// Pending: fails.
	r := e.request("pihole", "user", time.Hour)
	_, _, err = e.svc.Connect(e.ctx, e.agent, r.Grant.ID, nil)
	wantCode(t, err, "grant_pending")

	if c, _ := e.db.ListCommands(e.ctx, g.ID, 0); len(c) != 1 {
		t.Fatalf("revoked grant logged %d commands", len(c))
	}
	if c, _ := e.db.ListCommands(e.ctx, g2.ID, 0); len(c) != 0 {
		t.Fatalf("expired grant logged %d commands", len(c))
	}
}

func TestTruncateArgv(t *testing.T) {
	long := strings.Repeat("x", 10000)
	out := truncateArgv([]string{"bash", "-c", long, "after"})
	if jsonLen(out) > MaxArgvBytes {
		t.Fatalf("truncated argv is %d bytes", jsonLen(out))
	}
	if out[0] != "bash" || out[len(out)-1] != "…[truncated]" {
		t.Fatalf("out = %q…", out[0])
	}
	if got := truncateArgv(nil); got == nil || len(got) != 0 {
		t.Fatal("nil argv must become []")
	}
	short := []string{"ls", "-la"}
	if got := truncateArgv(short); strings.Join(got, " ") != "ls -la" {
		t.Fatal("short argv changed")
	}
}

func TestRelease(t *testing.T) {
	e := newEnv(t)
	g := e.active("pihole", "user", time.Hour)
	_, err := e.svc.Release(e.ctx, e.other, g.ID)
	wantCode(t, err, "grant_not_found")
	g2, err := e.svc.Release(e.ctx, e.agent, g.ID)
	if err != nil || g2.Status != store.StatusReleased {
		t.Fatalf("release: %v", err)
	}
	_, err = e.svc.Release(e.ctx, e.agent, g.ID)
	wantCode(t, err, "grant_released")
}

func TestSweeper(t *testing.T) {
	e := newEnv(t)
	active := e.active("pihole", "user", time.Hour)
	pending := e.request("nas", "user", time.Hour).Grant
	offer, _, _ := e.svc.CreateOffer(e.ctx, admin, OfferInput{AgentID: e.agent.ID, HostID: e.nas.ID, Level: "user", Duration: 20 * time.Minute})
	ext, _, _ := e.svc.RequestExtension(e.ctx, e.agent, active.ID, time.Hour, "Need a bit longer for logs.")
	e.svc.Connect(e.ctx, e.agent, active.ID, []string{"uptime"})

	e.clk.add(25 * time.Minute)
	if err := e.svc.Sweep(e.ctx); err != nil {
		t.Fatal(err)
	}
	if st := e.grant(offer.ID).Status; st != store.StatusOfferExpired {
		t.Fatalf("offer %s", st)
	}
	if st := e.grant(pending.ID).Status; st != store.StatusPending {
		t.Fatalf("pending %s", st)
	}

	e.clk.add(6 * time.Minute) // 31m: request + extension time out
	e.svc.Sweep(e.ctx)
	if st := e.grant(pending.ID).Status; st != store.StatusRequestTimeout {
		t.Fatalf("pending %s", st)
	}
	if x, _ := e.db.GetExtension(e.ctx, ext.ID); x.Status != store.ExtRequestTimeout {
		t.Fatalf("ext %s", x.Status)
	}

	e.clk.add(30 * time.Minute) // past the active grant's end
	e.svc.Sweep(e.ctx)
	g := e.grant(active.ID)
	if g.Status != store.StatusExpired || g.EndedAt != g.ValidBefore {
		t.Fatalf("active -> %s ended %d", g.Status, g.EndedAt)
	}
	events, _ := e.db.ListAudit(e.ctx, store.AuditFilter{Actor: "system"})
	actions := map[string]bool{}
	for _, ev := range events {
		actions[ev.Action] = true
	}
	for _, want := range []string{"grant.expired", "grant.offer_expired", "grant.request_timeout", "extension.request_timeout"} {
		if !actions[want] {
			t.Errorf("missing audit %s (have %v)", want, actions)
		}
	}

	// Command log retention.
	if c, _ := e.db.ListCommands(e.ctx, active.ID, 0); len(c) != 1 {
		t.Fatal("command should still exist")
	}
	e.clk.add(91 * 24 * time.Hour)
	e.svc.Sweep(e.ctx)
	if c, _ := e.db.ListCommands(e.ctx, active.ID, 0); len(c) != 0 {
		t.Fatal("old command not deleted")
	}
}

func TestWaitWakesOnDecision(t *testing.T) {
	e := newEnv(t)
	r := e.request("pihole", "user", time.Hour)
	done := make(chan *store.Grant)
	go func() {
		g, _ := e.svc.WaitAgentGrant(e.ctx, e.agent, r.Grant.ID, 60*time.Second)
		done <- g
	}()
	time.Sleep(50 * time.Millisecond)
	e.svc.Authorize(e.ctx, admin, AuthorizeInput{GrantID: r.Grant.ID, Level: "user", Duration: time.Hour})
	select {
	case g := <-done:
		if g.Status != store.StatusActive {
			t.Fatalf("status %s", g.Status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("long-poll did not wake")
	}
}

func TestConsentAndWarnings(t *testing.T) {
	e := newEnv(t)
	r := e.request("nas", "sudo", 4*time.Hour)
	c := e.svc.Consent(r.Grant, e.nas)
	if c == nil || c.DefaultLevel != "sudo" || len(c.Levels) != 2 || c.DefaultDuration != 15*time.Minute {
		t.Fatalf("consent %+v", c)
	}
	w := HostWarnings(e.nas)
	codes := []string{}
	for _, x := range w {
		codes = append(codes, x.Code)
	}
	if strings.Join(codes, ",") != "lor_infra,root_access" {
		t.Fatalf("warnings %v", codes)
	}
	hyper := &store.Host{Name: "pve1", MaxLevel: "sudo", Hypervisor: true}
	w = HostWarnings(hyper)
	if len(w) != 2 || w[1].Code != "hypervisor" || w[1].Levels[0] != "sudo" || len(w[1].Levels) != 1 {
		t.Fatalf("hypervisor warnings %+v", w)
	}
	e.svc.Deny(e.ctx, admin, r.Grant.ID, "")
	if e.svc.Consent(e.grant(r.Grant.ID), e.nas) != nil {
		t.Fatal("decided request must have no consent block")
	}
}

func TestHostKeyRequired(t *testing.T) {
	e := newEnv(t)
	e.db.SetHostKey(e.ctx, e.pihole.ID, "", 0)
	_, err := e.svc.Request(e.ctx, e.agent, RequestInput{Host: "pihole", Level: "user", Reason: reason, PublicKey: newPub(t)})
	wantCode(t, err, "host_key_missing")
}

func TestHostMaxLevelRecheckedOnClaimAndExtend(t *testing.T) {
	e := newEnv(t)
	g := e.active("nas", "sudo", time.Hour)
	offer, _, err := e.svc.CreateOffer(e.ctx, admin, OfferInput{AgentID: e.agent.ID, HostID: e.nas.ID, Level: "sudo", Duration: time.Hour, ConfirmSudo: true})
	if err != nil {
		t.Fatal(err)
	}
	e.nas.MaxLevel = "user"
	e.db.UpdateHost(e.ctx, e.nas)
	_, err = e.svc.Claim(e.ctx, e.agent, offer.ID, newPub(t))
	wantCode(t, err, "level_not_allowed")
	_, err = e.svc.Extend(e.ctx, admin, ExtendInput{GrantID: g.ID, Duration: time.Hour})
	wantCode(t, err, "level_not_allowed")
	ext, _, _ := e.svc.RequestExtension(e.ctx, e.agent, g.ID, time.Hour, "Scrub still running, need more time.")
	_, _, err = e.svc.ApproveExtension(e.ctx, admin, ApproveExtensionInput{ExtensionID: ext.ID, Duration: time.Hour})
	wantCode(t, err, "level_not_allowed")
}

func TestListAgentGrantsFiltersBeforeLimit(t *testing.T) {
	e := newEnv(t)
	g := e.active("pihole", "user", time.Hour)
	for i := 0; i < 105; i++ {
		r := e.request("nas", "user", time.Hour)
		e.svc.Deny(e.ctx, admin, r.Grant.ID, "")
	}
	list, err := e.svc.ListAgentGrants(e.ctx, e.agent, "", []string{store.StatusActive})
	if err != nil || len(list) != 1 || list[0].ID != g.ID {
		t.Fatalf("active grant hidden: %d %v", len(list), err)
	}
}
