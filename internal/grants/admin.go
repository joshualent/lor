package grants

import (
	"context"
	"fmt"
	"time"

	"github.com/joshualent/lor/internal/apierr"
	"github.com/joshualent/lor/internal/store"
)

// AuthorizeInput is the admin's decision on a pending request. It is the
// exact action stored by step-up.
type AuthorizeInput struct {
	GrantID     int64
	Level       string
	Duration    time.Duration
	ConfirmSudo bool
}

// checkAuthorize validates an authorization against the current state.
func (s *Service) checkAuthorize(ctx context.Context, tx *store.Tx, in AuthorizeInput) (*store.Grant, *store.Host, error) {
	g, err := s.loadSettled(ctx, tx, in.GrantID)
	if err != nil {
		return nil, nil, err
	}
	if g.Status != store.StatusPending {
		return nil, nil, apierr.Conflict("not_pending", "request %d is %s; it can no longer be authorized", g.ID, g.Status)
	}
	if !validLevel(in.Level) {
		return nil, nil, apierr.BadRequest("invalid_level", "level must be user or sudo")
	}
	if !LevelAtLeast(g.RequestedLevel, in.Level) {
		return nil, nil, apierr.BadRequest("level_raised", "the level can be lowered but never raised above the requested %s", g.RequestedLevel)
	}
	h, err := s.requestableHost(ctx, tx, g.HostID)
	if err != nil {
		return nil, nil, err
	}
	if !LevelAtLeast(h.MaxLevel, in.Level) {
		return nil, nil, apierr.BadRequest("level_not_allowed", "host %s allows at most %s access", h.Name, h.MaxLevel)
	}
	if in.Level == LevelSudo && !in.ConfirmSudo {
		return nil, nil, apierr.BadRequest("sudo_not_confirmed", "confirm that this gives root on %s", h.Name)
	}
	agent, err := tx.GetAgent(ctx, g.AgentID)
	if err != nil {
		return nil, nil, err
	}
	if err := activeAgent(agent); err != nil {
		return nil, nil, err
	}
	if err := s.validateDuration(in.Duration, s.cfg.AbsoluteMax, "invalid_duration"); err != nil {
		return nil, nil, err
	}
	return g, h, nil
}

// CheckAuthorize validates without changing anything (step-up begin).
func (s *Service) CheckAuthorize(ctx context.Context, in AuthorizeInput) (string, error) {
	var summary string
	err := s.withSettled(ctx, in.GrantID, func(tx *store.Tx) error {
		g, h, err := s.checkAuthorize(ctx, tx, in)
		if err != nil {
			return err
		}
		summary = fmt.Sprintf("Authorize %s on %s for %s (request #%d)", levelWord(in.Level), h.Name, formatDuration(in.Duration), g.ID)
		return nil
	})
	return summary, err
}

// Authorize approves a pending request, possibly lowering the level or
// changing the duration, and signs the certificate (valid_before = now + d).
func (s *Service) Authorize(ctx context.Context, actor string, in AuthorizeInput) (*store.Grant, error) {
	var g *store.Grant
	err := s.withSettled(ctx, in.GrantID, func(tx *store.Tx) error {
		var err error
		g, _, err = s.checkAuthorize(ctx, tx, in)
		if err != nil {
			return err
		}
		now := s.now()
		g.Level = in.Level
		g.ApprovedDurationS = int64(in.Duration / time.Second)
		g.Status = store.StatusActive
		g.DecidedBy, g.DecidedAt = actor, now.Unix()
		g.ValidAfter, g.ValidBefore = now.Unix(), now.Add(in.Duration).Unix()
		if err := tx.UpdateGrant(ctx, g, store.StatusPending); err != nil {
			return conflictErr(err)
		}
		if _, err := s.issue(ctx, tx, g, g.ValidBefore, actor); err != nil {
			return err
		}
		return s.audit(ctx, tx, actor, "grant.authorized", g, map[string]any{
			"level": g.Level, "requested_level": g.RequestedLevel,
			"duration_s": g.ApprovedDurationS, "requested_duration_s": g.RequestedDurationS,
			"changed": g.Level != g.RequestedLevel || g.ApprovedDurationS != g.RequestedDurationS,
		})
	})
	if err != nil {
		return nil, err
	}
	s.notify.Notify(g.ID)
	return g, nil
}

