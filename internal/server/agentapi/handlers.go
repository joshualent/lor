package agentapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/joshualent/lor/internal/apierr"
	"github.com/joshualent/lor/internal/auth"
	"github.com/joshualent/lor/internal/grants"
	"github.com/joshualent/lor/internal/hosts"
	"github.com/joshualent/lor/internal/server/reqctx"
	"github.com/joshualent/lor/internal/store"
)

// Handlers implements the agent API (plan section 9).
type Handlers struct {
	DB     *store.DB
	Grants *grants.Service
	Agents *auth.Agents
}

var _ StrictServerInterface = (*Handlers)(nil)

// PublicOperations need no bearer token.
var PublicOperations = map[string]bool{"Enroll": true}

// AuthMiddleware authenticates every operation except Enroll: bearer token
// plus source IP.
func (h *Handlers) AuthMiddleware(f StrictHandlerFunc, operationID string) StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, req any) (any, error) {
		ctx = reqctx.WithRequest(ctx, r)
		if !PublicOperations[operationID] {
			agent, err := h.Agents.Authenticate(ctx, r.Header.Get("Authorization"), reqctx.RemoteIP(r))
			if err != nil {
				return nil, err
			}
			ctx = reqctx.WithAgent(ctx, agent)
		}
		return f(ctx, w, r, req)
	}
}

func tp(unix int64) *time.Time {
	if unix == 0 {
		return nil
	}
	t := time.Unix(unix, 0).UTC()
	return &t
}

