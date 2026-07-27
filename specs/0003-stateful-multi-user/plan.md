# Implementation Plan: Stateful multi-user rework

**Branch**: `feat/0003-stateful-multi-user` | **Date**: 2026-07-27 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/0003-stateful-multi-user/spec.md`

## Summary

Turn SynoDL from a stateless proxy into a self-hosted multi-user service on **constitution v2.0.0**.
A single **SQLite** database on one mounted volume holds the operator config (NAS connection + public
URL), SynoDL's own user accounts, per-user folder grants, push subscriptions, and the instance VAPID
keys — NAS credentials and the VAPID private key **encrypted at rest** under `SECRETS_KEY`, user
passwords **salted-hashed**. A **first-run wizard** configures the instance; thereafter **app users**
sign into SynoDL (no NAS account) and SynoDL performs NAS actions through **one stored connection** it
keeps alive (re-prompting an admin for a 2FA code on expiry). The admin manages users and scopes each
user's **NAS download folders** (enforced server-side). Opt-in **Web Push** (VAPID, hand-rolled with Go
stdlib crypto) notifies opted-in devices when a **download completes** — detected by a server-side
**completion watcher** that polls the NAS on the stored connection, so it works while clients are
offline — plus plain-text **app-update** notices.

## Technical Context

**Language/Version**: Go 1.26 (server); TypeScript 5 / Vue 3 + Ionic 8 (client).

**Primary Dependencies**: Server — Go stdlib **plus one justified dependency: `modernc.org/sqlite`**
(pure-Go, cgo-free SQLite driver via `database/sql`). VAPID (ES256 JWT) and Web Push message encryption
(RFC 8291 `aes128gcm`: ECDH P-256 + HKDF-SHA256 + AES-128-GCM) are built on **stdlib crypto**
(`crypto/ecdsa`, `crypto/ecdh`, `crypto/hkdf` [Go 1.24+], `crypto/aes`, `crypto/cipher`) — no webpush
library. Client — Ionic Vue + the Service Worker Push API (already have `src/sw.ts`); no new npm dep.

**Storage**: One **SQLite file on a mounted volume** (`DATA_DIR`, default `/data/synodl.db`). The only
persistence. Client keeps its *own* session token + settings in IndexedDB (unchanged mechanism).

**Testing**: Server — `go test` with a throwaway SQLite file (or `:memory:`) per test; NAS calls against
the existing `synomock`. Client — vitest for pure modules; Playwright e2e (wizard → admin → user →
folder-scope → push) using `synomock` + the mock's clock to drive completion. Web Push crypto gets
dedicated unit tests (encrypt→decrypt round-trip, VAPID JWT verify).

**Target Platform**: Single container (PWA + API) + one volume + `SECRETS_KEY`, on Docker/compose or
k8s (PVC). Behind the operator's TLS-terminating reverse proxy (unchanged).

**Performance Goals**: Home-NAS scale (a handful of users/devices). Completion watcher polls the NAS on
a bounded cadence; push fan-out is small.

**Constraints (Principle III v2.0.0)**: one encrypted SQLite volume; NAS creds + VAPID private key
encrypted under `SECRETS_KEY`; salted password hashes; allowlist-only NAS access; app users never hold
NAS creds; folder scope server-enforced; no secrets in logs.

**Scale/Scope**: Large. New server packages (`store`, `auth`, `setup`, `nas`, `push`); reworked
`api`/`config`/`main`; new client views (wizard, SynoDL login, admin users + folder scopes, push
opt-in) and a session/auth composable; deploy changes (PVC + `SECRETS_KEY`); docs.

## Constitution Check (v2.0.0)

| Principle | Verdict |
|---|---|
| I. Spec-Driven | ✅ built from spec 0003 via the pipeline |
| II. TDD | ✅ tasks order failing tests first; crypto/store/auth/handlers get unit tests; user-facing flows get e2e |
| III. Custodial State & Credential Safety (NON-NEGOTIABLE) | ✅ one encrypted SQLite volume; NAS creds + VAPID private key encrypted under `SECRETS_KEY`; salted password hashes; allowlist-only NAS access; least privilege; Credential-Safety Impact present |
| IV. Offline-First Client Data | ✅ client still uses IndexedDB for its own session/settings; `DB_VERSION` bumped if a store is added |
| V. Quality Gates | ✅ all CI gates + REQUIRED checklist (credential boundary) |
| VI. Ionic-First UI | ✅ wizard/login/admin built from stock Ionic + `--app-*` tokens |
| VII. Traceable Delivery | ✅ merges via PR into main; version bump cuts a release |

## Complexity & Exceptions

*Required by Governance — this feature adds a dependency and server capabilities.*

| Addition | Why needed | Simpler option rejected because |
|---|---|---|
| `modernc.org/sqlite` (one dependency) | Real embedded SQL store for config/users/ACLs/subscriptions with transactions + concurrency | Stdlib JSON files can't safely do concurrent multi-user writes, relational user↔folder↔subscription data, or queries; hand-rolled locking is more error-prone than a proven DB. cgo drivers rejected (keep the static alpine build). |
| Hand-rolled VAPID + Web Push encryption (stdlib) | Offline push is the requested feature; needs RFC 8291/8292 | A third-party webpush library would add a larger, less-audited dependency; stdlib now has all primitives (`crypto/ecdh`, `crypto/hkdf`), so one small, tested internal package is preferable |
| Server-side completion watcher | "Notify while offline" requires the server to learn of completion without a client | Client-side detection can't fire when the app is closed; DSM has no push feed (verified). Bounded NAS polling on the stored connection is the minimal reliable source |
| Stored NAS credentials + own accounts | The whole multi-user model | Per-user NAS accounts (pass-through) rejected by the operator; that's the point of the rework |

## Project Structure

### Documentation (this feature)

```text
specs/0003-stateful-multi-user/
├── plan.md · research.md · data-model.md · quickstart.md
├── contracts/ (setup, auth, users, folders, push, tasks-authz)
├── checklists/ (requirements.md ✅; security checklist added at /speckit-checklist)
└── tasks.md (from /speckit-tasks)
```

### Source Code (repository root)

```text
server/
├── cmd/synodl/main.go                 # open store, load/generate secrets+VAPID, wire deps, graceful shutdown
└── internal/
    ├── config/                        # env: DATA_DIR, SECRETS_KEY, PORT, ALLOWED_ORIGINS (SYNO_URL now optional)
    ├── store/                         # NEW: SQLite (modernc), migrations, typed repos, crypto-at-rest helpers
    ├── auth/                          # NEW: SynoDL users (salted hash), sessions, middleware
    ├── setup/                         # NEW: first-run wizard state + handlers (guarded once configured)
    ├── nas/                           # NEW: connection manager — holds the stored NAS conn, keeps sid alive,
    │                                  #      2FA re-auth prompt state; wraps internal/syno client
    ├── push/                          # NEW: VAPID (ES256 JWT) + RFC 8291 aes128gcm encryption + sender + watcher
    ├── syno/                          # unchanged allowlist client
    ├── synomock/                      # extend if needed for completion-watcher tests
    └── api/                           # rework: SynoDL-session auth (not X-Syno-Sid); routes below

