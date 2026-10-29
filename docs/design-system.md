# Lor design system

Lor's web UI is a calm, dark, app-like interface: charcoal greys, one muted
blue accent, and a small set of semantic colors that **always carry
meaning**. Pages are composed from a typed Svelte 5 component library, so
styling lives in one place and pages read like an outline of the screen.

- Tokens: `web/src/app.css`
- Variants (every styling variant, via tailwind-variants): `web/src/lib/design/variants.ts`
- Domain → look (status, level, host, agent): `web/src/lib/status.ts`
- Audit events → label, category, tone, icon, day grouping: `web/src/lib/audit.ts`
- Primitives (shadcn-svelte, adapted): `web/src/lib/components/ui/`
- App components: `web/src/lib/components/lor/`
- **Gallery:** run `make dev` and open <http://localhost:5173/gallery>. It renders
  every component and variant. It exists only in dev builds: production
  builds compile an empty page in its place and its `load()` returns 404.
- **Screenshots:** `make screens SCREENS_DIR=/tmp/shots` seeds realistic data
  (hosts, agents, pending requests, active grants with command logs, offers,
  audit history, passkeys, sessions) and captures every page at 1280 px and
  390 px.

## Principles

1. **Minimal, but an app.** Grouped (inset) lists, rows with leading icon
   tiles, trailing metadata and chevrons, section headers, segmented
   controls, sheets. No decorative chrome, no gradients, no shadows on
   surfaces.
2. **Color is information.** Grey is the default. A color appears only when
   it tells the admin something: needs a decision, active, expiring,
   refused, risky. If everything is colored, nothing is.
3. **The phone is the primary device.** Approval links are opened from chat
   on a phone. Decisions sit in a sticky action bar; touch targets are at
   least 40 px; confirmations are bottom drawers on phones.
4. **One obvious action per screen.** The accent (primary) button is the
   page's main action. Everything else is secondary, outline or ghost.
5. **Say it in text, too.** Every status color has a label; every icon-only
   button has an `aria-label`.

## Tokens

All tokens are CSS custom properties in `:root` and mapped into Tailwind's
theme (`@theme inline`), so they're used as utilities (`bg-card`,
`text-warning`, `bg-success-tint`, `text-footnote`, …). Never use raw hex
values or Tailwind palette colors (`text-red-400`) in components.

### Surfaces

| Token | Value | Use |
|---|---|---|
| `background` | `#232326` | the page |
| `card` | `#2b2b2f` | grouped lists, cards, description lists |
| `card-hover` | `#323236` | hovered / pressed row inside a card |
| `popover` | `#2f2f34` | dialogs, drawers, toasts |
| `secondary` | `#36363c` | quiet buttons, selected nav item and segment |
| `muted` | `#2f2f34` | code wells, expanded audit details |
| `control-border` | `#85858e` | input, select, textarea and checkbox outlines (at least 3:1 against their fill) |
| `border` | `#3a3a40` | card and input edges |
| `separator` | `#36363b` | hairlines between rows inside a card |
| `foreground` / `muted-foreground` | `#ececee` / `#a9a9b2` | primary / secondary text |
| `icon-muted` | `#85858e` | chevrons, decorative icons (non-text, ≥ 3:1) |
| `primary` | `#9db9ee` | the accent: main action, selection, focus ring |

Elevation comes from lightness (background → card → popover), not shadows.
The only shadow is `shadow-overlay`, for dialogs and drawers.

### Semantic tones

Each tone is a triple: `--<tone>` (text/icon), `--<tone>-tint` (background),
`--<tone>-edge` (border). In components, pass a `tone` prop
(`'neutral' | 'info' | 'success' | 'warning' | 'danger' | 'violet' | 'teal'`);
`danger` maps to the `destructive` tokens.

| Tone | Means | Examples |
|---|---|---|
| `warning` (amber) | needs your decision · expiring soon · sudo (root) | pending requests, "Needs approval", countdown in the last 5 min, sudo badge, Hypervisor badge |
| `success` (green) | access given, healthy | Active, Approved, Authorized events, "This browser" |
| `danger` (red) | risk to Lor itself · refused or ended by you · destructive | Lor infra, Denied, Revoked, changed host key, countdown in the last minute, Revoke buttons |
| `info` (blue, = accent hue) | offers, informational | Offered, "Offer created", passkeys |
| `neutral` (grey) | ended normally, inactive, default | Expired, Released, Timed out, Disabled |
| `violet`, `teal` | audit categories only (agents; hosts and certificates) | never a status, never outside the audit log |

