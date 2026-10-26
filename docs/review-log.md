# Review log

Second-reviewer rounds (GPT 6.1 Sol, reasoning effort "xhigh", review-only)
and the decision taken on each finding. Each round was a fresh delegated
review with a self-contained brief.

## Round 1: CA + config (commit c4fa1ca)

No critical or high findings.

| # | Sev | Finding | Decision |
|---|-----|---------|----------|
| 1 | Med | CA key created with `O_EXCL` then written: a crash leaves a truncated `lor_ca` that breaks every later start. | **Fixed.** Write + fsync a 0600 temp file, publish with `link(2)` (fails instead of replacing; the loser reloads the winner's key), fsync the directory. |
| 2 | Med | Duration parsing (`d`/`w`) could overflow silently (`213504d` → ~25m). | **Fixed.** Overflow checks on multiply and add; tests for wraparound inputs. |
| 3 | Low | `time.Sub` saturates, so a huge `absolute_max_duration` let a year-9999 `ValidBefore` pass. | **Fixed** in `SignUserCert`, `CheckCert` and the grants-layer check: compare `ValidBefore` with `now.Add(max)`. Test added. |
| 4 | Low | Fractional Go durations (`1.5h`) rejected. | **Fixed.** Plain Go durations go straight to `time.ParseDuration`; mixed forms accept fractional Go units. |
| 5 | Low | `public_url` accepted query/fragment/userinfo, breaking approval links. | **Fixed.** Rejected; `public_url` is normalized to scheme + host. |

## Round 2: grants state machine, store, API specs (commit d35f782)

No critical or high findings.

| # | Sev | Finding | Decision |
|---|-----|---------|----------|
| 1 | Med | Host `max_level` not rechecked when a sudo offer is claimed or a sudo grant extended after the host was lowered to `user`. | **Fixed.** Claim and every extend path (admin extend, approve extension, and their step-up `Check*`) refuse with `level_not_allowed`. Test added. |
| 2 | Med | `ListAgentGrants` limited to 100 rows before filtering by status, so many denied requests could hide an active grant from the CLI. | **Fixed.** Open grants are settled first, then the status filter is applied in SQL before the limit. Test with 105 denied requests added. |
| 3 | Med | Agents couldn't see a decided extension's outcome, so `lor extend --wait` couldn't tell denial from timeout. | **Fixed.** Agent `Grant` has `latest_extension` (any status); `lor extend --wait` follows its extension request and exits 0 / 2 (denied, with note) / 3 (timed out or moot). |

## Round 3: auth (passkeys, sessions, step-up, agents) + bootstrap script (commits 301e82a, 907a6d9)

No critical or high findings. The reviewer also ran the sshd drop-in and
sudoers checks against a local Ubuntu 26.04 (OpenSSH + sudo-rs): they passed.

| # | Sev | Finding | Decision |
|---|-----|---------|----------|
| 1 | Med | Two concurrent first scans could both see "no key"; the second silently replaced the first key without step-up. | **Fixed.** The first key is stored with a conditional `UPDATE … WHERE host_key IS NULL AND address = ?`; on conflict the scan is re-evaluated as a possible change (which needs step-up). |
| 2 | Med | Concurrent passkey removals could delete every passkey. | **Fixed.** Existence and "not the last passkey" checks moved into the deleting transaction. |
| 3 | Med | Bootstrap disabled rollback before the sshd reload; uninstall committed before reloading. | **Fixed (mostly).** Install commits only after a successful reload; a failed reload restores the previous files and reloads again. Uninstall removes the drop-in, validates and reloads before committing. **Declined:** backing up agent users' home directories before `--uninstall` deletes them: those homes are scratch space for the agent, and `--uninstall` is explicitly destructive per plan section 4 (`--keep-users` exists for keeping them). |
| 4 | Med | Adopting an existing account with `--force` overwrote its password hash, and rollback didn't restore it. | **Fixed.** The previous shadow hash is saved and restored on rollback. |
| 5 | Med | Missing sudo/visudo/`/etc/sudoers.d` failed late (after creating users) or skipped the mandatory `visudo -cf`. | **Fixed.** Preflight requires `sudo`, `visudo` and `/etc/sudoers.d` when `agent-sudo` is wanted (with guidance to use `--no-sudo`); `visudo -cf` is no longer optional. |

## Round 4: HTTP layer, CLI, frontend (c077b61..33219c8)

No critical or high findings.

| # | Sev | Finding | Decision |
|---|-----|---------|----------|
| 1 | Med | CLI startup cleanup deleted a grant's key once the cached certificate expired, even if the grant had been extended on the server. | **Fixed** (CI's integration case 15 caught the same bug in parallel). Expired local certificates are only deleted once Lor confirms the grant isn't active (or after the absolute maximum lifetime). Regression test `TestExtendedWhileIdle`. |
| 2 | Med | `lor ssh` listed the host's whole history (capped at 100), so many newer requests could hide an active grant. | **Fixed.** Selection uses open statuses only; local keys not in that list are checked one by one (`GET /grants/{id}`) before deletion. |
| 3 | Low | Agent API `duration_s` multiplied into `time.Duration` could overflow past the limit checks. | **Fixed.** Bounded before conversion; test added. (Admin step-up durations were already bounded.) |
| 4 | Low | Choosing Custom with empty/invalid text kept the previous preset, so Authorize still used it. | **Fixed.** The picker reports validity; Authorize, Approve extension and Grant access are disabled until a custom duration parses. |
| 5 | Low | Offline `lor ssh` (Lor unreachable) skipped the "less than 5 minutes left" warning. | **Fixed.** |
| 6 | Low | Usage errors exited 1 (and `lor ssh` without a host didn't exit 255). | **Fixed.** Flag/argument errors exit 4; anything from `lor ssh` before connecting exits 255. Test added. |

## Round 5: final whole-repository review (d40b4a5)

No critical findings. The reviewer reported host rename as the only missing v1 item.

| # | Sev | Finding | Decision |
|---|-----|---------|----------|
| 1 | High | Bootstrap validated `sshd -T` only for `addr=127.0.0.1`; an earlier `Match Address <agent IP>` block setting `AuthorizedKeysFile` would pass validation yet let planted keys work for the real agent. (Reproduced by the reviewer.) | **Fixed.** The script is rendered with every registered agent IP and validates each address context. The drop-in also sets `AuthorizedKeysCommand none` / `AuthorizedPrincipalsCommand none`, and validation checks both. Integration case 11 now also covers the `Match Address` override, on all five distros. |
| 2 | High | Re-enabling a disabled agent (no step-up) let it claim offers made earlier; host edits could likewise unblock prohibited offers. | **Fixed** by invalidation rather than more step-up: disabling an agent revokes its active grants and offers and denies pending requests; disabling a host or lowering its max level ends the grants and offers it no longer allows. Re-enabling restores nothing. Test added. |
| 3 | Med | CLI cleanup still trusted a capped "active" list, and deleted keys 31 days after the cached expiry without asking Lor. | **Fixed.** Every expired local grant is confirmed with `GET /grants/{id}` and deleted only if it isn't active (or doesn't exist); if Lor can't be reached, keys are kept. |
| 4 | Low | README/deviations said Go 1.25+ but go.mod requires 1.26. | **Fixed** (docs now say 1.26+). |
| 5 | Low | Host-name editing (plan section 11) was missing. | **Implemented.** Rename is allowed when the host has no open grants; it's audited, and the UI reminds you to re-run the bootstrap. |

