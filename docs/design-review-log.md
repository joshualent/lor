# Design review log

Independent appearance/UX reviews of the redesigned web UI (see
[design-system.md](design-system.md)). Reviewer: GPT 6.1 Sol (extra high
reasoning), given full-page screenshots of every page and state at 1280 px
and 390 px (`make screens`) plus read access to the code. Guardrail given
to the reviewer: keep the grey palette and the minimalist, app-like
styling; anything that moves away from that is out of scope.

## Round 1

Screenshots: `lor-design-review/round-1/` (72 images).

| Category | Score |
|---|---:|
| Visual hierarchy | 7 |
| Typography | 8 |
| Color use and meaning | 7 |
| Consistency / design-system coherence | 8 |
| App-likeness and affordance | 7 |
| Navigation and information architecture | 7 |
| Information density and scannability | 7 |
| Feedback and states | 7 |
| Mobile experience | 6 |
| Accessibility | 6 |
| Overall polish | 7 |
| **Overall** | **7** |

Top 5 from the reviewer: (1) repair phone decision layouts (`ActionBar`,
`Confirm`); (2) consistent trust labels and semantic colors; (3) control
accessibility (checkbox outline, focus contrast, described-by, touch
targets); (4) navigation clipping and audit disclosure discoverability;
(5) put monitoring information first on grant and dashboard layouts.

### Decisions

| Finding | Decision | What changed / why |
|---|---|---|
| Sudo action bar takes ~184 px of an 844 px phone screen | **Accepted** | `ActionBar` is one row: Deny, then Authorize filling the rest with a wrapping label; the checkbox panel is tighter. The Authorize label still restates level, host and duration (plan rule), so no separate summary line was added. |
| Infra default of 15 min isn't obviously different from what was asked | **Accepted** | The duration card says "Asked for 1 h. Lor suggests 15 min for Lor infrastructure." when they differ. |
| Drawer primary button cropped (Add host on a phone) | **Accepted** | `Confirm` drawers: header and footer fixed, body scrolls. |
| Unchecked sudo checkbox nearly invisible on the amber panel (≈1.1:1) | **Accepted** | Its outline uses the warning color (5.8:1), with a contrast test. |
| 50%-opacity focus ring ≈2.9:1 on cards; test checked the opaque token | **Accepted** | Buttons and custom controls use a solid 2 px accent outline with an offset (covered by the existing ring-contrast test). |
| Field hints/errors and settings descriptions not tied to controls | **Accepted** | `Field` and `SettingsRow` provide a context; `Input`, `Textarea`, `NativeSelect` and `Switch` pick up `aria-describedby`/`aria-invalid` by id. |
| Small buttons and segments are 32 px | **Accepted** (without changing the look) | `sm`/`icon-sm` buttons and chips get an invisible 44 px hit area; segments are 36 px on phones. The visual size stays, since the owner likes the current buttons. |
| Navbar clips "Settings" at 390 px | **Accepted** | Tighter item padding on phones; same styling. |
| Expandable audit rows look static | **Accepted** | Disclosure rows show a chevron-down that turns when expanded (distinct from the navigation chevron) and a subtle expanded background. |
| Selected category chip can be scrolled out of view | **Accepted** | `ChipGroup` scrolls the selected chip into view. |
| Command log buried below settings on grant pages | **Accepted** | Order: time left and actions → warnings → pending extension → command log → details → reason → extensions → certificates → timeline. |
| Home "Access" mixes active grants and offers | **Accepted** | Separate "Active" and "Offers" sections. |
| Home command preview squeezed by the countdown | **Accepted** | The last command has its own full-width line under the row. |
| Hosts phone: "up to / sudo" fragments | **Accepted** | Address and access ceiling on separate lines. |
| Technical text breaks mid-word (`break-all`) | **Accepted** | `wrap-anywhere` (breaks at spaces first); command times sit above commands on phones. |
| Extension before/after times wrap awkwardly | **Accepted** | Three-column grid (before · arrow · after). |
| DurationPicker and ExtendDialog differ for custom durations | **Accepted** | The extend dialog uses `DurationPicker` with the +30 min…+1 day presets. |
| Healthy agent tiles violet, certificate tiles teal outside the audit log | **Accepted** | Neutral; violet/teal stay audit-only. Documented. |
| Unclaimed offer's time bar is green | **Accepted** | Blue (`info`). |
| Denied grants say "No commands yet… appear here" | **Accepted** | Grants that never had a certificate hide the command log; ended grants say "No commands were reported." |
| `/grants/new` has no loading/error states | **Accepted** | Skeleton, error with retry, and an empty state linking to Agents or Hosts when nothing is usable. |
| Host without a key: the problem and Rescan are below the settings | **Accepted** | A warning callout with "Rescan key" at the top; the Host key section shows only once there is a key. |
| hosts-empty repeats the primary action | **Accepted** | Empty-state action is secondary. |
| Login says "A request is waiting" from the URL alone | **Accepted** | "Sign in to review this request". |
| Add "Agent reason · unverified" labels to Home previews | **Partly accepted** | Previews are quoted (“…”) so they read as the agent's words; the full text with the explicit "unverified" label is one tap away on the consent page. A label on every row added noise without new information. |
| Label commands "Agent-reported · unverified" | **Partly accepted** | The explanation ("Reported by the agent's CLI. It shows what lor ssh was asked to run, not what actually happened.") moved from a footnote to the section description, above the log. |
| Out of scope (flagged by the reviewer) | **Declined** | Light theme, gradients, saturated surfaces, a new button/navbar/input style, heavier chrome, charts, notifications, bulk approvals, rich-text reasons, reworded warnings, fewer passkey taps. |

