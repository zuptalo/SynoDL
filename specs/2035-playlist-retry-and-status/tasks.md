# Tasks: A playlist's failed tracks are one tap to retry, and the playlist always says what its tracks add up to

**Input**: [spec.md](./spec.md)

**Tests**: REQUIRED; for a `2001+` fix the work begins with failing regressions.

No plan/research artifacts: no endpoint, stored column, RBAC verb or outbound
host is added. Retrying a group already existed server-side; reading a worker's
output already exists (`pods/log`, spec 0013).

- [x] T001 Diagnose the production failures: re-probe all 79 failed videos with the pinned yt-dlp image (61 transient, 18 permanent — see spec)
- [x] T002 Failing regressions in `server/internal/api/ytdl_reconcile_test.go`: a retried track reopens its failed playlist and it settles to completed; a stale "failed" playlist with nothing failed is corrected; a settled playlist is left alone — confirmed failing (`group = "failed" while its retried item runs`)
- [x] T003 `ListYtdlActiveGroups` selects every group whose state disagrees with its items; `ReopenYtdlGroup`; `refreshGroups` reopens and skips no-op writes (FR-002, FR-003)
- [x] T004 `ytdl.FailureFromOutput` + table test from the real messages; read once in `captureFinished` before the Job goes — and again when the list got there first with only the generic reason (regression confirmed failing) (FR-001)
- [x] T005 `retryGroupItems` removes each re-queued item's old Job, as the single retry does
- [x] T006 `YtdlItem.vue`: close the slide after retry/dismiss (FR-005); offer retry on a playlist with any failed track (FR-004)
- [x] T007 `YtdlGroupModal.vue`: "Retry N failed" in the sheet header (FR-004)
- [x] T008 e2e in `e2e/stateful/ytdl-expand.spec.ts`: one-tap retry brings the playlist to saved; a tapped swipe slides closed — the latter confirmed failing against the old row (`Received: 130`)
- [x] T009 Gate: `npm run build`, `npm run test:unit:coverage`, `go build/vet/test`, e2e
- [x] T010 Merge; confirm in production that no playlist disagrees with its tracks and new failures carry the worker's reason (retrying the existing failures is left to the user)
- [x] T011 Set `**Status**: shipped` and run `make roadmap`
