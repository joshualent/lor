// Package grants is the grant state machine (plan section 7): agent requests,
// admin offers, claims, extensions, release, revoke and the sweeper.
//
//	Agent request:  pending ──Authorize──▶ active      Admin offer:  offered ──claim──▶ active
//	                   ├─Deny──▶ denied                                 └─window ends──▶ offer_expired
//	                   └─30m undecided──▶ request_timeout
//	active ──end time passes──▶ expired  |  ──agent release──▶ released  |  ──admin revoke──▶ revoked
//
// Every transition is a conditional UPDATE inside a transaction, so concurrent
// callers can't both win (an offer is claimed once, a request decided once).
package grants

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/ssh"

	"github.com/joshualent/lor/internal/apierr"
	"github.com/joshualent/lor/internal/ca"
	"github.com/joshualent/lor/internal/store"
)

// Levels.
const (
	LevelUser = "user"
	LevelSudo = "sudo"
)

// Limits from the plan.
const (
	ReasonMin          = 10
	ReasonMax          = 400
	MaxPendingPerAgent = 3
	MinDuration        = time.Minute
	LorInfraDefault    = 15 * time.Minute
	ClockSkew          = 2 * time.Minute
	MaxNoteLen         = 1000
	MaxArgvBytes       = 4096
	SystemActor        = "system"
	originAgentRequest = "agent_request"
	originAdminOffer   = "admin_offer"
)

// Config holds the grant policy settings.
type Config struct {
	PublicURL           string
	DefaultDuration     time.Duration
	AgentRequestMax     time.Duration
	AbsoluteMax         time.Duration
	ApprovalTimeout     time.Duration
	CommandLogRetention time.Duration
}

// Service implements the grant state machine.
type Service struct {
	db     *store.DB
	ca     *ca.CA
	cfg    Config
	now    func() time.Time
	notify *Notifier
	log    *slog.Logger
}

// New returns a Service. now may be nil (time.Now).
func New(db *store.DB, authority *ca.CA, cfg Config, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, ca: authority, cfg: cfg, now: now, notify: NewNotifier(), log: slog.Default()}
}

// Config returns the policy settings.
func (s *Service) Config() Config { return s.cfg }

// Now returns the service clock's current time.
func (s *Service) Now() time.Time { return s.now() }

// Notifier returns the change notifier used for long-polling.
func (s *Service) Notifier() *Notifier { return s.notify }

// ApprovalURL is the consent-page link for a grant.
func (s *Service) ApprovalURL(grantID int64) string {
	return fmt.Sprintf("%s/r/%d", s.cfg.PublicURL, grantID)
}

// ExtensionURL is the consent-page link for an extension request.
func (s *Service) ExtensionURL(extID int64) string {
	return fmt.Sprintf("%s/e/%d", s.cfg.PublicURL, extID)
}

// ApprovalDeadline is when a pending request times out.
func (s *Service) ApprovalDeadline(g *store.Grant) time.Time {
	return time.Unix(g.RequestedAt, 0).Add(s.cfg.ApprovalTimeout)
}

// AgentActor and AdminActor build audit actor strings.
func AgentActor(a *store.Agent) string     { return "agent:" + a.Name }
func AdminActor(passkeyName string) string { return "admin:" + passkeyName }

// LevelAtLeast reports whether level a grants at least as much as b.
func LevelAtLeast(a, b string) bool { return a == LevelSudo || b == LevelUser }

func validLevel(l string) bool { return l == LevelUser || l == LevelSudo }

// SSHUser is the target account for a level.
func SSHUser(level string) string {
	if level == LevelSudo {
		return "agent-sudo"
	}
	return "agent"
}

// HostKeyAlias is the known_hosts alias used by the CLI for a host.
func HostKeyAlias(host string) string { return "lor-host-" + host }

// ValidateReason enforces the 10–400 character rule.
func ValidateReason(reason string) (string, error) {
	r := strings.TrimSpace(reason)
	n := utf8.RuneCountInString(r)
	if !utf8.ValidString(r) {
		return "", apierr.BadRequest("invalid_reason", "reason must be valid UTF-8")
	}
	if n < ReasonMin || n > ReasonMax {
		return "", apierr.BadRequest("invalid_reason", "reason is required: %d–%d characters (got %d)", ReasonMin, ReasonMax, n)
	}
	for _, c := range r {
		if unicode.IsControl(c) && c != '\n' && c != '\t' {
			return "", apierr.BadRequest("invalid_reason", "reason must not contain control characters")
		}
	}
	return r, nil
}