## Round 2

Screenshots: `lor-design-review/round-2/` (after the round-1 changes).

| Category | Round 1 | Round 2 |
|---|---:|---:|
| Visual hierarchy | 7 | 8 |
| Typography | 8 | 8 |
| Color use and meaning | 7 | 9 |
| Consistency / design-system coherence | 8 | 8 |
| App-likeness and affordance | 7 | 8 |
| Navigation and information architecture | 7 | 8 |
| Information density and scannability | 7 | 8 |
| Feedback and states | 7 | 7 |
| Mobile experience | 6 | 8 |
| Accessibility | 6 | 7 |
| Overall polish | 7 | 8 |
| **Overall** | **7** | **8** |

The reviewer marked every round-1 finding resolved except the two
trust-label items that were only partly accepted (still partly open) and
touch targets (partly resolved).

Top 5 from the reviewer: (1) finish the offer form's mobile layout;
(2) the shared duration picker's keyboard behavior; (3) one trust cue per
group; (4) connection and retry feedback; (5) consistent revocation
wording.

### Decisions

| Finding | Decision | What changed / why |
|---|---|---|
| Offer form sections touch (`space-y` on the form, but its child is a `display: contents` fieldset) | **Accepted** | Spacing lives on the fieldset. The submit button wraps like the consent button. |
| Arrow keys can focus "Custom" without selecting it; two tab stops when Custom is active | **Accepted** | `ChipGroup` routes appended chips through the same selection (they're clicked) and gives the tab stop only to the selected chip. Affects consent, extensions and offers. |
| No "unverified" cue on Home previews and command previews | **Accepted (one cue per group)** | "Reasons are written by the agents and unverified." under Waiting for you; "Last commands are reported by the agents and unverified." under Active; the command log description starts with "Unverified:". Quotes stay. |
| Blank screen while the session loads; no retry on connection failure | **Accepted** | "Checking your session…" with a spinner; "Can't reach Lor" has a Try again button (and no longer claims to retry by itself). |
| A failed background refresh replaced cached data with an error | **Accepted** (a real bug) | Live pages keep the last data and show a `StaleNotice` ("Couldn't refresh. Showing data from 21:03." + Retry); the error page is only for a failed first load. Covered by a new E2E test. |
| Audit "Load older" has no failure handling | **Accepted** | Inline error with Try again. |
| Revoke wording differs between Home and the grant page; no busy state on Home | **Accepted** | One `REVOKE_NOTE` (lor ssh stops at once; a saved certificate stays valid until it expires) and a separate note for withdrawing offers; Home's confirm button shows a spinner and is disabled while submitting. A revoked or released grant explains this next to its status while its last certificate is still valid. |
| Unchecked radio in `ChoiceCard` ≈1.5:1 | **Accepted** | Uses `icon-muted` (3.9:1). |
| `icon-sm` 40 px, segments 36 px, search clear 28 px hit areas | **Accepted** | 44 px invisible hit areas for all three, same visuals. |
| Extend dialog dates wrap unevenly | **Accepted** | Labelled "Current end" → "New end" columns. |
| Offer row lacks an explicit "Offered" status | **Accepted** | Blue Offered badge on Home's offer rows. |
| Home summary counts should link to their sections | **Accepted** | "4 waiting for you · 3 active · 1 offered" are links to the sections. |
| Expanded audit details should show the full timestamp | **Accepted** | Seconds and time zone shown next to the action in the expanded details. |
| Input and checkbox focus keep their halo treatment | **Kept** | Their accent border changes to the solid accent color on focus (≥ 3:1) in addition to the halo; the owner likes the current inputs, so their look stays. |
| Out of scope (flagged by the reviewer) | **Declined** | Light theme, gradients, saturated surfaces, heavier chrome, new control styling, charts, notifications, bulk approvals, rich-text reasons, weaker trust labels, reworded warnings, removing the sudo acknowledgement, fewer passkey taps. |

## Round 3 (final check)

Screenshots: `lor-design-review/final/` (taken again after the round-3
changes below).

| Category | Round 1 | Round 2 | Round 3 |
|---|---:|---:|---:|
| Visual hierarchy | 7 | 8 | 9 |
| Typography | 8 | 8 | 8 |
| Color use and meaning | 7 | 9 | 9 |
| Consistency / design-system coherence | 8 | 8 | 9 |
| App-likeness and affordance | 7 | 8 | 9 |
| Navigation and information architecture | 7 | 8 | 8 |
| Information density and scannability | 7 | 8 | 8 |
| Feedback and states | 7 | 7 | 8 |
| Mobile experience | 6 | 8 | 8 |
| Accessibility | 6 | 7 | 8 |
| Overall polish | 7 | 8 | 9 |
| **Overall** | **7** | **8** | **8.5** |

The reviewer marked the round-2 findings resolved except: stale-data
handling on the offer form and settings, overlapping enlarged hit areas,
and phone date wrapping (partly resolved). It recommended the first three
items below before sign-off; all five (and the smaller notes) were applied
after this review.

### Decisions

| Finding | Decision | What changed / why |
|---|---|---|
| Offer form disappears on a failed background refresh; Settings has no stale notice; Retry shows no progress | **Accepted** | The offer form keeps its draft and shows a `StaleNotice`; Settings too. `StaleNotice` and `ErrorState` retries show a spinner and are disabled while fetching. |
| Passkey icon buttons' 44 px hit areas overlap; scrolling chips clip theirs; nav and back links small | **Accepted** | Trailing row controls are 12 px apart (44 px centers); chip and nav scrollers get vertical room; nav items and the back link get 44 px hit areas. Visuals unchanged. |
| Choosing Custom keeps showing the previous "1 h" | **Accepted** | Empty custom input says "Type a duration…"; end-time lines and Authorize/Extend/Grant labels say a duration is needed until it's valid. The submit gate is unchanged. |
| Pending rows don't show the host's risk | **Accepted** | Server-provided Lor infra / Hypervisor badges on pending requests and extensions. |
| Phone time comparisons wrap unpredictably | **Accepted** | `TimeStack`: label, time (AM/PM kept together) with the zone, and the day ("Today", "Tomorrow", "Oct 27") on fixed lines, in the extend dialog and on the extension page. |
| Agents header: action drops below the subtitle | **Accepted** | `PageHeader` keeps title and actions on one row and puts the subtitle underneath. |
| Home summary links look passive | **Accepted** | Hit area and focus ring. |
| Home command previews truncated; repeated Extend/Revoke names | **Accepted** | Commands get two lines (recency moved to the subtitle); buttons are named "Extend nas", "Revoke nas"; the row's name includes sudo/offer state. |
| Out of scope (flagged by the reviewer) | **Declined** | Same list as before; the reviewer noted none is needed. |
