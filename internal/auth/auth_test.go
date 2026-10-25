package auth

import (
	"context"
	"net/http"
	"net/netip"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joshualent/lor/internal/apierr"
	"github.com/joshualent/lor/internal/auth/authtest"
	"github.com/joshualent/lor/internal/store"
)

const (
	rpID   = "lor.example.com"
	origin = "https://lor.example.com"
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
	t     *testing.T
	ctx   context.Context
	db    *store.DB
	clk   *clock
	admin *Admin
}

func newEnv(t *testing.T) *env {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "lor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	clk := &clock{t: time.Now()}
	a, err := NewAdmin(ctx, db, rpID, []string{origin}, clk.now)
	if err != nil {
		t.Fatal(err)
	}
	return &env{t: t, ctx: ctx, db: db, clk: clk, admin: a}
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if apierr.Code(err) != code {
		t.Fatalf("want %q, got %v", code, err)
	}
}

// setup registers the first passkey and returns it with the session token.
func (e *env) setup() (*authtest.Passkey, string, *SessionInfo) {
	e.t.Helper()
	code, _, err := CreateSetupCode(e.ctx, e.db, e.clk.now())
	if err != nil {
		e.t.Fatal(err)
	}
	pk := authtest.New(rpID, origin, false)
	id, opts, err := e.admin.SetupBegin(e.ctx, code, "iPhone")
	if err != nil {
		e.t.Fatal(err)
	}
	resp, err := pk.Register(opts)
	if err != nil {
		e.t.Fatal(err)
	}
	token, info, err := e.admin.SetupFinish(e.ctx, id, resp, RequestMeta{UserAgent: "test"})
	if err != nil {
		e.t.Fatalf("setup finish: %v", err)
	}
	return pk, token, info
}

func (e *env) login(pk *authtest.Passkey) (string, *SessionInfo, error) {
	id, opts, err := e.admin.LoginBegin(e.ctx)
	if err != nil {
		return "", nil, err
	}
	resp, err := pk.Assert(opts)
	if err != nil {
		e.t.Fatal(err)
	}
	return e.admin.LoginFinish(e.ctx, id, resp, RequestMeta{})
}

