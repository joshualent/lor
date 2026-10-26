// Package auth handles admin identity (passkeys, sessions, step-up) and agent
// identity (enrollment, token + source-IP check). Plan section 5.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/joshualent/lor/internal/apierr"
	"github.com/joshualent/lor/internal/store"
)

// Session and ceremony lifetimes from the plan.
const (
	SessionCookieName = "__Host-lor_session"
	SessionIdle       = 7 * 24 * time.Hour
	SessionAbsolute   = 30 * 24 * time.Hour
	CeremonyTTL       = 5 * time.Minute
	SetupCodeTTL      = 15 * time.Minute
	touchEvery        = time.Minute
	maxPasskeyName    = 64
	maxCeremonies     = 1000
)

// Ceremony kinds.
const (
	kindSetup    = "setup"
	kindLogin    = "login"
	kindStepUp   = "stepup"
	kindRegister = "register"
)

type ceremony struct {
	kind        string
	data        webauthn.SessionData
	expires     time.Time
	sessionID   int64  // bound session (step-up, register)
	action      any    // stored step-up action, applied exactly as stored
	summary     string // human restatement of action
	passkeyName string // setup, register
	setupHash   []byte
}

// SessionInfo is an authenticated admin session.
type SessionInfo struct {
	ID          int64
	PasskeyID   int64
	PasskeyName string
}

// RequestMeta is recorded with new sessions (information only).
type RequestMeta struct {
	UserAgent    string
	ForwardedFor string
}

// Admin implements passkey login, sessions and step-up for the single admin.
type Admin struct {
	db  *store.DB
	wa  *webauthn.WebAuthn
	now func() time.Time
	log *slog.Logger

	userHandle []byte

	mu         sync.Mutex
	ceremonies map[string]*ceremony
}

// NewAdmin builds the admin authenticator. rpID is the host of public_url;
// origins are the allowed WebAuthn origins.
func NewAdmin(ctx context.Context, db *store.DB, rpID string, origins []string, now func() time.Time) (*Admin, error) {
	if now == nil {
		now = time.Now
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: "Lor",
		RPOrigins:     origins,
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: CeremonyTTL, TimeoutUVD: CeremonyTTL},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: CeremonyTTL, TimeoutUVD: CeremonyTTL},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("webauthn: %w", err)
	}
	handle, err := db.GetOrCreateMeta(ctx, "webauthn_user_handle", func() []byte { return randomBytes(32) })
	if err != nil {
		return nil, err
	}
	return &Admin{db: db, wa: wa, now: now, log: slog.Default(), userHandle: handle, ceremonies: map[string]*ceremony{}}, nil
}

// adminUser is the single WebAuthn user.
type adminUser struct {
	handle []byte
	creds  []webauthn.Credential
}

