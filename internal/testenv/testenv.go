// Package testenv builds a complete in-process Lor (all services and both
// HTTP handlers) for tests, with an adjustable clock, a software passkey and
// fake host-key scanning.
package testenv

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/joshualent/lor/internal/auth"
	"github.com/joshualent/lor/internal/auth/authtest"
	"github.com/joshualent/lor/internal/ca"
	"github.com/joshualent/lor/internal/config"
	"github.com/joshualent/lor/internal/grants"
	"github.com/joshualent/lor/internal/hosts"
	"github.com/joshualent/lor/internal/server"
	"github.com/joshualent/lor/internal/store"
	"github.com/joshualent/lor/internal/tlscert"
)

// Addresses used by the harness.
const (
	PublicURL = "https://lor.example.com"
	ProxyIP   = "192.168.1.30"
	AgentIP   = "192.168.1.50"
)

// Clock is an adjustable clock.
type Clock struct {
	mu  sync.Mutex
	off time.Duration
}

// Now returns the current (shifted) time.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Now().Add(c.off)
}

// Advance moves the clock forward.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.off += d
	c.mu.Unlock()
}

// Env is a running in-process Lor.
type Env struct {
	T        testing.TB
	Ctx      context.Context
	Dir      string
	Cfg      *config.Config
	DB       *store.DB
	CA       *ca.CA
	Grants   *grants.Service
	Hosts    *hosts.Service
	Admin    *auth.Admin
	Agents   *auth.Agents
	Server   *server.Server
	Clock    *Clock
	TLS      string // TLS fingerprint
	TLSCert  tls.Certificate
	Passkey  *authtest.Passkey
	Cookie   string
	HostKeys map[string]ssh.PublicKey

	mu sync.Mutex
}

// Web is a minimal SPA bundle.
var Web = fstest.MapFS{
	"index.html":                      {Data: []byte("<!doctype html><title>Lor</title><div id=app></div>")},
	"favicon.svg":                     {Data: []byte("<svg/>")},
	"_app/immutable/entry/app.123.js": {Data: []byte("console.log(1)")},
	"_app/version.json":               {Data: []byte(`{"version":"1"}`)},
}

