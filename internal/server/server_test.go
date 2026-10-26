package server_test

import (
	"encoding/json"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/joshualent/lor/internal/server"
	"github.com/joshualent/lor/internal/testenv"
)

const reason = "Check the ZFS pool status and SMART data."

func TestRoutesOnlyOnTheirListener(t *testing.T) {
	e := testenv.New(t)
	e.Setup()
	_, token := e.AddAgent("claude", testenv.AgentIP)

	// Agent API on the admin listener: JSON 404.
	r := testenv.Req("GET", "/api/v1/whoami", nil)
	r.RemoteAddr = testenv.ProxyIP + ":1"
	r.Header.Set("Authorization", "Bearer "+token)
	if res := e.ServeAdmin(r); res.Code != 404 || res.ErrCode() != "not_found" {
		t.Fatalf("agent API on admin listener: %d %s", res.Code, res.Body)
	}
	// Admin API and SPA on the agent listener: JSON 404.
	for _, p := range []string{"/api/admin/v1/session", "/", "/r/42", "/bootstrap/nas.sh"} {
		res := e.AgentDo(testenv.AgentIP, token, "GET", p, nil)
		if res.Code != 404 || res.ErrCode() != "not_found" {
			t.Fatalf("%s on agent listener: %d %s", p, res.Code, res.Body)
		}
	}
	// And each works on its own listener.
	if res := e.AgentDo(testenv.AgentIP, token, "GET", "/api/v1/whoami", nil); res.Code != 200 {
		t.Fatalf("whoami: %d %s", res.Code, res.Body)
	}
	if res := e.AdminDo("GET", "/api/admin/v1/session", nil); res.Code != 200 {
		t.Fatalf("session: %d", res.Code)
	}
}

func TestAdminRejectsNonProxy(t *testing.T) {
	e := testenv.New(t)
	r := testenv.Req("GET", "/api/admin/v1/session", nil)
	r.RemoteAddr = testenv.AgentIP + ":1234"
	if res := e.ServeAdmin(r); res.Code != 403 || res.ErrCode() != "not_from_proxy" {
		t.Fatalf("from agent IP: %d %s", res.Code, res.Body)
	}
	r = testenv.Req("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	if res := e.ServeAdmin(r); res.Code != 403 {
		t.Fatalf("SPA from loopback: %d", res.Code)
	}
}

// The admin listener drops TCP connections that don't come from the proxy.
func TestFilterListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	for _, tc := range []struct {
		proxy  string
		accept bool
	}{{"127.0.0.1", true}, {"127.0.0.2", false}} {
		srv, url := server.StartFiltered(t, port, netip.MustParseAddr(tc.proxy))
		c := &http.Client{Timeout: 2 * time.Second}
		resp, err := c.Get(url)
		if tc.accept && (err != nil || resp.StatusCode != 200) {
			t.Fatalf("proxy %s: %v", tc.proxy, err)
		}
		if !tc.accept && err == nil {
			t.Fatalf("proxy %s: connection from 127.0.0.1 should be dropped, got %d", tc.proxy, resp.StatusCode)
		}
		if resp != nil {
			resp.Body.Close()
		}
		srv.Close()
	}
}

