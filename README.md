# Lor

Lor gives AI agents **temporary, approved SSH access** to homelab machines. It
is a small SSH certificate authority: every grant is a short-lived OpenSSH
certificate, so hosts enforce the end time themselves, even if Lor is down.

An agent asks with `lor request nas --for 1h --reason "..."` and shares the
approval link in chat. You open it on your phone, see exactly what would be
allowed, and authorize with a passkey tap. The agent then runs
`lor ssh nas -- <cmd>` until the certificate expires; each command shows up in
Lor. You can also grant access before the agent asks, extend, and revoke.

The full design is in [docs/plan.md](docs/plan.md). Deviations from it are in
[docs/deviations.md](docs/deviations.md); the operations runbook is in
[docs/runbook.md](docs/runbook.md).

```
 Your devices ──HTTPS──▶ Caddy ──HTTPS :8444 (pinned)──▶ lor-server  (admin API + web UI)
 Agent LXC ── lor CLI ──HTTPS :8443 (pinned, token + source IP)──▶ lor-server  (agent API, CA)
 Agent LXC ── ssh (certificate locked to the agent's IP) ──▶ targets (agent / agent-sudo)
```

## Repository

| Path | What |
|---|---|
| `cmd/lor-server`, `cmd/lor` | the server and the agent CLI |
| `api/admin.yaml`, `api/agent.yaml` | OpenAPI contracts (Go and TypeScript code is generated from them) |
| `internal/ca` | certificate signing and its refusal rules |
| `internal/grants` | the grant state machine and sweeper |
| `internal/auth` | passkeys, sessions, step-up, agent tokens |
| `internal/hosts`, `scripts/bootstrap.sh.tmpl` | host-key scans and the host bootstrap script |
| `internal/server` | the two HTTP listeners, CSRF, SPA serving |
| `web/` | the SvelteKit web UI (embedded into `lor-server`) |
| `skill/SKILL.md` | the agent skill |
| `deploy/` | example config, systemd unit and Caddy site |
| `test/e2e`, `test/integration` | Playwright and Docker/OpenSSH tests |

## Building

Needs Go 1.26+ and Node 22 with pnpm (Node is only needed at build time).

```sh
make build          # web UI → web/build, then CGO_ENABLED=0 go build → bin/lor-server, bin/lor
make test           # go vet + go test, svelte-check, ESLint, Prettier, Vitest
make generate       # regenerate code from api/*.yaml (output is committed; CI checks for drift)
```

`lor-server` embeds the web UI; `go build` fails if `web/build/` is missing,
so always build with `make build` (or `make web` first).

Other test suites:

```sh
go build -o bin/fakessh ./test/e2e/fakessh
pnpm -C web exec playwright install chromium
make e2e                                   # Playwright against lor-server --dev

DISTRO=ubuntu:26.04 make integration       # real OpenSSH in Docker (needs Docker)
```

CI runs all of them, with the integration tests on Debian 12, Debian 13,
Ubuntu 22.04, 24.04 and 26.04.

## Development

```sh
make dev
```

This runs `lor-server serve --dev` on `http://localhost:8444` (admin) and
`https://127.0.0.1:8443` (agent API) with a throwaway data dir in
`.dev-data/`, plus the Vite dev server on `http://localhost:5173` with hot
reload (it proxies `/api` to the Go server). Open
<http://localhost:5173/setup> and use the `LOR_DEV_SETUP_CODE` printed at
startup. `--dev` is a command-line flag only: plain HTTP, 127.0.0.1, passkey
RP ID `localhost`. It can't be set from the config file.

## Deployment

Lor runs in its own small LXC (the plan uses an Ubuntu 26.04 LXC on the NAS:
1 vCPU, 512 MB RAM). Copy the two static binaries; nothing else is needed.

Proxmox notes from the field test: unprivileged Ubuntu 26.04 LXCs (systemd
259) need `features: nesting=1`, or journald/networkd fail with
`243/CREDENTIALS`; Docker targets also need `keyctl=1`. `pct exec` runs with a
PATH that doesn't include `/usr/local/bin`, so call
`/usr/local/bin/lor-server` with its full path there.

