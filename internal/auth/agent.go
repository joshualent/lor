package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/joshualent/lor/internal/apierr"
	"github.com/joshualent/lor/internal/store"
)

// EnrollCodeTTL is how long an enrollment code is valid.
const EnrollCodeTTL = 15 * time.Minute

// TokenPrefix marks agent tokens.
const TokenPrefix = "lor_"

// codeAlphabet avoids look-alike characters (0/O, 1/I/L, U).
const codeAlphabet = "23456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewCode returns a one-time code like "7KQ-M2P-X9D" (~44 bits).
func NewCode() string {
	var b strings.Builder
	for i := 0; i < 9; i++ {
		if i > 0 && i%3 == 0 {
			b.WriteByte('-')
		}
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(codeAlphabet))))
		if err != nil {
			panic(err)
		}
		b.WriteByte(codeAlphabet[n.Int64()])
	}
	return b.String()
}

// NormalizeCode uppercases a code and strips separators, so "7kq m2p-x9d"
// matches "7KQ-M2P-X9D".
func NormalizeCode(code string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(code) {
		if r == '-' || r == ' ' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// AgentNameRe matches valid agent names.
var AgentNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// ParseLANAddrs validates and normalizes agent LAN IPs.
func ParseLANAddrs(addrs []string) ([]string, error) {
	if len(addrs) == 0 {
		return nil, apierr.BadRequest("invalid_lan_addrs", "at least one LAN IP is required")
	}
	if len(addrs) > 8 {
		return nil, apierr.BadRequest("invalid_lan_addrs", "at most 8 LAN IPs")
	}
	var out []string
	for _, a := range addrs {
		ip, err := netip.ParseAddr(strings.TrimSpace(a))
		if err != nil || ip.Zone() != "" {
			return nil, apierr.BadRequest("invalid_lan_addrs", "%q is not an IP address", a)
		}
		ip = ip.Unmap()
		if ip.IsUnspecified() || ip.IsMulticast() {
			return nil, apierr.BadRequest("invalid_lan_addrs", "%s can't be an agent address", ip)
		}
		if !slices.Contains(out, ip.String()) {
			out = append(out, ip.String())
		}
	}
	return out, nil
}

// Agents manages agent enrollment and authenticates agent API calls.
type Agents struct {
	db      *store.DB
	now     func() time.Time
	limiter *RateLimiter // per source IP
	global  *RateLimiter
}

// NewAgents returns an agent authenticator.
func NewAgents(db *store.DB, now func() time.Time) *Agents {
	if now == nil {
		now = time.Now
	}
	return &Agents{db: db, now: now, limiter: NewRateLimiter(5, time.Minute, now), global: NewRateLimiter(30, time.Minute, now)}
}

// Authenticate checks the bearer token and the source IP. Missing or wrong
// token → 401; right token from an unregistered IP or a disabled agent → 403.
func (a *Agents) Authenticate(ctx context.Context, authorization string, remote netip.Addr) (*store.Agent, error) {
	token, ok := strings.CutPrefix(authorization, "Bearer ")
	if !ok || !strings.HasPrefix(token, TokenPrefix) {
		return nil, apierr.Unauthorized("unauthorized", "missing or invalid agent token")
	}
	agent, err := a.db.GetAgentByTokenHash(ctx, Hash(token))
	if errors.Is(err, store.ErrNotFound) {
		return nil, apierr.Unauthorized("unauthorized", "missing or invalid agent token")
	}
	if err != nil {
		return nil, err
	}
	if !slices.Contains(agent.LANAddrs, remote.Unmap().String()) {
		return nil, apierr.Forbidden("wrong_source_ip", "requests for agent %s must come from its registered LAN IP", agent.Name)
	}
	if agent.Status != "active" {
		return nil, apierr.Forbidden("agent_disabled", "agent %s is disabled", agent.Name)
	}
	if now := a.now().Unix(); now-agent.LastSeenAt >= 60 {
		_ = a.db.TouchAgent(ctx, agent.ID, now)
	}
	return agent, nil
}

// Enroll exchanges a one-time code for a long-lived token. The code works
// once, expires after 15 minutes, and must be used from one of the agent's
// registered LAN IPs.
func (a *Agents) Enroll(ctx context.Context, code string, remote netip.Addr) (string, *store.Agent, error) {
	if !a.limiter.Allow(remote.String()) || !a.global.Allow("") {
		return "", nil, apierr.New(http.StatusTooManyRequests, "rate_limited", "too many enrollment attempts; wait a minute")
	}
	h := Hash(NormalizeCode(code))
	agent, err := a.db.GetAgentByEnrollCodeHash(ctx, h)
	if errors.Is(err, store.ErrNotFound) {
		return "", nil, apierr.Forbidden("invalid_enroll_code", "the enrollment code is wrong, used or expired")
	}
	if err != nil {
		return "", nil, err
	}
	now := a.now().Unix()
	if agent.EnrollCodeExpiresAt <= now {
		return "", nil, apierr.Forbidden("invalid_enroll_code", "the enrollment code is wrong, used or expired")
	}
	if !slices.Contains(agent.LANAddrs, remote.Unmap().String()) {
		return "", nil, apierr.Forbidden("wrong_source_ip", "this machine (%s) isn't a registered LAN IP of agent %s", remote.Unmap(), agent.Name)
	}
	if agent.Status != "active" {
		return "", nil, apierr.Forbidden("agent_disabled", "agent %s is disabled", agent.Name)
	}
	token := TokenPrefix + base64.RawURLEncoding.EncodeToString(randomBytes(32))
	err = a.db.WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.CompleteEnrollment(ctx, agent.ID, h, Hash(token), now); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return apierr.Forbidden("invalid_enroll_code", "the enrollment code is wrong, used or expired")
			}
			return err
		}
		return tx.InsertAudit(ctx, &store.AuditEvent{TS: now, Actor: "agent:" + agent.Name, Action: "agent.enrolled",
			AgentID: agent.ID, Details: map[string]any{"from": remote.Unmap().String()}})
	})
	if err != nil {
		return "", nil, err
	}
	agent.TokenHash = Hash(token)
	return token, agent, nil
}