func TestCSRF(t *testing.T) {
	e := testenv.New(t)
	e.Setup()
	base := func() *http.Request {
		r := testenv.Req("POST", "/api/admin/v1/logout", "{}")
		r.RemoteAddr = testenv.ProxyIP + ":1"
		r.Header.Set("Origin", testenv.PublicURL)
		r.Header.Set("X-Lor-Request", "1")
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(&http.Cookie{Name: "__Host-lor_session", Value: e.Cookie})
		return r
	}
	cases := map[string]func(r *http.Request){
		"missing origin":    func(r *http.Request) { r.Header.Del("Origin") },
		"wrong origin":      func(r *http.Request) { r.Header.Set("Origin", "https://evil.example.com") },
		"null origin":       func(r *http.Request) { r.Header.Set("Origin", "null") },
		"origin with path":  func(r *http.Request) { r.Header.Set("Origin", testenv.PublicURL+"/") },
		"missing header":    func(r *http.Request) { r.Header.Del("X-Lor-Request") },
		"wrong header":      func(r *http.Request) { r.Header.Set("X-Lor-Request", "yes") },
		"form content type": func(r *http.Request) { r.Header.Set("Content-Type", "application/x-www-form-urlencoded") },
		"text content type": func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") },
		"no content type":   func(r *http.Request) { r.Header.Del("Content-Type") },
	}
	for name, mod := range cases {
		r := base()
		mod(r)
		if res := e.ServeAdmin(r); res.Code != 403 || !strings.HasPrefix(res.ErrCode(), "csrf") {
			t.Errorf("%s: %d %s", name, res.Code, res.Body)
		}
	}
	// Login endpoints are covered too.
	r := testenv.Req("POST", "/api/admin/v1/login/begin", "{}")
	r.RemoteAddr = testenv.ProxyIP + ":1"
	r.Header.Set("Content-Type", "application/json")
	if res := e.ServeAdmin(r); res.Code != 403 {
		t.Errorf("login without CSRF headers: %d", res.Code)
	}
	// The well-formed request works.
	if res := e.ServeAdmin(base()); res.Code != 200 {
		t.Fatalf("valid logout: %d %s", res.Code, res.Body)
	}
	// No CORS headers, ever.
	r = base()
	r.Header.Set("Origin", "https://evil.example.com")
	if res := e.ServeAdmin(r); res.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS header sent")
	}
}

func TestSessionRequired(t *testing.T) {
	e := testenv.New(t)
	for _, p := range []string{"/api/admin/v1/dashboard", "/api/admin/v1/grants/1", "/api/admin/v1/hosts", "/api/admin/v1/audit"} {
		res := e.AdminDo("GET", p, nil)
		if res.Code != 401 || res.ErrCode() != "unauthenticated" {
			t.Errorf("%s without session: %d %s", p, res.Code, res.Body)
		}
	}
	if res := e.AdminDo("POST", "/api/admin/v1/stepup/begin", map[string]any{"action": map[string]any{"kind": "create_offer"}}); res.Code != 401 {
		t.Errorf("step-up without session: %d", res.Code)
	}
	if res := e.AdminDo("GET", "/api/admin/v1/session", nil); res.Code != 200 {
		t.Fatalf("session state is public: %d", res.Code)
	}
}

func TestLoginCookieFlags(t *testing.T) {
	e := testenv.New(t)
	e.Setup()
	_ = e.Login()
	var begin struct {
		CeremonyID string          `json:"ceremony_id"`
		Options    json.RawMessage `json:"options"`
	}
	e.AdminDo("POST", "/api/admin/v1/login/begin", map[string]any{}).JSON(t, &begin)
	cred, _ := e.Passkey.Assert(begin.Options)
	res := e.AdminDo("POST", "/api/admin/v1/login/finish", map[string]any{"ceremony_id": begin.CeremonyID, "credential": json.RawMessage(cred)})
	sc := res.Header().Get("Set-Cookie")
	for _, want := range []string{"__Host-lor_session=", "Path=/", "HttpOnly", "Secure", "SameSite=Strict"} {
		if !strings.Contains(sc, want) {
			t.Errorf("Set-Cookie %q missing %q", sc, want)
		}
	}
	if strings.Contains(sc, "Domain=") {
		t.Error("__Host- cookie must not set Domain")
	}
}

func TestAgentAuth(t *testing.T) {
	e := testenv.New(t)
	e.Setup()
	agentID, token := e.AddAgent("claude", testenv.AgentIP)

	if r := e.AgentDo(testenv.AgentIP, "", "GET", "/api/v1/whoami", nil); r.Code != 401 {
		t.Errorf("missing token: %d", r.Code)
	}
	if r := e.AgentDo(testenv.AgentIP, "lor_wrong", "GET", "/api/v1/whoami", nil); r.Code != 401 {
		t.Errorf("wrong token: %d", r.Code)
	}
	if r := e.AgentDo("192.168.1.99", token, "GET", "/api/v1/whoami", nil); r.Code != 403 || r.ErrCode() != "wrong_source_ip" {
		t.Errorf("wrong source IP: %d %s", r.Code, r.Body)
	}
	// X-Forwarded-For is never trusted.
	req := testenv.Req("GET", "/api/v1/whoami", nil)
	req.RemoteAddr = "192.168.1.99:5000"
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Forwarded-For", testenv.AgentIP)
	if r := e.ServeAgent(req); r.Code != 403 {
		t.Errorf("X-Forwarded-For spoof: %d", r.Code)
	}
	if r := e.AdminDo("POST", "/api/admin/v1/agents/"+itoa(agentID)+"/status", map[string]any{"status": "disabled"}); r.Code != 200 {
		t.Fatalf("disable: %d %s", r.Code, r.Body)
	}
	if r := e.AgentDo(testenv.AgentIP, token, "GET", "/api/v1/whoami", nil); r.Code != 403 || r.ErrCode() != "agent_disabled" {
		t.Errorf("disabled agent: %d %s", r.Code, r.Body)
	}
}