1. **Install the server** on the Lor LXC:

   ```sh
   install -m 0755 bin/lor-server /usr/local/bin/
   useradd --system --home /var/lib/lor --shell /usr/sbin/nologin lor
   install -d -m 0755 /etc/lor
   install -m 0644 deploy/lor-server.toml /etc/lor/lor-server.toml   # then edit it
   install -m 0644 deploy/lor-server.service /etc/systemd/system/
   systemctl daemon-reload && systemctl enable --now lor-server
   ```

   In `/etc/lor/lor-server.toml`, set `public_url` (your Caddy site),
   `admin_listen` and `agent_listen` (the Lor LXC's **fixed** LAN IP, ports
   8444 and 8443), and `trusted_proxy` (the Caddy LXC's LAN IP).

2. **First start.** Lor generates the CA key (`/var/lib/lor/ca/lor_ca`) and a
   self-signed TLS certificate (`/var/lib/lor/tls/cert.pem`, with the listen
   IPs as SANs). `journalctl -u lor-server` shows both fingerprints.
   **Back up `/var/lib/lor/ca/` off the box**: losing it means
   re-bootstrapping every host.

3. **Caddy.** Copy `/var/lib/lor/tls/cert.pem` to the Caddy LXC as
   `/etc/caddy/lor.crt` and add the site (also in `deploy/Caddyfile.example`):

   ```caddy
   lor.home.example.com {
       reverse_proxy https://192.168.1.10:8444 {
           transport http {
               tls_trust_pool file /etc/caddy/lor.crt
           }
       }
   }
   ```

   `tls_trust_pool` needs **Caddy 2.8 or newer** (tested with 2.11.7 from
   Caddy's official apt repository; Ubuntu 26.04's packaged 2.6.2 is too old).

   Caddy's own certificate for `lor.home.example.com` must be trusted by your
   devices (passkeys refuse pages with certificate warnings); use the same
   method as your other Caddy sites. Lor's admin port only accepts
   connections from `trusted_proxy`.

4. **Register your passkey.** On the Lor LXC:

   ```sh
   sudo -u lor lor-server admin setup-code --config /etc/lor/lor-server.toml
   ```

   Open `https://lor.home.example.com/setup`, enter the code (valid 15
   minutes) and register your first passkey. Add more passkeys (phone,
   laptop) under **Settings**.

5. **Proxmox firewall** (plan section 2): the Lor LXC accepts tcp/8443 only
   from agent LXCs and tcp/8444 only from the Caddy LXC; the agent LXC may
   reach tcp/22 on the LAN and tcp/8443 on Lor, and nothing else private.

### Bootstrapping a host

1. In the UI, **Hosts → Add host**: a name (it becomes the certificate
   principal, e.g. `nas`), the host's **LAN IP**, the most access allowed, and
   the `hypervisor` / `lor_infra` flags. Lor scans the ed25519 host key.
2. The host page shows one-liners with a signed link valid for 15 minutes.
   Run one from your machine:

   ```sh
   curl -fsSL 'https://lor.home.example.com/bootstrap/nas.sh?t=…' | ssh nas sudo sh
   # without agent-sudo:   … | ssh nas sudo sh -s -- --no-sudo
   # remove everything:    … | ssh nas sudo sh -s -- --uninstall [--keep-users]
   ```

   The script (POSIX sh, idempotent) creates `agent` and `agent-sudo`,
   installs the CA key, principals and the sshd drop-in
   `/etc/ssh/sshd_config.d/50-lor.conf`, validates with `sshd -t` and
   `sshd -T`, reloads sshd, and prints the host key fingerprint to compare
   with Lor's. If anything fails it restores the previous state. Tier 1:
   Proxmox VE 8/9, Debian 12/13, Ubuntu 22.04/24.04/26.04 (OpenSSH ≥ 8.9);
   others need `--force`. `agent-sudo` needs `sudo` (or sudo-rs) installed.
   The `agent` users get UIDs from 59700–59799 that no other account,
   process or data directory uses (so they never share a numeric UID with a
   container workload). On OpenSSH older than 10.5 the script notes that the
   15-minute idle timeout isn't enforced (an upstream bug with
   `ChannelTimeout` in `Match` blocks); the host page and consent page say so
   too.
   Validation checks the effective sshd settings for loopback and for every
   agent IP registered when the link was generated, so add agents before
   bootstrapping (or re-run the bootstrap after adding one).

### Enrolling an agent

1. Give the agent LXC a fixed LAN IP (unprivileged LXC; no Tailscale needed).
2. Install the CLI: `install -m 0755 bin/lor /usr/local/bin/`.
3. In the UI, **Agents → Add agent** with a name and that LAN IP. Lor shows a
   one-time command (valid 15 minutes); run it in the agent LXC:

   ```sh
   lor enroll --url https://192.168.1.10:8443 --fingerprint SHA256:… --code 7KQ-M2P-X9D
   ```

   The CLI pins Lor's TLS certificate and stores its token in
   `~/.config/lor/config.toml` (0600). It only works from the registered IP.
4. Put the skill where the agent finds it, e.g.
   `~/.claude/skills/lor-access/SKILL.md` (copy `skill/SKILL.md`).

## Using it

Agent side (`lor --help`):

```
lor status                                   # active / pending / offered grants
lor hosts
lor request <host> [--sudo] [--for 1h] --reason "..." [--wait]   # exits 10 with a link
lor extend  <host> --for 1h --reason "..." [--wait]
lor ssh     <host> -- <command...>           # reports the command, then execs ssh
lor release <host>
lor config-path                              # for scp/rsync: scp -F "$(lor config-path)" f lor-nas:/tmp/
```

Like `ssh`, `lor ssh` joins the command's arguments with spaces and the remote
shell runs the result, so `lor ssh nas -- 'zpool status | head'` works and
`lor ssh nas -- sh -c 'exit 7'` doesn't do what it looks like (quote for the
remote shell: `lor ssh nas -- "sh -c 'exit 7'"`).

Exit codes: `0` ok, `1` error, `2` denied, `3` expired or timed out,
`4` invalid, `10` pending. `lor ssh` returns the remote command's exit code,
or `255` with a `lor:` message if it stops before connecting.

Admin side: the dashboard lists requests waiting for you, active grants with
live countdowns and the last reported command, and recent activity. Anything
that creates or extends access (authorize, approve an extension, grant access,
extend, change an agent's IPs, accept a changed host key, add/remove
passkeys) needs a fresh passkey tap bound to that exact action.

## Security notes

- Certificates: one host-specific principal, `source-address` locked to the
  agent's LAN IP, only `permit-pty`, end time enforced by sshd.
- The command log is self-reported by `lor ssh` (visibility, not an audit);
  raw `ssh -F "$(lor config-path)"` bypasses it.
- Revoke stops `lor ssh` immediately; a raw ssh with a saved certificate works
  until it expires (revocation lists are a later phase).
- See plan section 14 for the failure modes, and [docs/runbook.md](docs/runbook.md)
  for recovery (lost passkeys, CA rotation).
