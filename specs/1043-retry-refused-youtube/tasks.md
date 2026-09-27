# Tasks: A YouTube download that YouTube turned away tries again by itself

**Input**: [spec.md](./spec.md)

**Tests**: REQUIRED, written first.

No plan/research artifacts: no endpoint, column, RBAC verb or outbound host is
added — this re-uses the existing retry edge (`RequeueYtdl`) from the reconciler
under a bounded, reason-gated policy.

- [x] T001 Evidence: raw worker errors captured live in production (403 on media, bot check); refusal rate at parallelism 4 vs 2; refused tracks re-probed OK
- [x] T002 Tests in `server/internal/api/ytdl_reconcile_test.go`: retried after the cool-down, not before; stops after the last attempt; a permanent failure is never retried; off means off; a playlist waits for its retries and then completes
- [x] T003 `YTDL_AUTO_RETRY_AFTER_SECONDS` / `YTDL_AUTO_RETRY_MAX_ATTEMPTS` in `internal/config`, with a test (FR-001)
- [x] T004 `ListYtdlRetryDue`, `CountYtdlAwaitingRetry`; `ListYtdlActiveGroups` also selects a finished group with an item awaiting a retry (FR-005, FR-006)
- [x] T005 Reconciler: `retryRefused` before `refreshGroups`; `ReasonRefusedRetrying` while waiting; no notification for a pending retry (FR-001–FR-005)
- [x] T006 Document the knobs: `deploy/k8s/10-synodl.yaml`, `docs/UPGRADING.md`, `CLAUDE.md`
- [x] T007 Gate: `go build/vet/test`, `npm run build`, unit coverage, stateful ytdl e2e
- [x] T008 Merge; confirm in production that refused downloads are re-queued after the cool-down and settle
- [x] T009 Set `**Status**: shipped` and run `make roadmap`
