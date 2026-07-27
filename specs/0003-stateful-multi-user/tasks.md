---
description: "Task list for 0003 stateful multi-user rework"
---

# Tasks: Stateful multi-user rework

**Input**: design docs in `/specs/0003-stateful-multi-user/`. **Tests**: REQUIRED (TDD, Principle II).
**Organization**: by the 4 delivery increments in plan.md (each = its own green PR + release + k3s verify).

## Format: `[ID] [P?] [Story] Description`

---

## Increment 1 — Foundation (dormant; safe to merge; ships the PVC)

- [ ] T001 [P] Add `modernc.org/sqlite` to `server/go.mod`; record it in plan Complexity & Exceptions; `go mod tidy`.
- [ ] T002 [P] `server/internal/store/crypto_test.go`: AES-256-GCM encrypt→decrypt round-trip under a `SECRETS_KEY`-derived key; wrong key fails; nonce is random. **(fails first)**
- [ ] T003 `server/internal/store/crypto.go`: HKDF(SECRETS_KEY)→key; `Seal`/`Open` (nonce-prepended AES-GCM). Green T002.
- [ ] T004 [P] `server/internal/store/store_test.go`: open a temp-file DB, run migrations, assert schema version + tables (per data-model.md). **(fails first)**
- [ ] T005 `server/internal/store/schema.go` + `store.go`: `database/sql` + modernc; migration runner (`schema_migrations`); create all tables. Green T004.
- [ ] T006 [P] Repo tests + impls (TDD): `operator_config` (singleton, enc password), `users` (unique ci username), `sessions` (token-hash), `folder_grants`, `push_subscriptions`, `instance` (VAPID + version HWM), `watched_tasks`. One `*_test.go` per repo, failing first.
- [ ] T007 [P] `server/internal/auth/hash_test.go` + `hash.go`: salted password hash + verify (stdlib scrypt via `golang.org/x/crypto`? NO — keep stdlib: use `crypto/pbkdf2` [Go 1.24+] or scrypt from stdlib if available; else document). **(test first)**
- [ ] T008 `server/internal/config`: add `DATA_DIR` (default `/data`), `SECRETS_KEY` (required when stateful), keep `SYNO_URL` optional; unit tests for the new env (config floor ≥85 holds).
- [ ] T009 Wire (dormant) in `cmd/synodl/main.go`: open the store, boot-time `SECRETS_KEY` canary check (refuse start on decrypt failure) — behind a flag so current stateless behavior is unchanged until Inc. 2.
- [ ] T010 Deploy: add a PVC + `SECRETS_KEY` Secret + `DATA_DIR` to `deploy/k8s` (installer generates `SECRETS_KEY` once, like Ring); Dockerfile/compose volume. **Apply the PVC/secret to k3s** so the volume exists before Inc. 2.
- [ ] T011 Gates: `go build/vet/test`, `npm run build`; version bump; PR → main; verify k3s rollout (dormant, still 0.0.x behavior).

## Increment 2 — Wizard + SynoDL auth (the switch)

- [ ] T012 [P] `internal/nas`: connection manager wrapping `internal/syno` — holds the stored conn, logs in, keeps the sid alive, exposes a "needs 2FA re-auth" state; unit tests vs `synomock` (incl. OTP account + expiry→reauth).
- [ ] T013 [P] `internal/auth`: session issue/validate/revoke + `RequireUser`/`RequireAdmin` middleware; tests.
- [ ] T014 [P] `internal/setup`: `GET /v1/setup/state`, `POST /v1/setup` (verify NAS login before storing; store nothing on failure; guarded once configured); handler tests.
- [ ] T015 `POST /v1/session` (SynoDL login) + `DELETE` + `GET /v1/me`; rework `internal/api` to authenticate by SynoDL session, not `X-Syno-Sid`; task/fs endpoints call through `internal/nas`. Handler tests.
- [ ] T016 First-boot migration: if `SYNO_URL`/`SYNO_*` env is set and no `operator_config` exists, seed it (keeps the running instance working after the switch).
- [ ] T017 [P] Client: `SetupWizard.vue` (multi-step, stock Ionic) + `router` setup-gate; rework `LoginPage.vue` to SynoDL creds; rework `useSession.ts` + `api.ts` to the SynoDL session token; admin NAS-2FA re-auth prompt.
- [ ] T018 [P] e2e: wizard → land as admin → restart keeps config; SynoDL login/lockout; NAS action via stored conn.
- [ ] T019 Gates + version bump + PR → main; **verify on k3s** (wizard reachable, existing config migrated).

## Increment 3 — Users + folder ACLs

- [ ] T020 [P] `/v1/users` CRUD (admin) + `/v1/users/{id}/folders` GET/PUT; server-side admin guard; handler tests.
- [ ] T021 Folder-scope enforcement: picker lists only granted subtrees; task-create validates normalized destination ⊆ a grant (reject traversal/out-of-scope) BEFORE any NAS call; tests incl. crafted paths.
- [ ] T022 [P] Client: admin Users + FolderScope UI (Settings); scoped picker.
- [ ] T023 [P] e2e: admin creates/disables user; scoped user sees only allowed folders; out-of-scope create refused.
- [ ] T024 Gates + version bump + PR → main; verify k3s.

## Increment 4 — Web Push

- [ ] T025 [P] `internal/push/vapid_test.go` + `vapid.go`: ES256 VAPID JWT (stdlib `crypto/ecdsa`); verify signature. **(test first)**
- [ ] T026 [P] `internal/push/encrypt_test.go` + `encrypt.go`: RFC 8291 `aes128gcm` (ECDH P-256 + HKDF-SHA256 + AES-128-GCM); round-trip against known vectors. **(test first)**
- [ ] T027 `internal/push/sender.go`: POST to the push endpoint with VAPID auth + encrypted payload; prune 404/410; tests vs an httptest push service.
- [ ] T028 `internal/push/watcher.go`: bounded NAS poll on the stored conn; detect →finished transitions via `watched_tasks`; exactly-once; attribute to `owner_user_id`; push to opted-in devices. Tests vs `synomock` clock.
- [ ] T029 `/v1/push/key`, `/v1/push/subscription` POST/DELETE; app-update push on version change (`last_version_notified`). Handler tests.
- [ ] T030 [P] Client: `sw.ts` push + notificationclick; PushOptIn UI (per-device); iOS install note.
- [ ] T031 [P] e2e: opt in → seed download → tick to completion (app closed) → push received naming the task; opted-out gets none.
- [ ] T032 Gates + version bump + PR → main; verify k3s; update docs (CLAUDE.md/README/UPGRADING: volume, SECRETS_KEY, wizard).

## Cross-cutting

- [ ] TX1 `/speckit-checklist` (REQUIRED — credential boundary) before Inc. 2 implementation lands.
- [ ] TX2 Docs sweep: drop "stateless/credential-free" framing; document the volume, `SECRETS_KEY`, backup/restore (volume + key), and the wizard.

## Dependencies

Inc. 1 → 2 → 3, and 2 → 4. Within each: failing test precedes impl. Merge order = increment order; each
merge is safe (Inc. 1 dormant; Inc. 2+ only after the PVC/secret are live on k3s).
