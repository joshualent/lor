package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/joshualent/lor/internal/apierr"
	"github.com/joshualent/lor/internal/auth"
	"github.com/joshualent/lor/internal/config"
	"github.com/joshualent/lor/internal/grants"
	"github.com/joshualent/lor/internal/hosts"
	"github.com/joshualent/lor/internal/server/reqctx"
	"github.com/joshualent/lor/internal/store"
)

// Handlers implements the admin API used by the web UI.
type Handlers struct {
	Config         *config.Config
	DB             *store.DB
	Grants         *grants.Service
	Hosts          *hosts.Service
	Admin          *auth.Admin
	Agents         *auth.Agents
	TLSFingerprint string
}

var _ StrictServerInterface = (*Handlers)(nil)

// PublicOperations need no session. Everything else does (deny by default).
var PublicOperations = map[string]bool{
	"GetSession":  true,
	"SetupBegin":  true,
	"SetupFinish": true,
	"LoginBegin":  true,
	"LoginFinish": true,
}

// SessionMiddleware requires a valid session for every non-public operation.
func (h *Handlers) SessionMiddleware(f StrictHandlerFunc, operationID string) StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, req any) (any, error) {
		ctx = reqctx.WithRequest(ctx, r)
		if !PublicOperations[operationID] {
			s, err := h.Admin.Authenticate(ctx, sessionToken(r))
			if err != nil {
				return nil, err
			}
			ctx = reqctx.WithSession(ctx, s)
		}
		return f(ctx, w, r, req)
	}
}

