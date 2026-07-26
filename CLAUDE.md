# CLAUDE.md

Guidance for AI assistants working in this repository. Read this first; it
captures the architecture, workflows, and conventions that aren't obvious from
any single file.

## What SynoDL is

SynoDL is a mobile-first, self-hostable client for **Synology Download Station**
(the download manager app on Synology NAS/DSM). It ships as an installable
**PWA** (Vue 3 + Ionic) backed by a small **Go** proxy (`synodl`). The defining
constraint, which shapes nearly every design decision, is that the server is a
**stateless, credential-free proxy**: it persists nothing, holds no credentials,
and only forwards an explicit allowlist of DSM Web APIs to the single
operator-configured NAS (`SYNO_URL`). The NAS session id (`sid`) lives on the
client (IndexedDB) and rides each request in the `X-Syno-Sid` header. Never add
a server feature that stores state or widens the proxy beyond the allowlist.

SynoDL is licensed **AGPL-3.0-only** (see `LICENSE`). SynoDL is not affiliated
with Synology Inc.; Synology, DSM, and Download Station are trademarks of
Synology Inc.

## Monorepo layout

One repo, two parts, shipped as a single container.

- **Client** — repo root. Vue 3 + Ionic PWA, built with Vite to `dist/`.
  - `src/views/` — pages. `tabs/` are the five bottom-tab roots (Tasks, Search,
    Browser, RSS, Settings); `LoginPage.vue` sits outside the tabs behind the
    session gate.
  - `src/components/` — reusable UI (task rows, filter sheet, new-task modal,
    folder picker).
  - `src/composables/` — Vue composition functions (`useSession`, `useTasks`,
    `useLiveQuery`, `useAppUpdate`). Reactive app state lives here.
  - `src/services/` — non-UI logic: `api.ts` (HTTP wrapper, attaches
    `X-Syno-Sid`, maps 401s), `session.ts`, plus pure modules (`task-sort.ts`,
    `syno-errors.ts`, `url-detect.ts`) that carry the unit-test coverage floors.
  - `src/db/` — IndexedDB layer. `idb.ts` is a tiny promise wrapper with a
    change-notification bus; `useLiveQuery` subscribes for reactivity.
  - `src/sw.ts` — minimal service worker (app-shell precache + prompt updates).
  - `src/router/index.ts` — routes + the session gate.
- **Server** (`server/`) — `synodl`, a Go 1.26 service on stdlib `net/http`
  (no web framework, **zero third-party dependencies**, no database).
  - `cmd/synodl/main.go` — entrypoint: config → syno client → router →
    graceful shutdown.
  - `cmd/synomock/main.go` — the **mock DSM**: a fake Synology NAS used by
    `make start` and the e2e suite so development never needs real hardware.
  - `internal/api/` — routing (`router.go`) + handlers (one file per area,
    each with a `_test.go` against a fake `syno.Client`).
  - `internal/syno/` — the DSM Web API client: the API **allowlist**,
    `SYNO.API.Info` version discovery (absorbs DSM6/DSM7 differences), typed
    error mapping. Tested against an `httptest` fake DSM.
  - `internal/synomock/` — the mock DSM implementation (shared by
    `cmd/synomock` and tests), with `/__mock/*` control endpoints.
  - `internal/{config,httpx}/` — env config (fail-fast), HTTP middleware
    (recover → log → CORS), JSON responses, rate limiting.
- **`e2e/`** — Playwright tests, hermetic: they build and boot their own
  `synodl` + `synomock` pair (no real NAS, no shared state with `make start`).

In production a single image runs `synodl`, which serves the built PWA at `/`
(via `STATIC_DIR`) and the API at `/v1` and `/healthz` on the same origin. In
dev, Vite serves the client and proxies the API to a local `synodl`.

## Local development

Requires **Go 1.26** and **Node 22**. No Docker needed for dev.

```sh
make start      # mock DSM (:8091) + synodl (air hot-reload, :8080) + Vite (:5173)
```

App comes up on http://localhost:5173 and proxies the API to `synodl` on
`:8080`, which talks to the mock DSM on `:8091`. Log in with the mock account
`admin` / `secret` (the OTP account is `otpuser` / `secret` + code `000000`).
Other targets: `make backend` / `make frontend` / `make mock` (run one piece),
`make stop`. Inside `server/`: `make run`, `make test`, `make vet`, `make fmt`,
`make tidy`. Point the backend at a real NAS with `SYNO_URL=https://nas:5001`
(add `SYNO_TLS_INSECURE=true` only for self-signed certs).

## Build, typecheck, and test