Identity tiles outside the audit log (hosts, agents, certificates) are
neutral unless the item carries risk (Lor infra = danger, hypervisor =
warning) or needs attention (not enrolled = warning). An unclaimed offer's
time bar is blue (`info`); an active grant's is green until it's expiring.

Server-computed warnings (`lor_infra`, `root_access`, `hypervisor`) use the
tone of their server `severity` and are shown exactly as the server words
them.

### Type scale

Inter (variable, self-hosted via `@fontsource-variable/inter`; no font CDN).
Body is 15 px. Use these utilities, not raw sizes:

| Utility | Size / weight | Use |
|---|---|---|
| `text-title` | 30 / 650, tight | page titles (one per page) |
| `text-headline` | 18 / 600 | dialog titles, hero numbers |
| `text-heading` | 15 / 600 | section headers, empty-state titles |
| `text-body` | 15 / 400 | row titles, values, paragraphs |
| `text-callout` | 14 / 400 | row subtitles, descriptions, callout text |
| `text-footnote` | 13 / 400 | metadata, footnotes, timestamps |
| `text-caption` | 12 / 500 | badges, counters |

`nums` gives tabular figures (times, countdowns, counts). Monospace
(`font-mono`) is for addresses, fingerprints, commands and keys.

The merge helpers (`cn()` and tailwind-variants) are configured
(`design/merge.ts`) so these sizes don't collide with text colors.

### Spacing, radii, motion, focus

- Rhythm: 4 px grid. Sections are `space-y-7`/`space-y-8` apart; a section
  header sits 8 px above its list; rows are `px-4 py-3` with a 56 px minimum
  height (44 px compact).
- Radii: `rounded-xl` (14 px) for cards and lists, `rounded-md` (8 px) for
  buttons and inputs, `rounded-full` for badges and chips.
- Motion: `--duration-fast` 120 ms, `--duration-medium` 200 ms,
  `--ease-standard`. Only color, opacity and small transforms animate.
  `prefers-reduced-motion` turns animations and transitions off globally.
- Focus: a solid 2 px accent outline with a 2 px offset (`focus-ring`, and
  built into buttons), so it shows on every surface including the accent
  button; `focus-ring-inset` (solid inset ring) for full-width rows inside
  cards. Inputs keep their accent border + halo.
- Touch targets: 40 px controls; small buttons (`sm`, `icon-sm`) and chips
  keep their look but get an invisible 44 px hit area (`after:` inset).

## Components

### Layout

| Component | Purpose |
|---|---|
| `Shell` | app frame: quiet side navigation on desktop, labelled icon navigation on phones, pending count on Home, skip link and route guard |
| `PageHeader` | back link (detail pages), leading tile, title (sets `<title>`), subtitle, `meta` badges, `actions` |
| `Section` / `SectionHeader` | a page region: title, count, description, action, footnote |
| `Card` | outlined surface for free-form content (forms, summaries) |
| `SummaryCard` | linked count, label and explanation; semantic color on nonzero counts, neutral zero state |
| `ActionBar` | the page's decision buttons in one row (secondary first, primary `flex-1`, label may wrap); sticky at the bottom when space allows; in the flow below 360 px width or 700 px height |
| `AuthCard` | signed-out screens (login, setup) |

### Lists and details

| Component | Purpose |
|---|---|
| `List` | grouped inset list (`<ul>`/`<ol>`, card surface, hairline dividers) |
| `ListRow` | link (`href`), button (`onclick`) or static row: `leading`, title/subtitle, `meta`, chevron, separate `trailing` controls, disclosure (`expanded`/`controls`: a chevron-down that turns, distinct from the navigation chevron), `tone`, `density`, phone metadata stacking |
| `DescriptionList` / `DescriptionItem` | label/value pairs, `inline` or `stacked`, `mono` |
| `SettingsRow` | a setting with icon, description and a control (`Switch`, button) labelled and described via `for` |
| `AuditEventRow` | outcome-toned category icon, label, who · what · grant, time; discloses details |
| `Timeline` | a grant's history with connecting line |
| `CommandLog` | commands reported by the CLI, as plain text |