## Live field test (Proxmox VE 9.2, Ubuntu 26.04 LXCs): findings and fixes

A separate deployment agent ran docs/field-tests.md on a live environment
(Lor + stand-in Caddy, a Docker/Vikunja target, an OpenCode agent LXC, with
the Proxmox firewall enabled). 27 cases: 22 pass, 4 fail, 1 skipped (OpenCode
free-tier quota). Fixed in 2dcf9f9 and the round 6 follow-up:

| Finding | Fix |
|---|---|
| `agent` got UID 1000 = Vikunja's container UID: it could write the app's DB, and `--uninstall` (`pkill -u agent`) killed the container | Dedicated, collision-checked UID/GID range 59700–59799; legacy users are recreated; removal never kills unrelated processes (integration case 19; re-verified live: migration and uninstall/reinstall left Vikunja untouched) |
| Idle sessions not closed after 15 min (OpenSSH bug) | Version-aware truth in the consent Limits, host page and bootstrap output; integration case 18 compares the claim with real sshd behaviour |
| Expired grant reported as "no access" | CLI remembers ended grants |
| `context canceled` logged as internal errors | Quiet cancellations; internal errors logged with method/path |
| Unlabelled, inconsistent time zones | Zone-labelled times; chat line in the admin's zone plus UTC |
| Deployment requirements | README: Caddy ≥ 2.8, LXC nesting, `pct exec` PATH |

