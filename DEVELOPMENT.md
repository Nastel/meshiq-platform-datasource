# Development & debugging

Build the plugin on the host (the dev container has Go, Node, and mage); a plain Grafana
container serves the built `dist/`. Run and debug through VS Code tasks.

## Prerequisites

Open this folder in VS Code and **Reopen in Container** — the dev container
([.devcontainer/](.devcontainer/)) provides Go, Node, mage, delve, and docker-in-docker.

## Configure the datasource

Credentials come from a gitignored `.env` (no secrets in git), injected into the provisioned
"meshIQ Platform" datasource:

```bash
cp .env.example .env    # set MESHIQ_SERVICE_URL and MESHIQ_ACCESS_TOKEN
```

Restart Grafana after editing `.env`.

## Run

**Ctrl/Cmd+Shift+B** runs **`Run: dev`** — the full loop: backend watch + frontend watch +
Grafana at **http://localhost:3000**. A backend `.go` save rebuilds and restarts the container;
a frontend save hot-reloads the browser.

To run the container alone against an existing `dist/`, use **`Run: Grafana`** /
**`Run: stop Grafana`**.

## Debug

Start `Run: dev`, then pick a config in the **Run and Debug** view (defined in
[.vscode/launch.json](.vscode/launch.json)):

- **Backend: attach to delve** (F5) — first hit the datasource once (Save & Test or a query) so
  Grafana spawns the plugin process; delve can't attach before it exists. This config's
  preLaunchTask starts the delve watcher, waits for `:2345`, and attaches. Breakpoints go in
  `pkg/plugin/*.go`. A `.go` save reloads and replaces the process, ending the session — just
  re-launch (the watcher re-opens `:2345`).
- **Frontend: debug in Chrome** — launches Chrome on your machine with source maps back to `src/`
  (set `"type": "msedge"` for Edge). Browser DevTools (Sources → `webpack://…/src/…`) also work
  with no setup.
- **Attach both (backend + frontend)** — compound that runs both of the above at once, on top of a
  running `Run: dev`.

## Tasks

`Run: dev` covers everyday work. The rest are for building artifacts and one-off runs.

**Build** — compile only, no container:

| Task | Does |
|---|---|
| `Build: frontend` | webpack build of `src/` → `dist/` (`module.js`) |
| `Build: backend (current platform)` | `mage build:linux` — the container's arch; quickest for a local run |
| `Build: backend (all platforms)` | `mage buildAll` — every arch a release ships (what CI builds) |
| `Build: all (backend + frontend)` | both of the above — a complete `dist/` |

**Run:**

| Task | Does |
|---|---|
| `Run: dev` (default) | backend watch + frontend watch + Grafana — the everyday loop |
| `Run: Grafana` | start the container against the current `dist/`, no watchers; pair with `Build: all` for a production-like check |
| `Run: stop Grafana` | `docker compose down` |

**Test / Check:**

| Task | Does |
|---|---|
| `Test: backend` · `frontend` · `e2e` | the three test suites |
| `Check: lint` · `typecheck` | static checks |

## Layout

| Path | What |
|---|---|
| [src/](src/) | frontend — datasource, query & config editors, jKQL completion (TypeScript/React) |
| [pkg/plugin/](pkg/plugin/) | backend — query handling, dataservice HTTP client, resources, health check (Go) |
| [pkg/jkql/](pkg/jkql/) | backend — jKQL result-set → data-frame conversion and type handling |
| [provisioning/](provisioning/) | dev datasource auto-loaded into the container |
| [tests/](tests/) | end-to-end tests (`@grafana/plugin-e2e`) |
| [docker-compose.yaml](docker-compose.yaml), [docker/](docker/) | the dev Grafana container and its startup script |
| [scripts/](scripts/) | delve attach helpers (used by the debug configs) |
| [.config/](.config/) | build config managed by create-plugin — **don't edit** |

Logs are in `logs/grafana.log` (previous runs in `logs/archive/`).
