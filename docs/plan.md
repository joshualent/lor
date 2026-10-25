# Lor: Implementation Plan (v0.4)

Lor gives AI agents **temporary, approved SSH access** to homelab machines. It is an **SSH certificate authority (CA)**: each grant is a short-lived OpenSSH certificate. Hosts check expiry themselves, so access ends even if Lor crashes or the grant is forgotten.

**v1 in one paragraph:** One agent, running in its own LXC, uses the `lor` CLI to request access to a host, giving a short reason. The CLI prints an approval link. The agent shares the link in chat. The admin opens it, signs in with a passkey, and sees a consent page listing exactly what would be allowed. If the admin authorizes (one more passkey tap), Lor signs a certificate, and `lor ssh <host> -- <cmd>` works until it expires. Every command run through `lor ssh` is reported to Lor and shown on the grant. The admin can also grant access from the web UI before asking the agent for help, and can extend any active grant.

---

## 1. Scope

**v1:**
- Agent requests with an approval link and a consent page (authorize, authorize with changes, deny).
- Admin pre-approval ("offers") from the web UI.
- Extensions, both admin-initiated and agent-requested.
- Revoke and release.
- Self-reported command log from `lor ssh` (section 10).
- Admin web UI: SvelteKit single-page app served by the Go binary, behind Caddy, passkey login (sections 5 and 11).
- Host bootstrap script, host-key scan, audit log.
- Tier 1 OSes for targets: Proxmox VE 8/9, Debian 12/13, Ubuntu 22.04/24.04/26.04 (hosts, LXCs and VMs). Minimum OpenSSH 8.9.

**Not v1:** see section 15.

**Non-goals:**
- Limiting *what* an agent does after it logs in. The access level (`agent` vs `agent-sudo`) is the only control.
- Credentials other than SSH.
- Multiple admins.

---

## 2. Topology and network

**Principle: agents and targets talk only over the LAN. You reach Lor's web UI through Caddy, like every other service** (on the LAN, or remotely through the Tailscale subnet router). This matches the current homelab:
- A subnet router VM serves `192.168.1.0/24`.
- Personal devices run Tailscale.
- Services are LAN-only, behind a Caddy `proxy` LXC.

It also keeps working if Tailscale is later installed on the NAS or on some containers. Lor itself doesn't use Tailscale.

```
 Your devices ──HTTPS──▶ Caddy proxy LXC ──HTTPS :8444 (pinned)──▶ ┌───────────────────────────┐
 (LAN, or off-LAN via    lor.home.example.com                      │ Lor  (Ubuntu 26.04 LXC    │
  the subnet router)                                               │       on the NAS)         │
                                                                   │ :8444 admin API + web UI  │
 ┌────────────────────────┐   LAN HTTPS :8443                      │ :8443 agent API           │
 │ Agent LXC              │ ─────────────────────────────────────▶ │ CA · SQLite               │
 │ lor CLI + skill        │  token + pinned TLS                    └─────────────┬─────────────┘
 │ fixed LAN IP           │                                                      │ LAN :22 (host-key scan)
 └───────────┬────────────┘                                                      ▼
             │ SSH :22 over LAN (certificate)                       Targets at their LAN IPs
             └────────────────────────────────────────────────────▶ (Proxmox hosts, LXCs, VMs)
```

| Path | Network | Authentication |
|---|---|---|
| You → web UI and approval links | `https://lor.home.example.com` via Caddy. Works on and off the LAN. | Passkey login (session cookie), plus a passkey tap for every action that creates or extends access |
| Caddy → Lor admin listener | LAN `:8444`, TLS with Lor's self-signed certificate, pinned in Caddy | Lor accepts this port only from Caddy's LAN IP |
| Agent → Lor API | LAN (`https://<lor-lan-ip>:8443`), **not** through Caddy | Per-agent token + request must come from the agent's registered LAN IP + Lor's self-signed TLS cert pinned by the CLI |
| Agent → targets | LAN | SSH certificate locked to the agent's LAN IP |
| Lor → targets | LAN, port 22, host-key scan only | n/a |

**Rules that keep this robust:**
- **Hosts are always addressed by LAN IP** in Lor, not by DNS names. A name could later resolve to the Caddy proxy or a tailnet address, which would change the path and the source IP.
- **Nothing that needs Tailscale runs in the agent LXC or on the targets.** The agent LXC needs no TUN device and no tailnet tag.
- **If Tailscale is later installed on a target** (the NAS, some container), nothing changes. The agent still connects to the target's LAN IP, so traffic stays on the LAN.
  - Keep `--accept-routes` off on LAN machines. It's off by default on Linux. With it on, a machine that accepts its own LAN's subnet route can start sending LAN traffic through Tailscale.
  - Don't enable Tailscale SSH on targets. It isn't needed.
- **Only the web UI goes through Caddy. The agent API never does:** it checks the caller's LAN IP, and Caddy would hide it.
- **Fails closed:** if agent traffic ever took an unexpected route (for example through the subnet router or Caddy, which shows up as their IP), the source IP check and the certificate's source address would both reject it.

**Proxmox firewall** (this is what limits LAN traffic):
- **Agent LXC, outbound:**
  - Allow tcp/22 to `192.168.1.0/24`, tcp/8443 to Lor's LAN IP, DNS and gateway.
  - Drop other private ranges (RFC1918). This includes the Caddy LXC, so the agent can't even load Lor's web UI.
  - Allow the internet.
  
  With credentials it found, the agent can still only reach SSH, which only accepts Lor certificates for the agent users. Databases, web UIs and file shares stay unreachable. Add explicit allows for any mounts the agent LXC needs.
- **Sandbox LXCs, outbound:** drop RFC1918 except DNS and gateway.
- **Lor LXC, inbound:** allow tcp/8443 from the agent LXC(s) only and tcp/8444 from the Caddy LXC only. Drop everything else.
- **The agent LXC, Lor and Caddy need fixed LAN IPs** (static or DHCP reservations).

