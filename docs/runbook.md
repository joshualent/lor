# Lor runbook

Paths assume the defaults from `deploy/` (`/etc/lor/lor-server.toml`,
`data_dir = /var/lib/lor`, service user `lor`).

## Backups

- **CA key**: `/var/lib/lor/ca/` → copy off the box once (it never changes
  unless you rotate it). Losing it means re-bootstrapping every host.
- **Database**: `sudo -u lor sqlite3 /var/lib/lor/lor.db ".backup /path/lor.db.bak"`.
  It holds grants, the audit log and your passkey registrations.
- The TLS certificate (`/var/lib/lor/tls/`) can be regenerated; Caddy and
  every agent's pinned fingerprint must then be updated (see below).

## All passkeys lost

On the Lor LXC console:

```sh
sudo -u lor lor-server admin reset --config /etc/lor/lor-server.toml
sudo -u lor lor-server admin setup-code --config /etc/lor/lor-server.toml
```

`reset` deletes all passkeys and sessions (audited as `admin.reset`). Then
register a new passkey at `/setup`.

## A session cookie may be stolen

Settings → **Sign out everywhere**. A stolen session can view, deny and revoke,
but can't create or extend access (every such action needs a passkey tap).

## An agent token may be stolen

Agents → **Rotate token** (the old token stops working immediately), then run
the new enroll command in the agent LXC. A stolen token is useless from another
IP anyway.

## A host was rebuilt (new host key)

`lor ssh` fails with a host key mismatch. Hosts → the host → **Rescan key**.
Compare the new fingerprint with the one the bootstrap script prints, then
**Accept new key** (passkey tap, audited). Re-run the bootstrap one-liner.

## CA key leaked (or a sudo session on a `lor_infra` host is in doubt)

1. Proxmox firewall: drop tcp/22 outbound from the agent LXC, or stop it.
2. Stop Lor: `systemctl stop lor-server`.
3. Move the old CA away: `mv /var/lib/lor/ca /var/lib/lor/ca.compromised`.
4. Start Lor: it generates a new CA key (audited as `ca.generated`).
5. Re-run the bootstrap one-liner on **every** host (it rewrites
   `/etc/ssh/lor/ca.pub`). Certificates from the old CA stop working on each
   host as soon as it's re-bootstrapped.
6. Back up the new `ca/` directory off the box.

## Regenerating the TLS certificate

Needed if the Lor LXC's LAN IP changes (the certificate's SANs must include
`admin_listen` and `agent_listen`; Lor logs a warning when they don't).

1. `systemctl stop lor-server && rm -r /var/lib/lor/tls && systemctl start lor-server`.
2. Copy the new `/var/lib/lor/tls/cert.pem` to Caddy (`/etc/caddy/lor.crt`)
   and reload Caddy.
3. Agents: Rotate token and re-enroll (the enroll command carries the new
   fingerprint).

## Removing Lor from a host

From the host page, run the **Uninstall** one-liner. It removes the sshd
drop-in, `/etc/ssh/lor`, the sudoers file and (unless `--keep-users`) the
`agent` and `agent-sudo` users and their home directories. Disable the host in
Lor so no new grants are made.

## Where to look

- `journalctl -u lor-server`: startup (CA/TLS fingerprints), sweeper errors.
- Audit log (UI → Audit): every grant, decision, certificate, login and
  configuration change. It is append-only (enforced by database triggers).
- On a target: `journalctl -u ssh` (or `/var/log/auth.log`) shows the
  certificate's key ID, e.g. `lor:g42:c57:agent=claude:host=nas:level=sudo`,
  and serial for every login, tying it back to the grant.

## The agent's SSH connections are dropped ("penalty: failed authentication")

OpenSSH ≥ 9.8 (Debian 13, Ubuntu 25.04+) temporarily refuses connections from
a source address after failed logins (`PerSourcePenalties`). An agent that
keeps retrying with an expired certificate or a planted key can lock its own
LXC out of a host for a while. That is sshd working as intended; it clears by
itself. If it gets in the way of legitimate use, exempt the agent LXC on that
host in the main sshd config (not in Lor's drop-in):
`PerSourcePenaltyExemptList 192.168.1.50`.

## Idle sessions aren't closed after 15 minutes

Expected on OpenSSH older than 10.5: it ignores `ChannelTimeout` inside
`Match` blocks (fixed upstream in 10.5), and Lor only sets it for its own
users. The host page shows "Idle timeout: not enforced" and the consent page
says so. Sessions still can't be *started* after a grant ends; an open one
lasts until it is closed. Upgrade OpenSSH on the host to 10.5+ (then Rescan
key so Lor records the new version), or end sessions by hand
(`pkill -u agent` on the host) when needed.

## A host's agent user shares a UID with other software

Lor (since the field test) gives `agent`/`agent-sudo` UIDs from 59700–59799
that nothing else uses. Hosts bootstrapped by an earlier build may have
`agent` at UID 1000, which a container image can share: re-run the bootstrap
one-liner; it recreates the users with fresh UIDs, without touching other
processes. Removal (`--uninstall`, `--no-sudo`) only ends the agent's own SSH
sessions.