Run the relevant gate before claiming work is done. These are the exact checks
CI runs (`.github/workflows/build-test.yml`):

```sh
npm run build                 # client: vue-tsc --noEmit (typecheck) THEN vite build
npm run test:unit:coverage    # vitest + coverage floors (pure-module allowlist)
cd server && go build ./...   # server build
cd server && go vet ./...     # server static checks
cd server && go test ./...    # server unit tests (fake syno.Client + httptest, NO NAS needed)
npm run test:e2e              # Playwright e2e (builds + boots its own synodl + mock DSM)
```

- `npm run build` is the typecheck. There is no separate lint script for the
  client. After editing TypeScript/Vue, run `npm run build` to confirm it still
  typechecks.
- Server tests need **no NAS and no network**: handlers are tested against a
  fake `syno.Client`; the real client is tested against an `httptest` fake DSM.
  Each handler file has a sibling `_test.go`; keep that pattern.
- The e2e harness (`e2e/global-setup.ts`) builds `synodl` + `synomock`, boots
  them on isolated ports (`:8081` / `:8091`) and a test Vite on `:5174`. Tests
  seed deterministic task fixtures through the mock's `/__mock/*` control
  endpoints. It does NOT touch your `make start` stack.

## Key architectural conventions

**Stateless, credential-free proxy.** The server persists nothing — no
database, no files, no volumes. Credentials cross the server only inside the
login forward; the `sid` returns to the client and rides each request in
`X-Syno-Sid`. Never log credentials, sids, OTP codes, or full task URIs. The
proxy exposes typed `/v1` endpoints only — it is NOT a transparent `/webapi`
passthrough, so the DSM API allowlist is structural (`internal/syno`).

**DSM version differences** are absorbed in `internal/syno` via `SYNO.API.Info`
discovery (cached per-API path + max supported version), not by UI branches.
DSM error codes map to typed errors once, server-side (105/106/107/119 →
session-expired → HTTP 401 → client returns to login).

**Offline-first client data.** IndexedDB is the source of truth for app data
(session, settings, favorites, history). Writes go through `src/db/idb.ts`,
which fires a change-notification bus; `useLiveQuery` subscribes so the UI is
reactive. Bump `DB_VERSION` and extend `onupgradeneeded` when adding an object
store. Download tasks themselves are never persisted — the NAS is the source of
truth; the app polls.

**Server HTTP style.** Stdlib `net/http` with method+pattern routes
(`mux.Handle("GET /v1/tasks", ...)`). Handlers depend on the small
`syno.Client` interface defined in `internal/syno`; tests pass fakes.
Middleware chain is recover → log → CORS; `POST /v1/session` is rate-limited
per IP so the proxy can't be used to brute-force the NAS.

**Versioning/update UX.** The build stamps the same `VERSION` into the PWA
(`__APP_VERSION__`) and the Go binary (`main.version`). The PWA is
`registerType: 'prompt'` — a new deploy never silently reloads; it surfaces a
prompt and applies on user accept (`useAppUpdate` + `sw.ts` `SKIP_WAITING`).

## Spec-driven development

New behavior is built spec-first with **Spec Kit**. The governing principles —
including the non-negotiable stateless-proxy boundary and a TDD mandate — live
in `.specify/memory/constitution.md`; every spec is checked against it. Full
contributor walkthrough is in `CONTRIBUTING.md`.

- **Start a spec** with `make spec CATEGORY=<planned|adhoc|hotfix> DESC="…"` (or
  `scripts/spec-new.sh …`). The number encodes the category: planned `0001+`,
  ad-hoc `1001+`, hotfix/bug `2001+`. The helper allocates the next free number in
  the band, creates the branch (`feat/NNNN-slug` for planned/ad-hoc, `fix/NNNN-slug`
  for hotfixes) and the **flat** `specs/<NNNN-slug>/spec.md` (the directory is never
  prefixed — only the branch is), and writes `.specify/feature.json` (gitignored) so
  the speckit commands target it.
- **Required pipeline** (the `/speckit-*` agent skills, in order): `specify →
  clarify → plan → tasks → analyze → taskstoissues → implement`. `analyze` only
  reports — fix the flagged artifact (spec/plan/tasks) and re-run downstream until
  clean before implementing. `checklist` is required for specs touching the
  credential boundary or the DSM allowlist.
- **`ROADMAP.md` is generated** from `specs/` — never hand-edit it. Run
  `make roadmap` after adding a spec or changing a `**Status**:` line
  (`planned → in-progress → in-review → shipped`); CI's `Roadmap up to date` guard
  fails if it's stale.