src/ (client)
├── views/SetupWizard.vue             # NEW multi-step wizard
├── views/LoginPage.vue               # rework: SynoDL creds (not NAS); NAS-2FA re-auth prompt for admins
├── views/tabs/SettingsPage.vue       # + push opt-in toggle, (admin) users + folder scopes, NAS-conn status
├── components/ (UserAdmin, FolderScope, PushOptIn, WizardSteps…)
├── composables/useSession.ts         # rework to SynoDL session token
├── services/api.ts                   # SynoDL-session header; new endpoints
├── sw.ts                             # + push + notificationclick handlers
└── router/index.ts                   # setup gate + auth gate + admin guard

deploy/k8s/                            # + PVC, + SECRETS_KEY Secret, DATA_DIR env; installer generates SECRETS_KEY once
Dockerfile / docker-compose.yml       # volume + SECRETS_KEY; SYNO_URL no longer required
```

**New `/v1` surface** (SynoDL-session authenticated unless noted): `GET /v1/setup/state` (public),
`POST /v1/setup` (public, only pre-config), `POST /v1/session` (SynoDL login), `DELETE /v1/session`,
`GET /v1/me`, `GET/POST/PATCH/DELETE /v1/users` (admin), `GET/PUT /v1/users/{id}/folders` (admin),
`GET /v1/push/key`, `POST/DELETE /v1/push/subscription`, `POST /v1/nas/reauth` (admin, 2FA code); the
existing task + fs endpoints stay but switch to SynoDL-session auth + folder-scope enforcement.

## Incremental delivery & deploy safety (drives tasks.md phases)

The redesign changes the deploy contract (needs a volume + `SECRETS_KEY`), so merges are sequenced so
`main`/k3s never breaks:

1. **Increment 1 — Foundation (no behavior change yet)**: `store` (SQLite + migrations + crypto),
   `config` (add `DATA_DIR`/`SECRETS_KEY`, keep `SYNO_URL` working), `auth` primitives. Fully unit-
   tested, wired but dormant. Safe to merge — old behavior intact. Update `deploy/k8s` to add the PVC +
   `SECRETS_KEY` **and apply it to k3s** so the volume exists before anything depends on it.
2. **Increment 2 — Wizard + SynoDL auth (the switch)**: setup gate, wizard, SynoDL sessions, NAS
   connection manager; the app now requires setup. Merge **only after** the k3s PVC/secret from
   Inc. 1 is live; on first boot, seed the DB from the existing single-NAS env config so the running
   instance keeps working. Verify on k3s.
3. **Increment 3 — Users + folder ACLs**: admin user management + per-user folder scope enforcement.
4. **Increment 4 — Web Push**: VAPID, subscriptions, encryption, completion watcher, opt-in UI,
   download-complete + app-update pushes.

Each increment is its own PR (green CI), its own version bump/release, and a verified k3s rollout.

## Complexity Tracking

Covered in **Complexity & Exceptions** above (SQLite dependency + hand-rolled push crypto + watcher +
stored creds are all justified there).