// Deny rejects a pending request. No step-up needed: denying never creates
// access.
func (s *Service) Deny(ctx context.Context, actor string, grantID int64, note string) (*store.Grant, error) {
	note, err := validateNote(note)
	if err != nil {
		return nil, err
	}
	var g *store.Grant
	err = s.withSettled(ctx, grantID, func(tx *store.Tx) error {
		var err error
		g, err = s.loadSettled(ctx, tx, grantID)
		if err != nil {
			return err
		}
		if g.Status != store.StatusPending {
			return apierr.Conflict("not_pending", "request %d is %s", g.ID, g.Status)
		}
		now := s.now().Unix()
		g.Status, g.DecidedBy, g.DecidedAt, g.DecisionNote, g.EndedAt = store.StatusDenied, actor, now, note, now
		if err := tx.UpdateGrant(ctx, g, store.StatusPending); err != nil {
			return conflictErr(err)
		}
		return s.audit(ctx, tx, actor, "grant.denied", g, map[string]any{"note": note})
	})
	if err != nil {
		return nil, err
	}
	s.notify.Notify(g.ID)
	return g, nil
}

// Revoke ends an active grant or withdraws an open offer. The certificate
// stays valid until it expires, but `lor ssh` refuses to use it (plan
// section 7).
func (s *Service) Revoke(ctx context.Context, actor string, grantID int64, note string) (*store.Grant, error) {
	note, err := validateNote(note)
	if err != nil {
		return nil, err
	}
	var g *store.Grant
	err = s.withSettled(ctx, grantID, func(tx *store.Tx) error {
		var err error
		g, err = s.loadSettled(ctx, tx, grantID)
		if err != nil {
			return err
		}
		if g.Status != store.StatusActive && g.Status != store.StatusOffered {
			return apierr.Conflict("not_active", "grant %d is %s", g.ID, g.Status)
		}
		from := g.Status
		now := s.now().Unix()
		g.Status, g.EndedAt = store.StatusRevoked, now
		if note != "" {
			g.DecisionNote = note
		}
		if err := tx.UpdateGrant(ctx, g, from); err != nil {
			return conflictErr(err)
		}
		if err := s.audit(ctx, tx, actor, "grant.revoked", g, map[string]any{"note": note, "was": from}); err != nil {
			return err
		}
		return s.settleExtensions(ctx, tx, g)
	})
	if err != nil {
		return nil, err
	}
	s.notify.Notify(g.ID)
	return g, nil
}

// OfferInput is an admin pre-approval (Grant access form).
type OfferInput struct {
	AgentID     int64
	HostID      int64
	Level       string
	Duration    time.Duration
	ConfirmSudo bool
	Note        string
}

func (s *Service) checkOffer(ctx context.Context, tx *store.Tx, in OfferInput) (*store.Agent, *store.Host, string, error) {
	agent, err := tx.GetAgent(ctx, in.AgentID)
	if err != nil {
		return nil, nil, "", mapNotFound(err, "agent")
	}
	if err := activeAgent(agent); err != nil {
		return nil, nil, "", err
	}
	h, err := s.requestableHost(ctx, tx, in.HostID)
	if err != nil {
		return nil, nil, "", err
	}
	if !validLevel(in.Level) {
		return nil, nil, "", apierr.BadRequest("invalid_level", "level must be user or sudo")
	}
	if !LevelAtLeast(h.MaxLevel, in.Level) {
		return nil, nil, "", apierr.BadRequest("level_not_allowed", "host %s allows at most %s access", h.Name, h.MaxLevel)
	}
	if in.Level == LevelSudo && !in.ConfirmSudo {
		return nil, nil, "", apierr.BadRequest("sudo_not_confirmed", "confirm that this gives root on %s", h.Name)
	}
	if err := s.validateDuration(in.Duration, s.cfg.AbsoluteMax, "invalid_duration"); err != nil {
		return nil, nil, "", err
	}
	note, err := validateNote(in.Note)
	if err != nil {
		return nil, nil, "", err
	}
	return agent, h, note, nil
}

