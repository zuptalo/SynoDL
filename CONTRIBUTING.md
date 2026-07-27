# Contributing to SynoDL

Thanks for your interest in SynoDL — a mobile-first, self-hostable client for
Synology Download Station (an installable Vue 3 + Ionic PWA backed by a small Go
proxy). This guide covers how we branch, test, and ship. For the architecture and
the conventions behind the code, read [`CLAUDE.md`](CLAUDE.md) first — it is the map.

## License of contributions

SynoDL is licensed **AGPL-3.0-only** (see [`LICENSE`](LICENSE)). By submitting a
contribution you agree it is licensed under the same terms. The network-copyleft
clause (AGPL §13) is intentional: anyone running a modified SynoDL server over a
network must offer its source.

## The credential-safety invariant (read this)

The server is a **stateless, credential-free proxy**: it persists nothing, holds
no credentials, and forwards only an explicit allowlist of DSM Web APIs to the
single operator-configured NAS (`SYNO_URL`). The NAS password crosses the server
only inside the login forward; the session id (`sid`) lives on the client and
rides each request in the `X-Syno-Sid` header. A PR that stores state on the
server, logs a secret, or widens the proxy beyond the allowlist will not be
accepted. When in doubt, say how your change keeps the server empty-handed.

## Local setup

Requires **Go 1.26** and **Node 22**. No Docker and no real NAS needed for dev.

```sh
npm install
make start      # mock DSM (:8291) + synodl (air hot-reload) + Vite, all at once
```

The app comes up on http://localhost:5273 and proxies the API to `synodl` on
`:8280`, which talks to the in-repo mock DSM. Log in with `admin` / `secret`
(OTP account: `otpuser` / `secret`, code `000000`). To develop against a real
NAS instead: `SYNO_URL=https://nas:5001 make backend`.

## Branching model (trunk-based)

- **`main`** is the only long-lived branch — the default branch, protected, and
  what gets deployed.
- **Feature branches** branch off `main` and open a PR back into `main`.
  Name them descriptively, e.g. `feat/0002-search-modules`, `fix/2001-stalled-eta`.
- A PR **auto-merges the moment its required checks are green** (the
  `Auto-merge PRs` workflow schedules it; branch protection's required checks —
  `CI gate`, `Roadmap up to date`, `Version guard` — are the gate). Mark a PR
  as a draft to keep it from merging while you iterate.
- **Every merge to `main` publishes**: release.yml re-verifies the merge commit
  and pushes the rolling `latest` + immutable `main-<sha>` images.