// Create adds an agent and returns its one-time enrollment code.
func (a *Agents) Create(ctx context.Context, actor, name string, lanAddrs []string) (*store.Agent, string, time.Time, error) {
	if !AgentNameRe.MatchString(name) {
		return nil, "", time.Time{}, apierr.BadRequest("invalid_name", "agent name must be lowercase letters, digits and dashes (max 32)")
	}
	addrs, err := ParseLANAddrs(lanAddrs)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	code := NewCode()
	now := a.now()
	exp := now.Add(EnrollCodeTTL)
	agent := &store.Agent{Name: name, Status: "active", LANAddrs: addrs, EnrollCodeHash: Hash(NormalizeCode(code)),
		EnrollCodeExpiresAt: exp.Unix(), CreatedAt: now.Unix()}
	err = a.db.WithTx(ctx, func(tx *store.Tx) error {
		if _, err := tx.GetAgentByName(ctx, name); err == nil {
			return apierr.Conflict("agent_exists", "an agent named %s already exists", name)
		}
		if err := tx.CreateAgent(ctx, agent); err != nil {
			return err
		}
		return tx.InsertAudit(ctx, &store.AuditEvent{TS: now.Unix(), Actor: actor, Action: "agent.created", AgentID: agent.ID,
			Details: map[string]any{"name": name, "lan_addrs": addrs}})
	})
	if err != nil {
		return nil, "", time.Time{}, err
	}
	return agent, code, exp, nil
}