// CheckOffer validates an offer without creating it (step-up begin).
func (s *Service) CheckOffer(ctx context.Context, in OfferInput) (string, error) {
	var summary string
	err := s.db.WithTx(ctx, func(tx *store.Tx) error {
		agent, h, _, err := s.checkOffer(ctx, tx, in)
		if err != nil {
			return err
		}
		summary = fmt.Sprintf("Offer %s %s on %s for %s", agent.Name, levelWord(in.Level), h.Name, formatDuration(in.Duration))
		return nil
	})
	return summary, err
}

// CreateOffer creates an offer. The timer starts now: there is no key yet,
// and the agent's first `lor ssh`/`lor request` claims it.
func (s *Service) CreateOffer(ctx context.Context, actor string, in OfferInput) (*store.Grant, string, error) {
	var g *store.Grant
	var message string
	err := s.db.WithTx(ctx, func(tx *store.Tx) error {
		agent, h, note, err := s.checkOffer(ctx, tx, in)
		if err != nil {
			return err
		}
		now := s.now()
		g = &store.Grant{
			AgentID: agent.ID, HostID: h.ID, Origin: originAdminOffer,
			Level: in.Level, RequestedLevel: in.Level,
			RequestedDurationS: int64(in.Duration / time.Second), ApprovedDurationS: int64(in.Duration / time.Second),
			Reason: note, Status: store.StatusOffered, DecidedBy: actor, DecidedAt: now.Unix(),
			RequestedAt: now.Unix(), ValidAfter: now.Unix(), ValidBefore: now.Add(in.Duration).Unix(),
		}
		if err := tx.CreateGrant(ctx, g); err != nil {
			return err
		}
		message = OfferMessage(h.Name, in.Level, time.Unix(g.ValidBefore, 0))
		return s.audit(ctx, tx, actor, "grant.offered", g, map[string]any{
			"level": g.Level, "duration_s": g.ApprovedDurationS, "agent": agent.Name, "note": note,
		})
	})
	if err != nil {
		return nil, "", err
	}
	s.notify.Notify(g.ID)
	return g, message, nil
}

// OfferMessage is the line the admin pastes into chat after creating an
// offer. It uses the server's local time zone.
func OfferMessage(host, level string, end time.Time) string {
	lvl := ""
	if level == LevelSudo {
		lvl = " (sudo)"
	}
	return fmt.Sprintf("You have Lor access to `%s`%s until %s. Use `lor ssh %s -- <cmd>`.", host, lvl, end.Local().Format("15:04"), host)
}

// ExtendInput is an admin-initiated extension.
type ExtendInput struct {
	GrantID  int64
	Duration time.Duration
}

func (s *Service) checkExtend(ctx context.Context, tx *store.Tx, in ExtendInput) (*store.Grant, error) {
	g, err := s.loadSettled(ctx, tx, in.GrantID)
	if err != nil {
		return nil, err
	}
	if g.Status != store.StatusActive && g.Status != store.StatusOffered {
		return nil, apierr.Conflict("not_active", "grant %d is %s; only active grants and open offers can be extended", g.ID, g.Status)
	}
	if err := s.validateDuration(in.Duration, s.cfg.AbsoluteMax, "invalid_duration"); err != nil {
		return nil, err
	}
	if err := s.checkAbsoluteMax(g.ValidBefore + int64(in.Duration/time.Second)); err != nil {
		return nil, err
	}
	if g.Status == store.StatusActive {
		agent, err := tx.GetAgent(ctx, g.AgentID)
		if err != nil {
			return nil, err
		}
		if err := activeAgent(agent); err != nil {
			return nil, err
		}
		if _, err := s.requestableHost(ctx, tx, g.HostID); err != nil {
			return nil, err
		}
	}
	return g, nil
}