func itoa(i int64) string { return jsonNum(i) }

func jsonNum(i int64) string {
	b, _ := json.Marshal(i)
	return string(b)
}

func TestEnrollmentCode(t *testing.T) {
	e := testenv.New(t)
	e.Setup()
	r := e.AdminDo("POST", "/api/admin/v1/agents", map[string]any{"name": "claude", "lan_addrs": []string{testenv.AgentIP}})
	var en struct{ Command string }
	r.JSON(t, &en)
	if !strings.HasPrefix(en.Command, "lor enroll --url https://192.168.1.10:8443 --fingerprint "+e.TLS+" --code ") {
		t.Fatalf("command %q", en.Command)
	}
	code := en.Command[strings.LastIndex(en.Command, " ")+1:]
	if r := e.AgentDo(testenv.AgentIP, "", "POST", "/api/v1/enroll", map[string]any{"code": code}); r.Code != 200 {
		t.Fatalf("enroll: %d %s", r.Code, r.Body)
	}
	if r := e.AgentDo(testenv.AgentIP, "", "POST", "/api/v1/enroll", map[string]any{"code": code}); r.Code != 403 {
		t.Fatalf("second enroll: %d", r.Code)
	}
	// Expiry.
	r = e.AdminDo("POST", "/api/admin/v1/agents", map[string]any{"name": "other", "lan_addrs": []string{"192.168.1.51"}})
	r.JSON(t, &en)
	code = en.Command[strings.LastIndex(en.Command, " ")+1:]
	e.Clock.Advance(15 * time.Minute)
	if r := e.AgentDo("192.168.1.51", "", "POST", "/api/v1/enroll", map[string]any{"code": code}); r.Code != 403 || r.ErrCode() != "invalid_enroll_code" {
		t.Fatalf("expired code: %d %s", r.Code, r.Body)
	}
}

type agentGrant struct {
	ID          int64  `json:"id"`
	Status      string `json:"status"`
	Level       string `json:"level"`
	ApprovalURL string `json:"approval_url"`
	Certificate string `json:"certificate"`
	SSH         *struct {
		Hostname       string `json:"hostname"`
		Port           int    `json:"port"`
		User           string `json:"user"`
		KnownHostsLine string `json:"known_hosts_line"`
		HostKeyAlias   string `json:"host_key_alias"`
	} `json:"ssh"`
}

func request(t *testing.T, e *testenv.Env, token, host, level string) agentGrant {
	t.Helper()
	r := e.AgentDo(testenv.AgentIP, token, "POST", "/api/v1/grants", map[string]any{
		"host": host, "level": level, "duration_s": 3600, "reason": reason, "public_key": testenv.NewPublicKey(),
	})
	if r.Code != 200 {
		t.Fatalf("request: %d %s", r.Code, r.Body)
	}
	var res struct {
		Grant       agentGrant `json:"grant"`
		ApprovalURL string     `json:"approval_url"`
	}
	r.JSON(t, &res)
	return res.Grant
}