func sessionToken(r *http.Request) string {
	c, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func actor(ctx context.Context) string {
	return grants.AdminActor(reqctx.Session(ctx).PasskeyName)
}

func (h *Handlers) uiConfig() *UIConfig {
	c := h.Config
	return &UIConfig{
		PublicUrl:               c.PublicURL,
		DefaultGrantDurationS:   int64(c.DefaultGrantDuration.D() / time.Second),
		AgentRequestMaxS:        int64(c.AgentRequestMax.D() / time.Second),
		AbsoluteMaxDurationS:    int64(c.AbsoluteMaxDuration.D() / time.Second),
		RequestApprovalTimeoutS: int64(c.RequestApprovalTimeout.D() / time.Second),
	}
}

// ---------------------------------------------------------------- session, setup, login

// GetSession reports whether the caller is signed in.
func (h *Handlers) GetSession(ctx context.Context, _ GetSessionRequestObject) (GetSessionResponseObject, error) {
	setup, err := h.Admin.SetupRequired(ctx)
	if err != nil {
		return nil, err
	}
	out := GetSession200JSONResponse{SetupRequired: setup}
	if s, err := h.Admin.Authenticate(ctx, sessionToken(reqctx.Request(ctx))); err == nil {
		out.Authenticated = true
		out.PasskeyName = &s.PasskeyName
		out.Config = h.uiConfig()
	}
	return out, nil
}

// SetupBegin starts first-passkey registration.
func (h *Handlers) SetupBegin(ctx context.Context, req SetupBeginRequestObject) (SetupBeginResponseObject, error) {
	id, opts, err := h.Admin.SetupBegin(ctx, req.Body.Code, req.Body.PasskeyName)
	if err != nil {
		return nil, err
	}
	return SetupBegin200JSONResponse{CeremonyId: id, Options: opts}, nil
}

// SetupFinish registers the first passkey and signs in.
func (h *Handlers) SetupFinish(ctx context.Context, req SetupFinishRequestObject) (SetupFinishResponseObject, error) {
	token, s, err := h.Admin.SetupFinish(ctx, req.Body.CeremonyId, req.Body.Credential, reqctx.Meta(reqctx.Request(ctx)))
	if err != nil {
		return nil, err
	}
	cookie := auth.SessionCookie(token)
	return SetupFinish200JSONResponse{
		Body:    SessionState{Authenticated: true, PasskeyName: &s.PasskeyName, Config: h.uiConfig()},
		Headers: SetupFinish200ResponseHeaders{SetCookie: &cookie},
	}, nil
}

// LoginBegin starts a passkey login.
func (h *Handlers) LoginBegin(ctx context.Context, _ LoginBeginRequestObject) (LoginBeginResponseObject, error) {
	id, opts, err := h.Admin.LoginBegin(ctx)
	if err != nil {
		return nil, err
	}
	return LoginBegin200JSONResponse{CeremonyId: id, Options: opts}, nil
}

// LoginFinish verifies the passkey and sets the session cookie.
func (h *Handlers) LoginFinish(ctx context.Context, req LoginFinishRequestObject) (LoginFinishResponseObject, error) {
	token, s, err := h.Admin.LoginFinish(ctx, req.Body.CeremonyId, req.Body.Credential, reqctx.Meta(reqctx.Request(ctx)))
	if err != nil {
		return nil, err
	}
	cookie := auth.SessionCookie(token)
	return LoginFinish200JSONResponse{
		Body:    SessionState{Authenticated: true, PasskeyName: &s.PasskeyName, Config: h.uiConfig()},
		Headers: LoginFinish200ResponseHeaders{SetCookie: &cookie},
	}, nil
}

// Logout ends this session.
func (h *Handlers) Logout(ctx context.Context, _ LogoutRequestObject) (LogoutResponseObject, error) {
	if err := h.Admin.Logout(ctx, reqctx.Session(ctx)); err != nil {
		return nil, err
	}
	cookie := auth.ClearSessionCookie()
	return Logout200JSONResponse{Body: Ok{Ok: true}, Headers: Logout200ResponseHeaders{SetCookie: &cookie}}, nil
}

// ---------------------------------------------------------------- step-up

// stepUpAction is the normalized action stored server-side between begin and
// finish. It's a copy: nothing the browser sends at finish can change it.
type stepUpAction struct {
	Kind        StepUpKind
	GrantID     int64
	ExtensionID int64
	AgentID     int64
	HostID      int64
	PasskeyID   int64
	Level       string
	Duration    time.Duration
	ConfirmSudo bool
	Note        string
	LANAddrs    []string
	HostKey     string
	PasskeyName string
}

func i64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func need(v int64, name string) error {
	if v <= 0 {
		return apierr.BadRequest("invalid_action", "%s is required", name)
	}
	return nil
}

func normalize(a StepUpAction) (stepUpAction, error) {
	x := stepUpAction{
		Kind: a.Kind, GrantID: i64(a.GrantId), ExtensionID: i64(a.ExtensionId), AgentID: i64(a.AgentId),
		HostID: i64(a.HostId), PasskeyID: i64(a.PasskeyId), Note: str(a.Note), HostKey: str(a.HostKey), PasskeyName: str(a.PasskeyName),
	}
	if a.Level != nil {
		x.Level = string(*a.Level)
	}
	if a.DurationS != nil {
		if *a.DurationS <= 0 || *a.DurationS > int64(100*365*24*time.Hour/time.Second) {
			return x, apierr.BadRequest("invalid_duration", "invalid duration")
		}
		x.Duration = time.Duration(*a.DurationS) * time.Second
	}
	if a.ConfirmSudo != nil {
		x.ConfirmSudo = *a.ConfirmSudo
	}
	if a.LanAddrs != nil {
		x.LANAddrs = append([]string(nil), *a.LanAddrs...)
	}
	switch x.Kind {
	case AuthorizeRequest, ExtendGrant:
		return x, need(x.GrantID, "grant_id")
	case ApproveExtension:
		return x, need(x.ExtensionID, "extension_id")
	case CreateOffer:
		if err := need(x.AgentID, "agent_id"); err != nil {
			return x, err
		}
		return x, need(x.HostID, "host_id")
	case SetAgentAddrs:
		return x, need(x.AgentID, "agent_id")
	case AcceptHostKey:
		return x, need(x.HostID, "host_id")
	case RemovePasskey:
		return x, need(x.PasskeyID, "passkey_id")
	case AddPasskey:
		return x, nil
	}
	return x, apierr.BadRequest("invalid_action", "unknown step-up action %q", x.Kind)
}

// check validates an action against current state and returns a summary.
func (h *Handlers) check(ctx context.Context, a stepUpAction) (string, error) {
	switch a.Kind {
	case AuthorizeRequest:
		return h.Grants.CheckAuthorize(ctx, grants.AuthorizeInput{GrantID: a.GrantID, Level: a.Level, Duration: a.Duration, ConfirmSudo: a.ConfirmSudo})
	case ApproveExtension:
		return h.Grants.CheckApproveExtension(ctx, grants.ApproveExtensionInput{ExtensionID: a.ExtensionID, Duration: a.Duration})
	case CreateOffer:
		return h.Grants.CheckOffer(ctx, grants.OfferInput{AgentID: a.AgentID, HostID: a.HostID, Level: a.Level, Duration: a.Duration, ConfirmSudo: a.ConfirmSudo, Note: a.Note})
	case ExtendGrant:
		return h.Grants.CheckExtend(ctx, grants.ExtendInput{GrantID: a.GrantID, Duration: a.Duration})
	case SetAgentAddrs:
		agent, norm, err := h.Agents.CheckSetAddrs(ctx, a.AgentID, a.LANAddrs)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Change %s's LAN IPs to %s", agent.Name, strings.Join(norm, ", ")), nil
	case AcceptHostKey:
		host, fp, err := h.Hosts.CheckAcceptKey(ctx, a.HostID, a.HostKey)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Accept the new host key %s for %s", fp, host.Name), nil
	case AddPasskey:
		if _, err := validPasskeyName(a.PasskeyName); err != nil {
			return "", err
		}
		return fmt.Sprintf("Add a passkey named %q", strings.TrimSpace(a.PasskeyName)), nil
	case RemovePasskey:
		p, err := h.Admin.CheckRemovePasskey(ctx, a.PasskeyID)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Remove the passkey %q", p.Name), nil
	}
	return "", apierr.BadRequest("invalid_action", "unknown step-up action")
}

func validPasskeyName(n string) (string, error) {
	n = strings.TrimSpace(n)
	if n == "" || len([]rune(n)) > 64 {
		return "", apierr.BadRequest("invalid_passkey_name", "passkey name must be 1–64 characters")
	}
	return n, nil
}

// StepUpBegin validates an action and returns a challenge bound to it.
func (h *Handlers) StepUpBegin(ctx context.Context, req StepUpBeginRequestObject) (StepUpBeginResponseObject, error) {
	a, err := normalize(req.Body.Action)
	if err != nil {
		return nil, err
	}
	summary, err := h.check(ctx, a)
	if err != nil {
		return nil, err
	}
	id, opts, exp, err := h.Admin.StepUpBegin(ctx, reqctx.Session(ctx), a, summary)
	if err != nil {
		return nil, err
	}
	return StepUpBegin200JSONResponse{CeremonyId: id, Options: opts, Summary: summary, ExpiresAt: exp.UTC()}, nil
}

// StepUpFinish verifies the tap and applies exactly the stored action.
func (h *Handlers) StepUpFinish(ctx context.Context, req StepUpFinishRequestObject) (StepUpFinishResponseObject, error) {
	s := reqctx.Session(ctx)
	stored, passkey, err := h.Admin.StepUpFinish(ctx, s, req.Body.CeremonyId, req.Body.Credential)
	if err != nil {
		return nil, err
	}
	a, ok := stored.(stepUpAction)
	if !ok {
		return nil, errors.New("step-up: unexpected stored action")
	}
	// The tap's passkey is recorded as the decider.
	who := grants.AdminActor(passkey.Name)
	out := StepUpFinish200JSONResponse{Kind: a.Kind}
	switch a.Kind {
	case AuthorizeRequest:
		g, err := h.Grants.Authorize(ctx, who, grants.AuthorizeInput{GrantID: a.GrantID, Level: a.Level, Duration: a.Duration, ConfirmSudo: a.ConfirmSudo})
		if err != nil {
			return nil, err
		}
		d, err := h.detail(ctx, g)
		if err != nil {
			return nil, err
		}
		out.Grant = &d
	case ApproveExtension:
		e, g, err := h.Grants.ApproveExtension(ctx, who, grants.ApproveExtensionInput{ExtensionID: a.ExtensionID, Duration: a.Duration})
		if err != nil {
			return nil, err
		}
		d, err := h.extensionDetail(ctx, e, g)
		if err != nil {
			return nil, err
		}
		out.Extension = &d
		out.Grant = &d.Grant
	case CreateOffer:
		g, msg, err := h.Grants.CreateOffer(ctx, who, grants.OfferInput{AgentID: a.AgentID, HostID: a.HostID, Level: a.Level, Duration: a.Duration, ConfirmSudo: a.ConfirmSudo, Note: a.Note})
		if err != nil {
			return nil, err
		}
		d, err := h.detail(ctx, g)
		if err != nil {
			return nil, err
		}
		out.Grant = &d
		out.OfferMessage = &msg
	case ExtendGrant:
		g, err := h.Grants.Extend(ctx, who, grants.ExtendInput{GrantID: a.GrantID, Duration: a.Duration})
		if err != nil {
			return nil, err
		}
		d, err := h.detail(ctx, g)
		if err != nil {
			return nil, err
		}
		out.Grant = &d
	case SetAgentAddrs:
		agent, err := h.Agents.SetAddrs(ctx, who, a.AgentID, a.LANAddrs)
		if err != nil {
			return nil, err
		}
		v := agentView(agent)
		out.Agent = &v
	case AcceptHostKey:
		host, err := h.Hosts.AcceptKey(ctx, who, a.HostID, a.HostKey)
		if err != nil {
			return nil, err
		}
		d := h.hostDetail(host, "")
		out.Host = &d
	case AddPasskey:
		id, opts, err := h.Admin.RegisterBegin(ctx, s, a.PasskeyName)
		if err != nil {
			return nil, err
		}
		out.Registration = &CeremonyBegin{CeremonyId: id, Options: opts}
	case RemovePasskey:
		if err := h.Admin.RemovePasskey(ctx, who, a.PasskeyID); err != nil {
			return nil, err
		}
		t := true
		out.PasskeyRemoved = &t
	default:
		return nil, apierr.BadRequest("invalid_action", "unknown step-up action")
	}
	return out, nil
}

// ---------------------------------------------------------------- dashboard and grants

// GetDashboard returns everything the dashboard shows.
func (h *Handlers) GetDashboard(ctx context.Context, _ GetDashboardRequestObject) (GetDashboardResponseObject, error) {
	if err := h.Grants.Sweep(ctx); err != nil {
		return nil, err
	}
	open, err := h.DB.ListGrants(ctx, store.GrantFilter{Statuses: []string{store.StatusPending, store.StatusActive, store.StatusOffered}})
	if err != nil {
		return nil, err
	}
	l := newLookup(h.DB)
	out := GetDashboard200JSONResponse{
		PendingRequests: []GrantSummary{}, Active: []GrantSummary{}, Offered: []GrantSummary{}, RecentAudit: []AuditEvent{},
		ServerTime: h.Grants.Now().UTC(),
	}
	out.PendingExtensions = []struct {
		Extension Extension    `json:"extension"`
		Grant     GrantSummary `json:"grant"`
	}{}
	for _, g := range open {
		s, err := h.summary(ctx, l, g)
		if err != nil {
			return nil, err
		}
		switch g.Status {
		case store.StatusPending:
			out.PendingRequests = append(out.PendingRequests, s)
		case store.StatusActive:
			out.Active = append(out.Active, s)
			if s.PendingExtension != nil {
				out.PendingExtensions = append(out.PendingExtensions, struct {
					Extension Extension    `json:"extension"`
					Grant     GrantSummary `json:"grant"`
				}{*s.PendingExtension, s})
			}
		case store.StatusOffered:
			out.Offered = append(out.Offered, s)
		}
	}
	events, err := h.DB.ListAudit(ctx, store.AuditFilter{Limit: 15})
	if err != nil {
		return nil, err
	}
	for _, e := range events {
		out.RecentAudit = append(out.RecentAudit, auditView(e))
	}
	return out, nil
}

// ListGrants lists grants with filters.
func (h *Handlers) ListGrants(ctx context.Context, req ListGrantsRequestObject) (ListGrantsResponseObject, error) {
	if err := h.Grants.Sweep(ctx); err != nil {
		return nil, err
	}
	f := store.GrantFilter{AgentID: i64(req.Params.AgentId), HostID: i64(req.Params.HostId), BeforeID: i64(req.Params.Before), Limit: 50}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	if req.Params.Status != nil {
		for _, s := range *req.Params.Status {
			f.Statuses = append(f.Statuses, string(s))
		}
	}
	list, err := h.DB.ListGrants(ctx, f)
	if err != nil {
		return nil, err
	}
	l := newLookup(h.DB)
	out := ListGrants200JSONResponse{Grants: []GrantSummary{}}
	for _, g := range list {
		s, err := h.summary(ctx, l, g)
		if err != nil {
			return nil, err
		}
		out.Grants = append(out.Grants, s)
	}
	return out, nil
}

func (h *Handlers) settled(ctx context.Context, id int64) (*store.Grant, error) {
	return h.Grants.Settle(ctx, id)
}

// GetGrant returns a grant's detail (and consent view while pending).
func (h *Handlers) GetGrant(ctx context.Context, req GetGrantRequestObject) (GetGrantResponseObject, error) {
	g, err := h.settled(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	d, err := h.detail(ctx, g)
	return GetGrant200JSONResponse(d), err
}

func note(n *NoteRequest) string {
	if n == nil || n.Note == nil {
		return ""
	}
	return *n.Note
}

// DenyGrant denies a pending request (no step-up: it creates no access).
func (h *Handlers) DenyGrant(ctx context.Context, req DenyGrantRequestObject) (DenyGrantResponseObject, error) {
	g, err := h.Grants.Deny(ctx, actor(ctx), req.Id, note(req.Body))
	if err != nil {
		return nil, err
	}
	d, err := h.detail(ctx, g)
	return DenyGrant200JSONResponse(d), err
}

// RevokeGrant revokes an active grant or open offer (no step-up).
func (h *Handlers) RevokeGrant(ctx context.Context, req RevokeGrantRequestObject) (RevokeGrantResponseObject, error) {
	g, err := h.Grants.Revoke(ctx, actor(ctx), req.Id, note(req.Body))
	if err != nil {
		return nil, err
	}
	d, err := h.detail(ctx, g)
	return RevokeGrant200JSONResponse(d), err
}

func (h *Handlers) extensionDetail(ctx context.Context, e *store.ExtensionRequest, g *store.Grant) (ExtensionDetail, error) {
	d, err := h.detail(ctx, g)
	if err != nil {
		return ExtensionDetail{}, err
	}
	out := ExtensionDetail{Extension: extensionView(e), Grant: d}
	pending := e.Status == store.ExtPending && g.Status == store.StatusActive
	out.Actions.Approve, out.Actions.Deny = pending, pending
	if g.ValidBefore != 0 {
		out.CurrentEnd = tp(g.ValidBefore)
		if pending {
			out.WouldEnd = tp(g.ValidBefore + e.RequestedDurationS)
		}
	}
	return out, nil
}

// GetExtension returns an extension request with its grant.
func (h *Handlers) GetExtension(ctx context.Context, req GetExtensionRequestObject) (GetExtensionResponseObject, error) {
	e, err := h.DB.GetExtension(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, apierr.NotFound("extension_not_found", "extension request not found")
	}
	if err != nil {
		return nil, err
	}
	g, err := h.settled(ctx, e.GrantID)
	if err != nil {
		return nil, err
	}
	if e, err = h.DB.GetExtension(ctx, req.Id); err != nil {
		return nil, err
	}
	d, err := h.extensionDetail(ctx, e, g)
	return GetExtension200JSONResponse(d), err
}

// DenyExtension denies an extension request.
func (h *Handlers) DenyExtension(ctx context.Context, req DenyExtensionRequestObject) (DenyExtensionResponseObject, error) {
	e, err := h.Grants.DenyExtension(ctx, actor(ctx), req.Id, note(req.Body))
	if err != nil {
		return nil, err
	}
	g, err := h.settled(ctx, e.GrantID)
	if err != nil {
		return nil, err
	}
	d, err := h.extensionDetail(ctx, e, g)
	return DenyExtension200JSONResponse(d), err
}

// ---------------------------------------------------------------- hosts

func (h *Handlers) hostDetail(host *store.Host, scanErr string) HostDetail {
	b := h.Hosts.BootstrapLinks(host)
	return HostDetail{
		Host: hostView(host), ScanError: sp(scanErr),
		Bootstrap: Bootstrap{Install: b.Install, InstallNoSudo: b.InstallNoSudo, Uninstall: b.Uninstall, ExpiresAt: b.ExpiresAt.UTC()},
	}
}

// ListHosts lists hosts.
func (h *Handlers) ListHosts(ctx context.Context, _ ListHostsRequestObject) (ListHostsResponseObject, error) {
	list, err := h.DB.ListHosts(ctx)
	if err != nil {
		return nil, err
	}
	out := ListHosts200JSONResponse{Hosts: []Host{}}
	for _, x := range list {
		out.Hosts = append(out.Hosts, hostView(x))
	}
	return out, nil
}

func boolv(p *bool) bool { return p != nil && *p }

// CreateHost adds a host and scans its key.
func (h *Handlers) CreateHost(ctx context.Context, req CreateHostRequestObject) (CreateHostResponseObject, error) {
	b := req.Body
	host, scanErr, err := h.Hosts.Create(ctx, actor(ctx), hosts.Input{
		Name: b.Name, Address: b.Address, MaxLevel: string(b.MaxLevel), Hypervisor: boolv(b.Hypervisor), LorInfra: boolv(b.LorInfra), Notes: str(b.Notes),
	})
	if err != nil {
		return nil, err
	}
	return CreateHost200JSONResponse(h.hostDetail(host, scanErr)), nil
}

func (h *Handlers) hostByName(ctx context.Context, name string) (*store.Host, error) {
	host, err := h.DB.GetHostByName(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		return nil, apierr.NotFound("host_not_found", "host not found")
	}
	return host, err
}

// GetHost returns a host with fresh bootstrap one-liners.
func (h *Handlers) GetHost(ctx context.Context, req GetHostRequestObject) (GetHostResponseObject, error) {
	host, err := h.hostByName(ctx, req.Name)
	if err != nil {
		return nil, err
	}
	return GetHost200JSONResponse(h.hostDetail(host, "")), nil
}

// UpdateHost edits a host.
func (h *Handlers) UpdateHost(ctx context.Context, req UpdateHostRequestObject) (UpdateHostResponseObject, error) {
	b := req.Body
	u := hosts.Update{Name: b.Name, Address: b.Address, Hypervisor: b.Hypervisor, LorInfra: b.LorInfra, Notes: b.Notes, Disabled: b.Disabled}
	if b.MaxLevel != nil {
		l := string(*b.MaxLevel)
		u.MaxLevel = &l
	}
	host, err := h.Hosts.Edit(ctx, actor(ctx), req.Name, u)
	if err != nil {
		return nil, err
	}
	// End open grants and offers the new settings no longer allow, so
	// re-enabling or raising the level later restores nothing by itself.
	if host.Disabled || host.MaxLevel != "sudo" {
		why := "host restricted to standard user"
		if host.Disabled {
			why = "host disabled"
		}
		if _, err := h.Grants.EndForHost(ctx, actor(ctx), host, why); err != nil {
			return nil, err
		}
	}
	return UpdateHost200JSONResponse(h.hostDetail(host, "")), nil
}

// ScanHost rescans a host key.
func (h *Handlers) ScanHost(ctx context.Context, req ScanHostRequestObject) (ScanHostResponseObject, error) {
	res, err := h.Hosts.Rescan(ctx, actor(ctx), req.Name)
	if err != nil {
		return nil, err
	}
	host, err := h.hostByName(ctx, req.Name)
	if err != nil {
		return nil, err
	}
	v := hostView(host)
	return ScanHost200JSONResponse{HostKey: res.Key, Fingerprint: res.Fingerprint, Changed: res.Changed, PreviousFingerprint: sp(res.PreviousFP), Host: &v}, nil
}

// ---------------------------------------------------------------- agents

func (h *Handlers) agentURL() string {
	host, port, _ := net.SplitHostPort(h.Config.AgentListen)
	return "https://" + net.JoinHostPort(host, port)
}

func (h *Handlers) enrollment(a *store.Agent, code string, exp time.Time) Enrollment {
	return Enrollment{Agent: agentView(a), Command: auth.EnrollCommand(h.agentURL(), h.TLSFingerprint, code), ExpiresAt: exp.UTC()}
}

// ListAgents lists agents.
func (h *Handlers) ListAgents(ctx context.Context, _ ListAgentsRequestObject) (ListAgentsResponseObject, error) {
	list, err := h.DB.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	out := ListAgents200JSONResponse{Agents: []Agent{}}
	for _, a := range list {
		out.Agents = append(out.Agents, agentView(a))
	}
	return out, nil
}

// CreateAgent adds an agent and returns its one-time enroll command.
func (h *Handlers) CreateAgent(ctx context.Context, req CreateAgentRequestObject) (CreateAgentResponseObject, error) {
	a, code, exp, err := h.Agents.Create(ctx, actor(ctx), req.Body.Name, req.Body.LanAddrs)
	if err != nil {
		return nil, err
	}
	return CreateAgent200JSONResponse(h.enrollment(a, code, exp)), nil
}

// RotateAgentToken revokes the token and issues a new enroll command.
func (h *Handlers) RotateAgentToken(ctx context.Context, req RotateAgentTokenRequestObject) (RotateAgentTokenResponseObject, error) {
	a, code, exp, err := h.Agents.Rotate(ctx, actor(ctx), req.Id)
	if err != nil {
		return nil, err
	}
	return RotateAgentToken200JSONResponse(h.enrollment(a, code, exp)), nil
}

// SetAgentStatus enables or disables an agent.
func (h *Handlers) SetAgentStatus(ctx context.Context, req SetAgentStatusRequestObject) (SetAgentStatusResponseObject, error) {
	a, err := h.Agents.SetStatus(ctx, actor(ctx), req.Id, string(req.Body.Status))
	if err != nil {
		return nil, err
	}
	if a.Status == "disabled" {
		if _, err := h.Grants.EndForAgent(ctx, actor(ctx), a.ID, "agent disabled"); err != nil {
			return nil, err
		}
	}
	return SetAgentStatus200JSONResponse(agentView(a)), nil
}

// ---------------------------------------------------------------- audit

// ListAudit lists audit events.
func (h *Handlers) ListAudit(ctx context.Context, req ListAuditRequestObject) (ListAuditResponseObject, error) {
	p := req.Params
	f := store.AuditFilter{Actor: str(p.Actor), Action: str(p.Action), GrantID: i64(p.GrantId), HostID: i64(p.HostId),
		AgentID: i64(p.AgentId), BeforeID: i64(p.Before), Limit: 100}
	if p.Limit != nil {
		f.Limit = *p.Limit
	}
	events, err := h.DB.ListAudit(ctx, f)
	if err != nil {
		return nil, err
	}
	out := ListAudit200JSONResponse{Events: []AuditEvent{}}
	for _, e := range events {
		out.Events = append(out.Events, auditView(e))
	}
	return out, nil
}

// ---------------------------------------------------------------- passkeys and sessions

func passkeyView(p *store.Passkey) Passkey {
	return Passkey{Id: p.ID, Name: p.Name, CreatedAt: tv(p.CreatedAt), LastUsedAt: tp(p.LastUsedAt)}
}

// ListPasskeys lists passkeys.
func (h *Handlers) ListPasskeys(ctx context.Context, _ ListPasskeysRequestObject) (ListPasskeysResponseObject, error) {
	list, err := h.DB.ListPasskeys(ctx)
	if err != nil {
		return nil, err
	}
	out := ListPasskeys200JSONResponse{Passkeys: []Passkey{}}
	for _, p := range list {
		out.Passkeys = append(out.Passkeys, passkeyView(p))
	}
	return out, nil
}

// PasskeyRegisterFinish completes adding a passkey.
func (h *Handlers) PasskeyRegisterFinish(ctx context.Context, req PasskeyRegisterFinishRequestObject) (PasskeyRegisterFinishResponseObject, error) {
	p, err := h.Admin.RegisterFinish(ctx, reqctx.Session(ctx), req.Body.CeremonyId, req.Body.Credential)
	if err != nil {
		return nil, err
	}
	return PasskeyRegisterFinish200JSONResponse(passkeyView(p)), nil
}

// RenamePasskey renames a passkey.
func (h *Handlers) RenamePasskey(ctx context.Context, req RenamePasskeyRequestObject) (RenamePasskeyResponseObject, error) {
	p, err := h.Admin.RenamePasskey(ctx, actor(ctx), req.Id, req.Body.Name)
	if err != nil {
		return nil, err
	}
	return RenamePasskey200JSONResponse(passkeyView(p)), nil
}

// ListSessions lists sessions.
func (h *Handlers) ListSessions(ctx context.Context, _ ListSessionsRequestObject) (ListSessionsResponseObject, error) {
	list, err := h.DB.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	names := map[int64]string{}
	pks, err := h.DB.ListPasskeys(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range pks {
		names[p.ID] = p.Name
	}
	cur := reqctx.Session(ctx).ID
	out := ListSessions200JSONResponse{Sessions: []Session{}}
	for _, s := range list {
		out.Sessions = append(out.Sessions, Session{
			Id: s.ID, PasskeyName: sp(names[s.PasskeyID]), CreatedAt: tv(s.CreatedAt), LastSeenAt: tv(s.LastSeenAt),
			ExpiresAt: tv(s.ExpiresAt), UserAgent: s.UserAgent, ForwardedFor: sp(s.ForwardedFor), Current: s.ID == cur,
		})
	}
	return out, nil
}

// RevokeAllSessions signs out everywhere, including this browser.
func (h *Handlers) RevokeAllSessions(ctx context.Context, _ RevokeAllSessionsRequestObject) (RevokeAllSessionsResponseObject, error) {
	if err := h.Admin.RevokeAllSessions(ctx, reqctx.Session(ctx)); err != nil {
		return nil, err
	}
	cookie := auth.ClearSessionCookie()
	return RevokeAllSessions200JSONResponse{Body: Ok{Ok: true}, Headers: RevokeAllSessions200ResponseHeaders{SetCookie: &cookie}}, nil
}

// ErrorHandler renders errors as {"error": {code, message}}.
func ErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	apierr.WriteFor(w, r, err)
}

// RequestErrorHandler renders request decoding errors.
func RequestErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var ae *apierr.Error
	if errors.As(err, &ae) {
		apierr.Write(w, ae)
		return
	}
	var se *json.SyntaxError
	if errors.As(err, &se) {
		apierr.Write(w, apierr.BadRequest("invalid_json", "request body isn't valid JSON"))
		return
	}
	apierr.Write(w, apierr.BadRequest("invalid_request", "%v", err))
}