// CheckExtend validates an extension without applying it (step-up begin).
func (s *Service) CheckExtend(ctx context.Context, in ExtendInput) (string, error) {
	var summary string
	err := s.withSettled(ctx, in.GrantID, func(tx *store.Tx) error {
		g, err := s.checkExtend(ctx, tx, in)
		if err != nil {
			return err
		}
		h, err := tx.GetHost(ctx, g.HostID)
		if err != nil {
			return err
		}
		newEnd := time.Unix(g.ValidBefore, 0).Add(in.Duration)
		summary = fmt.Sprintf("Extend grant #%d on %s by %s (ends %s)", g.ID, h.Name, formatDuration(in.Duration), newEnd.Local().Format("Jan 2 15:04"))
		return nil
	})
	return summary, err
}

// Extend adds time to an active grant (a new certificate for the same key,
// counted from the current end) or an open offer (its window moves).
func (s *Service) Extend(ctx context.Context, actor string, in ExtendInput) (*store.Grant, error) {
	var g *store.Grant
	err := s.withSettled(ctx, in.GrantID, func(tx *store.Tx) error {
		var err error
		g, err = s.checkExtend(ctx, tx, in)
		if err != nil {
			return err
		}
		return s.applyExtension(ctx, tx, actor, g, in.Duration, 0)
	})
	if err != nil {
		return nil, err
	}
	s.notify.Notify(g.ID)
	return g, nil
}

func (s *Service) applyExtension(ctx context.Context, tx *store.Tx, actor string, g *store.Grant, d time.Duration, extID int64) error {
	from := g.Status
	oldEnd := g.ValidBefore
	g.ValidBefore = oldEnd + int64(d/time.Second)
	if err := tx.UpdateGrant(ctx, g, from); err != nil {
		return conflictErr(err)
	}
	if g.Status == store.StatusActive {
		if _, err := s.issue(ctx, tx, g, g.ValidBefore, actor); err != nil {
			return err
		}
	}
	details := map[string]any{"added_s": int64(d / time.Second), "old_valid_before": oldEnd, "valid_before": g.ValidBefore}
	if extID != 0 {
		details["extension_id"] = extID
	}
	return s.audit(ctx, tx, actor, "grant.extended", g, details)
}

// ApproveExtensionInput is the admin's decision on an extension request.
type ApproveExtensionInput struct {
	ExtensionID int64
	Duration    time.Duration // may differ from the requested duration
}

func (s *Service) checkApproveExtension(ctx context.Context, tx *store.Tx, in ApproveExtensionInput) (*store.ExtensionRequest, *store.Grant, error) {
	e, err := tx.GetExtension(ctx, in.ExtensionID)
	if err != nil {
		return nil, nil, mapNotFound(err, "extension")
	}
	g, err := s.loadSettled(ctx, tx, e.GrantID)
	if err != nil {
		return nil, nil, err
	}
	// Settling may have mooted or timed out the extension.
	if e, err = tx.GetExtension(ctx, in.ExtensionID); err != nil {
		return nil, nil, err
	}
	if e.Status != store.ExtPending {
		return nil, nil, apierr.Conflict("not_pending", "extension request %d is %s", e.ID, e.Status)
	}
	if _, err := s.checkExtend(ctx, tx, ExtendInput{GrantID: g.ID, Duration: in.Duration}); err != nil {
		return nil, nil, err
	}
	if g.Status != store.StatusActive {
		return nil, nil, grantNotActive(g)
	}
	return e, g, nil
}

// withSettledExtension settles the grant an extension belongs to, then runs fn.
func (s *Service) withSettledExtension(ctx context.Context, extID int64, fn func(tx *store.Tx) error) error {
	if e, err := s.db.GetExtension(ctx, extID); err == nil {
		return s.withSettled(ctx, e.GrantID, fn)
	}
	return s.db.WithTx(ctx, fn)
}

// CheckApproveExtension validates without applying (step-up begin).
func (s *Service) CheckApproveExtension(ctx context.Context, in ApproveExtensionInput) (string, error) {
	var summary string
	err := s.withSettledExtension(ctx, in.ExtensionID, func(tx *store.Tx) error {
		e, g, err := s.checkApproveExtension(ctx, tx, in)
		if err != nil {
			return err
		}
		h, err := tx.GetHost(ctx, g.HostID)
		if err != nil {
			return err
		}
		newEnd := time.Unix(g.ValidBefore, 0).Add(in.Duration)
		summary = fmt.Sprintf("Approve extension #%d: %s on %s +%s (ends %s)", e.ID, levelWord(g.Level), h.Name, formatDuration(in.Duration), newEnd.Local().Format("Jan 2 15:04"))
		return nil
	})
	return summary, err
}

