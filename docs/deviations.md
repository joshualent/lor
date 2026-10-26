# Deviations from the plan (v0.4)

Everything in plan sections 1–15 marked v1 is implemented. These are the
places where the implementation differs from, or adds to, the plan's text,
and why. None of them weakens the security properties the plan lists
(certificate refusal rules, host-specific principals, `source-address`,
`AuthorizedKeysFile none`, proxy-only admin listener, step-up bound to the
exact action, CSRF rules, agent text never rendered as HTML).

## Toolchain and layout

| Plan | Implementation | Why |
|---|---|---|
| SvelteKit 2-style `$lib`, `svelte.config.js` | SvelteKit **3.1** (current): `#lib/…` subpath imports with explicit file extensions, Kit options passed to `sveltekit({...})` in `vite.config.ts` | Kit 3 removed `$lib` and moved the config. shadcn-svelte was initialised with `#lib` aliases. |
| `internal/server/adminapi/` "generated interface + handlers" | Same, plus `internal/server/reqctx` (request context helpers) and `internal/tlscert` (shared by server and CLI) | Small shared packages so the generated packages, the server and the CLI don't import each other. |
| `test/e2e/` Playwright | Specs, global setup and a tiny `fakessh` (Go) live in `test/e2e/`; the Playwright config is `web/playwright.config.ts`; `test/e2e/node_modules` is a committed symlink to `web/node_modules` | Playwright and its types are installed in `web/`; the symlink lets the specs resolve them without a second install. |
| `lor-server` (subcommands only `admin setup-code` / `admin reset`) | `lor-server serve` runs the server | cobra command tree; `serve` keeps the `admin …` commands unambiguous. |
| Go 1.25+ | **Go 1.26+** (`go 1.26.0` in go.mod); developed and CI-tested with Go 1.27 | A dependency (pulled in at its current version) requires Go 1.26. |

## Data model (section 8)

- **Added `admin_passkeys.credential_json`.** go-webauthn needs the full
  credential (backup flags, attestation type) to validate later assertions;
  the plan's columns are kept alongside it.
- **Added tables** `admin_setup_codes` (hashed one-time setup codes),
  `meta` (the WebAuthn user handle and the bootstrap-link HMAC key) and
  `schema_migrations`.
- **Offer notes** are stored in `grants.reason` (an offer has no agent reason).
- **`decided_by`** stores `admin:<passkey name>`, the same string as the audit
  actor.
- `audit_events` is append-only, enforced by SQLite triggers.

## API and behavior

- **Extension consent page `/e/<id>`.** The plan lists `/r/[id]` and
  `/grants/[id]`; an agent's extension request needs its own approval link,
  so `lor extend` prints `<public_url>/e/<id>`. It shows "ends 16:30 (12 min
  left) → would end 17:30" as the plan describes.
- **Host address may include a port** (`192.168.1.20:2222`). Still IP-only
  (DNS names are refused). Needed for non-standard sshd ports and the E2E
  fake SSH server. The agent API's `ssh` block therefore has a `port`.
- **Hosts need a scanned host key** before they can be requested or offered
  (the CLI uses `StrictHostKeyChecking yes` with Lor's key, so a grant without
  one could never be used).
- **Address changes never clear the stored host key**; a rescan that finds a
  different key needs the `accept_host_key` step-up. Otherwise a stolen
  session could re-point a host and "first-scan" an attacker's key.
- **Enrollment must come from a registered LAN IP** (stricter than the plan):
  a leaked enrollment command is useless elsewhere, like the token.
- **`setup-code` refuses while a passkey exists** and `/setup` only works with
  zero passkeys; add more passkeys from Settings (step-up), or `admin reset`
  first. The **last passkey can't be removed** (avoids accidental lockout;
  `admin reset` is the recovery path).
- **Adding a passkey** is an `add_passkey` step-up whose result is a
  registration ceremony bound to the same session.