### Status and identity

| Component | Purpose |
|---|---|
| `IconTile` | tinted square with an icon or monogram; `tone`, `size` |
| `StatusBadge` | grant/extension status (badge + dot, `data-status`) |
| `LevelBadge` | sudo (warning) / standard user |
| `StatusDot` | dot with optional pulse (pending) and `label` |
| `Countdown` | time left; turns warning ≤ 5 min, danger ≤ 1 min; text or badge |
| `TimeBar` | fraction of the window left (decorative) |
| `ui/badge` | tone badge for categories and states (`sm`/`md`) |

### Input and filtering

| Component | Purpose |
|---|---|
| `SegmentedControl` | 2–5 mutually exclusive views; radio group with arrow keys |
| `ChipGroup` | wrap-around or scrolling choice chips (durations, categories); radio group |
| `SearchField` | filter input with icon and clear button |
| `Toolbar` | groups filters above a list |
| `Field` | label + control + hint/error; the control gets `aria-describedby`/`aria-invalid` automatically (via `design/field-context.ts`) |
| `ChoiceCard` | large radio option with explanation (access level) |
| `DurationPicker` | presets + custom duration with validation |
| `ui/button`, `ui/input`, `ui/textarea`, `ui/checkbox`, `ui/switch`, `ui/native-select`, `ui/label` | primitives |

### Feedback

| Component | Purpose |
|---|---|
| `Callout` | message that needs reading; tone + icon + title |
| `Warnings` | server warnings for the selected level (`data-warning`) |
| `UnverifiedText` | agent-written text: plain text, in full, labelled unverified |
| `CopyField` | command/message with copy button (announces "Copied") |
| `EmptyState` | icon, title (a heading), one sentence, next step; `inset`/`page`/`plain`, compact size for dashboard secondary states |
| `ErrorState` | failed first load: server message as is, retry, way out; 404-aware |
| `StaleNotice` | a background refresh failed: keep showing the last data, say how old, retry |
| `ListSkeleton`, `PageSkeleton`, `ui/skeleton` | loading placeholders (`aria-busy` + "Loading…"); dashboard layout includes summary cards and its two list regions |
| `ui/spinner` | in-button progress (e.g. while a passkey prompt is open) |
| `Confirm` | dialog on wide screens, bottom drawer on phones (bounded height, scrolling body, visible footer and overflow cue) |
| `ui/sonner` | toasts with tone-colored icons |

## Patterns

- **Workspace:** desktop navigation uses a 208 px rail from 1024 px; phones and tablets use a compact sticky navigation with five labelled destinations. Main lists use the available width; forms and detail pages stay at a readable 768 px maximum. The Home dashboard places decisions beside live access from 1280 px; Settings groups security controls beside read-only server configuration.
- **Page anatomy:** `PageHeader` → (callouts) → `Section`s, each a header plus
  a `List`, `DescriptionList` or `Card`. No hand-rolled surfaces in pages.
- **List → detail:** list rows link to a detail page with a back link
  (`/hosts` → `/hosts/nas`). Where there's no detail route (agents), the row
  opens a sheet (`Confirm`) with details and action rows.
- **Consent pages** (`/r/[id]`, `/e/[id]`): header (who wants what, status,
  approval deadline), server warnings, the agent's reason, request details,
  choices (level, duration), limits, and the sticky `ActionBar`. The sudo
  confirmation checkbox sits in the action bar and gates Authorize. The
  Authorize button restates level, host and duration.
- **Destructive actions** use the `destructive` button (tinted, not solid)
  and a `Confirm`. Danger rows in action lists use `tone="danger"`.