func (u *adminUser) WebAuthnID() []byte                         { return u.handle }
func (u *adminUser) WebAuthnName() string                       { return "admin" }
func (u *adminUser) WebAuthnDisplayName() string                { return "Lor admin" }
func (u *adminUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

func (a *Admin) user(ctx context.Context) (*adminUser, error) {
	pks, err := a.db.ListPasskeys(ctx)
	if err != nil {
		return nil, err
	}
	u := &adminUser{handle: a.userHandle}
	for _, p := range pks {
		var c webauthn.Credential
		if err := json.Unmarshal([]byte(p.CredentialJSON), &c); err != nil {
			return nil, fmt.Errorf("passkey %d: %w", p.ID, err)
		}
		u.creds = append(u.creds, c)
	}
	return u, nil
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func randomID() string { return base64.RawURLEncoding.EncodeToString(randomBytes(18)) }

// Hash returns SHA-256 of a secret (tokens and codes are stored hashed).
func Hash(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

func (a *Admin) putCeremony(c *ceremony) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	for id, x := range a.ceremonies {
		if now.After(x.expires) {
			delete(a.ceremonies, id)
		}
	}
	if len(a.ceremonies) >= maxCeremonies {
		return "", apierr.New(http.StatusTooManyRequests, "too_many_ceremonies", "too many passkey prompts in progress; try again shortly")
	}
	id := randomID()
	c.expires = now.Add(CeremonyTTL)
	a.ceremonies[id] = c
	return id, nil
}

// takeCeremony removes and returns a ceremony: every challenge is single-use,
// whether or not verification succeeds.
func (a *Admin) takeCeremony(id, kind string) (*ceremony, error) {
	a.mu.Lock()
	c, ok := a.ceremonies[id]
	delete(a.ceremonies, id)
	a.mu.Unlock()
	if !ok || c.kind != kind {
		return nil, apierr.BadRequest("ceremony_not_found", "this passkey prompt is unknown or was already used; start again")
	}
	if a.now().After(c.expires) {
		return nil, apierr.BadRequest("ceremony_expired", "this passkey prompt expired; start again")
	}
	return c, nil
}

// CancelCeremonies drops all in-progress ceremonies (tests; restart does the same).
func (a *Admin) CancelCeremonies() {
	a.mu.Lock()
	a.ceremonies = map[string]*ceremony{}
	a.mu.Unlock()
}

func validPasskeyName(name string) (string, error) {
	n := strings.TrimSpace(name)
	if n == "" || utf8.RuneCountInString(n) > maxPasskeyName {
		return "", apierr.BadRequest("invalid_passkey_name", "passkey name must be 1–%d characters", maxPasskeyName)
	}
	for _, r := range n {
		if r < 0x20 || r == 0x7f {
			return "", apierr.BadRequest("invalid_passkey_name", "passkey name must not contain control characters")
		}
	}
	return n, nil
}

func devInfo(err error) string {
	var pe *protocol.Error
	if errors.As(err, &pe) {
		return pe.DevInfo
	}
	return ""
}

func marshalOptions(v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	return json.RawMessage(b), err
}

func (a *Admin) registrationOptions(ctx context.Context, u *adminUser) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
	var exclude []protocol.CredentialDescriptor
	for _, c := range u.creds {
		exclude = append(exclude, c.Descriptor())
	}
	return a.wa.BeginRegistration(u,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationRequired,
		}),
		webauthn.WithConveyancePreference(protocol.PreferNoAttestation),
		webauthn.WithExclusions(exclude),
	)
}

// ---------------------------------------------------------------- setup

// SetupRequired reports whether no passkey is registered yet.
func (a *Admin) SetupRequired(ctx context.Context) (bool, error) {
	n, err := a.db.CountPasskeys(ctx)
	return n == 0, err
}

// CreateSetupCode generates a one-time setup code valid for 15 minutes
// (lor-server admin setup-code). It refuses if a passkey already exists:
// run `admin reset` first.
func CreateSetupCode(ctx context.Context, db *store.DB, now time.Time) (string, time.Time, error) {
	n, err := db.CountPasskeys(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	if n > 0 {
		return "", time.Time{}, errors.New("a passkey is already registered; add more passkeys from Settings, or run `lor-server admin reset` first")
	}
	code := NewCode()
	exp := now.Add(SetupCodeTTL)
	if err := db.CreateSetupCode(ctx, Hash(NormalizeCode(code)), now.Unix(), exp.Unix()); err != nil {
		return "", time.Time{}, err
	}
	return code, exp, nil
}

// SetupBegin starts first-passkey registration.
func (a *Admin) SetupBegin(ctx context.Context, code, passkeyName string) (string, json.RawMessage, error) {
	name, err := validPasskeyName(passkeyName)
	if err != nil {
		return "", nil, err
	}
	required, err := a.SetupRequired(ctx)
	if err != nil {
		return "", nil, err
	}
	if !required {
		return "", nil, apierr.Conflict("setup_done", "a passkey is already registered; sign in instead")
	}
	h := Hash(NormalizeCode(code))
	ok, err := a.db.CheckSetupCode(ctx, h, a.now().Unix())
	if err != nil {
		return "", nil, err
	}
	if !ok {
		return "", nil, apierr.Forbidden("invalid_setup_code", "the setup code is wrong, used or expired; run `lor-server admin setup-code` again")
	}
	u, err := a.user(ctx)
	if err != nil {
		return "", nil, err
	}
	creation, data, err := a.registrationOptions(ctx, u)
	if err != nil {
		return "", nil, err
	}
	id, err := a.putCeremony(&ceremony{kind: kindSetup, data: *data, passkeyName: name, setupHash: h})
	if err != nil {
		return "", nil, err
	}
	opts, err := marshalOptions(creation.Response)
	return id, opts, err
}

// SetupFinish verifies the registration, consumes the setup code, stores the
// first passkey and signs in. The returned token is the session cookie value.
func (a *Admin) SetupFinish(ctx context.Context, id string, credential []byte, meta RequestMeta) (string, *SessionInfo, error) {
	c, err := a.takeCeremony(id, kindSetup)
	if err != nil {
		return "", nil, err
	}
	u, err := a.user(ctx)
	if err != nil {
		return "", nil, err
	}
	cred, err := a.verifyRegistration(u, c, credential)
	if err != nil {
		return "", nil, err
	}
	var token string
	var info *SessionInfo
	err = a.db.WithTx(ctx, func(tx *store.Tx) error {
		n, err := tx.CountPasskeys(ctx)
		if err != nil {
			return err
		}
		if n > 0 {
			return apierr.Conflict("setup_done", "a passkey is already registered; sign in instead")
		}
		if err := tx.ConsumeSetupCode(ctx, c.setupHash, a.now().Unix()); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return apierr.Forbidden("invalid_setup_code", "the setup code is wrong, used or expired")
			}
			return err
		}
		p, err := a.storePasskey(ctx, tx, c.passkeyName, cred)
		if err != nil {
			return err
		}
		if err := tx.InsertAudit(ctx, &store.AuditEvent{TS: a.now().Unix(), Actor: "admin:" + p.Name, Action: "admin.setup",
			Details: map[string]any{"passkey": p.Name, "forwarded_for": meta.ForwardedFor}}); err != nil {
			return err
		}
		token, info, err = a.createSession(ctx, tx, p, meta)
		return err
	})
	return token, info, err
}

