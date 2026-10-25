-- Lor schema v1 (plan section 8). All timestamps are Unix seconds.

CREATE TABLE agents (
    id                     INTEGER PRIMARY KEY,
    name                   TEXT NOT NULL UNIQUE,
    status                 TEXT NOT NULL CHECK (status IN ('active', 'disabled')),
    lan_addrs              TEXT NOT NULL,          -- JSON array of IPs
    token_hash             BLOB UNIQUE,            -- NULL until enrolled
    token_created_at       INTEGER,
    enroll_code_hash       BLOB UNIQUE,
    enroll_code_expires_at INTEGER,                -- NULL after enrollment
    created_at             INTEGER NOT NULL,
    last_seen_at           INTEGER
);

CREATE TABLE hosts (
    id               INTEGER PRIMARY KEY,
    name             TEXT NOT NULL UNIQUE,        -- principal base
    address          TEXT NOT NULL,               -- LAN IP
    host_key         TEXT,                        -- authorized_keys format, ed25519
    host_key_seen_at INTEGER,
    max_level        TEXT NOT NULL CHECK (max_level IN ('user', 'sudo')),
    hypervisor       INTEGER NOT NULL DEFAULT 0,
    lor_infra        INTEGER NOT NULL DEFAULT 0,
    notes            TEXT NOT NULL DEFAULT '',
    disabled         INTEGER NOT NULL DEFAULT 0,
    created_at       INTEGER NOT NULL
);

CREATE TABLE grants (
    id                   INTEGER PRIMARY KEY,
    agent_id             INTEGER NOT NULL REFERENCES agents(id),
    host_id              INTEGER NOT NULL REFERENCES hosts(id),
    origin               TEXT NOT NULL CHECK (origin IN ('agent_request', 'admin_offer')),
    level                TEXT NOT NULL CHECK (level IN ('user', 'sudo')),
    requested_level      TEXT NOT NULL CHECK (requested_level IN ('user', 'sudo')),
    requested_duration_s INTEGER NOT NULL,
    approved_duration_s  INTEGER,
    reason               TEXT NOT NULL DEFAULT '',
    public_key           TEXT,                    -- NULL until claimed
    public_key_fp        TEXT,
    status               TEXT NOT NULL CHECK (status IN (
                             'pending', 'offered', 'active', 'denied', 'expired',
                             'released', 'revoked', 'request_timeout', 'offer_expired')),
    decided_by           TEXT,
    decision_note        TEXT,
    requested_at         INTEGER NOT NULL,
    decided_at           INTEGER,
    valid_after          INTEGER,
    valid_before         INTEGER,                 -- current end; moves on extend
    ended_at             INTEGER
);
CREATE INDEX grants_agent_status ON grants(agent_id, status);
CREATE INDEX grants_status ON grants(status);

CREATE TABLE certificates (
    id           INTEGER PRIMARY KEY,             -- = SSH serial
    grant_id     INTEGER NOT NULL REFERENCES grants(id),
    valid_after  INTEGER NOT NULL,
    valid_before INTEGER NOT NULL,
    certificate  TEXT NOT NULL,
    issued_at    INTEGER NOT NULL,
    issued_by    TEXT NOT NULL
);
CREATE INDEX certificates_grant ON certificates(grant_id);

CREATE TABLE extension_requests (
    id                   INTEGER PRIMARY KEY,
    grant_id             INTEGER NOT NULL REFERENCES grants(id),
    requested_duration_s INTEGER NOT NULL,
    approved_duration_s  INTEGER,
    reason               TEXT NOT NULL,
    status               TEXT NOT NULL CHECK (status IN (
                             'pending', 'approved', 'denied', 'request_timeout', 'moot')),
    requested_at         INTEGER NOT NULL,
    decided_at           INTEGER,
    decided_by           TEXT,
    decision_note        TEXT
);
CREATE INDEX extension_requests_grant ON extension_requests(grant_id);
CREATE INDEX extension_requests_status ON extension_requests(status);

CREATE TABLE command_log (
    id             INTEGER PRIMARY KEY,
    grant_id       INTEGER NOT NULL REFERENCES grants(id),
    certificate_id INTEGER REFERENCES certificates(id),
    argv           TEXT NOT NULL,                 -- JSON; [] = interactive shell
    reported_at    INTEGER NOT NULL
);
CREATE INDEX command_log_grant ON command_log(grant_id);
CREATE INDEX command_log_reported ON command_log(reported_at);

CREATE TABLE admin_passkeys (
    id              INTEGER PRIMARY KEY,
    name            TEXT NOT NULL,
    credential_id   BLOB NOT NULL UNIQUE,
    public_key      BLOB NOT NULL,
    sign_count      INTEGER NOT NULL DEFAULT 0,
    aaguid          BLOB,
    transports      TEXT NOT NULL DEFAULT '[]',   -- JSON
    credential_json TEXT NOT NULL,                -- full go-webauthn credential (flags, attestation)
    created_at      INTEGER NOT NULL,
    last_used_at    INTEGER
);

CREATE TABLE admin_sessions (
    id            INTEGER PRIMARY KEY,
    token_hash    BLOB NOT NULL UNIQUE,
    passkey_id    INTEGER REFERENCES admin_passkeys(id) ON DELETE CASCADE,
    created_at    INTEGER NOT NULL,
    last_seen_at  INTEGER NOT NULL,
    expires_at    INTEGER NOT NULL,
    user_agent    TEXT NOT NULL DEFAULT '',
    forwarded_for TEXT NOT NULL DEFAULT ''
);

-- One-time admin setup codes (lor-server admin setup-code).
CREATE TABLE admin_setup_codes (
    id         INTEGER PRIMARY KEY,
    code_hash  BLOB NOT NULL UNIQUE,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    used_at    INTEGER
);

-- Server-wide values: WebAuthn user handle, bootstrap-link HMAC key.
CREATE TABLE meta (
    key   TEXT PRIMARY KEY,
    value BLOB NOT NULL
);

CREATE TABLE audit_events (
    id           INTEGER PRIMARY KEY,
    ts           INTEGER NOT NULL,
    actor        TEXT NOT NULL,
    action       TEXT NOT NULL,
    grant_id     INTEGER,
    host_id      INTEGER,
    agent_id     INTEGER,
    details_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX audit_events_ts ON audit_events(ts);
CREATE INDEX audit_events_grant ON audit_events(grant_id);

-- Audit log is append-only.
CREATE TRIGGER audit_events_no_update BEFORE UPDATE ON audit_events
BEGIN SELECT RAISE(ABORT, 'audit_events is append-only'); END;
CREATE TRIGGER audit_events_no_delete BEFORE DELETE ON audit_events
BEGIN SELECT RAISE(ABORT, 'audit_events is append-only'); END;