func TestSetupCodeWorksOnce(t *testing.T) {
	e := newEnv(t)
	code, exp, err := CreateSetupCode(e.ctx, e.db, e.clk.now())
	if err != nil {
		t.Fatal(err)
	}
	if exp.Sub(e.clk.now()) != 15*time.Minute {
		t.Fatalf("setup code ttl %v", exp.Sub(e.clk.now()))
	}
	_, _, err = e.admin.SetupBegin(e.ctx, "AAA-BBB-CCC", "iPhone")
	wantCode(t, err, "invalid_setup_code")

	pk := authtest.New(rpID, origin, false)
	// Lowercase and spaces are accepted.
	id, opts, err := e.admin.SetupBegin(e.ctx, strings.ToLower(strings.ReplaceAll(code, "-", " ")), "iPhone")
	if err != nil {
		t.Fatal(err)
	}
	resp, _ := pk.Register(opts)
	if _, _, err := e.admin.SetupFinish(e.ctx, id, resp, RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	// Second use fails, and so does a new code while a passkey exists.
	_, _, err = e.admin.SetupBegin(e.ctx, code, "Laptop")
	wantCode(t, err, "setup_done")
	if _, _, err := CreateSetupCode(e.ctx, e.db, e.clk.now()); err == nil {
		t.Fatal("setup-code must refuse while a passkey exists")
	}
}

func TestSetupCodeExpires(t *testing.T) {
	e := newEnv(t)
	code, _, _ := CreateSetupCode(e.ctx, e.db, e.clk.now())
	e.clk.add(15 * time.Minute)
	_, _, err := e.admin.SetupBegin(e.ctx, code, "iPhone")
	wantCode(t, err, "invalid_setup_code")
}

func TestLoginAndCookie(t *testing.T) {
	e := newEnv(t)
	pk, _, _ := e.setup()
	token, info, err := e.login(pk)
	if err != nil {
		t.Fatal(err)
	}
	if info.PasskeyName != "iPhone" || len(token) < 43 {
		t.Fatalf("info %+v token len %d", info, len(token))
	}
	got, err := e.admin.Authenticate(e.ctx, token)
	if err != nil || got.ID != info.ID {
		t.Fatalf("authenticate: %v", err)
	}
	// Cookie flags.
	c, err := http.ParseSetCookie(SessionCookie(token))
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "__Host-lor_session" || !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Domain != "" {
		t.Fatalf("cookie %+v", c)
	}
	// The token is stored hashed.
	if _, err := e.db.GetSessionByTokenHash(e.ctx, []byte(token)); err == nil {
		t.Fatal("token stored in clear")
	}
	_, err = e.admin.Authenticate(e.ctx, "nope")
	wantCode(t, err, "unauthenticated")
}

func TestLoginRequiresUserVerification(t *testing.T) {
	e := newEnv(t)
	pk, _, _ := e.setup()
	noUV := authtest.New(rpID, origin, true)
	noUV.Auth = pk.Auth
	noUV.Auth.Options.UserNotVerified = true
	noUV.Cred = pk.Cred
	_, _, err := e.login(noUV)
	wantCode(t, err, "passkey_failed")
}

func TestLoginWrongOrigin(t *testing.T) {
	e := newEnv(t)
	pk, _, _ := e.setup()
	evil := *pk
	evil.RP.Origin = "https://evil.example.com"
	_, _, err := e.login(&evil)
	wantCode(t, err, "passkey_failed")
}

func TestSessionIdleAndAbsoluteExpiry(t *testing.T) {
	e := newEnv(t)
	pk, _, _ := e.setup()
	token, _, _ := e.login(pk)
	// Activity every 6 days keeps it alive...
	for i := 0; i < 4; i++ {
		e.clk.add(6 * 24 * time.Hour)
		if _, err := e.admin.Authenticate(e.ctx, token); err != nil {
			t.Fatalf("day %d: %v", (i+1)*6, err)
		}
	}
	// ...until 30 days total.
	e.clk.add(6 * 24 * time.Hour) // day 30
	_, err := e.admin.Authenticate(e.ctx, token)
	wantCode(t, err, "session_expired")

	token, _, _ = e.login(pk)
	e.clk.add(7 * 24 * time.Hour)
	_, err = e.admin.Authenticate(e.ctx, token)
	wantCode(t, err, "session_expired")
}

func TestSignOutEverywhere(t *testing.T) {
	e := newEnv(t)
	pk, t1, info := e.setup()
	t2, _, _ := e.login(pk)
	if err := e.admin.RevokeAllSessions(e.ctx, info); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{t1, t2} {
		if _, err := e.admin.Authenticate(e.ctx, tok); err == nil {
			t.Fatal("session survived sign out everywhere")
		}
	}
}

type action struct {
	GrantID  int64
	Level    string
	Duration time.Duration
}

func (e *env) stepUp(pk *authtest.Passkey, s *SessionInfo, a action) (string, []byte) {
	e.t.Helper()
	id, opts, _, err := e.admin.StepUpBegin(e.ctx, s, a, "test")
	if err != nil {
		e.t.Fatal(err)
	}
	resp, err := pk.Assert(opts)
	if err != nil {
		e.t.Fatal(err)
	}
	return id, resp
}

func TestStepUpBoundToStoredAction(t *testing.T) {
	e := newEnv(t)
	pk, _, s := e.setup()
	want := action{GrantID: 42, Level: "user", Duration: 15 * time.Minute}
	id, resp := e.stepUp(pk, s, want)
	got, used, err := e.admin.StepUpFinish(e.ctx, s, id, resp)
	if err != nil {
		t.Fatal(err)
	}
	// Exactly the stored action comes back; the finish call carries nothing
	// else that could change it.
	if got.(action) != want || used.Name != "iPhone" {
		t.Fatalf("got %+v", got)
	}
}

func TestStepUpSingleUse(t *testing.T) {
	e := newEnv(t)
	pk, _, s := e.setup()
	id, resp := e.stepUp(pk, s, action{GrantID: 42})
	if _, _, err := e.admin.StepUpFinish(e.ctx, s, id, resp); err != nil {
		t.Fatal(err)
	}
	_, _, err := e.admin.StepUpFinish(e.ctx, s, id, resp)
	wantCode(t, err, "ceremony_not_found")
}

func TestStepUpFailedAttemptBurnsChallenge(t *testing.T) {
	e := newEnv(t)
	pk, _, s := e.setup()
	id, resp := e.stepUp(pk, s, action{GrantID: 42})
	if _, _, err := e.admin.StepUpFinish(e.ctx, s, id, []byte(`{"garbage":true}`)); err == nil {
		t.Fatal("garbage must fail")
	}
	_, _, err := e.admin.StepUpFinish(e.ctx, s, id, resp)
	wantCode(t, err, "ceremony_not_found")
}

func TestStepUpExpires(t *testing.T) {
	e := newEnv(t)
	pk, _, s := e.setup()
	id, resp := e.stepUp(pk, s, action{GrantID: 42})
	e.clk.add(5*time.Minute + time.Second)
	_, _, err := e.admin.StepUpFinish(e.ctx, s, id, resp)
	wantCode(t, err, "ceremony_expired")
}

func TestStepUpBoundToSession(t *testing.T) {
	e := newEnv(t)
	pk, _, s1 := e.setup()
	_, s2, _ := e.login(pk)
	id, resp := e.stepUp(pk, s1, action{GrantID: 42})
	_, _, err := e.admin.StepUpFinish(e.ctx, s2, id, resp)
	wantCode(t, err, "wrong_session")
}

func TestStepUpAssertionNotReusableForOtherChallenge(t *testing.T) {
	e := newEnv(t)
	pk, _, s := e.setup()
	// An assertion for "grant 42, user, 15m" can't authorize another stored
	// action ("grant 42, sudo, 1w" or grant 43): each challenge is unique.
	_, resp42 := e.stepUp(pk, s, action{GrantID: 42, Level: "user", Duration: 15 * time.Minute})
	id43, _, _, err := e.admin.StepUpBegin(e.ctx, s, action{GrantID: 43, Level: "sudo", Duration: 7 * 24 * time.Hour}, "x")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = e.admin.StepUpFinish(e.ctx, s, id43, resp42)
	wantCode(t, err, "passkey_failed")
}

func TestAddAndRemovePasskeys(t *testing.T) {
	e := newEnv(t)
	pk, _, s := e.setup()
	_, err := e.admin.CheckRemovePasskey(e.ctx, s.PasskeyID)
	wantCode(t, err, "last_passkey")

	laptop := authtest.New(rpID, origin, false)
	id, opts, err := e.admin.RegisterBegin(e.ctx, s, "Laptop")
	if err != nil {
		t.Fatal(err)
	}
	resp, _ := laptop.Register(opts)
	// Registration is bound to the session.
	_, other, _ := e.login(pk)
	_, err = e.admin.RegisterFinish(e.ctx, other, id, resp)
	wantCode(t, err, "wrong_session")

	id, opts, _ = e.admin.RegisterBegin(e.ctx, s, "Laptop")
	resp, _ = laptop.Register(opts)
	p, err := e.admin.RegisterFinish(e.ctx, s, id, resp)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.login(laptop); err != nil {
		t.Fatalf("login with new passkey: %v", err)
	}
	// Removing the iPhone passkey ends its sessions.
	tok, _, _ := e.login(pk)
	if err := e.admin.RemovePasskey(e.ctx, "admin:Laptop", s.PasskeyID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.admin.Authenticate(e.ctx, tok); err == nil {
		t.Fatal("sessions of a removed passkey must end")
	}
	if _, _, err := e.login(pk); err == nil {
		t.Fatal("removed passkey must not log in")
	}
	_ = p
}

func TestResetRemovesPasskeysAndSessions(t *testing.T) {
	e := newEnv(t)
	pk, tok, _ := e.setup()
	if err := Reset(e.ctx, e.db, e.clk.now()); err != nil {
		t.Fatal(err)
	}
	if _, err := e.admin.Authenticate(e.ctx, tok); err == nil {
		t.Fatal("session survived reset")
	}
	if n, _ := e.db.CountPasskeys(e.ctx); n != 0 {
		t.Fatal("passkeys survived reset")
	}
	_, _, err := e.login(pk)
	wantCode(t, err, "setup_required")
	// setup-code works again.
	if _, _, err := CreateSetupCode(e.ctx, e.db, e.clk.now()); err != nil {
		t.Fatal(err)
	}
}

func TestAgentEnrollAndAuth(t *testing.T) {
	e := newEnv(t)
	agents := NewAgents(e.db, e.clk.now)
	agentIP := netip.MustParseAddr("192.168.1.50")
	a, code, exp, err := agents.Create(e.ctx, "admin:iPhone", "claude", []string{"192.168.1.50"})
	if err != nil {
		t.Fatal(err)
	}
	if exp.Sub(e.clk.now()) != 15*time.Minute {
		t.Fatal("enroll code ttl")
	}
	_, _, _, err = agents.Create(e.ctx, "admin:iPhone", "claude", []string{"192.168.1.51"})
	wantCode(t, err, "agent_exists")

	// Wrong source IP can't enroll.
	_, _, err = agents.Enroll(e.ctx, code, netip.MustParseAddr("192.168.1.99"))
	wantCode(t, err, "wrong_source_ip")
	token, _, err := agents.Enroll(e.ctx, code, agentIP)
	if err != nil {
		t.Fatal(err)
	}
	// Single use.
	_, _, err = agents.Enroll(e.ctx, code, agentIP)
	wantCode(t, err, "invalid_enroll_code")

	if _, err := agents.Authenticate(e.ctx, "Bearer "+token, agentIP); err != nil {
		t.Fatal(err)
	}
	_, err = agents.Authenticate(e.ctx, "", agentIP)
	wantCode(t, err, "unauthorized")
	_, err = agents.Authenticate(e.ctx, "Bearer lor_wrong", agentIP)
	wantCode(t, err, "unauthorized")
	_, err = agents.Authenticate(e.ctx, "Bearer "+token, netip.MustParseAddr("192.168.1.51"))
	wantCode(t, err, "wrong_source_ip")
	// IPv4-mapped IPv6 is the same address.
	if _, err := agents.Authenticate(e.ctx, "Bearer "+token, netip.MustParseAddr("::ffff:192.168.1.50")); err != nil {
		t.Fatalf("mapped address: %v", err)
	}

	agents.SetStatus(e.ctx, "admin:iPhone", a.ID, "disabled")
	_, err = agents.Authenticate(e.ctx, "Bearer "+token, agentIP)
	wantCode(t, err, "agent_disabled")
	agents.SetStatus(e.ctx, "admin:iPhone", a.ID, "active")

	// Rotation revokes the old token.
	_, code2, _, err := agents.Rotate(e.ctx, "admin:iPhone", a.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = agents.Authenticate(e.ctx, "Bearer "+token, agentIP)
	wantCode(t, err, "unauthorized")
	if _, _, err := agents.Enroll(e.ctx, code2, agentIP); err != nil {
		t.Fatal(err)
	}
}

func TestEnrollCodeExpires(t *testing.T) {
	e := newEnv(t)
	agents := NewAgents(e.db, e.clk.now)
	_, code, _, _ := agents.Create(e.ctx, "admin:iPhone", "claude", []string{"192.168.1.50"})
	e.clk.add(15 * time.Minute)
	_, _, err := agents.Enroll(e.ctx, code, netip.MustParseAddr("192.168.1.50"))
	wantCode(t, err, "invalid_enroll_code")
}

func TestEnrollRateLimited(t *testing.T) {
	e := newEnv(t)
	agents := NewAgents(e.db, e.clk.now)
	ip := netip.MustParseAddr("192.168.1.66")
	var err error
	for i := 0; i < 6; i++ {
		_, _, err = agents.Enroll(e.ctx, "AAA-BBB-CCC", ip)
	}
	wantCode(t, err, "rate_limited")
	e.clk.add(time.Minute)
	_, _, err = agents.Enroll(e.ctx, "AAA-BBB-CCC", ip)
	wantCode(t, err, "invalid_enroll_code")
}

func TestParseLANAddrs(t *testing.T) {
	got, err := ParseLANAddrs([]string{" 192.168.1.50", "::ffff:192.168.1.50", "fd00::5"})
	if err != nil || strings.Join(got, ",") != "192.168.1.50,fd00::5" {
		t.Fatalf("got %v %v", got, err)
	}
	for _, bad := range [][]string{nil, {"0.0.0.0"}, {"nas.lan"}, {"224.0.0.1"}, {"fe80::1%eth0"}} {
		if _, err := ParseLANAddrs(bad); err == nil {
			t.Errorf("%v should be rejected", bad)
		}
	}
}

func TestNewCodeFormat(t *testing.T) {
	c := NewCode()
	if len(c) != 11 || c[3] != '-' || c[7] != '-' {
		t.Fatalf("code %q", c)
	}
	if NormalizeCode(" 7kq-m2p x9d ") != "7KQM2PX9D" {
		t.Fatal("normalize")
	}
}
