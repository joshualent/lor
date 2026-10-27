# Lor agent instructions

Lor is an SSH certificate authority for a homelab. It gives AI agents
short-lived, admin-approved SSH access to machines without leaving standing
access. One Go binary (`lor-server`) serves the API and an embedded SvelteKit
SPA; `lor` is the agent CLI.

## Before working

- Read [docs/plan.md](docs/plan.md), the agreed design, and
  [docs/deviations.md](docs/deviations.md) for where v1 differs from it.
  Read [docs/runbook.md](docs/runbook.md) for operations and
  [docs/design-system.md](docs/design-system.md) before touching the UI.
- Changes to the security model (certificate refusal rules, principals,
  listeners, step-up, CSRF) need the user's agreement. Record agreed
  deviations in `docs/deviations.md`.

## Safety rules

These rules always apply. The user's current instruction can override them for
that task only.

- **Never in Git**: CA keys, TLS keys, tokens, setup codes, passkey data,
  passwords or `.dev-data/`.
- **Hosts**: only the Proxmox host `nas` and CTs 216–218 are in scope. Don't
  scan or log in to other machines, and don't touch the router, DNS,
  Tailscale or the user's Caddy. Don't change `nas` networking, storage,
  cluster or SSH config.
- **Live preview**: don't redeploy, restart services or change firewall rules
  unless the task authorizes it. Deployment notes and secrets live outside
  the repo in `/home/agent/projects/lor-deploy/`; read `DEPLOYMENT.md` there
  and never paste its contents into chat or commits.

## Working style

- Go is at `~/.local/go/bin`. Build with `make build` (the SPA is embedded, so
  build the web first). Run `make test` before committing.
- The API is defined in `api/*.yaml`. Change the spec, run `make generate`
  and update every consumer in the same change; CI checks for drift.
- UI: keep the charcoal grey palette and the minimal buttons, navbar and
  inputs. Use the components and variants in `web/src/lib/design/`; color
  only where it carries meaning. Never use `{@html}`.
- Make coherent local commits at checkpoints. Don't push unless the user asks.