`main` is protected: changes land **only via pull request with all CI checks
green** (see [the branch-protection setup](#branch-protection)). Direct pushes
are rejected.

## Spec-driven development (required for new work)

SynoDL uses [Spec Kit](https://github.com/github/spec-kit). Anything that adds or
changes behavior starts as a **numbered spec** and moves through a fixed pipeline
before code is written. The governing principles live in
[`.specify/memory/constitution.md`](.specify/memory/constitution.md) — read it
once; it is the contract every spec is checked against (including the
non-negotiable stateless-proxy boundary).

**When this applies.** New features, behavioral changes, and non-trivial bug fixes
go through the full pipeline. Pure docs, comments, formatting, CI tweaks, and
one-line typo fixes do not need a spec — use judgement, and lean toward a spec
whenever a change has user-facing behavior or touches the proxy.

### Spec numbers and categories

Each spec lives in `specs/<NNNN-slug>/` and the number encodes its category:

| Category  | Band       | What it's for                                  |
|-----------|------------|------------------------------------------------|
| `planned` | `0001–0999`| Roadmap features                               |
| `adhoc`   | `1001–1999`| Unplanned but deliberate work                  |
| `hotfix`  | `2001+`    | Bug fixes / hotfixes                           |

Create one with the tracked helper. It allocates the next free number in the band,
creates the **branch** (`feat/NNNN-slug` for planned/ad-hoc, `fix/NNNN-slug` for
hotfixes), creates the **flat** spec directory `specs/NNNN-slug/` (only the branch
carries the `feat/`·`fix/` prefix — the folder never does), and points the speckit
commands at it:

```sh
make spec CATEGORY=planned DESC="Add search-module support"      # branch feat/0002-…
# or directly:
scripts/spec-new.sh hotfix "Fix ETA display for stalled tasks"   # branch fix/2001-…
```

Then follow this **required pipeline**, in order (the `/speckit-*` commands are
agent skills tracked in `.claude/skills/`):

1. **`/speckit-specify`** — fill the spec content in the directory `spec-new.sh`
   created (it reads `.specify/feature.json`; don't let it mint a new directory).
2. **`/speckit-clarify`** — resolve ambiguity before planning.
3. **`/speckit-plan`** — produce the implementation plan and design docs.
4. **`/speckit-tasks`** — generate the ordered, dependency-aware task list. Tasks
   put failing tests **before** the implementation that satisfies them (TDD).
5. **`/speckit-analyze`** — cross-artifact consistency check. It only reports; if it
   flags something, fix the artifact at fault (spec, plan, **or** tasks) and re-run
   from there down. Must be clean (or findings explicitly waived) before implementing.
6. **`/speckit-taskstoissues`** — open one GitHub issue per task (title, body, and
   labels) in `zuptalo/SynoDL`. Note the issue numbers for the PR.
7. **`/speckit-implement`** — work the tasks in order.

`/speckit-checklist` is **required** for any spec touching the credential boundary
or the DSM API allowlist, and optional otherwise.

As work progresses, bump the spec's **`**Status**:`** line
(`planned → in-progress → in-review → shipped`) — it drives the roadmap.

### ROADMAP.md is generated

[`ROADMAP.md`](ROADMAP.md) is **generated** from the specs — never hand-edit it.
Regenerate after adding a spec or changing a Status:

```sh
make roadmap        # == python3 scripts/roadmap-gen.py
```

CI's `Roadmap up to date` check fails the build if the committed `ROADMAP.md`
doesn't match `specs/`, so regenerate and commit it alongside your spec changes.

### Closing issues on merge

The PR into `main` **must reference every issue it implements** with a closing
keyword (`Closes #123`, one per line) so GitHub auto-closes them on merge. This
works because `main` is the repository's default branch — closing keywords only
fire on merges into the default branch.

## Making a change

1. Start a spec (`make spec …`) and run it through the pipeline above; it puts you
   on the feature branch.
2. Make your change, matching the surrounding code (see Code style in `CLAUDE.md` —
   we favor explanatory comments on the *why*).
3. Run the gates locally (below). Add or update tests (tests first, per TDD).
4. Open a PR into `main`. Fill in the PR template, including the
   credential-safety confirmation for anything touching the proxy, and
   `Closes #N` for each issue. Bump the version in the same PR if this change
   should ship as a tagged release (see Releases below).
5. Once CI is green, the PR merges itself. Your feature branch is deleted
   automatically on merge (protected `main` is never auto-deleted), and the
   referenced issues close themselves.

### Commit messages

Conventional Commits with a scope, e.g. `feat(tasks): …`, `fix(login): …`,
`feat(server): …`, `test(e2e): …`, `ci: …`, `docs: …`. The subject describes
user-facing behavior, not internals.

## Test gates (run before opening a PR)

These mirror exactly what CI runs (`.github/workflows/build-test.yml`):

```sh
npm run build                 # client: vue-tsc --noEmit (typecheck) THEN vite build
npm run test:unit             # client: Vitest unit tests (pure modules + idb)
cd server && go test ./...    # server unit tests (fake syno.Client, NO NAS needed)
npm run test:e2e              # Playwright e2e (builds + boots its own synodl + mock DSM)
```

- Server tests need no NAS and no network: handlers run against a fake
  `syno.Client`; the DSM client runs against an `httptest` fake DSM. Each handler
  file has a sibling `_test.go` — keep that pattern.
- The e2e harness builds and boots an isolated `synodl` + `synomock` pair on their
  own ports; it does **not** touch your `make start` stack.
- **Path-filtered CI:** a PR that changes only docs/specs/tooling (`**/*.md`,
  `specs/**`, `.specify/**`, `scripts/**`, `ROADMAP.md`) skips the heavy
  build/test/e2e suite — only the `Roadmap up to date` guard and the `CI gate`
  aggregate run. Any change under `src/`, `server/`, `e2e/`, deps, configs, the
  `Dockerfile`, or `.github/workflows/` runs the full suite. The single required
  check is **`CI gate`**, so doc-only PRs stay unblocked without it.

## Releases and release candidates

Every green merge to `main` already publishes a deployable image (`latest` +
`main-<sha>`). A **release** is simply a PR that also bumps `package.json`'s
version:

```sh
npm run release:patch    # 0.0.1 -> 0.0.2   (or release:minor / release:major)
```

Commit the bump inside the PR (its own PR, or riding the feature it ships).
When that PR merges, release.yml additionally tags `vX.Y.Z`, publishes the
`X.Y.Z` + `X.Y` image tags, and cuts a GitHub release whose notes are drawn
from the Conventional-Commit subjects since the last tag — another reason to
keep commit subjects clean. The
[release PR template](.github/PULL_REQUEST_TEMPLATE/release.md) is available
for bump PRs (add `?template=release.md` to the compose URL).

The CI **`Version guard`** keeps versions safe to auto-merge: an unchanged
version passes (rolling images only); a downgrade, or a bump onto a version
whose tag already exists, blocks the merge.

- **Release candidate:** push a `vX.Y.Z-rc.N` tag. It runs the full suite and
  publishes a single immutable `:X.Y.Z-rc.N` image + a GitHub pre-release.
  An RC never moves `:latest`/`:X.Y`.

Operator upgrade/rollback guidance lives in [`docs/UPGRADING.md`](docs/UPGRADING.md).

## Branch protection

The exact protected-branch ruleset is applied (and re-applied) with
[`scripts/setup-branch-protection.sh`](scripts/setup-branch-protection.sh). See that
script's header for what it enforces and the one prerequisite (an authenticated
`gh`). Note: GitHub requires a paid plan for branch protection on **private** repos;
it is free once a repo is public.