func TestConsentAndStepUpAuthorize(t *testing.T) {
	e := testenv.New(t)
	e.Setup()
	hostID := e.AddHost("nas", "192.168.1.20", "sudo", true, true)
	_ = hostID
	_, token := e.AddAgent("claude", testenv.AgentIP)
	g := request(t, e, token, "nas", "sudo")
	if g.Status != "pending" || g.ApprovalURL != "https://lor.example.com/r/1" {
		t.Fatalf("grant %+v", g)
	}

	// Consent view: server-computed warnings, consent block, actions.
	var d struct {
		Warnings []struct {
			Code   string
			Levels []string
		}
		Consent *struct {
			Levels           []string
			DefaultDurationS int64 `json:"default_duration_s"`
		}
		Actions struct{ Authorize, Deny bool }
	}
	e.AdminDo("GET", "/api/admin/v1/grants/1", nil).JSON(t, &d)
	codes := []string{}
	for _, w := range d.Warnings {
		codes = append(codes, w.Code)
	}
	if strings.Join(codes, ",") != "lor_infra,root_access,hypervisor" {
		t.Fatalf("warnings %v", codes)
	}
	if d.Consent == nil || d.Consent.DefaultDurationS != 900 || !d.Actions.Authorize {
		t.Fatalf("consent %+v actions %+v", d.Consent, d.Actions)
	}

	// The sudo confirmation is required.
	_, _, res := e.StepUpBegin(map[string]any{"kind": "authorize_request", "grant_id": 1, "level": "sudo", "duration_s": 900})
	if res.Code != 400 || res.ErrCode() != "sudo_not_confirmed" {
		t.Fatalf("no confirm: %d %s", res.Code, res.Body)
	}
	// Raising is refused at begin.
	g2 := request(t, e, token, "nas", "user")
	_, _, res = e.StepUpBegin(map[string]any{"kind": "authorize_request", "grant_id": g2.ID, "level": "sudo", "duration_s": 900, "confirm_sudo": true})
	if res.ErrCode() != "level_raised" {
		t.Fatalf("raise: %d %s", res.Code, res.Body)
	}

	// There is no way to authorize without step-up.
	for _, p := range []string{"/api/admin/v1/grants/1/authorize", "/api/admin/v1/grants/1/extend", "/api/admin/v1/offers"} {
		if r := e.AdminDo("POST", p, map[string]any{"level": "sudo"}); r.Code != 404 {
			t.Fatalf("%s: %d", p, r.Code)
		}
	}

	// Authorize with changes: downgrade to user, 2h.
	res = e.StepUp(map[string]any{"kind": "authorize_request", "grant_id": 1, "level": "user", "duration_s": 7200})
	var out struct {
		Grant struct {
			Status, Level string
			DecidedBy     string `json:"decided_by"`
			Actions       struct{ Authorize bool }
			Consent       any
		}
	}
	res.JSON(t, &out)
	if out.Grant.Status != "active" || out.Grant.Level != "user" || out.Grant.DecidedBy != "admin:iPhone" {
		t.Fatalf("authorized %+v", out.Grant)
	}
	// A decided request has no authorize action or consent block.
	if out.Grant.Actions.Authorize || out.Grant.Consent != nil {
		t.Fatal("decided request still offers authorize")
	}
	_, _, res = e.StepUpBegin(map[string]any{"kind": "authorize_request", "grant_id": 1, "level": "user", "duration_s": 900})
	if res.ErrCode() != "not_pending" {
		t.Fatalf("re-authorize: %d %s", res.Code, res.Body)
	}

	// Agent sees the cert and ssh block.
	var ag agentGrant
	e.AgentDo(testenv.AgentIP, token, "GET", "/api/v1/grants/1", nil).JSON(t, &ag)
	if ag.Status != "active" || ag.Certificate == "" || ag.SSH == nil || ag.SSH.User != "agent" || ag.SSH.HostKeyAlias != "lor-host-nas" ||
		!strings.HasPrefix(ag.SSH.KnownHostsLine, "lor-host-nas ssh-ed25519 ") || ag.SSH.Port != 22 {
		t.Fatalf("agent view %+v %+v", ag, ag.SSH)
	}
}