## Round 6: field-test fixes (2dcf9f9)

No critical findings.

| # | Sev | Finding | Decision |
|---|-----|---------|----------|
| 1 | High | The UID collision scan used `find -xdev`, missing files on nested mounts (a separate `/var/lib/docker`, datasets), and treated traversal errors as "unused". | **Fixed.** One scan per run, following nested mounts (skipping network/FUSE/overlay/pseudo filesystems); any traversal error other than vanishing files stops the bootstrap. Integration case 19 puts the workload's data on a separate Docker volume. |
| 2 | High | Recreating a legacy user deleted the old account and home before validation, so a failed run lost them. | **Fixed.** Account files are backed up and the old home moved aside until commit; rollback restores both. Case 19 forces a validation failure during migration and checks the old UID and home come back. |
| 3 | Med | `useradd --user-group` may pick a GID other than the checked one. | **Fixed.** `groupadd --gid <checked id>` then `useradd --gid`; created groups are removed on rollback. |
| 4 | Med | Killing SSH session processes one by one could reparent children so later ancestry checks missed them. | **Fixed.** In Lor's (unique) range every process with the UID is ended; for legacy shared UIDs each pass snapshots SSH descendants before signalling, repeated until none remain. Case 19 checks no process with the agent's UID survives. |
| 5 | Med | OpenSSH 10.5 still lets `session:shell`/`session:command` defaults override `session=15m` (fixed in 10.6; confirmed in the 10.6 release notes). | **Fixed.** Enforcement is claimed only from OpenSSH 10.6. |
| 6 | Med | The scanned SSH version was stored even when the scanned key was untrusted (changed), so an impostor could change the idle-timeout claim. | **Fixed.** The version is only recorded with the first (TOFU) key or a matching trusted key. Test added. |
| 7 | Low | The offer chat line was built from form state that stayed editable during the passkey prompt. | **Fixed.** Built from the returned grant; the form is locked while submitting. |

## Round 7: verification of the round 6 fixes (b00b512, 3ef2e15)

| # | Sev | Finding | Decision |
|---|-----|---------|----------|
| 1 | Med | Accepting a replacement host key kept the old server's SSH version, so the idle-timeout claim could be stale after a rebuild. | **Fixed.** Accepting a key clears the version and re-reads it with a scan that presents the newly trusted key. Test added. |
| 2 | Med | With `agent` absent and a legacy `agent-sudo`, migration snapshotted the account files after `agent` was created, so rollback resurrected `agent`'s account without its home. | **Fixed.** Account files are snapshotted once before any account change. |
| 3 | Low | A network filesystem mounted exactly at a scan root (e.g. NFS `/home`) wasn't pruned. | **Fixed.** Mounts equal to a scan root are pruned too. |