func sp(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (h *Handlers) extension(e *store.ExtensionRequest) ExtensionRequest {
	out := ExtensionRequest{
		Id: e.ID, GrantId: e.GrantID, RequestedDurationS: e.RequestedDurationS, Status: ExtensionStatus(e.Status),
		Reason: e.Reason, RequestedAt: time.Unix(e.RequestedAt, 0).UTC(), DecidedAt: tp(e.DecidedAt), DecisionNote: sp(e.DecisionNote),
	}
	if e.ApprovedDurationS != 0 {
		v := e.ApprovedDurationS
		out.ApprovedDurationS = &v
	}
	return out
}

// grant renders the agent's view of a grant. The certificate and ssh block
// are included only while the grant is active.
func (h *Handlers) grant(ctx context.Context, g *store.Grant) (Grant, error) {
	host, err := h.DB.GetHost(ctx, g.HostID)
	if err != nil {
		return Grant{}, err
	}
	out := Grant{
		Id: g.ID, Host: host.Name, Level: Level(g.Level), RequestedLevel: Level(g.RequestedLevel),
		Status: GrantStatus(g.Status), Origin: GrantOrigin(g.Origin), Reason: sp(g.Reason), DecisionNote: sp(g.DecisionNote),
		RequestedAt: time.Unix(g.RequestedAt, 0).UTC(), DecidedAt: tp(g.DecidedAt),
		ValidAfter: tp(g.ValidAfter), ValidBefore: tp(g.ValidBefore), PublicKeyFp: sp(g.PublicKeyFP),
	}
	switch g.Status {
	case store.StatusPending:
		u := h.Grants.ApprovalURL(g.ID)
		out.ApprovalUrl = &u
		d := h.Grants.ApprovalDeadline(g).UTC()
		out.ApprovalDeadline = &d
	case store.StatusActive:
		cert, err := h.DB.LatestCertificate(ctx, g.ID)
		if err != nil {
			return Grant{}, err
		}
		out.Certificate = &cert.Certificate
		out.CertificateSerial = &cert.ID
		ip, port := hosts.SplitAddress(host.Address)
		alias := grants.HostKeyAlias(host.Name)
		out.Ssh = &SSHInfo{
			Hostname: ip, Port: port, User: SSHInfoUser(grants.SSHUser(g.Level)),
			KnownHostsLine: alias + " " + host.HostKey, HostKeyAlias: alias,
		}
		ext, err := h.Grants.PendingExtension(ctx, g.ID)
		if err != nil {
			return Grant{}, err
		}
		if ext != nil {
			e := h.extension(ext)
			out.PendingExtension = &e
		}
	}
	return out, nil
}

// Enroll exchanges a one-time code for a token.
func (h *Handlers) Enroll(ctx context.Context, req EnrollRequestObject) (EnrollResponseObject, error) {
	r := reqctx.Request(ctx)
	token, agent, err := h.Agents.Enroll(ctx, req.Body.Code, reqctx.RemoteIP(r))
	if err != nil {
		return nil, err
	}
	return Enroll200JSONResponse{Token: token, Agent: AgentInfo{Name: agent.Name, LanAddrs: agent.LANAddrs}}, nil
}

// Whoami returns the agent's identity.
func (h *Handlers) Whoami(ctx context.Context, _ WhoamiRequestObject) (WhoamiResponseObject, error) {
	a := reqctx.Agent(ctx)
	return Whoami200JSONResponse{Name: a.Name, LanAddrs: a.LANAddrs}, nil
}

// ListHosts lists requestable hosts.
func (h *Handlers) ListHosts(ctx context.Context, _ ListHostsRequestObject) (ListHostsResponseObject, error) {
	list, err := h.DB.ListHosts(ctx)
	if err != nil {
		return nil, err
	}
	cfg := h.Grants.Config()
	out := ListHosts200JSONResponse{AgentRequestMaxS: int64(cfg.AgentRequestMax / time.Second), DefaultDurationS: int64(cfg.DefaultDuration / time.Second)}
	out.Hosts = []struct {
		MaxLevel Level  `json:"max_level"`
		Name     string `json:"name"`
	}{}
	for _, x := range list {
		if x.Disabled || x.HostKey == "" {
			continue
		}
		out.Hosts = append(out.Hosts, struct {
			MaxLevel Level  `json:"max_level"`
			Name     string `json:"name"`
		}{Level(x.MaxLevel), x.Name})
	}
	return out, nil
}

// CreateGrant requests access.
func (h *Handlers) CreateGrant(ctx context.Context, req CreateGrantRequestObject) (CreateGrantResponseObject, error) {
	if req.Body == nil {
		return nil, apierr.BadRequest("invalid_request", "missing body")
	}
	in := grants.RequestInput{Host: req.Body.Host, Level: string(req.Body.Level), Reason: req.Body.Reason, PublicKey: req.Body.PublicKey}
	if req.Body.DurationS != nil {
		if *req.Body.DurationS <= 0 {
			return nil, apierr.BadRequest("invalid_duration", "duration must be positive")
		}
		in.Duration = time.Duration(*req.Body.DurationS) * time.Second
	}
	res, err := h.Grants.Request(ctx, reqctx.Agent(ctx), in)
	if err != nil {
		return nil, err
	}
	g, err := h.grant(ctx, res.Grant)
	if err != nil {
		return nil, err
	}
	out := CreateGrant200JSONResponse{Grant: g, ApprovalUrl: g.ApprovalUrl}
	if res.Reused {
		out.Reused = &res.Reused
	}
	if res.Claimed {
		out.Claimed = &res.Claimed
	}
	return out, nil
}

// GetGrant returns a grant, long-polling while it's pending.
func (h *Handlers) GetGrant(ctx context.Context, req GetGrantRequestObject) (GetGrantResponseObject, error) {
	agent := reqctx.Agent(ctx)
	var g *store.Grant
	var err error
	if req.Params.Wait != nil && *req.Params.Wait > 0 {
		wait := *req.Params.Wait
		if wait > 60 {
			wait = 60
		}
		g, err = h.Grants.WaitAgentGrant(ctx, agent, req.Id, time.Duration(wait)*time.Second)
	} else {
		g, err = h.Grants.AgentGrant(ctx, agent, req.Id)
	}
	if err != nil {
		return nil, err
	}
	out, err := h.grant(ctx, g)
	return GetGrant200JSONResponse(out), err
}

// ListGrants lists this agent's grants.
func (h *Handlers) ListGrants(ctx context.Context, req ListGrantsRequestObject) (ListGrantsResponseObject, error) {
	host := ""
	if req.Params.Host != nil {
		host = *req.Params.Host
	}
	var statuses []string
	if req.Params.Status != nil {
		for _, s := range *req.Params.Status {
			statuses = append(statuses, string(s))
		}
	}
	list, err := h.Grants.ListAgentGrants(ctx, reqctx.Agent(ctx), host, statuses)
	if err != nil {
		return nil, err
	}
	out := ListGrants200JSONResponse{Grants: []Grant{}}
	for _, g := range list {
		x, err := h.grant(ctx, g)
		if err != nil {
			return nil, err
		}
		out.Grants = append(out.Grants, x)
	}
	return out, nil
}

// ClaimGrant claims an offer.
func (h *Handlers) ClaimGrant(ctx context.Context, req ClaimGrantRequestObject) (ClaimGrantResponseObject, error) {
	if req.Body == nil {
		return nil, apierr.BadRequest("invalid_request", "missing body")
	}
	g, err := h.Grants.Claim(ctx, reqctx.Agent(ctx), req.Id, req.Body.PublicKey)
	if err != nil {
		return nil, err
	}
	out, err := h.grant(ctx, g)
	return ClaimGrant200JSONResponse(out), err
}

// ConnectGrant records the command and returns the current certificate.
func (h *Handlers) ConnectGrant(ctx context.Context, req ConnectGrantRequestObject) (ConnectGrantResponseObject, error) {
	if req.Body == nil {
		return nil, apierr.BadRequest("invalid_request", "missing body")
	}
	g, _, err := h.Grants.Connect(ctx, reqctx.Agent(ctx), req.Id, req.Body.Argv)
	if err != nil {
		return nil, err
	}
	out, err := h.grant(ctx, g)
	return ConnectGrant200JSONResponse(out), err
}

// ExtendGrant asks for more time.
func (h *Handlers) ExtendGrant(ctx context.Context, req ExtendGrantRequestObject) (ExtendGrantResponseObject, error) {
	if req.Body == nil {
		return nil, apierr.BadRequest("invalid_request", "missing body")
	}
	if req.Body.DurationS <= 0 {
		return nil, apierr.BadRequest("invalid_duration", "duration must be positive")
	}
	ext, reused, err := h.Grants.RequestExtension(ctx, reqctx.Agent(ctx), req.Id, time.Duration(req.Body.DurationS)*time.Second, req.Body.Reason)
	if err != nil {
		return nil, err
	}
	out := ExtendGrant200JSONResponse{ExtensionRequest: h.extension(ext), ApprovalUrl: h.Grants.ExtensionURL(ext.ID)}
	if reused {
		out.Reused = &reused
	}
	return out, nil
}

// ReleaseGrant gives up a grant.
func (h *Handlers) ReleaseGrant(ctx context.Context, req ReleaseGrantRequestObject) (ReleaseGrantResponseObject, error) {
	g, err := h.Grants.Release(ctx, reqctx.Agent(ctx), req.Id)
	if err != nil {
		return nil, err
	}
	out, err := h.grant(ctx, g)
	return ReleaseGrant200JSONResponse(out), err
}

// ErrorHandler renders errors as {"error": {code, message}}.
func ErrorHandler(w http.ResponseWriter, _ *http.Request, err error) {
	apierr.Write(w, err)
}

// RequestErrorHandler renders request decoding errors.
func RequestErrorHandler(w http.ResponseWriter, _ *http.Request, err error) {
	var ae *apierr.Error
	if errors.As(err, &ae) {
		apierr.Write(w, ae)
		return
	}
	apierr.Write(w, apierr.BadRequest("invalid_request", "%v", err))
}