func TestStepUpAppliesStoredActionOnly(t *testing.T) {
	e := testenv.New(t)
	e.Setup()
	e.AddHost("nas", "192.168.1.20", "sudo", false, false)
	_, token := e.AddAgent("claude", testenv.AgentIP)
	g1 := request(t, e, token, "nas", "sudo")
	g2 := request(t, e, token, "nas", "sudo")

	// Begin for "g1, user, 15m"; the finish request tries to smuggle other params.
	id, opts, _ := e.StepUpBegin(map[string]any{"kind": "authorize_request", "grant_id": g1.ID, "level": "user", "duration_s": 900})
	cred, _ := e.Passkey.Assert(opts)
	res := e.AdminDo("POST", "/api/admin/v1/stepup/finish", map[string]any{
		"ceremony_id": id, "credential": json.RawMessage(cred),
		"action": map[string]any{"kind": "authorize_request", "grant_id": g2.ID, "level": "sudo", "duration_s": 604800, "confirm_sudo": true},
	})
	if res.Code != 200 {
		t.Fatalf("finish: %d %s", res.Code, res.Body)
	}
	var a1, a2 agentGrant
	e.AgentDo(testenv.AgentIP, token, "GET", "/api/v1/grants/"+jsonNum(g1.ID), nil).JSON(t, &a1)
	e.AgentDo(testenv.AgentIP, token, "GET", "/api/v1/grants/"+jsonNum(g2.ID), nil).JSON(t, &a2)
	if a1.Status != "active" || a1.Level != "user" || a2.Status != "pending" {
		t.Fatalf("g1 %s/%s g2 %s", a1.Status, a1.Level, a2.Status)
	}
	// The same tap can't be replayed.
	res = e.AdminDo("POST", "/api/admin/v1/stepup/finish", map[string]any{"ceremony_id": id, "credential": json.RawMessage(cred)})
	if res.Code != 400 || res.ErrCode() != "ceremony_not_found" {
		t.Fatalf("replay: %d %s", res.Code, res.Body)
	}
	// Another session can't finish this session's step-up.
	id, opts, _ = e.StepUpBegin(map[string]any{"kind": "authorize_request", "grant_id": g2.ID, "level": "user", "duration_s": 900})
	mine := e.Cookie
	e.Cookie = e.Login()
	res = e.StepUpFinish(id, opts)
	if res.Code != 403 || res.ErrCode() != "wrong_session" {
		t.Fatalf("other session: %d %s", res.Code, res.Body)
	}
	e.Cookie = mine
}

func TestOfferConnectAndCommandLog(t *testing.T) {
	e := testenv.New(t)
	e.Setup()
	hostID := e.AddHost("pihole", "192.168.1.21:2222", "user", false, false)
	agentID, token := e.AddAgent("claude", testenv.AgentIP)
	res := e.StepUp(map[string]any{"kind": "create_offer", "agent_id": agentID, "host_id": hostID, "level": "user", "duration_s": 1800})
	var out struct {
		OfferMessage string `json:"offer_message"`
		Grant        struct{ ID int64 }
	}
	res.JSON(t, &out)
	if !strings.Contains(out.OfferMessage, "You have Lor access to `pihole` until") {
		t.Fatalf("offer message %q", out.OfferMessage)
	}
	id := jsonNum(out.Grant.ID)
	r := e.AgentDo(testenv.AgentIP, token, "POST", "/api/v1/grants/"+id+"/claim", map[string]any{"public_key": testenv.NewPublicKey()})
	var g agentGrant
	r.JSON(t, &g)
	if g.Status != "active" || g.SSH.Port != 2222 || g.SSH.Hostname != "192.168.1.21" {
		t.Fatalf("claim: %d %s", r.Code, r.Body)
	}
	r = e.AgentDo(testenv.AgentIP, token, "POST", "/api/v1/grants/"+id+"/connect", map[string]any{"argv": []string{"pihole", "status"}})
	if r.Code != 200 {
		t.Fatalf("connect: %d %s", r.Code, r.Body)
	}
	var d struct {
		Commands []struct{ Argv []string }
	}
	e.AdminDo("GET", "/api/admin/v1/grants/"+id, nil).JSON(t, &d)
	if len(d.Commands) != 1 || strings.Join(d.Commands[0].Argv, " ") != "pihole status" {
		t.Fatalf("commands %+v", d.Commands)
	}
	// Revoke: connect fails and logs nothing.
	if r := e.AdminDo("POST", "/api/admin/v1/grants/"+id+"/revoke", map[string]any{}); r.Code != 200 {
		t.Fatalf("revoke: %d %s", r.Code, r.Body)
	}
	r = e.AgentDo(testenv.AgentIP, token, "POST", "/api/v1/grants/"+id+"/connect", map[string]any{"argv": []string{"id"}})
	if r.Code != 409 || r.ErrCode() != "grant_revoked" {
		t.Fatalf("connect after revoke: %d %s", r.Code, r.Body)
	}
	e.AdminDo("GET", "/api/admin/v1/grants/"+id, nil).JSON(t, &d)
	if len(d.Commands) != 1 {
		t.Fatal("revoked connect was logged")
	}
}

