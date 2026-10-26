package adminapi

import (
	"context"
	"time"

	"github.com/joshualent/lor/internal/grants"
	"github.com/joshualent/lor/internal/hosts"
	"github.com/joshualent/lor/internal/store"
)

func tp(unix int64) *time.Time {
	if unix == 0 {
		return nil
	}
	t := time.Unix(unix, 0).UTC()
	return &t
}

func tv(unix int64) time.Time { return time.Unix(unix, 0).UTC() }

func sp(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ip64(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

func levels(ls []string) []Level {
	out := make([]Level, 0, len(ls))
	for _, l := range ls {
		out = append(out, Level(l))
	}
	return out
}

func warnings(h *store.Host) []Warning {
	out := []Warning{}
	for _, w := range grants.HostWarnings(h) {
		out = append(out, Warning{Code: w.Code, Severity: WarningSeverity(w.Severity), Levels: levels(w.Levels), Title: w.Title, Message: w.Message})
	}
	return out
}

func agentRef(a *store.Agent) AgentRef {
	return AgentRef{Id: a.ID, Name: a.Name, LanAddrs: nonNil(a.LANAddrs)}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func hostRef(h *store.Host) HostRef {
	return HostRef{Id: h.ID, Name: h.Name, Address: h.Address, MaxLevel: Level(h.MaxLevel), Hypervisor: h.Hypervisor, LorInfra: h.LorInfra}
}

func hostView(h *store.Host) Host {
	return Host{
		Id: h.ID, Name: h.Name, Address: h.Address, HostKey: sp(h.HostKey), HostKeyFingerprint: sp(hosts.Fingerprint(h.HostKey)),
		HostKeySeenAt: tp(h.HostKeySeenAt), MaxLevel: Level(h.MaxLevel), Hypervisor: h.Hypervisor, LorInfra: h.LorInfra,
		Notes: h.Notes, Disabled: h.Disabled, CreatedAt: tv(h.CreatedAt), Warnings: warnings(h),
	}
}

func agentView(a *store.Agent) Agent {
	out := Agent{
		Id: a.ID, Name: a.Name, Status: AgentStatus(a.Status), LanAddrs: nonNil(a.LANAddrs),
		Enrolled: len(a.TokenHash) > 0, TokenCreatedAt: tp(a.TokenCreatedAt), CreatedAt: tv(a.CreatedAt), LastSeenAt: tp(a.LastSeenAt),
	}
	if len(a.EnrollCodeHash) > 0 {
		out.EnrollCodeExpiresAt = tp(a.EnrollCodeExpiresAt)
	}
	return out
}

func extensionView(e *store.ExtensionRequest) Extension {
	return Extension{
		Id: e.ID, GrantId: e.GrantID, RequestedDurationS: e.RequestedDurationS, ApprovedDurationS: ip64(e.ApprovedDurationS),
		Status: ExtensionStatus(e.Status), Reason: e.Reason, RequestedAt: tv(e.RequestedAt), DecidedAt: tp(e.DecidedAt),
		DecidedBy: sp(e.DecidedBy), DecisionNote: sp(e.DecisionNote),
	}
}

func commandView(c *store.CommandLogEntry) CommandEntry {
	return CommandEntry{Id: c.ID, Argv: nonNil(c.Argv), ReportedAt: tv(c.ReportedAt), CertificateId: ip64(c.CertificateID)}
}

func auditView(e *store.AuditEvent) AuditEvent {
	d := e.Details
	if d == nil {
		d = map[string]any{}
	}
	return AuditEvent{Id: e.ID, Ts: tv(e.TS), Actor: e.Actor, Action: e.Action,
		GrantId: ip64(e.GrantID), HostId: ip64(e.HostID), AgentId: ip64(e.AgentID), Details: d}
}

// lookup caches agents and hosts while rendering a list.
type lookup struct {
	db     *store.DB
	agents map[int64]*store.Agent
	hosts  map[int64]*store.Host
}

func newLookup(db *store.DB) *lookup {
	return &lookup{db: db, agents: map[int64]*store.Agent{}, hosts: map[int64]*store.Host{}}
}

func (l *lookup) agent(ctx context.Context, id int64) (*store.Agent, error) {
	if a, ok := l.agents[id]; ok {
		return a, nil
	}
	a, err := l.db.GetAgent(ctx, id)
	if err != nil {
		return nil, err
	}
	l.agents[id] = a
	return a, nil
}

func (l *lookup) host(ctx context.Context, id int64) (*store.Host, error) {
	if h, ok := l.hosts[id]; ok {
		return h, nil
	}
	h, err := l.db.GetHost(ctx, id)
	if err != nil {
		return nil, err
	}
	l.hosts[id] = h
	return h, nil
}

func (h *Handlers) summary(ctx context.Context, l *lookup, g *store.Grant) (GrantSummary, error) {
	a, err := l.agent(ctx, g.AgentID)
	if err != nil {
		return GrantSummary{}, err
	}
	host, err := l.host(ctx, g.HostID)
	if err != nil {
		return GrantSummary{}, err
	}
	out := GrantSummary{
		Id: g.ID, Agent: agentRef(a), Host: hostRef(host), Origin: GrantSummaryOrigin(g.Origin),
		Level: Level(g.Level), RequestedLevel: Level(g.RequestedLevel), RequestedDurationS: ip64(g.RequestedDurationS),
		ApprovedDurationS: ip64(g.ApprovedDurationS), Status: GrantStatus(g.Status), Reason: g.Reason,
		DecisionNote: sp(g.DecisionNote), DecidedBy: sp(g.DecidedBy), RequestedAt: tv(g.RequestedAt), DecidedAt: tp(g.DecidedAt),
		ValidAfter: tp(g.ValidAfter), ValidBefore: tp(g.ValidBefore), EndedAt: tp(g.EndedAt),
	}
	if g.Status == store.StatusPending {
		d := h.Grants.ApprovalDeadline(g).UTC()
		out.ApprovalDeadline = &d
	}
	cmds, err := h.DB.ListCommands(ctx, g.ID, 1)
	if err != nil {
		return GrantSummary{}, err
	}
	if len(cmds) > 0 {
		c := commandView(cmds[0])
		out.LastCommand = &c
	}
	if g.Status == store.StatusActive {
		ext, err := h.Grants.PendingExtension(ctx, g.ID)
		if err != nil {
			return GrantSummary{}, err
		}
		if ext != nil {
			e := extensionView(ext)
			out.PendingExtension = &e
		}
	}
	return out, nil
}

func (h *Handlers) detail(ctx context.Context, g *store.Grant) (GrantDetail, error) {
	l := newLookup(h.DB)
	s, err := h.summary(ctx, l, g)
	if err != nil {
		return GrantDetail{}, err
	}
	host, _ := l.host(ctx, g.HostID)
	agent, _ := l.agent(ctx, g.AgentID)
	out := GrantDetail{
		Id: s.Id, Agent: s.Agent, Host: s.Host, Origin: GrantDetailOrigin(s.Origin), Level: s.Level, RequestedLevel: s.RequestedLevel,
		RequestedDurationS: s.RequestedDurationS, ApprovedDurationS: s.ApprovedDurationS, Status: s.Status, Reason: s.Reason,
		DecisionNote: s.DecisionNote, DecidedBy: s.DecidedBy, RequestedAt: s.RequestedAt, DecidedAt: s.DecidedAt,
		ValidAfter: s.ValidAfter, ValidBefore: s.ValidBefore, EndedAt: s.EndedAt, ApprovalDeadline: s.ApprovalDeadline,
		LastCommand: s.LastCommand, PendingExtension: s.PendingExtension, PublicKeyFp: g.PublicKeyFP,
		Warnings: warnings(host),
		Actions: Actions{
			Authorize: g.Status == store.StatusPending,
			Deny:      g.Status == store.StatusPending,
			Extend:    g.Status == store.StatusActive || g.Status == store.StatusOffered,
			Revoke:    g.Status == store.StatusActive || g.Status == store.StatusOffered,
		},
		Certificates: []CertificateInfo{}, Commands: []CommandEntry{}, Extensions: []Extension{}, Timeline: []AuditEvent{},
	}
	if c := h.Grants.Consent(g, host); c != nil {
		out.Consent = &Consent{
			Levels: levels(c.Levels), DefaultLevel: Level(c.DefaultLevel), DefaultDurationS: int64(c.DefaultDuration / time.Second),
			ApprovalDeadline: c.Deadline.UTC(), Limits: grants.Limits(agent),
		}
	}
	certs, err := h.DB.ListCertificates(ctx, g.ID)
	if err != nil {
		return GrantDetail{}, err
	}
	for _, c := range certs {
		out.Certificates = append(out.Certificates, CertificateInfo{Id: c.ID, ValidAfter: tv(c.ValidAfter), ValidBefore: tv(c.ValidBefore), IssuedAt: tv(c.IssuedAt), IssuedBy: c.IssuedBy})
	}
	cmds, err := h.DB.ListCommands(ctx, g.ID, 500)
	if err != nil {
		return GrantDetail{}, err
	}
	for _, c := range cmds {
		out.Commands = append(out.Commands, commandView(c))
	}
	exts, err := h.DB.ListExtensions(ctx, g.ID, "")
	if err != nil {
		return GrantDetail{}, err
	}
	for _, e := range exts {
		out.Extensions = append(out.Extensions, extensionView(e))
	}
	events, err := h.DB.ListAudit(ctx, store.AuditFilter{GrantID: g.ID, Limit: 200})
	if err != nil {
		return GrantDetail{}, err
	}
	for i := len(events) - 1; i >= 0; i-- { // oldest first
		out.Timeline = append(out.Timeline, auditView(events[i]))
	}
	return out, nil
}