func validateNote(note string) (string, error) {
	n := strings.TrimSpace(note)
	if utf8.RuneCountInString(n) > MaxNoteLen || !utf8.ValidString(n) {
		return "", apierr.BadRequest("invalid_note", "note must be at most %d characters", MaxNoteLen)
	}
	return n, nil
}

func (s *Service) validateDuration(d time.Duration, max time.Duration, code string) error {
	if d < MinDuration {
		return apierr.BadRequest(code, "duration must be at least %s", MinDuration)
	}
	if d > max {
		return apierr.BadRequest(code, "duration must be at most %s", formatDuration(max))
	}
	return nil
}

func formatDuration(d time.Duration) string {
	switch {
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", d/(24*time.Hour))
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", d/time.Minute)
	}
	return d.String()
}

// FormatDuration renders a duration compactly ("15m", "1h", "3d").
func FormatDuration(d time.Duration) string { return formatDuration(d) }

func (s *Service) audit(ctx context.Context, tx *store.Tx, actor, action string, g *store.Grant, details map[string]any) error {
	e := &store.AuditEvent{TS: s.now().Unix(), Actor: actor, Action: action, Details: details}
	if g != nil {
		e.GrantID, e.HostID, e.AgentID = g.ID, g.HostID, g.AgentID
	}
	return tx.InsertAudit(ctx, e)
}

func mapNotFound(err error, what string) error {
	if errors.Is(err, store.ErrNotFound) {
		return apierr.NotFound(what+"_not_found", "%s not found", strings.ReplaceAll(what, "_", " "))
	}
	return err
}

func conflictErr(err error) error {
	if errors.Is(err, store.ErrConflict) {
		return apierr.Conflict("state_changed", "the grant changed state; reload and try again")
	}
	return err
}

// ---------------------------------------------------------------- settling

// settle applies any time-based transition that is due for g (the sweeper
// does the same every 30 seconds). Callers always settle before deciding, so
// correctness never depends on the sweeper's timing.
func (s *Service) settle(ctx context.Context, tx *store.Tx, g *store.Grant) error {
	now := s.now().Unix()
	from := g.Status
	switch {
	case g.Status == store.StatusActive && g.ValidBefore <= now:
		g.Status, g.EndedAt = store.StatusExpired, g.ValidBefore
	case g.Status == store.StatusOffered && g.ValidBefore <= now:
		g.Status, g.EndedAt = store.StatusOfferExpired, g.ValidBefore
	case g.Status == store.StatusPending && s.ApprovalDeadline(g).Unix() <= now:
		g.Status, g.EndedAt = store.StatusRequestTimeout, s.ApprovalDeadline(g).Unix()
	default:
		return s.settleExtensions(ctx, tx, g)
	}
	if err := tx.UpdateGrant(ctx, g, from); err != nil {
		return err
	}
	if err := s.audit(ctx, tx, SystemActor, "grant."+g.Status, g, nil); err != nil {
		return err
	}
	s.notify.Notify(g.ID)
	return s.settleExtensions(ctx, tx, g)
}

// settleExtensions moots pending extensions of ended grants and times out
// undecided ones.
func (s *Service) settleExtensions(ctx context.Context, tx *store.Tx, g *store.Grant) error {
	exts, err := tx.ListExtensions(ctx, g.ID, store.ExtPending)
	if err != nil {
		return err
	}
	now := s.now().Unix()
	for _, e := range exts {
		switch {
		case g.Status != store.StatusActive:
			e.Status = store.ExtMoot
		case e.RequestedAt+int64(s.cfg.ApprovalTimeout/time.Second) <= now:
			e.Status = store.ExtRequestTimeout
		default:
			continue
		}
		e.DecidedAt = now
		if err := tx.UpdateExtension(ctx, e, store.ExtPending); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, SystemActor, "extension."+e.Status, g, map[string]any{"extension_id": e.ID}); err != nil {
			return err
		}
		s.notify.Notify(g.ID)
	}
	return nil
}

// loadSettled loads a grant inside tx and settles it.
func (s *Service) loadSettled(ctx context.Context, tx *store.Tx, id int64) (*store.Grant, error) {
	g, err := tx.GetGrant(ctx, id)
	if err != nil {
		return nil, mapNotFound(err, "grant")
	}
	if err := s.settle(ctx, tx, g); err != nil {
		return nil, err
	}
	return g, nil
}

// withSettled first settles grantID in its own committed transaction, then
// runs fn in a new transaction. Without this, an operation that fails after
// settling (say, "authorize" on a request that just timed out) would roll the
// time-based transition back along with everything else.
func (s *Service) withSettled(ctx context.Context, grantID int64, fn func(tx *store.Tx) error) error {
	if _, err := s.Settle(ctx, grantID); err != nil && apierr.Code(err) != "grant_not_found" {
		return err
	}
	return s.db.WithTx(ctx, fn)
}