func TestExtensionFlowAPI(t *testing.T) {
	e := testenv.New(t)
	e.Setup()
	e.AddHost("nas", "192.168.1.20", "sudo", false, false)
	_, token := e.AddAgent("claude", testenv.AgentIP)
	g := request(t, e, token, "nas", "user")
	e.StepUp(map[string]any{"kind": "authorize_request", "grant_id": g.ID, "level": "user", "duration_s": 3600})
	r := e.AgentDo(testenv.AgentIP, token, "POST", "/api/v1/grants/"+jsonNum(g.ID)+"/extend", map[string]any{"duration_s": 3600, "reason": "Scrub still running, need more time."})
	var x struct {
		ApprovalURL      string             `json:"approval_url"`
		ExtensionRequest struct{ ID int64 } `json:"extension_request"`
	}
	r.JSON(t, &x)
	if x.ApprovalURL != "https://lor.example.com/e/1" {
		t.Fatalf("extension url %q", x.ApprovalURL)
	}
	var ed struct {
		CurrentEnd time.Time `json:"current_end"`
		WouldEnd   time.Time `json:"would_end"`
		Actions    struct{ Approve bool }
	}
	e.AdminDo("GET", "/api/admin/v1/extensions/1", nil).JSON(t, &ed)
	if !ed.Actions.Approve || ed.WouldEnd.Sub(ed.CurrentEnd) != time.Hour {
		t.Fatalf("extension detail %+v", ed)
	}
	e.StepUp(map[string]any{"kind": "approve_extension", "extension_id": 1, "duration_s": 1800})
	var ag agentGrant
	e.AgentDo(testenv.AgentIP, token, "GET", "/api/v1/grants/"+jsonNum(g.ID), nil).JSON(t, &ag)
	var d struct {
		ValidBefore  time.Time `json:"valid_before"`
		Certificates []any
	}
	e.AdminDo("GET", "/api/admin/v1/grants/"+jsonNum(g.ID), nil).JSON(t, &d)
	if len(d.Certificates) != 2 || d.ValidBefore.Sub(ed.CurrentEnd) != 30*time.Minute {
		t.Fatalf("after approval: %d certs, end moved %v", len(d.Certificates), d.ValidBefore.Sub(ed.CurrentEnd))
	}
}

func TestSPAServing(t *testing.T) {
	e := testenv.New(t)
	get := func(p string) testenv.Response {
		r := testenv.Req("GET", p, nil)
		r.RemoteAddr = testenv.ProxyIP + ":1"
		return e.ServeAdmin(r)
	}
	for _, p := range []string{"/r/42", "/", "/grants/new", "/hosts/nas", "/index.html"} {
		res := get(p)
		if res.Code != 200 || !strings.Contains(res.Body.String(), "<title>Lor</title>") || res.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: %d cache=%q", p, res.Code, res.Header().Get("Cache-Control"))
		}
	}
	res := get("/api/admin/v1/nope")
	if res.Code != 404 || res.ErrCode() != "not_found" || !strings.HasPrefix(res.Header().Get("Content-Type"), "application/json") {
		t.Errorf("unknown API path: %d %q %s", res.Code, res.Header().Get("Content-Type"), res.Body)
	}
	if res := get("/api/other"); res.Code != 404 || res.ErrCode() != "not_found" {
		t.Errorf("/api/other: %d", res.Code)
	}
	res = get("/_app/immutable/entry/app.123.js")
	if res.Code != 200 || res.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("immutable: %d %q", res.Code, res.Header().Get("Cache-Control"))
	}
	if res := get("/favicon.svg"); res.Code != 200 || res.Header().Get("Cache-Control") != "" {
		t.Errorf("static file: %d %q", res.Code, res.Header().Get("Cache-Control"))
	}
	for _, p := range []string{"/", "/api/admin/v1/session", "/r/1"} {
		h := get(p).Header()
		if h.Get("Content-Security-Policy") != "frame-ancestors 'none'" || h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Referrer-Policy") != "same-origin" {
			t.Errorf("%s headers: %v", p, h)
		}
	}
}

