<!--
Sync Impact Report
- Version: 1.0.0 → 1.1.0 (MINOR: Development Workflow redefined from GitFlow to
  trunk-based; Principle VII's delivery bullet updated to match)
- Modified: "Development Workflow" — dropped the develop branch and the
  bump-at-cycle-start doctrine. main is now the single protected branch; every
  branch merges into it via a PR that auto-merges when the required checks are
  green; every merge publishes rolling images; a version bump inside a PR makes
  that merge a tagged release (Version guard keeps versions monotonic + unique).
  Principle VII: "feature→develop PR" → "PR into main" for the Closes #N rule.
- Prior version (1.0.0): initial ratification, derived from the Ring
  constitution v1.2.2 with the Stateless, Credential-Free Proxy principle
  replacing zero-knowledge/crypto and the Postgres principle dropped.
- Templates / docs reviewed for sync:
  - .specify/templates/*.md — ✅ no change needed (branch model not encoded there).
  - CLAUDE.md / CONTRIBUTING.md / README.md — ✅ updated in the same change.
- Deferred TODOs: none.
-->
# SynoDL Constitution

SynoDL is a mobile-first, self-hostable client for Synology Download Station: an
installable PWA (Vue 3 + Ionic) backed by a small stateless Go proxy (`synodl`)
that forwards an allowlisted subset of the DSM Web API to the operator's NAS. This
constitution governs every spec, plan, task, and change made in this repository.
It supersedes habit and convenience. Where a principle says **MUST**, a violation
blocks merge unless it is explicitly justified in the spec's *Complexity &
Exceptions* section and accepted by a maintainer. Where it says **SHOULD**, a
deviation must be reasoned in the plan.

## Core Principles

### I. Spec-Driven Development

No implementation without an approved spec, plan, and task list.

- All work is initiated by a numbered spec under `specs/` in its category band
  (planned `0001+`, ad-hoc `1001+`, hotfix/bug `2001+`). Code that lands without a
  spec id is a defect.
- The required pipeline for every work item, in order, is:
  **specify → clarify → plan → tasks → analyze → (fix the flagged artifact + re-run
  downstream) → taskstoissues → implement**.
- `/speckit-analyze` MUST be run and MUST be clean — or every finding explicitly
  waived in writing — before `/speckit-implement`.
- Every branch, commit, issue, and PR MUST be traceable to its spec id.

### II. Test-Driven Development

Tests come first wherever the work is testable.

- `tasks.md` MUST order failing tests before the implementation tasks that satisfy
  them (Red → Green → Refactor).
- New or changed proxy, session, or HTTP-handler logic MUST ship unit tests.
  New or changed user-facing behavior MUST add or extend an e2e spec under `e2e/`.
- Coverage floors are a ratchet: they may rise, never regress. The pure client
  modules (the vitest coverage allowlist) and `config`/`syno` (server) stay above
  their gated floors.
- A bug fix (`2001+`) MUST begin with a failing regression test that reproduces the
  bug before the fix lands.

### III. Stateless, Credential-Free Proxy (NON-NEGOTIABLE)

The server is a pass-through with no memory: it persists nothing and can leak
nothing it never held.

- The server MUST NOT persist any state — no database, no files, no volumes. A
  container restart loses nothing because there is nothing to lose.
- NAS credentials and session ids belong to the client. The password crosses the
  server only inside the login forward and MUST never be stored, cached, or
  logged; the resulting `sid` is returned to the client, kept in IndexedDB, and
  sent back on each request (`X-Syno-Sid`). Server-side session storage is a
  defect.
- Credentials, sids, OTP codes, and full task URIs MUST never appear in log lines,
  error payloads, metrics, or panics. Log the route and the outcome, not the
  secrets.
- The proxy forwards ONLY the explicit DSM API allowlist implemented in
  `server/internal/syno` to the single operator-configured target (`SYNO_URL`).
  No open-proxy behavior: no client-supplied target hosts, no passthrough of
  arbitrary `SYNO.*` APIs, no query-string smuggling past the typed `/v1`
  endpoints.
- Every spec MUST contain a **Credential-Safety Impact** section answering: what
  crosses the proxy, what is forwarded to the NAS, what could appear in logs or
  errors, and why nothing sensitive is retained.

### IV. Offline-First Client Data

IndexedDB is the source of truth on the device; upgrades never lose data.

- App data (session, settings, favorites, history) is written through
  `src/db/idb.ts` and its change-notification bus; the UI stays reactive via
  `useLiveQuery`.
- Adding or altering an object store MUST bump `DB_VERSION` and extend
  `onupgradeneeded` with a forward migration that preserves existing data.
- Task state itself is never persisted client-side beyond display caches — the
  NAS is the source of truth for downloads; the app re-syncs by polling.

### V. Quality Gates Are the Definition of Done

"Done" means the CI gates are green — nothing less.

- A change is not done until: `npm run build` (vue-tsc typecheck + vite build)
  passes; `go build ./... && go vet ./... && go test ./...` pass; vitest + its
  coverage floors pass; and e2e passes where behavior changed.
- Commits follow Conventional Commits with a scope describing user-facing behavior
  (`feat(tasks):`, `fix(login):`, `test(e2e):`, `ci:`, `docs:`).
- Release-note copy: for user-facing commit types (`feat`, `fix`, `perf`,
  `security`) the subject AFTER the `type(scope):` prefix is shown verbatim to end
  users as the "What's new" line, so it MUST read as plain-language, benefit-focused
  release-note copy — no internal jargon, no implementation shorthand, and no
  spec/issue/PR references (`(spec 1016)`, `(#248)`, `US2/US3`, `FR-014`). Write what
  the user gains ("Downloads can now be paused with a swipe"), not how it was built
  ("wire Task.pause through fake-client handler (spec 0001)"). Non-user-facing types
  (`chore`, `ci`, `build`, `docs`, `refactor`, `style`, `test`, `deps`) are exempt —
  they never reach "What's new".
- The PWA stays `registerType: 'prompt'`: a deploy MUST NOT silently reload; the
  prompt-and-accept update flow is preserved.

### VI. Ionic-First UI

The interface is built from stock Ionic, not hand-rolled widgets.

- New UI MUST be composed from stock Ionic components, styled only with the
  project's existing theme tokens (the `--app-*` CSS variables and the
  `ion-palette-dark` class). No ad-hoc per-component restyling and no hand-rolled
  widget that duplicates an Ionic primitive (e.g. don't build a custom list row,
  toggle, modal, or toast when `ion-item`, `ion-toggle`, `ion-modal`, `ion-toast`
  exist).
- A custom component is justified ONLY when no Ionic component covers the need;
  even then it MUST be composed from existing Ionic components with the minimum
  necessary customization, reusing the existing theme tokens rather than inventing
  new colours/spacings.
- Rationale: this keeps the UI consistent, accessible, theme-correct (light/dark +
  RTL), and upgrade-safe. Deviations are reasoned in the plan; a bespoke widget
  that an Ionic primitive could have provided is a defect.

### VII. Traceable, Auto-Closing Delivery

Every unit of work is visible from roadmap to merge.

- `ROADMAP.md` is generated from spec metadata, never hand-edited, and kept current
  by CI. Its sections are Planned (`0001+`), Ad-hoc (`1001+`), and Hotfixes & Bug
  Fixes (`2001+`).
- Each task (or task group) becomes a GitHub issue with a descriptive title, a
  comprehensive body drawn from the spec/plan, and labels for category band, spec
  id, and area.
- The PR into `main` MUST list `Closes #N` for every issue it implements so
  they auto-close on merge (`main` is the default branch; closing keywords only
  fire on merges into the default branch).

## Domain Constraints

These are project-specific guardrails every relevant spec MUST respect.

- **Single image.** Client and server ship as one container: `synodl` serves the
  built PWA at `/` and the API at `/v1` and `/healthz`. No sidecars, no volumes.
- **DSM API allowlist.** Every DSM API the proxy may call is declared and
  implemented in `server/internal/syno`; adding an API to the allowlist is a spec-
  level decision, not an implementation detail.
- **NAS TLS verification is on by default.** `SYNO_TLS_INSECURE=true` is an
  explicit operator opt-in for self-signed NAS certificates, applies only to the
  outbound NAS connection, and MUST stay default-off.
- **Mock-DSM dev parity.** Local dev (`make start`) and the hermetic e2e stack run
  against the in-repo mock DSM (`cmd/synomock`); neither MUST ever require a real
  NAS. DSM version differences are absorbed by `SYNO.API.Info` discovery in
  `internal/syno`, not by UI branches.
- **Not affiliated with Synology.** Synology, DSM, and Download Station are
  trademarks of Synology Inc.; user-facing copy and docs MUST NOT imply official
  status.

## Development Workflow

- **Trunk-based: one protected `main`.** `main` is the only long-lived branch
  and the GitHub default branch. Every change lands via a PR into `main` that
  runs the full verification suite; a green PR auto-merges (the Auto-merge
  workflow schedules it; branch protection's required checks are the gate).
  Direct pushes to `main` are blocked.
- **Every merge publishes.** Each push to `main` re-verifies the merge commit
  and publishes the rolling `latest` + immutable `main-<sha>` images. A merge
  is deployable by definition; there is no separate integration channel.
- **A release is a version bump inside a PR.** When a PR also bumps
  `package.json`'s version (patch by default; minor/major when warranted), its
  merge additionally tags `vX.Y.Z`, publishes the `X.Y.Z` + `X.Y` image tags,
  and cuts the GitHub release. The CI **Version guard** keeps versions
  monotonic and never reused: an unchanged version passes; a downgrade or a
  bump onto an already-shipped version blocks the merge.
- **Supply-chain scan at the start of new work.** Before starting a new feature or
  bug fix, review the Docker Scout vulnerability report for the latest published
  image (Docker Hub → `zuptalo/synodl` → the current tag). Any flagged
  vulnerability that has a fix version available MUST be applied as part of that
  work — bump the Go module (`go get pkg@fixed && go mod tidy`) or the base image
  (`Dockerfile`), rebuild and test, and let it ride the same branch. A
  vulnerability with **no fix available** upstream is noted (in the PR) and left
  until one exists. A fix that is itself a shippable improvement (dependency/base-
  image CVE patch) is `fix`/`security`-typed so it reaches users, and is released
  rather than parked.
- **Spec numbering.** Bands are assigned by category and never reused: planned
  `0001–0999`, ad-hoc `1001–1999`, hotfix/bug `2001+`. The next free number in the
  band is allocated by `.specify/scripts/bash/create-new-feature.sh --category`
  (via `make spec` / `scripts/spec-new.sh`).
- **Branch vs. directory.** The feature branch carries a GitFlow type prefix —
  `feat/NNNN-slug` for planned and ad-hoc work, `fix/NNNN-slug` for hotfixes — while
  the spec directory stays flat (`specs/NNNN-slug/`). Only the branch is prefixed.
- **Spec lifecycle.** A spec's `Status` moves `planned → in-progress → in-review →
  shipped`; the value drives its row in `ROADMAP.md`.
- **Gate sequencing.** `/speckit-clarify` runs before `/speckit-plan`;
  `/speckit-analyze` runs after `/speckit-tasks` and before `/speckit-implement`.
  `/speckit-checklist` is REQUIRED for any spec touching Principle III (the
  credential boundary or the DSM allowlist) and optional otherwise.

## Governance

- This constitution supersedes other practices. Amendments are made by a PR that
  edits this file and bumps its version using semantic versioning: **MAJOR** for a
  removed or redefined principle, **MINOR** for a new principle or section, **PATCH**
  for clarifications and wording.
- `/speckit-analyze` checks every spec/plan/tasks set against these principles;
  unresolved violations block `/speckit-implement` and merge unless waived in the
  spec's *Complexity & Exceptions* section with maintainer sign-off.
- Complexity must be justified: anything that adds a moving part, a dependency, or a
  server capability must show why a simpler, stateless-preserving option won't do.
- Runtime engineering guidance that is not constitutional lives in `CLAUDE.md` and
  `CONTRIBUTING.md`; where they conflict with this document, this document wins.

**Version**: 1.1.0 | **Ratified**: 2026-07-26 | **Last Amended**: 2026-07-27