- **States:** every query renders loading (skeleton), error (`ErrorState`
  with the server's message and a retry) and empty (`EmptyState` with the
  next step) states. When a live page's background refresh fails, the last
  data stays on screen under a `StaleNotice` (check `isError && !data` for
  the error branch). Ended things use final wording ("No commands were
  reported"), never a promise ("appear here as …"). An empty state's action
  is secondary when the page header already has the primary one.
- **Agent words** are quoted (“…”) in previews, with one "unverified" cue
  per group (section description), and labelled unverified in full
  (`UnverifiedText`); the command log says it's agent-reported and
  unverified.
- **Revoking** always explains the v1 limit (`REVOKE_NOTE` in `status.ts`):
  lor ssh stops at once, but a saved certificate stays valid until it
  expires; a revoked grant repeats this while its last certificate is valid.
- **Detail pages put the live part first:** a grant shows time left and
  actions, warnings, a pending extension, then the command log, then the
  administrative details and history.
- **Passkey actions** show a `KeyRound` icon, and `Spinner` + "Waiting for
  passkey…" while the prompt is open.

## Rules

Do:

- Compose pages from components; add a variant to `variants.ts` (or a prop)
  instead of overriding classes in a page.
- Use `status.ts`/`audit.ts` to decide labels and tones, so the same state
  looks the same everywhere.
- Badge only states that need noticing (pending, sudo, Lor infra, not
  enrolled, disabled). A healthy host or agent has no badge.
- Keep row subtitles to one idea; put times and short values in `meta`.
- Give every icon-only button an `aria-label`, every list without a visible
  heading a `label`.
- Check new color pairs: `src/lib/theme.spec.ts` tests every token pair for
  WCAG AA automatically (new tones must be added as `--x`, `--x-tint`,
  `--x-edge`).

Don't:

- Render agent-written text as HTML. `{@html}` is a lint error; use
  `UnverifiedText` or plain interpolation.
- Reword, filter or recolor server warnings beyond their severity.
- Use color without a label, or use `violet`/`teal` for status.
- Add gradients, heavy shadows, solid colored backgrounds, or a second
  accent color.
- Load anything remote (fonts, icons, images). The CSP allows `'self'` only;
  icons come from the bundled `@lucide/svelte`.
- Use raw font sizes (`text-[13px]`) or palette colors (`text-amber-300`).

## Adding a component

1. Put styling variants in `design/variants.ts` (or reuse one).
2. Write the component in `components/lor/` (app) or `components/ui/`
   (primitive) with typed `$props()`, a top comment saying when to use it,
   and semantics/ARIA built in.
3. Add it to the gallery (`src/routes/gallery/+page.svelte`) with its
   variants, and to the inventory above.
4. Pure logic (mapping, formatting) goes in a `.ts` module with a Vitest spec.

## Interaction details

- Home sorts active access by expiry and links directly to a grant expiring within five minutes. Summary links work in both zero and nonzero states.
- Consent repeats the selected level and duration near its header, alongside the requested duration when it differs, and offers a direct jump to the controls. Full server warnings and the agent's reason remain visible as plain text.
- Audit uses a native category picker on phones and wrapping chips on larger screens. Grant-number entry is debounced; a new filter keeps previous results with an explicit updating notice until matching data arrives.
- Disabling an agent requires confirmation naming its consequences. A failed disable or sign-out keeps the current view available for retry. Backend authentication and authorization requirements are unchanged.

- Grant actions and confirmations identify host, agent, level and grant ID;
  accessible active-link names also retain expiry. Pending-link names stay
  concise while the full unverified reason remains on the consent page.
- Audit discards late pagination responses from an earlier filter. Invalid
  grant numbers show an inline error; # prefixes are accepted. Viewing
  older history pauses automatic refresh, with a Back to latest action.
- Narrow audit and host rows stack metadata; long description values can
  use the responsive layout. Selected segments include an accent marker.

- Pending-request and audit rows use a compact layout below 400 px, preserving
  identity, risk labels, timestamps and 44 px targets while hiding decorative
  leading tiles. Audit offers a return-to-filters link when the toolbar is
  out of view, and repeats paused-history controls near the toolbar.
- Consent provides Read full reason from its header. Compact windows also
  have Go to decision after the choices. These links focus their destination;
  they do not alter the selected access or acknowledgement.
- Timeline entries keep recorded order. Equal timestamps carry an explanation
  of their one-second precision instead of implying an exact action sequence.
