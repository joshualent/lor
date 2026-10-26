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
