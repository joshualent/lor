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
