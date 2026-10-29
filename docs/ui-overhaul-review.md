# UI overhaul council review

This overhaul keeps Lor's charcoal palette, restrained controls, semantic
status colors, and existing authentication and authorization rules. It
changes the responsive workspace, dashboard composition, page hierarchy,
feedback, and filtering to make everyday access administration easier.

Reviews use fresh independent agents for each round. They receive current
screenshots and read access to the code, are asked to grade honestly, and
must separate visual evidence from code-based inference. Reviews do not
constitute real-device usability studies or measured performance audits.

## Review setup

- GPT-6.1 Sol: `gpt-6.1-sol`, xhigh reasoning.
- GPT-6.1 Terra: `gpt-6.1-terra`, xhigh reasoning.
- Claude Opus 5.5: `claude-opus-5-5`, high effort, added in round 2 after both
  initial reviewers scored every category at least 7.
- Acceptance target: every category at least 8 from all three reviewers.
- Evidence is local and untracked: `/tmp/lor-ui-overhaul/`. Screenshots use
  seeded, throwaway data and simulated SSH endpoints. No live deployment
  or homelab host access is part of this task.

## Round 1

Evidence: `before/` and `round-1/`, each with desktop and phone captures.
Full reports: `round-1/REVIEW-gpt-6.1-sol.md` and
`round-1/REVIEW-gpt-6.1-terra.md` under the evidence directory.

| Category | Sol | Terra |
|---|---:|---:|
| Usability | 7 | 7 |
| Visual Hierarchy | 8 | 8 |
| Consistency | 8 | 8 |
| Responsiveness | 7 | 7 |
| Clarity | 7 | 7 |
| Efficiency | 7 | 7 |
| Learnability | 8 | 7 |
| Layout & Composition | 8 | 7 |
| Typography | 9 | 8 |
| Color theory & contrast | 8 | 8 |
| Whitespace | 7 | 7 |
| Perceived performance | 7 | 7 |
| Accessibility | 7 | 7 |

The first pass introduced the desktop rail, labelled phone navigation,
linked dashboard summaries, parallel decisions and access monitoring,
persistent inventory filters, and grouped security/server settings.

## Changes for round 2

- Reduced the sticky phone navigation to one compact row; retained five
  labelled destinations and touch targets.
- Added the selected level/duration and a Change access jump to consent;
  preserved full warnings, unverified text, sudo acknowledgement, and
  passkey requirements. Limits use neutral bullets so caveats do not look
  like verified protections.
- Added expiry ordering and an early expiry cue, balanced dashboard columns,
  compacted empty states, and matched loading placeholders to the dashboard.
- Added a dedicated input-outline token and contrast tests against actual
  filled input surfaces; kept the original base palette.
- Bounded desktop confirmations, kept action footers visible, and added a
  scroll cue when more content lies below the body viewport.
- Confirmed agent disabling and retained retry context after action failures.
  Sign-out views change only after a successful server response.
- Improved phone metadata, list names, summary link cues, and audit detail
  touch targets. Exposed all audit categories through the phone's native
  picker and retained previous audit results with an explicit updating cue.

Round 2 evidence: `round-2/`, 92 screenshots at 1280 and 390 px plus
checkpoints at 320, 768, and 1440 px. PNG manifests are included locally.

## Round 2 scores

| Category | Sol | Terra | Opus 5.5 |
|---|---:|---:|---:|
| Usability | 7 | 7 | 8 |
| Visual Hierarchy | 9 | 8 | 8 |
| Consistency | 8 | 8 | 7 |
| Responsiveness | 7 | 7 | 8 |
| Clarity | 8 | 7 | 7 |
| Efficiency | 8 | 7 | 8 |
| Learnability | 8 | 8 | 8 |
| Layout & Composition | 8 | 7 | 8 |
| Typography | 8 | 8 | 8 |
| Color theory & contrast | 8 | 8 | 8 |
| Whitespace | 8 | 8 | 8 |
| Perceived performance | 8 | 7 | 8 |
| Accessibility | 7 | 6 | 8 |

Scores above are the reviewers' delivered final verdicts. Two reviewers
used the same generic report filename; their final delivered verdicts are
retained separately in the local evidence to avoid attribution ambiguity.

## Changes for round 3

- Identified grants by host, agent, access level and ID in action names and
  confirmation content; active link names include their remaining time.
- Stacked narrow audit/host metadata and long description values, reserved
  audit disclosure space, strengthened selected segments and sized the sudo
  acknowledgement to a 44 px target.