- **Request reuse/claim details:** an active grant is reused when its level is
  ≥ the requested level; an offer is claimed when its level is ≥ the requested
  level (the grant keeps the offer's level and end time). A second
  `lor extend` while one is pending returns the pending request (`reused`).
- **Admin Extend also works on open offers** (it moves the offer window; no
  certificate exists yet).
- **Disabling an agent ends its open grants and offers** (revoked; pending
  requests denied), and **disabling a host or lowering its max level** ends
  the grants and offers the new settings don't allow. Re-enabling restores
  nothing by itself, so these switches needn't be step-up actions.
- **Renaming a host** (plan section 11 "edit name") is allowed only while it
  has no open grants; the UI reminds you to re-run the bootstrap (the
  principals file holds the old name).
- **API timestamps are RFC 3339**; the database stores Unix seconds.
- **Time-based transitions are applied on read** as well as by the 30 s
  sweeper (each operation first settles the grant in its own transaction), so
  correctness never depends on sweeper timing.
- **`lor ssh` passes the command to ssh unchanged**: like `ssh`, the remote
  shell runs the arguments joined by spaces. The command log shows that
  joined string. (Shell-quoting each argument would break the common
  `lor ssh h -- "a | b"` usage.)
- `lor ssh` also clears `SSH_AUTH_SOCK` and the generated config sets
  `IdentityAgent none`, `ForwardAgent no`, `GlobalKnownHostsFile none` and
  `UpdateHostKeys no` in addition to the plan's entry.

## Web UI (section 11)

- **CSP `style-src` includes `'unsafe-inline'`.** Svelte and bits-ui set
  inline `style` attributes (positioning, transitions); scripts stay
  `'self'` plus hashes, `connect-src 'self'`, as in the plan. lor-server adds
  `frame-ancestors 'none'` as a header (meta tags can't carry it).
- **Dark theme only**, as requested; shadcn variants were trimmed to what the
  UI uses.
- Confirmations use a dialog on wide screens and a bottom drawer on phones.

## Development mode

`--dev` adds command-line-only flags: `--dev-admin-listen` (must be
127.0.0.1), `--dev-agent-listen`, `--dev-data-dir` (default: a temporary
dir), `--dev-origin` (the Vite dev server). It prints a setup code at
startup. The agent listener stays TLS in dev mode so the CLI's pinning is
exercised.

## Bootstrap script (section 4)

- **The drop-in also sets `AuthorizedKeysCommand none` and
  `AuthorizedPrincipalsCommand none`** for the agent users, so a host-wide
  keys/principals command can't add another way in; validation checks both.
- **Validation runs `sshd -T` for loopback and for every registered agent IP**
  (rendered into the script), not only `addr=127.0.0.1`, so a pre-existing
  `Match Address <agent IP>` block can't override Lor's settings unnoticed.

- **`agent-sudo` requires `sudo`, `visudo` and `/etc/sudoers.d`** (checked in
  preflight); otherwise use `--no-sudo`. `visudo -cf` is never skipped.
- **`--force` also adopts pre-existing `agent`/`agent-sudo` accounts** not
  created by Lor (the script refuses them otherwise; their old password hash
  is restored on rollback). Lor-created users are tracked in
  `/etc/ssh/lor/managed-users`.
- The one-liner uses `ssh <host-name>`; adjust it if your SSH alias differs
  (e.g. `ssh root@<ip> sh`).

## Tests (section 13)

- **Integration tests** run the bootstrap script exactly as served by Lor's
  signed link, but pipe it to `docker exec … sh` as root instead of
  `ssh host sudo sh`. The test runner plays Caddy (127.0.0.1 is the
  `trusted_proxy`), so Caddy itself isn't exercised. Debian 13 stands in for
  Proxmox VE 9, as the plan says. `ubuntu:26.04` exists, so no substitute
  distro was needed; its image installs `sudo-rs` explicitly (a real 26.04
  install has it by default) and the build fails if `sudo` isn't sudo-rs.
- Certificate-level cases 1–9 use certificates minted with Lor's CA key by
  the test (so expired or mis-targeted certificates can be built); cases
  12–17 go through the real `lor` CLI and the admin API.
- Case 15 uses 75-second offers to keep the run short.
- Playwright uses the real `lor` CLI as the agent and a small Go `fakessh`
  server for host-key scans.