func (a *Admin) verifyRegistration(u *adminUser, c *ceremony, credential []byte) (*webauthn.Credential, error) {
	parsed, err := protocol.ParseCredentialCreationResponseBytes(credential)
	if err != nil {
		return nil, apierr.BadRequest("invalid_credential", "could not read the passkey response")
	}
	cred, err := a.wa.CreateCredential(u, c.data, parsed)
	if err != nil {
		a.log.Info("passkey registration failed", "err", err, "detail", devInfo(err))
		return nil, apierr.Forbidden("passkey_failed", "the passkey could not be verified")
	}
	return cred, nil
}

func (a *Admin) storePasskey(ctx context.Context, tx *store.Tx, name string, cred *webauthn.Credential) (*store.Passkey, error) {
	b, err := json.Marshal(cred)
	if err != nil {
		return nil, err
	}
	var transports []string
	for _, t := range cred.Transport {
		transports = append(transports, string(t))
	}
	p := &store.Passkey{
		Name: name, CredentialID: cred.ID, PublicKey: cred.PublicKey, SignCount: cred.Authenticator.SignCount,
		AAGUID: cred.Authenticator.AAGUID, Transports: transports, CredentialJSON: string(b), CreatedAt: a.now().Unix(),
	}
	if err := tx.CreatePasskey(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// ---------------------------------------------------------------- login

func (a *Admin) assertionOptions(ctx context.Context) (*adminUser, *protocol.CredentialAssertion, *webauthn.SessionData, error) {
	u, err := a.user(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(u.creds) == 0 {
		return nil, nil, nil, apierr.Conflict("setup_required", "no passkey is registered yet; use /setup")
	}
	assertion, data, err := a.wa.BeginLogin(u, webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return nil, nil, nil, err
	}
	return u, assertion, data, nil
}

// LoginBegin starts a passkey login.
func (a *Admin) LoginBegin(ctx context.Context) (string, json.RawMessage, error) {
	_, assertion, data, err := a.assertionOptions(ctx)
	if err != nil {
		return "", nil, err
	}
	id, err := a.putCeremony(&ceremony{kind: kindLogin, data: *data})
	if err != nil {
		return "", nil, err
	}
	opts, err := marshalOptions(assertion.Response)
	return id, opts, err
}

// verifyAssertion validates an assertion (user verification required) and
// records the new sign count. It returns the passkey that was used.
func (a *Admin) verifyAssertion(ctx context.Context, c *ceremony, credential []byte) (*store.Passkey, error) {
	parsed, err := protocol.ParseCredentialRequestResponseBytes(credential)
	if err != nil {
		return nil, apierr.BadRequest("invalid_credential", "could not read the passkey response")
	}
	u, err := a.user(ctx)
	if err != nil {
		return nil, err
	}
	cred, err := a.wa.ValidateLogin(u, c.data, parsed)
	if err != nil {
		a.log.Info("passkey assertion failed", "err", err, "detail", devInfo(err))
		return nil, apierr.Forbidden("passkey_failed", "the passkey could not be verified")
	}
	p, err := a.db.GetPasskeyByCredentialID(ctx, cred.ID)
	if err != nil {
		return nil, apierr.Forbidden("passkey_failed", "unknown passkey")
	}
	if cred.Authenticator.CloneWarning {
		a.log.Warn("passkey sign counter went backwards", "passkey", p.Name)
	}
	b, err := json.Marshal(cred)
	if err != nil {
		return nil, err
	}
	if err := a.db.UpdatePasskeyUse(ctx, p.ID, cred.Authenticator.SignCount, string(b), a.now().Unix()); err != nil {
		return nil, err
	}
	return p, nil
}

// LoginFinish verifies the assertion and creates a session.
func (a *Admin) LoginFinish(ctx context.Context, id string, credential []byte, meta RequestMeta) (string, *SessionInfo, error) {
	c, err := a.takeCeremony(id, kindLogin)
	if err != nil {
		return "", nil, err
	}
	p, err := a.verifyAssertion(ctx, c, credential)
	if err != nil {
		return "", nil, err
	}
	var token string
	var info *SessionInfo
	err = a.db.WithTx(ctx, func(tx *store.Tx) error {
		var err error
		token, info, err = a.createSession(ctx, tx, p, meta)
		if err != nil {
			return err
		}
		return tx.InsertAudit(ctx, &store.AuditEvent{TS: a.now().Unix(), Actor: "admin:" + p.Name, Action: "admin.login",
			Details: map[string]any{"session_id": info.ID, "forwarded_for": meta.ForwardedFor, "user_agent": meta.UserAgent}})
	})
	return token, info, err
}

// ---------------------------------------------------------------- sessions

func (a *Admin) createSession(ctx context.Context, tx *store.Tx, p *store.Passkey, meta RequestMeta) (string, *SessionInfo, error) {
	token := base64.RawURLEncoding.EncodeToString(randomBytes(32)) // 256-bit
	now := a.now()
	s := &store.Session{
		TokenHash: Hash(token), PasskeyID: p.ID, CreatedAt: now.Unix(), LastSeenAt: now.Unix(),
		ExpiresAt: now.Add(SessionAbsolute).Unix(), UserAgent: truncate(meta.UserAgent, 300), ForwardedFor: truncate(meta.ForwardedFor, 200),
	}
	if err := tx.CreateSession(ctx, s); err != nil {
		return "", nil, err
	}
	return token, &SessionInfo{ID: s.ID, PasskeyID: p.ID, PasskeyName: p.Name}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// SessionCookie returns the Set-Cookie value for a new session.
func SessionCookie(token string) string {
	c := &http.Cookie{Name: SessionCookieName, Value: token, Path: "/", MaxAge: int(SessionAbsolute / time.Second),
		Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode}
	return c.String()
}

// ClearSessionCookie returns a Set-Cookie value that deletes the cookie.
func ClearSessionCookie() string {
	c := &http.Cookie{Name: SessionCookieName, Value: "", Path: "/", MaxAge: -1,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode}
	return c.String()
}

// Authenticate checks a session cookie value. Sessions end after 7 days idle
// or 30 days total.
func (a *Admin) Authenticate(ctx context.Context, token string) (*SessionInfo, error) {
	if token == "" {
		return nil, apierr.Unauthorized("unauthenticated", "sign in first")
	}
	s, err := a.db.GetSessionByTokenHash(ctx, Hash(token))
	if errors.Is(err, store.ErrNotFound) {
		return nil, apierr.Unauthorized("unauthenticated", "sign in first")
	}
	if err != nil {
		return nil, err
	}
	now := a.now()
	if now.Unix() >= s.ExpiresAt || now.Sub(time.Unix(s.LastSeenAt, 0)) >= SessionIdle {
		_ = a.db.DeleteSession(ctx, s.ID)
		return nil, apierr.Unauthorized("session_expired", "your session expired; sign in again")
	}
	if now.Sub(time.Unix(s.LastSeenAt, 0)) >= touchEvery {
		if err := a.db.TouchSession(ctx, s.ID, now.Unix()); err != nil {
			return nil, err
		}
	}
	p, err := a.db.GetPasskey(ctx, s.PasskeyID)
	if err != nil {
		// Passkey removed: its sessions are gone too (cascade); be safe.
		_ = a.db.DeleteSession(ctx, s.ID)
		return nil, apierr.Unauthorized("unauthenticated", "sign in first")
	}
	return &SessionInfo{ID: s.ID, PasskeyID: p.ID, PasskeyName: p.Name}, nil
}

// Logout deletes one session.
func (a *Admin) Logout(ctx context.Context, s *SessionInfo) error {
	if err := a.db.DeleteSession(ctx, s.ID); err != nil {
		return err
	}
	return a.db.InsertAudit(ctx, &store.AuditEvent{TS: a.now().Unix(), Actor: "admin:" + s.PasskeyName, Action: "admin.logout"})
}

// RevokeAllSessions is "Sign out everywhere".
func (a *Admin) RevokeAllSessions(ctx context.Context, s *SessionInfo) error {
	n, err := a.db.DeleteAllSessions(ctx)
	if err != nil {
		return err
	}
	a.mu.Lock()
	for id, c := range a.ceremonies {
		if c.sessionID != 0 {
			delete(a.ceremonies, id)
		}
	}
	a.mu.Unlock()
	return a.db.InsertAudit(ctx, &store.AuditEvent{TS: a.now().Unix(), Actor: "admin:" + s.PasskeyName, Action: "admin.sessions_revoked",
		Details: map[string]any{"count": n}})
}

// Sweep deletes stale sessions and expired ceremonies.
func (a *Admin) Sweep(ctx context.Context) error {
	now := a.now()
	a.mu.Lock()
	for id, c := range a.ceremonies {
		if now.After(c.expires) {
			delete(a.ceremonies, id)
		}
	}
	a.mu.Unlock()
	return a.db.DeleteStaleSessions(ctx, now.Unix(), now.Add(-SessionIdle).Unix())
}

// ---------------------------------------------------------------- step-up

// StepUpBegin stores action (opaque to this package) bound to the session
// with a one-time challenge. summary restates the action for the UI.
func (a *Admin) StepUpBegin(ctx context.Context, s *SessionInfo, action any, summary string) (string, json.RawMessage, time.Time, error) {
	_, assertion, data, err := a.assertionOptions(ctx)
	if err != nil {
		return "", nil, time.Time{}, err
	}
	id, err := a.putCeremony(&ceremony{kind: kindStepUp, data: *data, sessionID: s.ID, action: action, summary: summary})
	if err != nil {
		return "", nil, time.Time{}, err
	}
	opts, err := marshalOptions(assertion.Response)
	return id, opts, a.now().Add(CeremonyTTL), err
}

// StepUpFinish verifies the tap and returns exactly the stored action and the
// passkey used. The challenge is single-use, expires after 5 minutes and only
// works for the session that started it.
func (a *Admin) StepUpFinish(ctx context.Context, s *SessionInfo, id string, credential []byte) (any, *store.Passkey, error) {
	c, err := a.takeCeremony(id, kindStepUp)
	if err != nil {
		return nil, nil, err
	}
	if c.sessionID != s.ID {
		return nil, nil, apierr.Forbidden("wrong_session", "this passkey prompt was started in another session")
	}
	p, err := a.verifyAssertion(ctx, c, credential)
	if err != nil {
		return nil, nil, err
	}
	return c.action, p, nil
}

// ---------------------------------------------------------------- passkey management

// RegisterBegin starts adding a passkey. Call only after an add_passkey
// step-up; the registration is bound to the session.
func (a *Admin) RegisterBegin(ctx context.Context, s *SessionInfo, name string) (string, json.RawMessage, error) {
	name, err := validPasskeyName(name)
	if err != nil {
		return "", nil, err
	}
	u, err := a.user(ctx)
	if err != nil {
		return "", nil, err
	}
	creation, data, err := a.registrationOptions(ctx, u)
	if err != nil {
		return "", nil, err
	}
	id, err := a.putCeremony(&ceremony{kind: kindRegister, data: *data, sessionID: s.ID, passkeyName: name})
	if err != nil {
		return "", nil, err
	}
	opts, err := marshalOptions(creation.Response)
	return id, opts, err
}

// RegisterFinish verifies and stores a new passkey.
func (a *Admin) RegisterFinish(ctx context.Context, s *SessionInfo, id string, credential []byte) (*store.Passkey, error) {
	c, err := a.takeCeremony(id, kindRegister)
	if err != nil {
		return nil, err
	}
	if c.sessionID != s.ID {
		return nil, apierr.Forbidden("wrong_session", "this passkey prompt was started in another session")
	}
	u, err := a.user(ctx)
	if err != nil {
		return nil, err
	}
	cred, err := a.verifyRegistration(u, c, credential)
	if err != nil {
		return nil, err
	}
	var p *store.Passkey
	err = a.db.WithTx(ctx, func(tx *store.Tx) error {
		var err error
		p, err = a.storePasskey(ctx, tx, c.passkeyName, cred)
		if err != nil {
			return err
		}
		return tx.InsertAudit(ctx, &store.AuditEvent{TS: a.now().Unix(), Actor: "admin:" + s.PasskeyName, Action: "passkey.added",
			Details: map[string]any{"passkey": p.Name, "passkey_id": p.ID}})
	})
	return p, err
}

// CheckRemovePasskey validates removal (step-up begin). The last passkey
// can't be removed; use `lor-server admin reset` instead.
func (a *Admin) CheckRemovePasskey(ctx context.Context, id int64) (*store.Passkey, error) {
	p, err := a.db.GetPasskey(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, apierr.NotFound("passkey_not_found", "passkey not found")
	}
	if err != nil {
		return nil, err
	}
	n, err := a.db.CountPasskeys(ctx)
	if err != nil {
		return nil, err
	}
	if n <= 1 {
		return nil, apierr.Conflict("last_passkey", "you can't remove your only passkey; add another first")
	}
	return p, nil
}

// RemovePasskey deletes a passkey and its sessions (after step-up). The
// checks run inside the deleting transaction, so two concurrent removals
// can't leave zero passkeys.
func (a *Admin) RemovePasskey(ctx context.Context, actor string, id int64) error {
	return a.db.WithTx(ctx, func(tx *store.Tx) error {
		p, err := tx.GetPasskey(ctx, id)
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("passkey_not_found", "passkey not found")
		}
		if err != nil {
			return err
		}
		n, err := tx.CountPasskeys(ctx)
		if err != nil {
			return err
		}
		if n <= 1 {
			return apierr.Conflict("last_passkey", "you can't remove your only passkey; add another first")
		}
		if err := tx.DeletePasskey(ctx, id); err != nil {
			return err
		}
		return tx.InsertAudit(ctx, &store.AuditEvent{TS: a.now().Unix(), Actor: actor, Action: "passkey.removed",
			Details: map[string]any{"passkey": p.Name, "passkey_id": p.ID}})
	})
}

// RenamePasskey renames a passkey (no step-up: it grants nothing).
func (a *Admin) RenamePasskey(ctx context.Context, actor string, id int64, name string) (*store.Passkey, error) {
	name, err := validPasskeyName(name)
	if err != nil {
		return nil, err
	}
	err = a.db.WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.RenamePasskey(ctx, id, name); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return apierr.NotFound("passkey_not_found", "passkey not found")
			}
			return err
		}
		return tx.InsertAudit(ctx, &store.AuditEvent{TS: a.now().Unix(), Actor: actor, Action: "passkey.renamed",
			Details: map[string]any{"passkey_id": id, "name": name}})
	})
	if err != nil {
		return nil, err
	}
	return a.db.GetPasskey(ctx, id)
}

// Reset deletes all passkeys and sessions (lor-server admin reset).
func Reset(ctx context.Context, db *store.DB, now time.Time) error {
	return db.WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.ResetAdmin(ctx); err != nil {
			return err
		}
		return tx.InsertAudit(ctx, &store.AuditEvent{TS: now.Unix(), Actor: "console", Action: "admin.reset"})
	})
}
