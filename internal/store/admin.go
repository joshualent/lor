package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------- passkeys

// Passkey is a registered admin WebAuthn credential.
type Passkey struct {
	ID             int64
	Name           string
	CredentialID   []byte
	PublicKey      []byte
	SignCount      uint32
	AAGUID         []byte
	Transports     []string
	CredentialJSON string
	CreatedAt      int64
	LastUsedAt     int64
}

const passkeyCols = `id, name, credential_id, public_key, sign_count, COALESCE(aaguid, x''), transports, credential_json, created_at, COALESCE(last_used_at,0)`

func scanPasskey(s scanner) (*Passkey, error) {
	var p Passkey
	var transports string
	if err := s.Scan(&p.ID, &p.Name, &p.CredentialID, &p.PublicKey, &p.SignCount, &p.AAGUID, &transports, &p.CredentialJSON, &p.CreatedAt, &p.LastUsedAt); err != nil {
		return nil, notFound(err)
	}
	if err := json.Unmarshal([]byte(transports), &p.Transports); err != nil {
		return nil, err
	}
	return &p, nil
}

// CreatePasskey inserts a passkey and sets its ID.
func (q q) CreatePasskey(ctx context.Context, p *Passkey) error {
	tr := p.Transports
	if tr == nil {
		tr = []string{}
	}
	b, _ := json.Marshal(tr)
	res, err := q.db.ExecContext(ctx, `INSERT INTO admin_passkeys (name, credential_id, public_key, sign_count, aaguid, transports, credential_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, p.Name, p.CredentialID, p.PublicKey, p.SignCount, nullBytes(p.AAGUID), string(b), p.CredentialJSON, p.CreatedAt)
	if err != nil {
		return err
	}
	p.ID, err = res.LastInsertId()
	return err
}

// ListPasskeys returns all passkeys, oldest first.
func (q q) ListPasskeys(ctx context.Context) ([]*Passkey, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT `+passkeyCols+` FROM admin_passkeys ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Passkey
	for rows.Next() {
		p, err := scanPasskey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPasskey returns a passkey by ID.
func (q q) GetPasskey(ctx context.Context, id int64) (*Passkey, error) {
	return scanPasskey(q.db.QueryRowContext(ctx, `SELECT `+passkeyCols+` FROM admin_passkeys WHERE id = ?`, id))
}

// GetPasskeyByCredentialID returns a passkey by WebAuthn credential ID.
func (q q) GetPasskeyByCredentialID(ctx context.Context, credID []byte) (*Passkey, error) {
	return scanPasskey(q.db.QueryRowContext(ctx, `SELECT `+passkeyCols+` FROM admin_passkeys WHERE credential_id = ?`, credID))
}

// CountPasskeys returns the number of registered passkeys.
func (q q) CountPasskeys(ctx context.Context) (int, error) {
	var n int
	err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_passkeys`).Scan(&n)
	return n, err
}

// UpdatePasskeyUse stores the new sign count and credential state after a
// successful assertion.
func (q q) UpdatePasskeyUse(ctx context.Context, id int64, signCount uint32, credJSON string, now int64) error {
	return affected(q.db.ExecContext(ctx, `UPDATE admin_passkeys SET sign_count = ?, credential_json = ?, last_used_at = ? WHERE id = ?`, signCount, credJSON, now, id))
}

// RenamePasskey renames a passkey.
func (q q) RenamePasskey(ctx context.Context, id int64, name string) error {
	return affected(q.db.ExecContext(ctx, `UPDATE admin_passkeys SET name = ? WHERE id = ?`, name, id))
}

// DeletePasskey removes a passkey and (by cascade) its sessions.
func (q q) DeletePasskey(ctx context.Context, id int64) error {
	return affected(q.db.ExecContext(ctx, `DELETE FROM admin_passkeys WHERE id = ?`, id))
}

// ---------------------------------------------------------------- sessions

// Session is an admin login session.
type Session struct {
	ID           int64
	TokenHash    []byte
	PasskeyID    int64
	CreatedAt    int64
	LastSeenAt   int64
	ExpiresAt    int64
	UserAgent    string
	ForwardedFor string
}

const sessionCols = `id, token_hash, COALESCE(passkey_id,0), created_at, last_seen_at, expires_at, user_agent, forwarded_for`

func scanSession(s scanner) (*Session, error) {
	var x Session
	if err := s.Scan(&x.ID, &x.TokenHash, &x.PasskeyID, &x.CreatedAt, &x.LastSeenAt, &x.ExpiresAt, &x.UserAgent, &x.ForwardedFor); err != nil {
		return nil, notFound(err)
	}
	return &x, nil
}

// CreateSession inserts a session and sets its ID.
func (q q) CreateSession(ctx context.Context, s *Session) error {
	res, err := q.db.ExecContext(ctx, `INSERT INTO admin_sessions (token_hash, passkey_id, created_at, last_seen_at, expires_at, user_agent, forwarded_for)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, s.TokenHash, nullInt(s.PasskeyID), s.CreatedAt, s.LastSeenAt, s.ExpiresAt, s.UserAgent, s.ForwardedFor)
	if err != nil {
		return err
	}
	s.ID, err = res.LastInsertId()
	return err
}

// GetSessionByTokenHash returns the session for a token hash.
func (q q) GetSessionByTokenHash(ctx context.Context, h []byte) (*Session, error) {
	return scanSession(q.db.QueryRowContext(ctx, `SELECT `+sessionCols+` FROM admin_sessions WHERE token_hash = ?`, h))
}

// ListSessions returns all sessions, newest first.
func (q q) ListSessions(ctx context.Context) ([]*Session, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT `+sessionCols+` FROM admin_sessions ORDER BY last_seen_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// TouchSession updates last_seen_at.
func (q q) TouchSession(ctx context.Context, id, now int64) error {
	_, err := q.db.ExecContext(ctx, `UPDATE admin_sessions SET last_seen_at = ? WHERE id = ?`, now, id)
	return err
}

// DeleteSession deletes one session.
func (q q) DeleteSession(ctx context.Context, id int64) error {
	_, err := q.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE id = ?`, id)
	return err
}

