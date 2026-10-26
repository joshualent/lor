package grants

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/joshualent/lor/internal/apierr"
	"github.com/joshualent/lor/internal/ca"
	"github.com/joshualent/lor/internal/store"
)

// RequestInput is an agent's access request (`lor request`).
type RequestInput struct {
	Host      string
	Level     string
	Duration  time.Duration // 0 = default
	Reason    string
	PublicKey string
}

// RequestResult is the outcome of Request.
type RequestResult struct {
	Grant   *store.Grant
	Reused  bool // an active grant at >= the requested level already existed
	Claimed bool // a matching offer was claimed with the new key
}

// Request validates and records an agent request, reusing an active grant or
// claiming a matching offer when one exists.
func (s *Service) Request(ctx context.Context, agent *store.Agent, in RequestInput) (*RequestResult, error) {
	if err := activeAgent(agent); err != nil {
		return nil, err
	}
	if !validLevel(in.Level) {
		return nil, apierr.BadRequest("invalid_level", "level must be user or sudo")
	}
	reason, err := ValidateReason(in.Reason)
	if err != nil {
		return nil, err
	}
	pk, err := ca.ParsePublicKey(in.PublicKey)
	if err != nil {
		return nil, apierr.BadRequest("invalid_public_key", "%v", err)
	}
	pubKey := string(ssh.MarshalAuthorizedKey(pk))
	pubKey = pubKey[:len(pubKey)-1]

	if err := s.settleAgent(ctx, agent.ID); err != nil {
		return nil, err
	}
	var res *RequestResult
	err = s.db.WithTx(ctx, func(tx *store.Tx) error {
		h, err := tx.GetHostByName(ctx, in.Host)
		if err != nil {
			return mapNotFound(err, "host")
		}
		if _, err := s.requestableHost(ctx, tx, h.ID); err != nil {
			return err
		}
		if !LevelAtLeast(h.MaxLevel, in.Level) {
			return apierr.BadRequest("level_not_allowed", "host %s allows at most %s access", h.Name, h.MaxLevel)
		}
		dur := in.Duration
		if dur == 0 {
			dur = s.cfg.DefaultDuration
			if h.LorInfra && dur > LorInfraDefault {
				dur = LorInfraDefault
			}
		}
		if err := s.validateDuration(dur, s.cfg.AgentRequestMax, "invalid_duration"); err != nil {
			return err
		}

		existing, err := tx.ListGrants(ctx, store.GrantFilter{AgentID: agent.ID, HostID: h.ID,
			Statuses: []string{store.StatusActive, store.StatusOffered}})
		if err != nil {
			return err
		}
		// Reuse: an active grant at >= the requested level.
		for _, g := range existing {
			if err := s.settle(ctx, tx, g); err != nil {
				return err
			}
			if g.Status == store.StatusActive && LevelAtLeast(g.Level, in.Level) {
				if err := s.audit(ctx, tx, AgentActor(agent), "grant.reused", g, map[string]any{"requested_level": in.Level}); err != nil {
					return err
				}
				res = &RequestResult{Grant: g, Reused: true}
				return nil
			}
		}
		// Claim: a matching offer at >= the requested level.
		for _, g := range existing {
			if g.Status == store.StatusOffered && LevelAtLeast(g.Level, in.Level) {
				if err := s.claim(ctx, tx, agent, g, pubKey, pk); err != nil {
					return err
				}
				res = &RequestResult{Grant: g, Claimed: true}
				return nil
			}
		}

		pending, err := tx.ListGrants(ctx, store.GrantFilter{AgentID: agent.ID, Statuses: []string{store.StatusPending}})
		if err != nil {
			return err
		}
		n := 0
		for _, g := range pending {
			if err := s.settle(ctx, tx, g); err != nil {
				return err
			}
			if g.Status == store.StatusPending {
				n++
			}
		}
		if n >= MaxPendingPerAgent {
			return apierr.Conflict("too_many_pending", "at most %d pending requests per agent; wait for a decision", MaxPendingPerAgent)
		}

		g := &store.Grant{
			AgentID: agent.ID, HostID: h.ID, Origin: originAgentRequest,
			Level: in.Level, RequestedLevel: in.Level, RequestedDurationS: int64(dur / time.Second),
			Reason: reason, PublicKey: pubKey, PublicKeyFP: ssh.FingerprintSHA256(pk),
			Status: store.StatusPending, RequestedAt: s.now().Unix(),
		}
		if err := tx.CreateGrant(ctx, g); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, AgentActor(agent), "grant.requested", g, map[string]any{
			"level": g.Level, "duration_s": g.RequestedDurationS, "public_key_fp": g.PublicKeyFP,
		}); err != nil {
			return err
		}
		res = &RequestResult{Grant: g}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.notify.Notify(res.Grant.ID)
	return res, nil
}

// claim activates an offer with the agent's key. Caller holds tx and has
// settled g.
func (s *Service) claim(ctx context.Context, tx *store.Tx, agent *store.Agent, g *store.Grant, pubKey string, pk ssh.PublicKey) error {
	if g.AgentID != agent.ID {
		return apierr.NotFound("grant_not_found", "grant not found")
	}
	if g.Status != store.StatusOffered {
		return apierr.Conflict("not_offered", "grant %d is %s, not an open offer", g.ID, g.Status)
	}
	h, err := s.requestableHost(ctx, tx, g.HostID)
	if err != nil {
		return err
	}
	if !LevelAtLeast(h.MaxLevel, g.Level) {
		return apierr.BadRequest("level_not_allowed", "host %s now allows at most %s access; ask for a new grant", h.Name, h.MaxLevel)
	}
	from := g.Status
	g.PublicKey, g.PublicKeyFP = pubKey, ssh.FingerprintSHA256(pk)
	g.Status = store.StatusActive
	if err := tx.UpdateGrant(ctx, g, from); err != nil {
		return conflictErr(err)
	}
	if _, err := s.issue(ctx, tx, g, g.ValidBefore, AgentActor(agent)); err != nil {
		return err
	}
	return s.audit(ctx, tx, AgentActor(agent), "grant.offer_claimed", g, map[string]any{"public_key_fp": g.PublicKeyFP})
}

// Claim activates an offer (`lor ssh` on first use). The end time is the
// offer's window end, and an offer can be claimed only once.
func (s *Service) Claim(ctx context.Context, agent *store.Agent, grantID int64, publicKey string) (*store.Grant, error) {
	if err := activeAgent(agent); err != nil {
		return nil, err
	}
	pk, err := ca.ParsePublicKey(publicKey)
	if err != nil {
		return nil, apierr.BadRequest("invalid_public_key", "%v", err)
	}
	pubKey := string(ssh.MarshalAuthorizedKey(pk))
	pubKey = pubKey[:len(pubKey)-1]
	var g *store.Grant
	err = s.withSettled(ctx, grantID, func(tx *store.Tx) error {
		var err error
		g, err = s.agentGrant(ctx, tx, agent, grantID)
		if err != nil {
			return err
		}
		return s.claim(ctx, tx, agent, g, pubKey, pk)
	})
	if err != nil {
		return nil, err
	}
	s.notify.Notify(g.ID)
	return g, nil
}

// agentGrant loads and settles a grant owned by agent. Other agents' grants
// look like they don't exist.
func (s *Service) agentGrant(ctx context.Context, tx *store.Tx, agent *store.Agent, id int64) (*store.Grant, error) {
	g, err := s.loadSettled(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if g.AgentID != agent.ID {
		return nil, apierr.NotFound("grant_not_found", "grant not found")
	}
	return g, nil
}

// AgentGrant returns a settled grant owned by agent.
func (s *Service) AgentGrant(ctx context.Context, agent *store.Agent, id int64) (*store.Grant, error) {
	var g *store.Grant
	err := s.db.WithTx(ctx, func(tx *store.Tx) error {
		var err error
		g, err = s.agentGrant(ctx, tx, agent, id)
		return err
	})
	return g, err
}

// WaitAgentGrant long-polls: while the grant (or one of its extensions) is
// pending, it waits up to timeout for a change.
func (s *Service) WaitAgentGrant(ctx context.Context, agent *store.Agent, id int64, timeout time.Duration) (*store.Grant, error) {
	ch, cancel := s.notify.Subscribe(id)
	defer cancel()
	deadline := s.now().Add(timeout)
	for {
		g, err := s.AgentGrant(ctx, agent, id)
		if err != nil {
			return nil, err
		}
		waiting := g.Status == store.StatusPending
		if !waiting && g.Status == store.StatusActive {
			exts, err := s.db.ListExtensions(ctx, g.ID, store.ExtPending)
			if err != nil {
				return nil, err
			}
			waiting = len(exts) > 0
		}
		remaining := deadline.Sub(s.now())
		if !waiting || remaining <= 0 || ctx.Err() != nil {
			return g, nil
		}
		// Wake at least every few seconds so time-based transitions are seen.
		if remaining > 5*time.Second {
			remaining = 5 * time.Second
		}
		Wait(ctx, ch, remaining)
	}
}

// ListAgentGrants returns the agent's grants (newest first, at most 100),
// settling open grants first so the status filter sees effective states.
func (s *Service) ListAgentGrants(ctx context.Context, agent *store.Agent, host string, statuses []string) ([]*store.Grant, error) {
	if err := s.settleAgent(ctx, agent.ID); err != nil {
		return nil, err
	}
	f := store.GrantFilter{AgentID: agent.ID, Limit: 100, Statuses: statuses}
	if host != "" {
		h, err := s.db.GetHostByName(ctx, host)
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		f.HostID = h.ID
	}
	return s.db.ListGrants(ctx, f)
}

// LatestExtension returns the grant's most recent extension request, if any.
func (s *Service) LatestExtension(ctx context.Context, grantID int64) (*store.ExtensionRequest, error) {
	exts, err := s.db.ListExtensions(ctx, grantID, "")
	if err != nil || len(exts) == 0 {
		return nil, err
	}
	return exts[len(exts)-1], nil
}

// Connect records a command reported by `lor ssh` and returns the grant's
// current certificate. It fails, and logs nothing, if the grant isn't active.
func (s *Service) Connect(ctx context.Context, agent *store.Agent, grantID int64, argv []string) (*store.Grant, *store.Certificate, error) {
	if err := activeAgent(agent); err != nil {
		return nil, nil, err
	}
	var g *store.Grant
	var cert *store.Certificate
	err := s.withSettled(ctx, grantID, func(tx *store.Tx) error {
		var err error
		g, err = s.agentGrant(ctx, tx, agent, grantID)
		if err != nil {
			return err
		}
		if g.Status != store.StatusActive {
			return grantNotActive(g)
		}
		cert, err = tx.LatestCertificate(ctx, g.ID)
		if err != nil {
			return err
		}
		return tx.InsertCommand(ctx, &store.CommandLogEntry{
			GrantID: g.ID, CertificateID: cert.ID, Argv: truncateArgv(argv), ReportedAt: s.now().Unix(),
		})
	})
	if err != nil {
		return nil, nil, err
	}
	s.notify.Notify(g.ID)
	return g, cert, nil
}

func grantNotActive(g *store.Grant) error {
	code := "grant_not_active"
	switch g.Status {
	case store.StatusRevoked:
		code = "grant_revoked"
	case store.StatusReleased:
		code = "grant_released"
	case store.StatusExpired, store.StatusOfferExpired, store.StatusRequestTimeout:
		code = "grant_expired"
	case store.StatusDenied:
		code = "grant_denied"
	case store.StatusPending:
		code = "grant_pending"
	case store.StatusOffered:
		code = "grant_offered"
	}
	return apierr.Conflict(code, "grant %d is %s", g.ID, g.Status)
}

// truncateArgv keeps the JSON encoding of argv within MaxArgvBytes, cutting
// the first argument that doesn't fit and appending a marker.
func truncateArgv(argv []string) []string {
	const marker = "…[truncated]"
	out := []string{}
	for _, a := range argv {
		if jsonLen(append(out[:len(out):len(out)], a)) <= MaxArgvBytes {
			out = append(out, a)
			continue
		}
		for len(out) > 0 && jsonLen(append(out[:len(out):len(out)], marker)) > MaxArgvBytes {
			out = out[:len(out)-1]
		}
		r := []rune(a)
		lo, hi := 0, len(r)
		for lo < hi {
			mid := (lo + hi + 1) / 2
			if jsonLen(append(out[:len(out):len(out)], string(r[:mid]), marker)) <= MaxArgvBytes {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		if lo > 0 {
			out = append(out, string(r[:lo]))
		}
		return append(out, marker)
	}
	return out
}

func jsonLen(v []string) int {
	b, _ := json.Marshal(v)
	return len(b)
}

// RequestExtension records an agent's request for more time
// (`lor extend`). A pending extension for the same grant is returned instead
// of creating a second one.
func (s *Service) RequestExtension(ctx context.Context, agent *store.Agent, grantID int64, dur time.Duration, reason string) (*store.ExtensionRequest, bool, error) {
	if err := activeAgent(agent); err != nil {
		return nil, false, err
	}
	reason, err := ValidateReason(reason)
	if err != nil {
		return nil, false, err
	}
	if err := s.validateDuration(dur, s.cfg.AgentRequestMax, "invalid_duration"); err != nil {
		return nil, false, err
	}
	var ext *store.ExtensionRequest
	reused := false
	err = s.withSettled(ctx, grantID, func(tx *store.Tx) error {
		g, err := s.agentGrant(ctx, tx, agent, grantID)
		if err != nil {
			return err
		}
		if g.Status != store.StatusActive {
			return grantNotActive(g)
		}
		pending, err := tx.ListExtensions(ctx, g.ID, store.ExtPending)
		if err != nil {
			return err
		}
		if len(pending) > 0 {
			ext, reused = pending[0], true
			return nil
		}
		ext = &store.ExtensionRequest{GrantID: g.ID, RequestedDurationS: int64(dur / time.Second), Reason: reason,
			Status: store.ExtPending, RequestedAt: s.now().Unix()}
		if err := tx.CreateExtension(ctx, ext); err != nil {
			return err
		}
		return s.audit(ctx, tx, AgentActor(agent), "extension.requested", g, map[string]any{
			"extension_id": ext.ID, "duration_s": ext.RequestedDurationS,
		})
	})
	if err != nil {
		return nil, false, err
	}
	s.notify.Notify(grantID)
	return ext, reused, nil
}

// Release ends an active grant at the agent's request.
func (s *Service) Release(ctx context.Context, agent *store.Agent, grantID int64) (*store.Grant, error) {
	var g *store.Grant
	err := s.withSettled(ctx, grantID, func(tx *store.Tx) error {
		var err error
		g, err = s.agentGrant(ctx, tx, agent, grantID)
		if err != nil {
			return err
		}
		if g.Status != store.StatusActive {
			return grantNotActive(g)
		}
		g.Status, g.EndedAt = store.StatusReleased, s.now().Unix()
		if err := tx.UpdateGrant(ctx, g, store.StatusActive); err != nil {
			return conflictErr(err)
		}
		if err := s.audit(ctx, tx, AgentActor(agent), "grant.released", g, nil); err != nil {
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

// PendingExtension returns the grant's pending extension request, if any.
func (s *Service) PendingExtension(ctx context.Context, grantID int64) (*store.ExtensionRequest, error) {
	exts, err := s.db.ListExtensions(ctx, grantID, store.ExtPending)
	if err != nil || len(exts) == 0 {
		return nil, err
	}
	return exts[0], nil
}
