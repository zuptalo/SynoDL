# Tasks: YouTube sign-in for download workers

**Input**: spec.md, plan.md, research.md, data-model.md, contracts/. Tests first in every group (Principle II).
**Issues**: G1 #323, G2 #324, G3 #325, G4 #326, G5 #327, G6 #328, G7 #329, G8 #330

## G1 — Converter and store (FR-002, FR-004..FR-006, FR-013)
- [x] T001 [P] Table tests for `ytsignin.Parse`: header with/without `Cookie:` prefix, whitespace/newlines, Netscape passthrough, `#HttpOnly_` lines, foreign domains dropped, too few, oversize, control chars, no value in any error — `server/internal/ytsignin/parse_test.go`
- [x] T002 Implement `Parse` (+ login-cookie summary) — `server/internal/ytsignin/parse.go`
- [x] T003 [P] Store tests: save/replace/delete, metadata read never returns values, sealed bytes contain no marker, wrong key reads as absent, outcome stamps — `server/internal/store/youtube_signin_test.go`
- [x] T004 Migration 0041 + repository + golden hash — `server/internal/store/schema.go`, `youtube_signin.go`, `migrations_golden_test.go`

## G2 — Grants and the worker fetch endpoint (FR-015, FR-016, SC-007)
- [x] T005 [P] Tests: mint/redeem, single use, expiry, wrong Job, address ≠ pod IP, `X-Forwarded-For` present, no pod, no sign-in saved → identical empty 404; grant never in logs — `server/internal/api/youtube_signin_grants_test.go`
- [x] T006 `k8s.Pod.Status.PodIP`; grant table on `Deps` (pointer); `GET /v1/internal/ytdl-signin` — `server/internal/k8s/types.go`, `api/youtube_signin_grants.go`, `router.go`

## G3 — Admin API (FR-001, FR-004..FR-008, FR-012, FR-014)
- [x] T007 [P] Handler tests: admin-only (403), stateless absent, PUT shapes/warnings/errors without text, GET metadata-only, DELETE — `server/internal/api/youtube_signin_handlers_test.go`
- [x] T008 Implement `GET/PUT/DELETE /v1/youtube/signin` — `server/internal/api/youtube_signin_handlers.go`, `router.go`

## G4 — Job assembly (FR-009..FR-011, FR-016..FR-018)
- [x] T009 [P] Tests: with grant → init container, memory volume, label, `--cookies` argv (discrete); without → byte-identical to today; non-YouTube target never; no marker in the Job JSON; expansion Job too — `server/internal/ytdl/job_test.go`, `command_test.go`
- [x] T010 `k8s.EmptyDirSource{Medium,SizeLimit}`; `Options.SignIn`; init container + volume + label in `BuildJob` and `BuildExpansionJob`; config `SIGNIN_FETCH_URL` — `server/internal/k8s/types.go`, `ytdl/job.go`, `command.go`, `config/config.go`
- [x] T011 Mint a grant when a sign-in exists and the target is YouTube, at both Job-creation sites — `server/internal/api/ytdl_reconcile.go`, `ytdl_handlers.go`

## G5 — Outcomes and wording (FR-019..FR-021)
- [x] T012 [P] Tests: signin-labelled Job refused with "not a bot" → `ReasonSignInRefused` (still retried, same timing) and `last_refused_at`; success → `last_ok_at`; grant revoked at Job end; unlabelled unchanged — `server/internal/api/ytdl_reconcile_test.go`, `ytdl/failure_test.go`
- [x] T013 Implement reason, summary text, retry treatment, outcome stamps, revocation — `server/internal/ytdl/failure.go`, `summary.go`, `api/ytdl_reconcile.go`

## G6 — Client (US1..US4)
- [x] T014 [P] API functions + types — `src/services/api.ts`
- [x] T015 `YoutubeSignInModal.vue` (status, paste, file pick, save/remove, guidance, sensitivity note, memory-only paste) — `src/components/YoutubeSignInModal.vue`
- [x] T016 Admin-only Settings row `settings-youtube-signin`; admin hint on a refused download in `YtdlDetailModal.vue` — `src/views/tabs/SettingsPage.vue`, `src/components/YtdlDetailModal.vue`
- [x] T017 [P] Unit tests for any pure helper (message wording for refusal states) — `src/services/*.test.ts`

## G7 — e2e and leak checks (SC-002, SC-003)
- [x] T018 Mock cluster: report a pod IP and pods by `job-name` — `server/internal/k8smock/`
- [x] T019 e2e (stateful): save with marker values; status has no marker; download's Job has label + init + no marker; second/other-address fetch refused; server log, DB file bytes and every response free of markers; remove → later Job anonymous; non-admin blocked — `e2e/stateful/youtube-signin.spec.ts`
- [x] T020 e2e stateless build has no row — `e2e/youtube-signin-hidden.spec.ts`

## G8 — Docs, release, deploy (FR-017, SC-001)
- [x] T021 Verify `deploy/k8s/30-rbac.yaml` byte-identical to `main`; note `SIGNIN_FETCH_URL` in `deploy/k8s/10-synodl.yaml`
- [x] T022 Docs: `CLAUDE.md` pointer, `docs/` note on getting the header, spec Verification section
- [x] T023 Run all gates (build, vet, tests, vitest coverage, full e2e)
- [x] T024 `npm run release:minor`; `**Status**: in-review`; `make roadmap`; PR with `Closes #…`; merge when green; confirm tag, `:latest` digest, running pod, Flux
- [x] T025 Real-cluster acceptance (needs the user to paste a sign-in): a refused download succeeds; then mark `shipped`

## Dependencies
G1 → G2, G3; G1+G2 → G4; G4 → G5; G3 → G6; G4+G5+G6 → G7 → G8.
