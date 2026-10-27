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