func TestBootstrapLink(t *testing.T) {
	e := testenv.New(t)
	e.Setup()
	e.AddHost("nas", "192.168.1.20", "sudo", false, false)
	var d struct {
		Bootstrap struct{ Install, Uninstall string }
	}
	e.AdminDo("GET", "/api/admin/v1/hosts/nas", nil).JSON(t, &d)
	link := d.Bootstrap.Install[strings.Index(d.Bootstrap.Install, "'")+1:]
	link = link[:strings.Index(link, "'")]
	path := strings.TrimPrefix(link, testenv.PublicURL)
	fetch := func() testenv.Response {
		r := testenv.Req("GET", path, nil)
		r.RemoteAddr = testenv.ProxyIP + ":1"
		return e.ServeAdmin(r) // no session cookie: curl doesn't have one
	}
	res := fetch()
	if res.Code != 200 || !strings.Contains(res.Body.String(), "LOR_HOST='nas'") || !strings.HasPrefix(res.Body.String(), "#!/bin/sh") {
		t.Fatalf("bootstrap: %d %.100s", res.Code, res.Body)
	}
	// Wrong host name with a valid token for nas.
	r := testenv.Req("GET", strings.Replace(path, "/nas.sh", "/pihole.sh", 1), nil)
	r.RemoteAddr = testenv.ProxyIP + ":1"
	if res := e.ServeAdmin(r); res.Code != 403 {
		t.Fatalf("token for another host: %d", res.Code)
	}
	e.Clock.Advance(15 * time.Minute)
	if res := fetch(); res.Code != 403 {
		t.Fatalf("after 15 minutes: %d", res.Code)
	}
}

func TestHostKeyChangeNeedsStepUp(t *testing.T) {
	e := testenv.New(t)
	e.Setup()
	hostID := e.AddHost("nas", "192.168.1.20", "sudo", false, false)
	e.SetHostKey("192.168.1.20")
	var scan struct {
		Changed bool
		HostKey string `json:"host_key"`
	}
	e.AdminDo("POST", "/api/admin/v1/hosts/nas/scan", map[string]any{}).JSON(t, &scan)
	if !scan.Changed {
		t.Fatal("expected changed key")
	}
	var h struct {
		Host struct {
			HostKey string `json:"host_key"`
		}
	}
	e.AdminDo("GET", "/api/admin/v1/hosts/nas", nil).JSON(t, &h)
	if h.Host.HostKey == scan.HostKey {
		t.Fatal("changed key stored without step-up")
	}
	e.StepUp(map[string]any{"kind": "accept_host_key", "host_id": hostID, "host_key": scan.HostKey})
	e.AdminDo("GET", "/api/admin/v1/hosts/nas", nil).JSON(t, &h)
	if h.Host.HostKey != scan.HostKey {
		t.Fatal("accepted key not stored")
	}
}

func TestSignOutEverywhereAPI(t *testing.T) {
	e := testenv.New(t)
	e.Setup()
	other := e.Login()
	if r := e.AdminDo("POST", "/api/admin/v1/sessions/revoke-all", map[string]any{}); r.Code != 200 {
		t.Fatalf("revoke-all: %d", r.Code)
	}
	for _, c := range []string{e.Cookie, other} {
		e.Cookie = c
		if r := e.AdminDo("GET", "/api/admin/v1/dashboard", nil); r.Code != 401 {
			t.Fatalf("session survived: %d", r.Code)
		}
	}
}

func TestDashboard(t *testing.T) {
	e := testenv.New(t)
	e.Setup()
	e.AddHost("nas", "192.168.1.20", "sudo", false, false)
	_, token := e.AddAgent("claude", testenv.AgentIP)
	request(t, e, token, "nas", "user")
	var d struct {
		PendingRequests []struct {
			Reason string
			Agent  struct{ Name string }
		} `json:"pending_requests"`
		RecentAudit []any `json:"recent_audit"`
	}
	e.AdminDo("GET", "/api/admin/v1/dashboard", nil).JSON(t, &d)
	if len(d.PendingRequests) != 1 || d.PendingRequests[0].Reason != reason || d.PendingRequests[0].Agent.Name != "claude" || len(d.RecentAudit) == 0 {
		t.Fatalf("dashboard %+v", d)
	}
}