// Rotate revokes the agent's token and issues a new enrollment code.
func (a *Agents) Rotate(ctx context.Context, actor string, id int64) (*store.Agent, string, time.Time, error) {
	code := NewCode()
	now := a.now()
	exp := now.Add(EnrollCodeTTL)
	var agent *store.Agent
	err := a.db.WithTx(ctx, func(tx *store.Tx) error {
		var err error
		agent, err = tx.GetAgent(ctx, id)
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("agent_not_found", "agent not found")
		}
		if err != nil {
			return err
		}
		if err := tx.SetAgentEnrollCode(ctx, id, Hash(NormalizeCode(code)), exp.Unix()); err != nil {
			return err
		}
		return tx.InsertAudit(ctx, &store.AuditEvent{TS: now.Unix(), Actor: actor, Action: "agent.token_rotated", AgentID: id})
	})
	if err != nil {
		return nil, "", time.Time{}, err
	}
	agent, err = a.db.GetAgent(ctx, id)
	return agent, code, exp, err
}

// SetStatus enables or disables an agent.
func (a *Agents) SetStatus(ctx context.Context, actor string, id int64, status string) (*store.Agent, error) {
	if status != "active" && status != "disabled" {
		return nil, apierr.BadRequest("invalid_status", "status must be active or disabled")
	}
	err := a.db.WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.SetAgentStatus(ctx, id, status); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return apierr.NotFound("agent_not_found", "agent not found")
			}
			return err
		}
		action := "agent.enabled"
		if status == "disabled" {
			action = "agent.disabled"
		}
		return tx.InsertAudit(ctx, &store.AuditEvent{TS: a.now().Unix(), Actor: actor, Action: action, AgentID: id})
	})
	if err != nil {
		return nil, err
	}
	return a.db.GetAgent(ctx, id)
}

// CheckSetAddrs validates a LAN IP change (step-up begin).
func (a *Agents) CheckSetAddrs(ctx context.Context, id int64, addrs []string) (*store.Agent, []string, error) {
	agent, err := a.db.GetAgent(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, apierr.NotFound("agent_not_found", "agent not found")
	}
	if err != nil {
		return nil, nil, err
	}
	norm, err := ParseLANAddrs(addrs)
	return agent, norm, err
}

// SetAddrs changes an agent's LAN IPs (after step-up). New certificates use
// the new addresses; existing ones keep theirs until they expire.
func (a *Agents) SetAddrs(ctx context.Context, actor string, id int64, addrs []string) (*store.Agent, error) {
	agent, norm, err := a.CheckSetAddrs(ctx, id, addrs)
	if err != nil {
		return nil, err
	}
	err = a.db.WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.SetAgentLANAddrs(ctx, id, norm); err != nil {
			return err
		}
		return tx.InsertAudit(ctx, &store.AuditEvent{TS: a.now().Unix(), Actor: actor, Action: "agent.addrs_changed", AgentID: id,
			Details: map[string]any{"old": agent.LANAddrs, "new": norm}})
	})
	if err != nil {
		return nil, err
	}
	return a.db.GetAgent(ctx, id)
}

// EnrollCommand renders the one-time command shown in the UI.
func EnrollCommand(agentURL, fingerprint, code string) string {
	return fmt.Sprintf("lor enroll --url %s --fingerprint %s --code %s", agentURL, fingerprint, code)
}

// ---------------------------------------------------------------- rate limiter

// RateLimiter is a fixed-window limiter keyed by string.
type RateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	now    func() time.Time
	hits   map[string]*window
}

type window struct {
	start time.Time
	n     int
}

// NewRateLimiter allows limit events per key per window.
func NewRateLimiter(limit int, w time.Duration, now func() time.Time) *RateLimiter {
	return &RateLimiter{limit: limit, window: w, now: now, hits: map[string]*window{}}
}

// Allow records an event for key and reports whether it is within the limit.
func (r *RateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	if len(r.hits) > 10000 {
		for k, w := range r.hits {
			if now.Sub(w.start) >= r.window {
				delete(r.hits, k)
			}
		}
	}
	w := r.hits[key]
	if w == nil || now.Sub(w.start) >= r.window {
		w = &window{start: now}
		r.hits[key] = w
	}
	w.n++
	return w.n <= r.limit
}