// DeleteAllSessions deletes every session ("Sign out everywhere").
func (q q) DeleteAllSessions(ctx context.Context) (int64, error) {
	res, err := q.db.ExecContext(ctx, `DELETE FROM admin_sessions`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteStaleSessions deletes sessions past their absolute or idle expiry.
func (q q) DeleteStaleSessions(ctx context.Context, now, idleCutoff int64) error {
	_, err := q.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE expires_at <= ? OR last_seen_at <= ?`, now, idleCutoff)
	return err
}

// ResetAdmin deletes all passkeys, sessions and setup codes
// (lor-server admin reset).
func (q q) ResetAdmin(ctx context.Context) error {
	for _, stmt := range []string{`DELETE FROM admin_sessions`, `DELETE FROM admin_passkeys`, `DELETE FROM admin_setup_codes`} {
		if _, err := q.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- setup codes

// CreateSetupCode stores a new one-time setup code, invalidating older ones.
func (q q) CreateSetupCode(ctx context.Context, hash []byte, now, expiresAt int64) error {
	if _, err := q.db.ExecContext(ctx, `DELETE FROM admin_setup_codes WHERE used_at IS NULL`); err != nil {
		return err
	}
	_, err := q.db.ExecContext(ctx, `INSERT INTO admin_setup_codes (code_hash, created_at, expires_at) VALUES (?, ?, ?)`, hash, now, expiresAt)
	return err
}

// CheckSetupCode reports whether an unused, unexpired code exists.
func (q q) CheckSetupCode(ctx context.Context, hash []byte, now int64) (bool, error) {
	var n int
	err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_setup_codes WHERE code_hash = ? AND used_at IS NULL AND expires_at > ?`, hash, now).Scan(&n)
	return n > 0, err
}

// ConsumeSetupCode marks a code used. It fails with ErrConflict if the code is
// unknown, used or expired.
func (q q) ConsumeSetupCode(ctx context.Context, hash []byte, now int64) error {
	return affected(q.db.ExecContext(ctx, `UPDATE admin_setup_codes SET used_at = ? WHERE code_hash = ? AND used_at IS NULL AND expires_at > ?`, now, hash, now))
}

// ---------------------------------------------------------------- meta

// GetOrCreateMeta returns meta[key], storing gen() first if it's absent.
func (q q) GetOrCreateMeta(ctx context.Context, key string, gen func() []byte) ([]byte, error) {
	var v []byte
	err := q.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if err == nil {
		return v, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if _, err := q.db.ExecContext(ctx, `INSERT OR IGNORE INTO meta (key, value) VALUES (?, ?)`, key, gen()); err != nil {
		return nil, err
	}
	err = q.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	return v, err
}

// ---------------------------------------------------------------- audit

// AuditEvent is one append-only audit record.
type AuditEvent struct {
	ID      int64
	TS      int64
	Actor   string
	Action  string
	GrantID int64
	HostID  int64
	AgentID int64
	Details map[string]any
}

// InsertAudit appends an audit event.
func (q q) InsertAudit(ctx context.Context, e *AuditEvent) error {
	details := e.Details
	if details == nil {
		details = map[string]any{}
	}
	b, err := json.Marshal(details)
	if err != nil {
		return err
	}
	res, err := q.db.ExecContext(ctx, `INSERT INTO audit_events (ts, actor, action, grant_id, host_id, agent_id, details_json) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.TS, e.Actor, e.Action, nullInt(e.GrantID), nullInt(e.HostID), nullInt(e.AgentID), string(b))
	if err != nil {
		return err
	}
	e.ID, err = res.LastInsertId()
	return err
}

// AuditFilter filters ListAudit. Zero values mean "any".
type AuditFilter struct {
	Actor    string // prefix match, e.g. "agent:" or "admin:"
	Action   string // prefix match, e.g. "grant."
	GrantID  int64
	HostID   int64
	AgentID  int64
	BeforeID int64
	Limit    int
}

// ListAudit returns audit events, newest first.
func (q q) ListAudit(ctx context.Context, f AuditFilter) ([]*AuditEvent, error) {
	var where []string
	var args []any
	like := func(col, prefix string) {
		where = append(where, col+` LIKE ? ESCAPE '\'`)
		r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
		args = append(args, r.Replace(prefix)+"%")
	}
	if f.Actor != "" {
		like("actor", f.Actor)
	}
	if f.Action != "" {
		like("action", f.Action)
	}
	for col, v := range map[string]int64{"grant_id": f.GrantID, "host_id": f.HostID, "agent_id": f.AgentID} {
		if v != 0 {
			where = append(where, col+" = ?")
			args = append(args, v)
		}
	}
	if f.BeforeID != 0 {
		where = append(where, "id < ?")
		args = append(args, f.BeforeID)
	}
	query := `SELECT id, ts, actor, action, COALESCE(grant_id,0), COALESCE(host_id,0), COALESCE(agent_id,0), details_json FROM audit_events`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query += fmt.Sprintf(" ORDER BY id DESC LIMIT %d", limit)
	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AuditEvent
	for rows.Next() {
		var e AuditEvent
		var details string
		if err := rows.Scan(&e.ID, &e.TS, &e.Actor, &e.Action, &e.GrantID, &e.HostID, &e.AgentID, &details); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(details), &e.Details); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}