**Agent placement (decided):** agent LXCs may run on any Proxmox host, **including the NAS**.
- Use unprivileged LXCs. Don't enable nesting or device passthrough unless a task needs it.
- An LXC escape on the NAS would mean root on the NAS, and with it Lor's CA key. This is an accepted risk. If that changes, move agents to VMs or to other Proxmox hosts. Nothing in Lor depends on where the agent runs.

**Proxmox cautions** (enforced through host flags in Lor, section 11):
- `hypervisor`: sudo on a Proxmox host means control of every container and VM on it.
- `lor_infra`: the host runs Lor (the NAS) or proxies its web UI (the Caddy LXC).
  - Root on the NAS can read Lor's CA key and give itself access to every host.
  - Root on the Caddy LXC can change what you see on approval pages.
  - Grants on these hosts default to 15 minutes and get the strongest warning. If a sudo session on one of them is ever in doubt, rotate the CA (section 14).

---

## 3. Stack and repo layout

**One repo, `lor`, holding the backend, the frontend and the CLI.** The build produces two static binaries:
- `lor-server`, with the web UI embedded.
- `lor`, the agent CLI.

Node is needed only at build time.

**Backend: Go 1.25+**
- `golang.org/x/crypto/ssh` for certificate signing and host-key scanning.
- Stdlib `net/http`, using Go 1.22+ `ServeMux` route patterns. There are two listeners, admin (`:8444`) and agent (`:8443`), both using one self-signed TLS certificate that Lor generates.
- **API contract: OpenAPI 3.0 specs in `api/`**, one for the admin API and one for the agent API.
  - `oapi-codegen` generates Go server interfaces (`std-http-server` + `strict-server`) and the CLI's typed client. A handler that doesn't match the spec is a compile error.
  - `openapi-typescript` + `openapi-fetch` give the frontend typed calls from the same spec.
  - The result: the backend and frontend can't silently disagree about a route or field.
- `github.com/go-webauthn/webauthn` for passkeys.
- `modernc.org/sqlite` (pure Go, WAL mode) with embedded `.sql` migrations.
- `cobra` for the CLI.

**Frontend: `web/`**
- SvelteKit with Svelte 5 and TypeScript, built with `adapter-static` as a **single-page app**:
  - `fallback: 'index.html'`
  - `ssr = false` in the root layout
  - SvelteKit's client-side router handles navigation.