- **Auto-close issues**: `taskstoissues` opens one GitHub issue per task; the
  feature → `develop` PR must list `Closes #N` for each so they close on merge
  (works because `develop` is the default branch).
- **Bootstrap note**: the initial scaffold commits (speckit workflow, server,
  client shell, e2e harness, CI/CD) predate the constitution's enforcement —
  everything after them goes through specs.

## Git, branching, and releases

GitFlow. **`develop`** is the integration branch; **`main`** is production.

- Every PR runs the full build+test suite (path-filtered: doc/spec/tooling-only
  changes skip the heavy jobs). A push to `develop` does NOT re-run the suite —
  the merged PR already tested the identical tree — it only rebuilds and publishes
  the rolling `ghcr.io/zuptalo/synodl:develop` image.
- **Bump the version at the start of each release cycle.** After a release ships,
  `develop` and `main` sit at the same `package.json` version, so the next
  `develop → main` PR fails the release guard until `develop` moves forward. The
  first change of a new cycle bumps `develop`'s `version` to the next intended
  value (patch by default). This is manual on purpose — GitHub Actions can't open
  the bump PR (org policy blocks Actions from creating PRs), and the release guard
  enforces it at release time. See constitution "Development Workflow."
- **Scan the latest image at the start of new work.** Check the Docker Scout report
  for the current `zuptalo/synodl` tag (Docker Hub) and apply any vulnerability that
  has a fix version: bump the Go module or the base image in `Dockerfile`, rebuild +
  test, ride the same branch. "No fix available" ones are noted and left. A
  CVE-patch bump is `fix`/`security`-typed (so it reaches "What's new") and gets
  released, not parked.
- Releases are driven by `package.json` `version`: bump it on `develop`, open a
  PR into `main`. That PR **auto-merges** once green (`auto-merge-release.yml`),
  which then dispatches `release.yml` (a workflow-token push can't trigger it
  directly). If green and the `vX.Y.Z` tag doesn't already exist, it tags `main`,
  publishes the production image (`latest`, `X.Y.Z`, `X.Y`), and cuts a GitHub
  release. A merge without a version bump re-runs CI but does not re-release.
- Release candidates are cut by pushing a `vX.Y.Z-rc.N` tag (off `develop`):
  `release-candidate.yml` runs the full suite and, if green, publishes a single
  immutable `:X.Y.Z-rc.N` image + a GitHub pre-release. RCs never move `:latest`
  or `:X.Y`; the RC version comes from the tag, not `package.json`. Operator
  upgrade/rollback runbook: `docs/UPGRADING.md`.

**Commit messages** follow Conventional Commits with a scope, e.g.
`feat(tasks): ...`, `fix(login): ...`, `feat(server): ...`, `test(e2e): ...`,
`ci: ...`, `docs: ...`. The subject describes user-facing behavior, not internals.

**Release-note subjects for end users** (constitution Principle V). For user-facing
types (`feat`, `fix`, `perf`, `security`) the subject *after* the `type(scope):` prefix is
shown verbatim to users as the "What's new" line on update. Write it as plain-language,
benefit-focused release-note copy — no internal jargon, no implementation shorthand, and
no spec/issue/PR references (`(spec 1016)`, `(#248)`, `US2/US3`, `FR-014`).

- ✅ `feat(tasks): pause and resume downloads with a swipe`
- ❌ `feat(tasks): wire Task.pause via fake-client handler (spec 0001, US4)`

Non-user-facing types (`chore`, `ci`, `build`, `docs`, `refactor`, `style`, `test`,
`deps`) never reach "What's new", so they keep developer phrasing.

### Working in this environment

- Develop on the branch the task assigns; create it locally if missing. Commit
  with clear messages and push with `git push -u origin <branch>`. Do **not**
  push to a different branch without explicit permission.
- Do **not** open a pull request unless explicitly asked.
- Use the GitHub MCP tools (`mcp__github__*`) for any GitHub interaction when
  the `gh` CLI is not available.

## Code style

- **Match the surrounding code.** This codebase favors thorough explanatory
  comments on the *why* (not the *what*) — see the comments in `vite.config.ts`,
  `idb.ts`, `router.go`, `static.go`. New non-obvious code should carry similar
  reasoning; don't strip existing comments.
- TypeScript: ES modules, `@/` alias → `src/`. Vue 3 `<script setup>` + Ionic
  components. Composition API; reactive state via composables.
- Go: stdlib-first, small interfaces at call sites, `gofmt`'d, table-ish tests
  against fakes. Run `go vet` before finishing.
- Keep the credential-safety invariant intact in every change that touches the
  proxy: no secrets in logs, no state on the server, no APIs beyond the allowlist.
