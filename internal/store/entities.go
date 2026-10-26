package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------- agents

// Agent is an enrolled (or enrolling) agent LXC.
type Agent struct {
	ID                  int64
	Name                string
	Status              string // active | disabled
	LANAddrs            []string
	TokenHash           []byte
	TokenCreatedAt      int64
	EnrollCodeHash      []byte
	EnrollCodeExpiresAt int64
	CreatedAt           int64
	LastSeenAt          int64
}

const agentCols = `id, name, status, lan_addrs, token_hash, COALESCE(token_created_at,0),
	enroll_code_hash, COALESCE(enroll_code_expires_at,0), created_at, COALESCE(last_seen_at,0)`

func scanAgent(s scanner) (*Agent, error) {
	var a Agent
	var addrs string
	if err := s.Scan(&a.ID, &a.Name, &a.Status, &addrs, &a.TokenHash, &a.TokenCreatedAt,
		&a.EnrollCodeHash, &a.EnrollCodeExpiresAt, &a.CreatedAt, &a.LastSeenAt); err != nil {
		return nil, notFound(err)
	}
	if err := json.Unmarshal([]byte(addrs), &a.LANAddrs); err != nil {
		return nil, fmt.Errorf("agent %d lan_addrs: %w", a.ID, err)
	}
	return &a, nil
}

func marshalAddrs(addrs []string) string {
	if addrs == nil {
		addrs = []string{}
	}
	b, _ := json.Marshal(addrs)
	return string(b)
}