// settleAgent settles all open grants of an agent in their own transactions.
func (s *Service) settleAgent(ctx context.Context, agentID int64) error {
	open, err := s.db.ListGrants(ctx, store.GrantFilter{AgentID: agentID,
		Statuses: []string{store.StatusPending, store.StatusOffered, store.StatusActive}})
	if err != nil {
		return err
	}
	for _, g := range open {
		if _, err := s.Settle(ctx, g.ID); err != nil {
			return err
		}
	}
	return nil
}

// Settle loads and settles a grant in its own transaction (read paths).
func (s *Service) Settle(ctx context.Context, id int64) (*store.Grant, error) {
	var g *store.Grant
	err := s.db.WithTx(ctx, func(tx *store.Tx) error {
		var err error
		g, err = s.loadSettled(ctx, tx, id)
		return err
	})
	return g, err
}

// ---------------------------------------------------------------- certificates

// issue signs a certificate for g's key, valid until validBefore, and records
// it. The grant's agent and host are re-read inside the transaction so the
// source address is always the agent's current registered IPs.
func (s *Service) issue(ctx context.Context, tx *store.Tx, g *store.Grant, validBefore int64, issuedBy string) (*store.Certificate, error) {
	agent, err := tx.GetAgent(ctx, g.AgentID)
	if err != nil {
		return nil, err
	}
	host, err := tx.GetHost(ctx, g.HostID)
	if err != nil {
		return nil, err
	}
	pk, err := ca.ParsePublicKey(g.PublicKey)
	if err != nil {
		return nil, apierr.BadRequest("invalid_public_key", "%v", err)
	}
	addrs := make([]netip.Addr, 0, len(agent.LANAddrs))
	for _, a := range agent.LANAddrs {
		ip, err := netip.ParseAddr(a)
		if err != nil {
			return nil, fmt.Errorf("agent %s has invalid LAN address %q", agent.Name, a)
		}
		addrs = append(addrs, ip)
	}
	now := s.now()
	validAfter := now.Add(-ClockSkew).Unix()
	c := &store.Certificate{GrantID: g.ID, ValidAfter: validAfter, ValidBefore: validBefore, IssuedAt: now.Unix(), IssuedBy: issuedBy}
	if err := tx.ReserveCertificate(ctx, c); err != nil {
		return nil, err
	}
	cert, err := s.ca.SignUserCert(ca.UserCertRequest{
		PublicKey:   pk,
		Serial:      uint64(c.ID),
		KeyID:       ca.KeyID(g.ID, c.ID, agent.Name, host.Name, g.Level),
		Principal:   ca.Principal(host.Name, g.Level),
		ValidAfter:  time.Unix(validAfter, 0),
		ValidBefore: time.Unix(validBefore, 0),
		SourceAddrs: addrs,
	})
	if err != nil {
		if errors.Is(err, ca.ErrRefused) {
			return nil, apierr.BadRequest("ca_refused", "%v", err)
		}
		return nil, err
	}
	c.Certificate = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(cert)))
	if err := tx.FinishCertificate(ctx, c.ID, c.Certificate); err != nil {
		return nil, err
	}
	if err := s.audit(ctx, tx, issuedBy, "cert.issued", g, map[string]any{
		"certificate_id": c.ID, "principal": ca.Principal(host.Name, g.Level),
		"valid_before": validBefore, "source_address": agent.LANAddrs,
	}); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) checkAbsoluteMax(validBefore int64) error {
	if time.Unix(validBefore, 0).Sub(s.now()) > s.cfg.AbsoluteMax {
		return apierr.BadRequest("duration_too_long", "the end time would be more than %s from now", formatDuration(s.cfg.AbsoluteMax))
	}
	return nil
}

// ---------------------------------------------------------------- shared checks

func (s *Service) requestableHost(ctx context.Context, tx *store.Tx, hostID int64) (*store.Host, error) {
	h, err := tx.GetHost(ctx, hostID)
	if err != nil {
		return nil, mapNotFound(err, "host")
	}
	if h.Disabled {
		return nil, apierr.BadRequest("host_disabled", "host %s is disabled", h.Name)
	}
	if h.HostKey == "" {
		return nil, apierr.BadRequest("host_key_missing", "host %s has no scanned host key yet; scan it in Lor first", h.Name)
	}
	return h, nil
}

func activeAgent(a *store.Agent) error {
	if a.Status != "active" {
		return apierr.Forbidden("agent_disabled", "agent %s is disabled", a.Name)
	}
	return nil
}