// New builds an Env.
func New(t testing.TB) *Env {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.PublicURL = PublicURL
	cfg.AdminListen = "127.0.0.1:0"
	cfg.AgentListen = "192.168.1.10:8443"
	cfg.TrustedProxy = ProxyIP
	clock := &Clock{}
	cfg.Now = clock.Now
	if err := cfg.Finish(); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, filepath.Join(dir, "lor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	authority, _, err := ca.LoadOrGenerate(filepath.Join(dir, "ca"), ca.WithClock(clock.Now), ca.WithAbsoluteMax(cfg.AbsoluteMaxDuration.D()))
	if err != nil {
		t.Fatal(err)
	}
	cert, fp, _, err := tlscert.LoadOrGenerate(filepath.Join(dir, "tls"), []string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	e := &Env{T: t, Ctx: ctx, Dir: dir, Cfg: &cfg, DB: db, CA: authority, Clock: clock, TLS: fp, TLSCert: cert, HostKeys: map[string]ssh.PublicKey{}}
	e.Grants = grants.New(db, authority, grants.Config{
		PublicURL: cfg.PublicURL, DefaultDuration: cfg.DefaultGrantDuration.D(), AgentRequestMax: cfg.AgentRequestMax.D(),
		AbsoluteMax: cfg.AbsoluteMaxDuration.D(), ApprovalTimeout: cfg.RequestApprovalTimeout.D(), CommandLogRetention: cfg.CommandLogRetention.D(),
	}, clock.Now)
	e.Hosts, err = hosts.New(ctx, db, authority, cfg.PublicURL, e.scan, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	e.Admin, err = auth.NewAdmin(ctx, db, cfg.RPID, cfg.AllowedOrigins(), clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	e.Agents = auth.NewAgents(db, clock.Now)
	e.Server, err = server.New(server.Options{Config: &cfg, DB: db, CA: authority, Grants: e.Grants, Hosts: e.Hosts,
		Admin: e.Admin, Agents: e.Agents, TLS: cert, TLSFingerprint: fp, Web: Web})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// SSHVersion is what the fake scanner reports as the host's SSH software.
var SSHVersion = "OpenSSH_10.2p1 Ubuntu-2ubuntu3.8"

func (e *Env) scan(_ context.Context, addr string) (ssh.PublicKey, string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if k, ok := e.HostKeys[addr]; ok {
		return k, SSHVersion, nil
	}
	return nil, "", fmt.Errorf("connection refused")
}

// StartAgentTLS serves the agent API over TLS on 127.0.0.1 (Lor's
// self-signed certificate) and returns its URL.
func (e *Env) StartAgentTLS() string {
	srv := httptest.NewUnstartedServer(e.Server.AgentHandler())
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{e.TLSCert}}
	srv.StartTLS()
	e.T.Cleanup(srv.Close)
	return srv.URL
}

// SetHostKey makes the fake scanner return a fresh key for addr.
func (e *Env) SetHostKey(addr string) ssh.PublicKey {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	k, _ := ssh.NewPublicKey(pub)
	e.mu.Lock()
	e.HostKeys[addr] = k
	e.mu.Unlock()
	return k
}

// Response is a recorded HTTP response.
type Response struct {
	*httptest.ResponseRecorder
}

// JSON decodes the body into v (fails the test on error).
func (r Response) JSON(t testing.TB, v any) {
	t.Helper()
	if err := json.Unmarshal(r.Body.Bytes(), v); err != nil {
		t.Fatalf("decode %d %s: %v", r.Code, r.Body.String(), err)
	}
}

// ErrCode returns the error code of an error response.
func (r Response) ErrCode() string {
	var b struct {
		Error struct{ Code string } `json:"error"`
	}
	json.Unmarshal(r.Body.Bytes(), &b)
	return b.Error.Code
}

// Map decodes the body into a map.
func (r Response) Map(t testing.TB) map[string]any {
	var m map[string]any
	r.JSON(t, &m)
	return m
}

// Req builds a request; body may be nil, a string or a value to marshal.
func Req(method, path string, body any) *http.Request {
	var rd *bytes.Reader
	switch b := body.(type) {
	case nil:
		rd = bytes.NewReader(nil)
	case string:
		rd = bytes.NewReader([]byte(b))
	case []byte:
		rd = bytes.NewReader(b)
	default:
		j, _ := json.Marshal(b)
		rd = bytes.NewReader(j)
	}
	r := httptest.NewRequest(method, path, rd)
	return r
}

// ServeAdmin sends r to the admin handler as-is (set RemoteAddr yourself).
func (e *Env) ServeAdmin(r *http.Request) Response {
	w := httptest.NewRecorder()
	e.Server.AdminHandler().ServeHTTP(w, r)
	return Response{w}
}

// ServeAgent sends r to the agent handler as-is.
func (e *Env) ServeAgent(r *http.Request) Response {
	w := httptest.NewRecorder()
	e.Server.AgentHandler().ServeHTTP(w, r)
	return Response{w}
}

// AdminDo sends a well-formed admin API request from the proxy, with the CSRF
// headers and the session cookie (if signed in).
func (e *Env) AdminDo(method, path string, body any) Response {
	r := Req(method, path, body)
	r.RemoteAddr = ProxyIP + ":40000"
	if method != http.MethodGet {
		r.Header.Set("Origin", PublicURL)
		r.Header.Set("X-Lor-Request", "1")
		r.Header.Set("Content-Type", "application/json")
		if body == nil {
			r.Body = http.NoBody
			r2 := Req(method, path, "{}")
			r.Body = r2.Body
			r.ContentLength = 2
		}
	}
	if e.Cookie != "" {
		r.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: e.Cookie})
	}
	return e.ServeAdmin(r)
}

// AgentDo sends an agent API request from ip with the bearer token.
func (e *Env) AgentDo(ip, token, method, path string, body any) Response {
	r := Req(method, path, body)
	r.RemoteAddr = ip + ":50000"
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	return e.ServeAgent(r)
}

func cookieValue(t testing.TB, r Response) string {
	for _, c := range r.Result().Cookies() {
		if c.Name == auth.SessionCookieName {
			return c.Value
		}
	}
	t.Fatalf("no session cookie in %d %s", r.Code, r.Body.String())
	return ""
}

// Setup registers the first passkey through the API and signs in.
func (e *Env) Setup() {
	t := e.T
	t.Helper()
	code, _, err := auth.CreateSetupCode(e.Ctx, e.DB, e.Clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	e.Passkey = authtest.New(e.Cfg.RPID, e.Cfg.PublicOrigin, false)
	var begin struct {
		CeremonyID string          `json:"ceremony_id"`
		Options    json.RawMessage `json:"options"`
	}
	r := e.AdminDo("POST", "/api/admin/v1/setup/begin", map[string]any{"code": code, "passkey_name": "iPhone"})
	if r.Code != 200 {
		t.Fatalf("setup begin: %d %s", r.Code, r.Body)
	}
	r.JSON(t, &begin)
	cred, err := e.Passkey.Register(begin.Options)
	if err != nil {
		t.Fatal(err)
	}
	r = e.AdminDo("POST", "/api/admin/v1/setup/finish", map[string]any{"ceremony_id": begin.CeremonyID, "credential": json.RawMessage(cred)})
	if r.Code != 200 {
		t.Fatalf("setup finish: %d %s", r.Code, r.Body)
	}
	e.Cookie = cookieValue(t, r)
}

// Login signs in again with the passkey and returns the new cookie value.
func (e *Env) Login() string {
	t := e.T
	t.Helper()
	var begin struct {
		CeremonyID string          `json:"ceremony_id"`
		Options    json.RawMessage `json:"options"`
	}
	r := e.AdminDo("POST", "/api/admin/v1/login/begin", map[string]any{})
	if r.Code != 200 {
		t.Fatalf("login begin: %d %s", r.Code, r.Body)
	}
	r.JSON(t, &begin)
	cred, err := e.Passkey.Assert(begin.Options)
	if err != nil {
		t.Fatal(err)
	}
	r = e.AdminDo("POST", "/api/admin/v1/login/finish", map[string]any{"ceremony_id": begin.CeremonyID, "credential": json.RawMessage(cred)})
	if r.Code != 200 {
		t.Fatalf("login finish: %d %s", r.Code, r.Body)
	}
	return cookieValue(t, r)
}

// StepUpBegin starts a step-up for action and returns the ceremony ID and
// options, or the error response.
func (e *Env) StepUpBegin(action map[string]any) (string, json.RawMessage, Response) {
	r := e.AdminDo("POST", "/api/admin/v1/stepup/begin", map[string]any{"action": action})
	if r.Code != 200 {
		return "", nil, r
	}
	var b struct {
		CeremonyID string          `json:"ceremony_id"`
		Options    json.RawMessage `json:"options"`
	}
	r.JSON(e.T, &b)
	return b.CeremonyID, b.Options, r
}

// StepUpFinish taps the passkey for a ceremony.
func (e *Env) StepUpFinish(id string, options json.RawMessage) Response {
	cred, err := e.Passkey.Assert(options)
	if err != nil {
		e.T.Fatal(err)
	}
	return e.AdminDo("POST", "/api/admin/v1/stepup/finish", map[string]any{"ceremony_id": id, "credential": json.RawMessage(cred)})
}

// StepUp runs begin + finish and fails the test on error.
func (e *Env) StepUp(action map[string]any) Response {
	e.T.Helper()
	id, opts, r := e.StepUpBegin(action)
	if id == "" {
		e.T.Fatalf("step-up begin %v: %d %s", action, r.Code, r.Body)
	}
	r = e.StepUpFinish(id, opts)
	if r.Code != 200 {
		e.T.Fatalf("step-up finish %v: %d %s", action, r.Code, r.Body)
	}
	return r
}

// AddHost creates a host through the API (its key scans successfully).
func (e *Env) AddHost(name, addr, maxLevel string, lorInfra, hypervisor bool) int64 {
	e.T.Helper()
	e.SetHostKey(addr)
	r := e.AdminDo("POST", "/api/admin/v1/hosts", map[string]any{"name": name, "address": addr, "max_level": maxLevel, "lor_infra": lorInfra, "hypervisor": hypervisor})
	if r.Code != 200 {
		e.T.Fatalf("add host: %d %s", r.Code, r.Body)
	}
	var d struct {
		Host struct{ ID int64 } `json:"host"`
	}
	r.JSON(e.T, &d)
	return d.Host.ID
}

// AddAgent creates and enrolls an agent; returns its ID and token.
func (e *Env) AddAgent(name, ip string) (int64, string) {
	e.T.Helper()
	r := e.AdminDo("POST", "/api/admin/v1/agents", map[string]any{"name": name, "lan_addrs": []string{ip}})
	if r.Code != 200 {
		e.T.Fatalf("add agent: %d %s", r.Code, r.Body)
	}
	var en struct {
		Agent   struct{ ID int64 } `json:"agent"`
		Command string             `json:"command"`
	}
	r.JSON(e.T, &en)
	code := en.Command[strings.LastIndex(en.Command, " ")+1:]
	r = e.AgentDo(ip, "", "POST", "/api/v1/enroll", map[string]any{"code": code})
	if r.Code != 200 {
		e.T.Fatalf("enroll: %d %s", r.Code, r.Body)
	}
	var tok struct{ Token string }
	r.JSON(e.T, &tok)
	return en.Agent.ID, tok.Token
}

// NewPublicKey returns a fresh ed25519 public key (authorized_keys format).
func NewPublicKey() string {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	k, _ := ssh.NewPublicKey(pub)
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(k)))
}