// CreateAgent inserts an agent and sets its ID.
func (q q) CreateAgent(ctx context.Context, a *Agent) error {
	res, err := q.db.ExecContext(ctx, `INSERT INTO agents (name, status, lan_addrs, enroll_code_hash, enroll_code_expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, a.Name, a.Status, marshalAddrs(a.LANAddrs), nullBytes(a.EnrollCodeHash), nullInt(a.EnrollCodeExpiresAt), a.CreatedAt)
	if err != nil {
		return err
	}
	a.ID, err = res.LastInsertId()
	return err
}

// GetAgent returns an agent by ID.
func (q q) GetAgent(ctx context.Context, id int64) (*Agent, error) {
	return scanAgent(q.db.QueryRowContext(ctx, `SELECT `+agentCols+` FROM agents WHERE id = ?`, id))
}

// GetAgentByName returns an agent by name.
func (q q) GetAgentByName(ctx context.Context, name string) (*Agent, error) {
	return scanAgent(q.db.QueryRowContext(ctx, `SELECT `+agentCols+` FROM agents WHERE name = ?`, name))
}

// GetAgentByTokenHash returns the agent holding a token.
func (q q) GetAgentByTokenHash(ctx context.Context, h []byte) (*Agent, error) {
	return scanAgent(q.db.QueryRowContext(ctx, `SELECT `+agentCols+` FROM agents WHERE token_hash = ?`, h))
}

// GetAgentByEnrollCodeHash returns the agent with a pending enrollment code.
func (q q) GetAgentByEnrollCodeHash(ctx context.Context, h []byte) (*Agent, error) {
	return scanAgent(q.db.QueryRowContext(ctx, `SELECT `+agentCols+` FROM agents WHERE enroll_code_hash = ?`, h))
}

// ListAgents returns all agents ordered by name.
func (q q) ListAgents(ctx context.Context) ([]*Agent, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT `+agentCols+` FROM agents ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetAgentEnrollCode sets a new enrollment code and revokes the current token
// (rotation means re-enrollment).
func (q q) SetAgentEnrollCode(ctx context.Context, id int64, hash []byte, expiresAt int64) error {
	return affected(q.db.ExecContext(ctx, `UPDATE agents SET enroll_code_hash = ?, enroll_code_expires_at = ?,
		token_hash = NULL, token_created_at = NULL WHERE id = ?`, hash, expiresAt, id))
}

// CompleteEnrollment atomically consumes the enrollment code and stores the
// token hash. It fails with ErrConflict if the code was already used.
func (q q) CompleteEnrollment(ctx context.Context, id int64, codeHash, tokenHash []byte, now int64) error {
	return affected(q.db.ExecContext(ctx, `UPDATE agents SET token_hash = ?, token_created_at = ?,
		enroll_code_hash = NULL, enroll_code_expires_at = NULL
		WHERE id = ? AND enroll_code_hash = ? AND enroll_code_expires_at > ?`, tokenHash, now, id, codeHash, now))
}

// SetAgentStatus sets active or disabled.
func (q q) SetAgentStatus(ctx context.Context, id int64, status string) error {
	return affected(q.db.ExecContext(ctx, `UPDATE agents SET status = ? WHERE id = ?`, status, id))
}

// SetAgentLANAddrs replaces the agent's registered LAN IPs.
func (q q) SetAgentLANAddrs(ctx context.Context, id int64, addrs []string) error {
	return affected(q.db.ExecContext(ctx, `UPDATE agents SET lan_addrs = ? WHERE id = ?`, marshalAddrs(addrs), id))
}

// TouchAgent records the agent's last API call.
func (q q) TouchAgent(ctx context.Context, id, now int64) error {
	_, err := q.db.ExecContext(ctx, `UPDATE agents SET last_seen_at = ? WHERE id = ?`, now, id)
	return err
}

// ---------------------------------------------------------------- hosts

// Host is an SSH target.
type Host struct {
	ID            int64
	Name          string
	Address       string
	HostKey       string
	HostKeySeenAt int64
	MaxLevel      string // user | sudo
	Hypervisor    bool
	LorInfra      bool
	Notes         string
	Disabled      bool
	CreatedAt     int64
}

const hostCols = `id, name, address, COALESCE(host_key,''), COALESCE(host_key_seen_at,0), max_level,
	hypervisor, lor_infra, notes, disabled, created_at`

func scanHost(s scanner) (*Host, error) {
	var h Host
	if err := s.Scan(&h.ID, &h.Name, &h.Address, &h.HostKey, &h.HostKeySeenAt, &h.MaxLevel,
		&h.Hypervisor, &h.LorInfra, &h.Notes, &h.Disabled, &h.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	return &h, nil
}

// CreateHost inserts a host and sets its ID.
func (q q) CreateHost(ctx context.Context, h *Host) error {
	res, err := q.db.ExecContext(ctx, `INSERT INTO hosts (name, address, host_key, host_key_seen_at, max_level, hypervisor, lor_infra, notes, disabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, h.Name, h.Address, nullStr(h.HostKey), nullInt(h.HostKeySeenAt), h.MaxLevel,
		b2i(h.Hypervisor), b2i(h.LorInfra), h.Notes, b2i(h.Disabled), h.CreatedAt)
	if err != nil {
		return err
	}
	h.ID, err = res.LastInsertId()
	return err
}

// GetHost returns a host by ID.
func (q q) GetHost(ctx context.Context, id int64) (*Host, error) {
	return scanHost(q.db.QueryRowContext(ctx, `SELECT `+hostCols+` FROM hosts WHERE id = ?`, id))
}

// GetHostByName returns a host by name.
func (q q) GetHostByName(ctx context.Context, name string) (*Host, error) {
	return scanHost(q.db.QueryRowContext(ctx, `SELECT `+hostCols+` FROM hosts WHERE name = ?`, name))
}

// ListHosts returns all hosts ordered by name.
func (q q) ListHosts(ctx context.Context) ([]*Host, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT `+hostCols+` FROM hosts ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Host
	for rows.Next() {
		h, err := scanHost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// UpdateHost writes the editable host fields (not the host key).
func (q q) UpdateHost(ctx context.Context, h *Host) error {
	return affected(q.db.ExecContext(ctx, `UPDATE hosts SET name = ?, address = ?, max_level = ?, hypervisor = ?, lor_infra = ?, notes = ?, disabled = ? WHERE id = ?`,
		h.Name, h.Address, h.MaxLevel, b2i(h.Hypervisor), b2i(h.LorInfra), h.Notes, b2i(h.Disabled), h.ID))
}

// SetFirstHostKey stores a host's first key, only while it still has none
// and its address is unchanged (concurrent scans can't overwrite each other).
func (q q) SetFirstHostKey(ctx context.Context, id int64, address, key string, seenAt int64) error {
	return affected(q.db.ExecContext(ctx, `UPDATE hosts SET host_key = ?, host_key_seen_at = ? WHERE id = ? AND host_key IS NULL AND address = ?`,
		key, seenAt, id, address))
}

// SetHostKey stores a scanned host key.
func (q q) SetHostKey(ctx context.Context, id int64, key string, seenAt int64) error {
	return affected(q.db.ExecContext(ctx, `UPDATE hosts SET host_key = ?, host_key_seen_at = ? WHERE id = ?`, nullStr(key), nullInt(seenAt), id))
}

// ---------------------------------------------------------------- grants

// Grant statuses.
const (
	StatusPending        = "pending"
	StatusOffered        = "offered"
	StatusActive         = "active"
	StatusDenied         = "denied"
	StatusExpired        = "expired"
	StatusReleased       = "released"
	StatusRevoked        = "revoked"
	StatusRequestTimeout = "request_timeout"
	StatusOfferExpired   = "offer_expired"
)

// Grant is one access grant (agent request or admin offer).
type Grant struct {
	ID                 int64
	AgentID            int64
	HostID             int64
	Origin             string // agent_request | admin_offer
	Level              string
	RequestedLevel     string
	RequestedDurationS int64
	ApprovedDurationS  int64
	Reason             string
	PublicKey          string
	PublicKeyFP        string
	Status             string
	DecidedBy          string
	DecisionNote       string
	RequestedAt        int64
	DecidedAt          int64
	ValidAfter         int64
	ValidBefore        int64
	EndedAt            int64
}

const grantCols = `id, agent_id, host_id, origin, level, requested_level, requested_duration_s,
	COALESCE(approved_duration_s,0), reason, COALESCE(public_key,''), COALESCE(public_key_fp,''), status,
	COALESCE(decided_by,''), COALESCE(decision_note,''), requested_at, COALESCE(decided_at,0),
	COALESCE(valid_after,0), COALESCE(valid_before,0), COALESCE(ended_at,0)`

func scanGrant(s scanner) (*Grant, error) {
	var g Grant
	if err := s.Scan(&g.ID, &g.AgentID, &g.HostID, &g.Origin, &g.Level, &g.RequestedLevel, &g.RequestedDurationS,
		&g.ApprovedDurationS, &g.Reason, &g.PublicKey, &g.PublicKeyFP, &g.Status,
		&g.DecidedBy, &g.DecisionNote, &g.RequestedAt, &g.DecidedAt,
		&g.ValidAfter, &g.ValidBefore, &g.EndedAt); err != nil {
		return nil, notFound(err)
	}
	return &g, nil
}

// CreateGrant inserts a grant and sets its ID.
func (q q) CreateGrant(ctx context.Context, g *Grant) error {
	res, err := q.db.ExecContext(ctx, `INSERT INTO grants (agent_id, host_id, origin, level, requested_level, requested_duration_s,
		approved_duration_s, reason, public_key, public_key_fp, status, decided_by, decision_note, requested_at, decided_at,
		valid_after, valid_before, ended_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		g.AgentID, g.HostID, g.Origin, g.Level, g.RequestedLevel, g.RequestedDurationS,
		nullInt(g.ApprovedDurationS), g.Reason, nullStr(g.PublicKey), nullStr(g.PublicKeyFP), g.Status,
		nullStr(g.DecidedBy), nullStr(g.DecisionNote), g.RequestedAt, nullInt(g.DecidedAt),
		nullInt(g.ValidAfter), nullInt(g.ValidBefore), nullInt(g.EndedAt))
	if err != nil {
		return err
	}
	g.ID, err = res.LastInsertId()
	return err
}

// GetGrant returns a grant by ID.
func (q q) GetGrant(ctx context.Context, id int64) (*Grant, error) {
	return scanGrant(q.db.QueryRowContext(ctx, `SELECT `+grantCols+` FROM grants WHERE id = ?`, id))
}

// UpdateGrant writes all mutable fields of g, but only if the grant is still
// in fromStatus. It returns ErrConflict otherwise. This makes every state
// transition atomic (for example, an offer can be claimed only once).
func (q q) UpdateGrant(ctx context.Context, g *Grant, fromStatus string) error {
	return affected(q.db.ExecContext(ctx, `UPDATE grants SET level = ?, approved_duration_s = ?, public_key = ?, public_key_fp = ?,
		status = ?, decided_by = ?, decision_note = ?, decided_at = ?, valid_after = ?, valid_before = ?, ended_at = ?
		WHERE id = ? AND status = ?`,
		g.Level, nullInt(g.ApprovedDurationS), nullStr(g.PublicKey), nullStr(g.PublicKeyFP),
		g.Status, nullStr(g.DecidedBy), nullStr(g.DecisionNote), nullInt(g.DecidedAt), nullInt(g.ValidAfter), nullInt(g.ValidBefore), nullInt(g.EndedAt),
		g.ID, fromStatus))
}

// GrantFilter filters ListGrants. Zero values mean "any".
type GrantFilter struct {
	AgentID  int64
	HostID   int64
	Statuses []string
	Limit    int
	BeforeID int64
}

// ListGrants returns grants, newest first.
func (q q) ListGrants(ctx context.Context, f GrantFilter) ([]*Grant, error) {
	var where []string
	var args []any
	if f.AgentID != 0 {
		where = append(where, "agent_id = ?")
		args = append(args, f.AgentID)
	}
	if f.HostID != 0 {
		where = append(where, "host_id = ?")
		args = append(args, f.HostID)
	}
	if len(f.Statuses) > 0 {
		where = append(where, "status IN (?"+strings.Repeat(",?", len(f.Statuses)-1)+")")
		for _, s := range f.Statuses {
			args = append(args, s)
		}
	}
	if f.BeforeID != 0 {
		where = append(where, "id < ?")
		args = append(args, f.BeforeID)
	}
	query := `SELECT ` + grantCols + ` FROM grants`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY id DESC"
	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Grant
	for rows.Next() {
		g, err := scanGrant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- certificates

// Certificate is one signed SSH certificate. ID is the SSH serial.
type Certificate struct {
	ID          int64
	GrantID     int64
	ValidAfter  int64
	ValidBefore int64
	Certificate string // authorized_keys format
	IssuedAt    int64
	IssuedBy    string
}

// ReserveCertificate inserts a placeholder row so the ID can be used as the
// SSH serial; FinishCertificate fills in the signed certificate.
func (q q) ReserveCertificate(ctx context.Context, c *Certificate) error {
	res, err := q.db.ExecContext(ctx, `INSERT INTO certificates (grant_id, valid_after, valid_before, certificate, issued_at, issued_by)
		VALUES (?, ?, ?, '', ?, ?)`, c.GrantID, c.ValidAfter, c.ValidBefore, c.IssuedAt, c.IssuedBy)
	if err != nil {
		return err
	}
	c.ID, err = res.LastInsertId()
	return err
}

// FinishCertificate stores the signed certificate.
func (q q) FinishCertificate(ctx context.Context, id int64, cert string) error {
	return affected(q.db.ExecContext(ctx, `UPDATE certificates SET certificate = ? WHERE id = ? AND certificate = ''`, cert, id))
}

const certCols = `id, grant_id, valid_after, valid_before, certificate, issued_at, issued_by`

func scanCert(s scanner) (*Certificate, error) {
	var c Certificate
	if err := s.Scan(&c.ID, &c.GrantID, &c.ValidAfter, &c.ValidBefore, &c.Certificate, &c.IssuedAt, &c.IssuedBy); err != nil {
		return nil, notFound(err)
	}
	return &c, nil
}

// LatestCertificate returns the newest signed certificate of a grant.
func (q q) LatestCertificate(ctx context.Context, grantID int64) (*Certificate, error) {
	return scanCert(q.db.QueryRowContext(ctx, `SELECT `+certCols+` FROM certificates WHERE grant_id = ? AND certificate != '' ORDER BY id DESC LIMIT 1`, grantID))
}

// ListCertificates returns a grant's certificates, oldest first.
func (q q) ListCertificates(ctx context.Context, grantID int64) ([]*Certificate, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT `+certCols+` FROM certificates WHERE grant_id = ? AND certificate != '' ORDER BY id`, grantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Certificate
	for rows.Next() {
		c, err := scanCert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- extension requests

// Extension request statuses.
const (
	ExtPending        = "pending"
	ExtApproved       = "approved"
	ExtDenied         = "denied"
	ExtRequestTimeout = "request_timeout"
	ExtMoot           = "moot"
)

// ExtensionRequest is an agent's request to extend an active grant.
type ExtensionRequest struct {
	ID                 int64
	GrantID            int64
	RequestedDurationS int64
	ApprovedDurationS  int64
	Reason             string
	Status             string
	RequestedAt        int64
	DecidedAt          int64
	DecidedBy          string
	DecisionNote       string
}

const extCols = `id, grant_id, requested_duration_s, COALESCE(approved_duration_s,0), reason, status, requested_at,
	COALESCE(decided_at,0), COALESCE(decided_by,''), COALESCE(decision_note,'')`

func scanExt(s scanner) (*ExtensionRequest, error) {
	var e ExtensionRequest
	if err := s.Scan(&e.ID, &e.GrantID, &e.RequestedDurationS, &e.ApprovedDurationS, &e.Reason, &e.Status, &e.RequestedAt,
		&e.DecidedAt, &e.DecidedBy, &e.DecisionNote); err != nil {
		return nil, notFound(err)
	}
	return &e, nil
}

// CreateExtension inserts an extension request and sets its ID.
func (q q) CreateExtension(ctx context.Context, e *ExtensionRequest) error {
	res, err := q.db.ExecContext(ctx, `INSERT INTO extension_requests (grant_id, requested_duration_s, reason, status, requested_at)
		VALUES (?, ?, ?, ?, ?)`, e.GrantID, e.RequestedDurationS, e.Reason, e.Status, e.RequestedAt)
	if err != nil {
		return err
	}
	e.ID, err = res.LastInsertId()
	return err
}

// GetExtension returns an extension request by ID.
func (q q) GetExtension(ctx context.Context, id int64) (*ExtensionRequest, error) {
	return scanExt(q.db.QueryRowContext(ctx, `SELECT `+extCols+` FROM extension_requests WHERE id = ?`, id))
}

// UpdateExtension writes the decision fields if the request is still in
// fromStatus.
func (q q) UpdateExtension(ctx context.Context, e *ExtensionRequest, fromStatus string) error {
	return affected(q.db.ExecContext(ctx, `UPDATE extension_requests SET approved_duration_s = ?, status = ?, decided_at = ?, decided_by = ?, decision_note = ?
		WHERE id = ? AND status = ?`, nullInt(e.ApprovedDurationS), e.Status, nullInt(e.DecidedAt), nullStr(e.DecidedBy), nullStr(e.DecisionNote), e.ID, fromStatus))
}

// ListExtensions returns extension requests, filtered by grant (0 = any) and
// status (empty = any), oldest first.
func (q q) ListExtensions(ctx context.Context, grantID int64, status string) ([]*ExtensionRequest, error) {
	query := `SELECT ` + extCols + ` FROM extension_requests WHERE 1=1`
	var args []any
	if grantID != 0 {
		query += ` AND grant_id = ?`
		args = append(args, grantID)
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	rows, err := q.db.QueryContext(ctx, query+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ExtensionRequest
	for rows.Next() {
		e, err := scanExt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- command log

// CommandLogEntry is one command reported by `lor ssh`.
type CommandLogEntry struct {
	ID            int64
	GrantID       int64
	CertificateID int64
	Argv          []string
	ReportedAt    int64
}

// InsertCommand records a reported command.
func (q q) InsertCommand(ctx context.Context, e *CommandLogEntry) error {
	argv := e.Argv
	if argv == nil {
		argv = []string{}
	}
	b, err := json.Marshal(argv)
	if err != nil {
		return err
	}
	res, err := q.db.ExecContext(ctx, `INSERT INTO command_log (grant_id, certificate_id, argv, reported_at) VALUES (?, ?, ?, ?)`,
		e.GrantID, nullInt(e.CertificateID), string(b), e.ReportedAt)
	if err != nil {
		return err
	}
	e.ID, err = res.LastInsertId()
	return err
}

// ListCommands returns a grant's command log, newest first (limit 0 = all).
func (q q) ListCommands(ctx context.Context, grantID int64, limit int) ([]*CommandLogEntry, error) {
	query := `SELECT id, grant_id, COALESCE(certificate_id,0), argv, reported_at FROM command_log WHERE grant_id = ? ORDER BY id DESC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := q.db.QueryContext(ctx, query, grantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*CommandLogEntry
	for rows.Next() {
		var e CommandLogEntry
		var argv string
		if err := rows.Scan(&e.ID, &e.GrantID, &e.CertificateID, &argv, &e.ReportedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(argv), &e.Argv); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

// DeleteCommandsBefore deletes command-log entries reported before ts.
func (q q) DeleteCommandsBefore(ctx context.Context, ts int64) (int64, error) {
	res, err := q.db.ExecContext(ctx, `DELETE FROM command_log WHERE reported_at < ?`, ts)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