// ApproveExtension approves an agent's extension request.
func (s *Service) ApproveExtension(ctx context.Context, actor string, in ApproveExtensionInput) (*store.ExtensionRequest, *store.Grant, error) {
	var e *store.ExtensionRequest
	var g *store.Grant
	err := s.withSettledExtension(ctx, in.ExtensionID, func(tx *store.Tx) error {
		var err error
		e, g, err = s.checkApproveExtension(ctx, tx, in)
		if err != nil {
			return err
		}
		e.Status, e.ApprovedDurationS, e.DecidedAt, e.DecidedBy = store.ExtApproved, int64(in.Duration/time.Second), s.now().Unix(), actor
		if err := tx.UpdateExtension(ctx, e, store.ExtPending); err != nil {
			return conflictErr(err)
		}
		if err := s.audit(ctx, tx, actor, "extension.approved", g, map[string]any{"extension_id": e.ID, "duration_s": e.ApprovedDurationS}); err != nil {
			return err
		}
		return s.applyExtension(ctx, tx, actor, g, in.Duration, e.ID)
	})
	if err != nil {
		return nil, nil, err
	}
	s.notify.Notify(g.ID)
	return e, g, nil
}

// DenyExtension rejects an extension request.
func (s *Service) DenyExtension(ctx context.Context, actor string, extID int64, note string) (*store.ExtensionRequest, error) {
	note, err := validateNote(note)
	if err != nil {
		return nil, err
	}
	var e *store.ExtensionRequest
	err = s.withSettledExtension(ctx, extID, func(tx *store.Tx) error {
		var err error
		e, err = tx.GetExtension(ctx, extID)
		if err != nil {
			return mapNotFound(err, "extension")
		}
		g, err := s.loadSettled(ctx, tx, e.GrantID)
		if err != nil {
			return err
		}
		if e, err = tx.GetExtension(ctx, extID); err != nil {
			return err
		}
		if e.Status != store.ExtPending {
			return apierr.Conflict("not_pending", "extension request %d is %s", e.ID, e.Status)
		}
		e.Status, e.DecidedAt, e.DecidedBy, e.DecisionNote = store.ExtDenied, s.now().Unix(), actor, note
		if err := tx.UpdateExtension(ctx, e, store.ExtPending); err != nil {
			return conflictErr(err)
		}
		return s.audit(ctx, tx, actor, "extension.denied", g, map[string]any{"extension_id": e.ID, "note": note})
	})
	if err != nil {
		return nil, err
	}
	s.notify.Notify(e.GrantID)
	return e, nil
}

func levelWord(level string) string {
	if level == LevelSudo {
		return "sudo"
	}
	return "standard user access"
}

// ---------------------------------------------------------------- consent

// ConsentInfo is the server-computed part of the consent page.
type ConsentInfo struct {
	Levels          []string
	DefaultLevel    string
	DefaultDuration time.Duration
	Deadline        time.Time
}

// Consent returns the consent options for a pending request, or nil.
func (s *Service) Consent(g *store.Grant, h *store.Host) *ConsentInfo {
	if g.Status != store.StatusPending {
		return nil
	}
	max := g.RequestedLevel
	if !LevelAtLeast(h.MaxLevel, max) {
		max = h.MaxLevel
	}
	levels := []string{LevelUser}
	if max == LevelSudo {
		levels = []string{LevelSudo, LevelUser}
	}
	d := time.Duration(g.RequestedDurationS) * time.Second
	if h.LorInfra && d > LorInfraDefault {
		d = LorInfraDefault
	}
	return &ConsentInfo{Levels: levels, DefaultLevel: max, DefaultDuration: d, Deadline: s.ApprovalDeadline(g)}
}