- Tailwind CSS v4 + shadcn-svelte. Components are copied into `src/lib/components/ui/` (shadcn's model), so there's no runtime UI dependency to track.
- `@tanstack/svelte-query` for server data:
  - caching, so pages show instantly when you navigate back
  - polling and refetch on window focus
  - invalidation after mutations
- `@simplewebauthn/browser` for the passkey prompts.
- pnpm, Vitest, Playwright, `svelte-check`, ESLint (with `svelte/no-at-html-tags` as an error), Prettier.
- `pnpm build` writes `web/build/`. `web/embed.go` (`//go:embed all:build`) puts it inside `lor-server`.

```
lor/
  go.mod                 module root; `ignore ./web/node_modules` (Go 1.25) keeps ./... out of node_modules
  Makefile               generate · web · build · dev · test
  api/
    admin.yaml           OpenAPI: web UI ↔ lor-server
    agent.yaml           OpenAPI: lor CLI ↔ lor-server
  cmd/lor-server/  cmd/lor/
  internal/ca/           SignUserCert(), CA key load/generate. Pure, heavily tested.
  internal/auth/         admin: passkeys, sessions, step-up; agent: token + source-IP check
  internal/store/        SQLite + migrations
  internal/grants/       state machine: request, offer, claim, extend, release, revoke, sweeper
  internal/hosts/        host-key scan, bootstrap rendering
  internal/server/
    adminapi/            generated interface + handlers
    agentapi/            generated interface + handlers
    spa.go               serves the embedded web UI (rules in section 11)
    middleware.go        proxy-IP check, session, CSRF, agent auth
  internal/client/       generated agent-API client, used by the CLI
  web/
    src/routes/          pages (section 11)
    src/lib/api/         generated types (admin.d.ts) + client wrapper (adds X-Lor-Request, handles 401)
    src/lib/components/ui/   shadcn-svelte
    embed.go             package web; //go:embed all:build
  scripts/bootstrap.sh.tmpl
  skill/SKILL.md
  test/integration/      real OpenSSH in containers
  test/e2e/              Playwright against lor-server --dev
  docs/runbook.md
```

**Build and dev:**
- **`make generate`:** runs oapi-codegen and openapi-typescript. Generated code is committed. CI runs it and fails on `git diff`, so the specs and the code can't drift apart.
- **`make build`:**
  1. `pnpm -C web install --frozen-lockfile && pnpm -C web build`
  2. `CGO_ENABLED=0 go build ./cmd/...`
  
  `go build` fails loudly if `web/build/` is missing.
- **`make dev`:**
  - Runs `lor-server --dev` and `pnpm -C web dev`. Vite proxies `/api` to the Go server, so you get hot reload.
  - `--dev` serves the admin listener on `127.0.0.1` over plain HTTP, with passkey RP ID `localhost` and a separate throwaway data dir. It's a command-line flag only and can't be set from the config file.

---

## 4. Hosts

### Users created on each target
| User | Sudo | Accepts principal |
|---|---|---|
| `agent` | no | `<host>` |
| `agent-sudo` | `NOPASSWD: ALL` | `<host>:sudo` (left out with `--no-sudo`) |

- Principals are **host-specific**, so a certificate for `nas` does nothing on `pihole`.
- Passwords are set to `*` (no password login, but not "locked", which some PAM setups treat differently).

### sshd drop-in (`/etc/ssh/sshd_config.d/50-lor.conf`)
```
# Managed by Lor. Re-run bootstrap instead of editing.
Match User agent,agent-sudo
    TrustedUserCAKeys /etc/ssh/lor/ca.pub
    AuthorizedPrincipalsFile /etc/ssh/lor/principals/%u
    AuthorizedKeysFile none
    PasswordAuthentication no
    KbdInteractiveAuthentication no
    DisableForwarding yes
    PermitUserRC no
    ChannelTimeout session=15m      # OpenSSH >= 9.2 only
    UnusedConnectionTimeout 5m      # OpenSSH >= 9.2 only
Match all
```
- CA trust is scoped to the agent users. A Lor certificate can never log in as root or as you.
- `AuthorizedKeysFile none` makes sshd ignore keys an agent plants in `authorized_keys`.
- `DisableForwarding yes` blocks tunneling to services behind the host.
- `Match all` stops the block from capturing config that follows the `Include`.

Other files: `/etc/ssh/lor/ca.pub`, `/etc/ssh/lor/principals/{agent,agent-sudo}` (containing `nas` and `nas:sudo`), `/etc/sudoers.d/50-lor-agent`.

### Bootstrap script
POSIX `sh`, idempotent, rolls back on failure. Rendered by Lor with the CA public key and host name.

The host page shows a one-liner to run from the admin's machine. Its link is signed and valid for 15 minutes, so `curl` doesn't need a session:
```
curl -fsSL https://lor.home.example.com/bootstrap/nas.sh?t=… | ssh nas sudo sh [-s -- --no-sudo | --uninstall]
```
1. **Preflight:**
   - Running as root; OpenSSH ≥ 8.9.
   - OS from `/etc/os-release` is tier 1, otherwise refuse unless `--force`.
   - Make sure `sshd_config` has `Include /etc/ssh/sshd_config.d/*.conf` near the top (back up first).
2. Create the users (`useradd`, home directory, `/bin/bash`, password `*`).
3. Write the CA key, principals, sudoers (check with `visudo -cf`) and the sshd drop-in. Write the `ChannelTimeout` lines only on OpenSSH ≥ 9.2.
4. **Validate:** `sshd -t` and `sshd -T -C user=agent,host=x,addr=127.0.0.1` must show `authorizedkeysfile none` and the CA path. **If anything fails, restore the backups and exit non-zero.**
5. **Reload sshd:**
   - The service is `ssh` on Debian/Ubuntu.
   - On socket-activated Ubuntu (≥ 22.10), reload `ssh.service` only if it's running.
   - Fall back to SIGHUP if there's no systemd.
6. Print the host's ed25519 key fingerprint so it can be compared with Lor's scan.

`--uninstall` removes everything and the users (unless `--keep-users`). `--no-sudo` on an existing host removes `agent-sudo`.

Ubuntu ≥ 25.10 uses **sudo-rs**. The sudoers drop-in should work unchanged, but it must be covered by tests.

---

## 5. Identity

### Admin (web UI through Caddy, admin listener `config.admin_listen`)

**Network:**
- The admin listener accepts connections only from `config.trusted_proxy` (Caddy's LAN IP). The Proxmox firewall enforces the same rule.
- The admin API and web UI aren't served on the agent listener, and the agent API isn't served on the admin listener.
- `X-Forwarded-For` from Caddy is recorded in the audit log for information only. It's never used for authentication.

**TLS between Caddy and Lor: yes, keep it.**
- Caddy's HTTPS only protects the hop from your device to Caddy. The hop from Caddy to Lor carries your session cookie and approvals across a LAN that also holds agents, sandboxes and other devices.
- It costs almost nothing: Lor already generates a self-signed certificate (with its LAN IP as a SAN) for the agent listener and reuses it here.
- Caddy trusts only that certificate (`tls_trust_pool file`, section 15). In effect Caddy pins it, the same way the CLI does.

**Passkeys** (WebAuthn, `go-webauthn`):
- One admin, who can register several passkeys (phone, laptop).
- The relying-party ID is the host of `public_url`. Login requires user verification (Face ID, fingerprint or PIN).
- **Sessions:**
  - Login sets `__Host-lor_session` (`Secure`, `HttpOnly`, `SameSite=Strict`, `Path=/`).
  - The value is a random 256-bit token, stored hashed. Sessions expire after 7 days idle or 30 days total.
  - **Settings** lists passkeys and sessions and has **Sign out everywhere**.
- **First setup:**
  1. Run `lor-server admin setup-code` in the Lor LXC. It prints a one-time code valid for 15 minutes.
  2. Open `/setup`, enter the code, and register your first passkey.
- **Recovery:**
  - `lor-server admin reset`, run on the Lor LXC console, deletes all passkeys and sessions. Then run `setup-code` again.
  - Root on Lor's LXC already controls everything, so this adds no new risk.
- **Step-up: every action that creates access needs a fresh passkey tap**, even with a valid session.
  - **Which actions:**
    - authorize a request (with or without changes)
    - approve an extension
    - create an offer
    - extend a grant
    - change an agent's LAN IPs
    - accept a changed host key
    - add or remove passkeys
  - **How it's bound to the action:**
    1. The browser sends the action, for example `{authorize grant 42, sudo, 15m}`, to a `…/begin` endpoint.
    2. Lor stores it with a one-time WebAuthn challenge (5 minutes, in memory) and returns the challenge.
    3. After the tap, the `…/finish` call carries only the assertion. Lor verifies it and applies **exactly the stored action**.
    
    The browser can't swap parameters between the tap and the action.
  - **Result:** a stolen session cookie or a CSRF slip can view, deny and revoke, but can't create or extend access.
- **CSRF:**
  - Every state-changing request must carry `Origin` equal to `public_url`, the header `X-Lor-Request: 1` (the API client wrapper adds it), and `Content-Type: application/json`.
  - Lor never sends CORS headers.
  - The `SameSite=Strict` cookie is a third layer.

### Agent (agent API, LAN listener `config.agent_listen`, for example `192.168.1.10:8443`)
- **TLS:** the CLI pins Lor's certificate's SHA-256 fingerprint at enrollment and refuses anything else, so no CA or Caddy is involved.
- **Enrollment:**
  1. In the UI, **Add agent**: enter a name and the agent LXC's LAN IP.
  2. Lor shows a one-time command, valid for 15 minutes:
     ```
     lor enroll --url https://192.168.1.10:8443 --fingerprint SHA256:… --code 7KQ-M2P-X9D
     ```
  3. Run it in the agent LXC. The CLI exchanges the code for a long-lived **agent token** and stores it in `~/.config/lor/config.toml` (0600). Lor stores only a hash.
- **Every agent request must have:**
  - `Authorization: Bearer <token>` matching an enabled agent, **and**
  - a source IP equal to one of that agent's registered LAN IPs.
- **What the token can do:** request, claim offers, extend, release, and report commands. It **can't approve anything**. A stolen token used from another machine fails the IP check. Even if the IP were spoofed, any certificate it obtained would only work from the agent's IP.
- The UI can **rotate** a token (re-enroll) or **disable** the agent.

**The consent page is the source of truth.** An agent or a prompt injection could misdescribe a request in chat. The page shows only server-side data:
- **Warnings** are computed by the server and returned in the API response. The frontend displays them; it doesn't decide what's dangerous.
- **Agent-written text** is rendered as plain text by Svelte (never `{@html}`; lint enforces this) and labelled as unverified.

---

## 6. Certificates

The CA is an ed25519 key at `<data_dir>/ca/lor_ca` (mode 0600). It's generated on first start, which is logged and audited.

```go
type UserCertRequest struct {
    PublicKey   ssh.PublicKey
    Serial      uint64       // certificates.id
    KeyID       string       // "lor:g42:c57:agent=claude:host=nas:level=sudo"
    Principal   string       // "nas" or "nas:sudo"
    ValidAfter  time.Time    // now - 2m (clock skew)
    ValidBefore time.Time    // grant's current end time
    SourceAddrs []netip.Addr // agent's registered LAN IP(s)
}
```
`SignUserCert` must **refuse** anything that breaks these rules (one unit test per rule):
- User certificate, exactly one principal matching `^[a-z0-9-]+(:sudo)?$`.
- Finite `ValidBefore` in the future, no more than `absolute_max_duration` (default 30 days, configurable) from now. This catches bugs; it isn't a policy limit.
- A non-empty `source-address` critical option.
- Only the `permit-pty` extension: no forwarding, no user-rc.
- ed25519 public key.

**Source address:** the agent's registered LAN IP(s), from enrollment. Certificates are only usable from those addresses.

The KeyID and serial appear in each host's sshd auth log, which ties logins back to grants.

---

## 7. Grants

```
Agent request:  pending ──Authorize──▶ active      Admin offer:  offered ──claim──▶ active
                   ├─Deny──▶ denied                                 └─window ends──▶ offer_expired
                   └─30m undecided──▶ request_timeout

active ──end time passes──▶ expired  |  ──agent release──▶ released  |  ──admin revoke──▶ revoked
```

**Agent request** (`lor request`):
- **Validation:**
  - The agent isn't disabled; the host exists and is enabled.
  - `level ≤ host.max_level`.
  - Duration ≤ `agent_request_max` (8h; the default is 1h).
  - **Reason is required, 10–400 characters** (roughly 10 words to 3 sentences).
  - Fresh ed25519 public key.
  - At most 3 pending requests per agent.
- **Reuse:** if an active grant at ≥ the requested level exists, return it. If a matching offer exists, claim it.
- **Result:** `pending` plus `approval_url = <public_url>/r/<id>`. The link opens wherever your devices can reach Caddy, on or off the LAN.
- **On the consent page the admin can:**
  - Authorize (passkey tap).
  - Authorize with changes (passkey tap): lower the level, or choose any duration. The level can never be raised.
  - Deny, with an optional note.
- On approval, Lor signs with `valid_before = now + duration`.
- Requests left undecided for `request_approval_timeout` (30m) expire. This only stops stale requests; it has nothing to do with access length.

**Admin offer** (Grant access form):
- Pick agent, host, level, duration and an optional note, then confirm with a passkey tap. **The timer starts at creation**, so a 30m offer made at 14:00 ends at 14:30.
- There's no key yet, so no certificate. The agent's next `lor ssh <host>` or `lor request <host>` claims the offer: the CLI generates a key, Lor signs it with the offer's end time, and the claim can happen only once.
- After creating it, the UI shows a line to paste into chat (with a copy button): *"You have Lor access to `nas` (sudo) until 14:30. Use `lor ssh nas -- <cmd>`."*

**Extend:**
- A certificate's end time can't be changed, so an extension is a **new certificate for the same key** with a later end time.
- The added time counts from the current end time. It's allowed at any moment while the grant is active.
- **Admin:** an Extend button with +30m, +1h, +4h, +1 day or custom, confirmed with a passkey tap.
- **Agent:** `lor extend <host> --for 1h --reason "..."` returns an approval link. The consent page shows "ends 16:30 (12 min left) → would end 17:30".
- The level never changes through an extension.
- If the grant ends before a pending extension is decided, the extension becomes `moot`.

**Revoke and release:** v1 only marks the grant in Lor's records. The certificate stays valid until it expires, but `lor ssh` checks with Lor before every connection and deletes the key once it sees the grant was revoked. True revocation (KRL) comes later.

**Sweeper:**
- Runs every 30 seconds and at startup.
- Moves expired, timed-out and moot items to their final states and writes audit events.
- Deletes command-log entries older than `command_log_retention`.

Hosts enforce expiry regardless of the sweeper.

---

## 8. Data model (SQLite, Unix-second timestamps)

```sql
agents(id, name UNIQUE, status /*active|disabled*/,
       lan_addrs /*JSON; API source-IP check + certificate source-address*/,
       token_hash, token_created_at,
       enroll_code_hash, enroll_code_expires_at /*NULL after enrollment*/,
       created_at, last_seen_at)

hosts(id, name UNIQUE /*principal base*/, address /*LAN IP*/,
      host_key, host_key_seen_at, max_level /*user|sudo*/,
      hypervisor, lor_infra, notes, disabled, created_at)

grants(id, agent_id, host_id, origin /*agent_request|admin_offer*/,
       level, requested_level, requested_duration_s, approved_duration_s,
       reason, public_key /*NULL until claimed*/, public_key_fp,
       status /*pending|offered|active|denied|expired|released|revoked|request_timeout|offer_expired*/,
       decided_by /*passkey name*/, decision_note, requested_at, decided_at,
       valid_after, valid_before /*current end; moves on extend*/, ended_at)

certificates(id /*= SSH serial*/, grant_id, valid_after, valid_before,
             certificate, issued_at, issued_by)

extension_requests(id, grant_id, requested_duration_s, approved_duration_s, reason,
                   status /*pending|approved|denied|request_timeout|moot*/,
                   requested_at, decided_at, decided_by, decision_note)

command_log(id, grant_id, certificate_id,
            argv /*JSON; truncated to 4 KB; [] = interactive shell*/, reported_at)
            -- self-reported by the CLI (section 10)

admin_passkeys(id, name /*"iPhone"*/, credential_id UNIQUE, public_key, sign_count,
               aaguid, transports, created_at, last_used_at)

admin_sessions(id, token_hash UNIQUE, passkey_id, created_at, last_seen_at,
               expires_at, user_agent, forwarded_for)

audit_events(id, ts, actor, action, grant_id, host_id, agent_id, details_json)  -- append-only
```
Pending WebAuthn challenges and step-up actions live only in memory. A restart cancels any that are in progress.

---

## 9. Agent API (LAN listener, `/api/v1`, spec `api/agent.yaml`, Bearer token + source-IP check)

| Endpoint | Purpose |
|---|---|
| `POST /enroll` | `{code}` → `{token}` (unauthenticated, one-time code, rate-limited) |
| `GET /whoami` | Agent info |
| `GET /hosts` | Requestable hosts `[{name, max_level}]`, `agent_request_max_s` |
| `POST /grants` | `{host, level, duration_s, reason, public_key}` → `{grant, approval_url?, reused?}` |
| `GET /grants/{id}?wait=60` | Long-poll (used by `--wait`). When active, includes the current certificate and an `ssh` block (`hostname`, `user`, `known_hosts_line`, `host_key_alias`) |
| `GET /grants?host=…&status=…` | This agent's grants |
| `POST /grants/{id}/claim` | `{public_key}` → activates an offer |
| `POST /grants/{id}/connect` | `{argv}` → records the command in `command_log` and returns the current certificate + `ssh` block, or an error if the grant is no longer active. Called by `lor ssh` before every connection. |
| `POST /grants/{id}/extend` | `{duration_s, reason}` → `{extension_request, approval_url}` |
| `POST /grants/{id}/release` | Release |

Errors are returned as `{error: {code, message}}`, with stable codes. The admin API (`/api/admin/v1`, spec `api/admin.yaml`) is only used by the web UI and isn't listed here. It follows the same error format.

---

## 10. CLI (`lor`)

```
lor enroll --url … --fingerprint … --code …  # one-time setup (command copied from the UI)
lor status                                   # active / pending / offered grants
lor hosts
lor request <host> [--sudo] [--for 1h] --reason "..." [--wait]
lor extend  <host> --for 1h --reason "..."
lor ssh     <host> -- <command...>
lor release <host>
lor config-path                              # for scp/rsync: scp -F "$(lor config-path)" f lor-nas:/tmp/
```
- **`request` and `extend` don't block by default.** Agent tool calls time out after about 2 minutes. They print the approval link and what was asked for, then exit with code `10` (pending). `--wait` long-polls instead.

**Why `lor ssh` wraps `ssh` instead of the agent calling `ssh` directly.** The certificate on disk isn't the whole story: it can be replaced by an extension, made obsolete by a revoke, or not exist yet because an offer hasn't been claimed. `lor ssh` handles all of that so the agent doesn't have to:
- picks up the new certificate after an extension
- refuses to connect, and deletes the key, after a revoke or release
- claims an offer on first use
- prints the approval link again if the grant is still pending
- keeps everything out of `~/.ssh/`
- gives clear exit codes and `lor:` error messages

It also gives you the **command log**:
- Before each connection, `lor ssh` reports the command to Lor (`POST /grants/{id}/connect`). The grant page and dashboard show what the agent ran and when.
- The log is **self-reported**, so treat it as visibility, not an audit:
  - An agent can bypass it with raw `ssh -F "$(lor config-path)"`.
  - It sees only the outer command (`bash -c '…'` shows the whole script, but not what that script does).
  - It records no output or exit codes, because `lor ssh` hands the process over to `ssh`.
- Access itself doesn't depend on the log: the certificate is what sshd checks. Trustworthy, host-side recording is phase 4 (section 15).

**`lor ssh <host>` steps:**
1. Find the agent's grant for the host (`GET /grants?host=…`). If it's an offer, generate a key and claim it. If it's pending, print the link again and exit `255`.
2. Call `POST /grants/{id}/connect` with the command. This records it and returns the current certificate. If Lor can't be reached, use the local certificate as long as it's still valid; the command then isn't logged.
3. If less than 5 minutes are left, warn on stderr.
4. Run `ssh -F ~/.config/lor/ssh_config lor-<host> <cmd>` by exec.

**Exit codes:**
- `lor ssh` returns the remote command's exit code. If `lor` stops before connecting, it exits `255` with a `lor:` line on stderr.
- Other commands: `0` ok, `1` error, `2` denied, `3` expired or timed out, `4` invalid, `10` pending.

**Files:**
- Everything lives under `~/.config/lor/`.
- Each grant gets `grants/<id>/` (0700 directory, 0600 key). Expired directories are deleted on every run.
- The CLI never touches `~/.ssh/`.

The generated config entry:
```
Host lor-nas
  HostName 192.168.1.20
  User agent-sudo
  IdentityFile ~/.config/lor/grants/42/id_ed25519
  CertificateFile ~/.config/lor/grants/42/id_ed25519-cert.pub
  IdentitiesOnly yes
  HostKeyAlias lor-host-nas
  UserKnownHostsFile ~/.config/lor/known_hosts
  StrictHostKeyChecking yes
  BatchMode yes
```

---

## 11. Admin web UI (SvelteKit SPA, mobile-first)

The UI is a single-page app embedded in `lor-server` and served on the admin listener through Caddy. Pages load once, and navigation happens in the browser. Data comes from the admin API and is cached by svelte-query, so moving between pages is instant.

| Route | Contents |
|---|---|
| `/` Dashboard | Pending requests and extensions (each links to its consent page). Active and offered grants with live countdowns, the last reported command, **Extend** and **Revoke**. A **Grant access** button. Recent audit events. |
| `/r/[id]`, `/grants/[id]` | Consent view while pending. Otherwise status, timeline, certificates and the **command log** (labelled "reported by the agent's CLI"). |
| `/grants/new` | Grant access form (offer) with a confirmation card |
| `/hosts`, `/hosts/[name]` | Add/edit (name, address, max level, `hypervisor`, `lor_infra`), bootstrap one-liners, host-key fingerprint, **Rescan key** (a changed key needs a passkey tap and is audited) |
| `/agents` | **Add agent** (name + LAN IP → enroll command), rotate token, edit LAN IP(s), disable |
| `/audit` | Filterable log |
| `/settings` | Passkeys (add, rename, remove) and sessions (list, sign out everywhere) |
| `/login`, `/setup` | Passkey login; first-passkey setup with the one-time code |

**UX rules:**
- **Live data:**
  - The dashboard and an open grant page poll every 5 seconds and also refetch when the tab regains focus.
  - Countdowns tick locally every second from `valid_before`.
  - Mutations invalidate the affected queries, so the page updates in place without reloading.
- **Deep links work.** `/r/42` opened from chat loads the app on that page. If you're not signed in, the API returns 401, and the app sends you to `/login?next=/r/42`. After the passkey tap you land back on the consent page.
- **Feedback:**
  - Buttons show a pending state while a passkey prompt is open.
  - Results appear as toasts (shadcn-svelte Sonner).
  - Errors show the server's message.
- **Phone first:** the consent page and dashboard must be usable one-handed on a phone. Use shadcn-svelte's dialog and drawer for confirmations.

**Serving rules in `lor-server` (`internal/server/spa.go`):**
- `/api/admin/v1/*` → the API. Unknown API paths return a JSON 404, never `index.html`.
- `/_app/immutable/*` → `Cache-Control: public, max-age=31536000, immutable`.
- Other files that exist in the bundle are served as is.
- Anything else → `index.html` with `Cache-Control: no-cache`. This is the SPA fallback that makes deep links work.
- **Headers:**
  - `X-Content-Type-Options: nosniff` and `Referrer-Policy: same-origin`.
  - `Content-Security-Policy: frame-ancestors 'none'`.
  - The script and style policy comes from SvelteKit's `kit.csp` (hash mode), which writes a CSP meta tag into `index.html`: `script-src 'self'` plus hashes, `connect-src 'self'`.

**Consent view:**
```
┌────────────────────────────────────────────────────────────┐
│  claude wants access to  nas                               │
│  Request #42 · 2 min ago · approve within 28 min           │
├────────────────────────────────────────────────────────────┤
│  ⚠ ROOT ACCESS (sudo): logs in as agent-sudo.              │
│    Can read, change or delete anything on nas.             │
│  ⚠ nas is Lor infrastructure: root here can read Lor's     │
│    signing key and give itself access to every host.       │
│                                                            │
│  Host          nas · 192.168.1.20                          │
│  Agent         claude (enrolled agent LXC)                 │
│  Connect from  only 192.168.1.50                           │
│  Duration      [ 15 min ▾ ]  → ends 14:45 (extendable)     │
│  Level         (•) sudo   ( ) standard user                │
│  Limits        cert login only · no forwarding · 15m idle  │
│                                                            │
│  Reason (written by the agent, unverified):                │
│  ┃ ZFS pool reports a degraded disk. I need to run         │
│  ┃ zpool status and check SMART data to identify it.       │
│                                                            │
│  [ ] I understand this gives root on nas                   │
│  [ 🔑 Authorize sudo on nas for 15 min ]      [ Deny ]     │
└────────────────────────────────────────────────────────────┘
```
- **Duration picker:** 15m, 30m, 1h, 2h, 4h, 8h, 1d, 3d, 1w, or custom. Anything over 24h shows a note but isn't blocked.
- **Standard user** is explained as: "Logs in as `agent` (no sudo). Can run programs and read files any normal user can read. Can't install software or change system settings."
- **Warnings** come from the server:
  - `hypervisor` + sudo → "controls every container and VM on it".
  - `lor_infra` → the strongest warning, at any level.
- **Authorize:**
  - The button restates the final level, host and duration, then asks for a passkey tap.
  - For sudo it stays disabled until the checkbox is ticked, both here and on the Grant access form.
- **The reason is shown in full.** It's rendered as plain text, wrapped, and labelled as unverified.
- A decided or expired request shows its outcome and never an Authorize button.

---

## 12. Agent skill (`skill/SKILL.md`)

```markdown
---
name: lor-access
description: Get temporary SSH access to homelab machines through Lor. Use whenever a task requires running commands on another machine.
---
You have no standing access to other machines. Access is granted per task with the `lor` CLI.

1. `lor status`. The user may already have granted access. If so, go to step 4.
2. `lor request <host> --for <duration> --reason "<1–3 sentences: what you will do and why>"`
   Use the shortest duration that fits the task. Add `--sudo` only if the task needs root.
3. It prints an approval link and exits with code 10. Show the user the link exactly as printed,
   plus what you asked for (host, sudo or not, duration). Then stop and wait for them to say
   it's approved. Don't poll.
4. `lor ssh <host> -- <command>`. Run every remote command this way; the user sees each one in Lor.
   If it exits 255 with a `lor:` message, read it: the grant may be pending (share the link again),
   denied, or expired. If it warns that time is short and you still need access, use
   `lor extend <host> --for <duration> --reason "..."` the same way.
   Use `lor config-path` only for scp/rsync, which `lor ssh` can't do.
5. `lor release <host>` when done.

Rules:
- Never try to keep access beyond a grant: no adding SSH keys, users, sshd/sudoers changes,
  remote-access tools, reverse shells, or cron/systemd jobs meant to give you access later.
- Don't copy credentials found on a host into notes, memory or other machines.
- If denied, don't retry repeatedly or look for another way in. Tell the user what you needed.
```

---

## 13. Tests

**Unit tests (Go):**
- **`ca`:**
  - Every refusal rule.
  - Sign then verify with `ssh.CertChecker`.
  - Output parses with `ssh-keygen -L`.
- **`grants`:** state machine table tests.
  - Reuse.
  - An offer is claimed once, by the right agent, with the window end as its end time.
  - Approve with a downgrade or any duration, never an upgrade.
  - Extensions add to the current end; an extension of an ended grant is `moot`.
  - Reason length limits.
  - `connect` on a revoked or expired grant fails and logs nothing.
  - The sweeper, using an injected clock.
- **`auth`:** passkey ceremonies with a software authenticator (`github.com/descope/virtualwebauthn`).
  - Login creates a session with the right cookie flags.
  - Idle and absolute expiry.
  - Sign out everywhere.
  - **Step-up:**
    - An assertion is single-use and expires after 5 minutes.
    - It's bound to the session.
    - It applies only the stored action: a tap for "grant 42, user, 15m" can't authorize "grant 42, sudo, 1w" or grant 43.
    - Step-up actions fail without it.
  - `setup-code` works once; `reset` removes all passkeys and sessions.
- **`server`:**
  - Admin routes exist only on the admin listener; agent routes only on the agent listener.
  - The admin listener rejects connections that don't come from `trusted_proxy`.
  - **Agent auth:**
    - Missing or wrong token → 401.
    - Right token from the wrong source IP → 403.
    - Disabled agent → 403.
    - The enrollment code is single-use and expires.
  - The CLI refuses a server whose TLS fingerprint doesn't match the pinned one.
  - CSRF rules (missing or wrong `Origin`, or a missing header → 403).
  - **Consent API:** returns server-computed warnings; the sudo confirmation is required; a decided request has no authorize action.
  - **SPA serving:**
    - `/r/42` returns `index.html`.
    - `/api/admin/v1/nope` returns JSON 404.
    - Immutable assets get long cache headers; `index.html` gets `no-cache`.
    - CSP headers are present.
  - The bootstrap link stops working after 15 minutes.

**Frontend:**
- `svelte-check` and ESLint (`no-at-html-tags`) in CI.
- Vitest for logic: countdown formatting, the duration picker, and mapping API errors to messages.
- **Playwright E2E** against `lor-server --dev`, using Chrome's virtual authenticator:
  - First-passkey setup → login.
  - Open `/r/<id>` while signed out → login → back on the consent page.
  - Authorize with changes → the grant shows as active with the changed level.
  - A hostile reason (`<script>`, markup) is shown as text.
  - The sudo checkbox gates the button.
  - The command log appears after a `connect` call.
  - Phone-sized viewport for the consent page.

**Integration tests** (Docker, real OpenSSH, bootstrap run exactly as a user would; approvals go through the admin API with a software authenticator). Each case runs on Debian 12, Debian 13 (standing in for Proxmox VE 9), Ubuntu 22.04, 24.04 and 26.04 (sudo-rs):
1. A valid certificate logs in as `agent`. ✅
2. Certificate rejected on a different host (wrong principal). ❌
3. A user-level certificate is rejected for `agent-sudo`. ❌
4. Certificate rejected from a different source IP. ❌
5. An expired certificate is rejected. ❌
6. A key planted in `~agent/.ssh/authorized_keys` is ignored. ❌
7. A Lor certificate can't log in as root or as other users. ❌
8. `ssh -L` port forwarding is refused. ❌
9. `agent` can't sudo; `agent-sudo` can.
10. Bootstrap is idempotent. `--uninstall` makes 1 and 9 fail.
11. Bootstrap rolls back and leaves sshd working when validation fails.
12. CLI request flow: `lor request` exits 10 with a link → admin approves → `lor ssh` works, and the command appears in the command log.
13. Offer flow: offer created → `lor ssh` claims it and works.
14. Downgrade: sudo requested, user approved → logs in as `agent`, and sudo fails.
15. Extension in the last 30s keeps access working past the original end time; without it, access fails.
16. Revoked grant: `lor ssh` deletes the key and refuses to connect.
17. `lor ssh` exit codes: the remote exit code passes through; lor-side failures give 255.

---

## 14. Failure modes

| Scenario | Outcome |
|---|---|
| Grant forgotten | The certificate expires; sshd enforces it. Nothing is permanent. |
| Lor down or deleted | No new grants; existing ones expire. Fails closed. |
| Agent plants a key | Ignored (`AuthorizedKeysFile none`). |
| Agent copies its certificate to another machine | Rejected (source address). |
| Agent token stolen | Useless from another IP. It can only request, never approve. Rotate it in the UI. |
| Agent bypasses `lor ssh` (raw `ssh -F`) | Access works as normal until expiry; those commands are missing from the command log. The log is self-reported (section 10). |
| Traffic takes an unexpected route (subnet router, Caddy) | The source IP no longer matches, so the agent API and SSH both reject it. Fails closed. |
| Agent reuses passwords it found on a host | Only SSH is reachable from the agent LXC (Proxmox firewall); other LAN services, including Caddy and Lor's UI, are blocked. |
| Sandbox tries to reach other hosts | Blocked by the Proxmox firewall on the sandbox. |
| Agent misdescribes a request in chat | The consent page shows the real details. |
| Something on the LAN opens Lor's web UI | It needs a passkey to sign in. Lor's admin port only accepts connections from Caddy. |
| Session cookie stolen | It can view, deny and revoke, but can't create or extend access (step-up). Use Sign out everywhere. |
| Caddy LXC compromised | It can't sign anything, but it could show you a misleading page and collect a passkey tap. It's marked `lor_infra`, so sudo there gets the strongest warning. |
| All passkeys lost | `lor-server admin reset` on the Lor LXC console, then `setup-code`. |
| Session still open when the grant ends | Continues until closed, or until the 15m idle timeout. |
| Revoke | The CLI enforces it on its next call. A raw `ssh` call with a saved key works until expiry (KRL later). |
| Sudo on the NAS | Could read the CA key. `lor_infra` warning, 15m default; rotate the CA if in doubt. |
| Agent escapes its LXC on the NAS | Equivalent to root on the NAS, including the CA key. Accepted risk (section 2); unprivileged LXCs make it harder. |
| **CA key leaks** | Emergency: (1) Proxmox firewall: drop tcp/22 outbound from the agent LXC, or stop it; (2) delete and regenerate the CA; (3) re-run bootstrap on every host. |
| Host rebuilt (new host key) | The agent's ssh refuses to connect until you click Rescan and confirm. |

---

## 15. Config, deployment, later phases

**Config** (`lor-server.toml`):
```toml
data_dir = "/var/lib/lor"
public_url = "https://lor.home.example.com"  # Caddy site: approval links, passkey RP ID, Origin check
admin_listen = "192.168.1.10:8444"           # admin API + web UI; Caddy only
trusted_proxy = "192.168.1.30"               # Caddy LXC's LAN IP
agent_listen = "192.168.1.10:8443"           # agent API; agents connect directly
default_grant_duration = "1h"
agent_request_max = "8h"                     # agents can't ask for more; admin can grant more
absolute_max_duration = "720h"               # bug guard in the ca package
request_approval_timeout = "30m"
command_log_retention = "90d"
```

**First start:**
1. Lor generates the CA key and the self-signed TLS certificate (`<data_dir>/tls/cert.pem`).
2. Copy the certificate to the Caddy LXC and add the site:
   ```
   lor.home.example.com {
       reverse_proxy https://192.168.1.10:8444 {
           transport http {
               tls_trust_pool file /etc/caddy/lor.crt   # copy of <data_dir>/tls/cert.pem
           }
       }
   }
   ```
   Caddy's own certificate for `lor.home.example.com` must be one your devices trust. Passkeys refuse to work on a page with a certificate warning. Use the same method as your other Caddy sites.
3. Run `lor-server admin setup-code` and register your first passkey at `/setup`.

**Deployment:**
- Lor runs in a dedicated Ubuntu 26.04 LXC on the NAS (1 vCPU, 512 MB RAM), with a systemd unit (`ProtectSystem=strict`, `NoNewPrivileges=yes`).
- Build with `make build`. You copy over two static binaries; Node isn't needed on the server.
- **Back up `data_dir/ca/` off the box.** Losing it means re-bootstrapping every host.
- Back up `lor.db` with `sqlite3 .backup`. It also holds your passkey registrations.

**Agent LXC setup:**
1. An unprivileged LXC with a fixed LAN IP. No Tailscale is needed.
2. Proxmox firewall rules from section 2.
3. Install the `lor` binary, then run the enroll command from **Add agent** in the UI.
4. Put the skill in `~/.claude/skills/lor-access/`.

**Later phases** (not for v1):
- **Phase 2:**
  - Auto-approve rules.
  - Host self-test: Lor logs in with a 2-minute certificate to verify the setup.
  - Push notifications (ntfy) for unattended agents.
  - RHEL-family and Arch bootstrap.
  - Off-LAN agents (for example a VPS): through an agent gateway LXC on the LAN (Tailscale + SSH `ProxyJump`, forwarding only). The gateway's LAN IP is registered as that agent's address. SNAT on the subnet router stays as is.
- **Phase 3:** a revocation list (KRL) pulled by hosts, for immediate revoke.
- **Phase 4: monitoring**, using an optional `lor-host` helper on watched hosts, enabled automatically for `lor_infra` hosts. This is the host-side, trustworthy counterpart to v1's self-reported command log:
  - Per-grant command log and output (a `ForceCommand` wrapper), streamed to Lor so a sudo agent can't edit it afterwards.
  - Commands run under sudo (auditd).
  - Live view.
  - Before/after change report (users, sudoers, crontabs, systemd units, `authorized_keys`, packages).
  - "Disconnect now".
  - Heartbeat alert if the helper is stopped during a grant.

---

## 16. Open questions

None right now.

**Recently decided:**
- Agent LXCs may run on the NAS (section 2).
- The web UI is a SvelteKit SPA behind Caddy, with passkey login (sections 3, 5 and 11).