- Validated positive grant numbers (including pasted # prefixes), discarded
  pagination responses after filter changes, and prevented empty/completion
  claims while matching results load. Older-history viewing explicitly
  pauses polling so refreshed first pages do not leave gaps between pages.
- Unified contextual links, clarified passkey names in session rows, fixed
  separator spacing, and used Grant consistently for approved access.
- Kept decisions sticky on roomy screens, with an in-flow layout on narrow
  or short windows and scroll clearance for choices and focused controls.
- Added regression coverage for audit races and transitions, accessible
  grant identity, and unobscured focused choices in compact windows.


Round 3 evidence: `round-3/`, 100 images including focused consent controls
at 320×568, 390×620 and 390×844, desktop decision-bar viewports, and the
controlled audit updating and invalid-filter states. The screenshot suite
also verifies accessible grant identity and that focused choices are not
obscured by navigation or actions. Round 3 scoring is recorded below.

## Round 3 scores

| Category | Sol | Terra | Opus 5.5 |
|---|---:|---:|---:|
| Usability | 7 | 7 | 8 |
| Visual Hierarchy | 9 | 8 | 8 |
| Consistency | 8 | 8 | 8 |
| Responsiveness | 8 | 8 | 8 |
| Clarity | 8 | 7 | 8 |
| Efficiency | 8 | 7 | 8 |
| Learnability | 8 | 8 | 8 |
| Layout & Composition | 8 | 8 | 8 |
| Typography | 8 | 8 | 8 |
| Color theory & contrast | 9 | 8 | 8 |
| Whitespace | 8 | 8 | 8 |
| Perceived performance | 8 | 8 | 8 |
| Accessibility | 8 | 8 | 8 |

Opus reached the acceptance target. Sol identified a pending-pagination reset
race; Terra requested a direct reason jump, denser narrow lists and an
explanation of timestamp ties. All three reviewed commit 82a6687 against the
same screenshot evidence.

## Changes for round 4

- History resets invalidate pending pagination success, failure and cleanup,
  including Back to latest with unchanged filters. Regression tests cover
  both delayed outcomes and correct continuation from the fresh latest page.
- Consent offers a direct Read full reason link, preserves the complete
  unverified text, and adds a focused decision jump for compact windows.
  Tests exercise long reasons and both ends of the text without changing
  the acknowledgement or authorization gate.
- Narrow pending/audit rows give text more room while keeping complete
  identity, time, risk labels and touch targets. Audit exposes history
  controls near the filters and a return-to-filters shortcut after scrolling.
- Deny and withdraw confirmations include complete identity. Timestamp ties
  get an explanation based on the server's Unix-second audit precision;
  recorded event order is preserved.

Round 4 evidence: `round-4/`, 112 screenshots, including long-reason start/end
views, compact decision jumps, and phone audit-history controls. The full
suite passes 170 frontend tests, 21 production E2E tests and eight screenshot
tests.

## Round 4 scores

| Category | Sol | Terra | Opus 5.5 |
|---|---:|---:|---:|
| Usability | 8 | 8 | 8 |
| Visual Hierarchy | 9 | 8 | 8 |
| Consistency | 8 | 9 | 7 |
| Responsiveness | 8 | 8 | 8 |
| Clarity | 8 | 8 | 8 |
| Efficiency | 8 | 7 | 8 |
| Learnability | 8 | 8 | 8 |
| Layout & Composition | 9 | 8 | 8 |
| Typography | 8 | 8 | 8 |
| Color theory & contrast | 9 | 8 | 8 |
| Whitespace | 8 | 8 | 8 |
| Perceived performance | 8 | 8 | 8 |
| Accessibility | 8 | 8 | 8 |

Sol passed all categories. Terra requested less scrolling through audit
history; Opus requested consistent phone status cues. Both identified the
floating return-to-filters shortcut overlapping the pagination action at
320 px. These findings prevent completion at this round.

## Changes for round 5

- Audit shows 25 events per page with Newer/Older controls both above and
  below the list, while buffering the existing 100-event API responses.
  Page ranges are explicit, the whole browsing snapshot pauses updates until
  Back to latest, and page changes and
  Back to latest restore focus and position at the top pager. The footer
  return-to-filters link is in flow, with no floating control to cover taps.
- Pending history results still require the current generation/filter.
  Regression coverage exercises resets during delayed success/failure,
  cached Newer/Older navigation, retries with the same cursor, final partial
  pages, keyboard activation, and unobscured 44 px footer controls on phones.
- Full outcome icons remain on regular phones; the compact layout applies
  only below 360 px and retains a coloured audit cue and complete identity.
  Expanded compact details use the available width.
- Read full reason has a stronger button affordance. Confirmation summaries
  have more space, drawer headings align with fields, extension denial names
  its host/agent/level/grant/extension, and the recovery command is monospace.

Round 5 evidence: `round-5/`, 122 screenshots, including phone pagination
loading, retry, history, final partial page and frozen first-page states.
Fresh independent council scoring is pending. Final verification uses a
single rebuilt production bundle. An overlapping build/test run had a
transient gallery-check failure; no UI changes were needed for the full
rerun to pass. `make test` and `make build` both regenerate the SPA and
should run sequentially.


## Validation

- `make test`: Go vet/tests, Svelte checks, lint, and 170 Vitest tests pass.
- `make e2e`: 22 tests pass against the production build, including failed
  sign-out/revoke-all, agent disable confirmation/failure, inventory filters,
  responsive navigation and overflow at 320–1440 px, and confirmation action
  visibility in short desktop and phone windows. Audit cases also cover
  bounded pages, frozen history, keyboard retry, cursor continuity, delayed
  responses, focus recovery and unobscured 44 px phone pagination controls.
- `make screens`: all eight seeded screenshot tests pass; responsive
  checkpoints also assert no horizontal page overflow.
