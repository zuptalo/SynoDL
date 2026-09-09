# Tasks: YouTube downloads update as they happen

**Spec**: [spec.md](./spec.md) · **Plan**: [plan.md](./plan.md)

Tests come before the implementations they cover (Principle II). Every test in
this list must be seen to FAIL before its implementation lands — spec 0013's
post-mortem is that a test which was never seen red proves nothing.

## Phase 1: Foundational

- [X] T001 Add `ListYtdlUnfinished()` to `server/internal/store/ytdl_downloads.go` — every download of either kind not in a terminal state, ordered by queue_seq
- [X] T002 [P] Test it in `server/internal/store/ytdl_downloads_test.go`: includes queued, resolving, scheduled, downloading and group rows, excludes completed and failed

## Phase 2: The hub (US3 — the instance is not made to work harder)

**Goal**: a fan-out that costs one render per change rather than one per viewer,
and that cannot be stalled by a reader.

- [X] T003 [P] Test in `server/internal/api/ytdl_hub_test.go`: a subscriber receives a published change it may see
- [X] T004 [P] Test: a non-admin receives only its own downloads; an admin receives everyone's; an unowned download reaches admins only (FR-006)
- [X] T005 [P] Test: publishing to a subscriber whose buffer is full drops that subscriber and does not block the publisher (FR-012a, FR-012b)
- [X] T006 [P] Test: `hasSubscribers()` is false with nobody connected, and a cancelled subscription is not counted
- [X] T007 Implement `ytdlHub` in `server/internal/api/ytdl_hub.go`

## Phase 3: The diff (US1 — the list updates itself)

**Goal**: the reconciler knows what changed and says so, at the moment it knows.

- [X] T008 [P] Test in `server/internal/api/ytdl_watch_test.go`: the first cycle publishes nothing and seeds the fingerprints (a restart must not announce the world as new)
- [X] T009 [P] Test: a state change publishes exactly that download, and an unchanged download is not republished
- [X] T010 [P] Test: a progress reading moving publishes the row — change derived from the cache, not from a stored write
- [X] T011 [P] Test: a download reaching a terminal state publishes its final view once and then stops
- [X] T012 [P] Test: a dismissed download publishes a removal (FR-013)
- [X] T013 [P] Test: a newly created top-level download is published as `created`; an expanded item is published with its `parentId` (FR-004a)
- [X] T014 [P] Test: with no subscribers, a cycle performs no additional store read (FR-010, SC-002a)
- [X] T015 Implement `watchYtdl` in `server/internal/api/ytdl_watch.go` and call it last in `reconcileYtdlOnce`

## Phase 4: The endpoint (US1, US2)

- [X] T016 [P] Test in `server/internal/api/ytdl_stream_test.go`: no session → 401; over the cap → 503 `stream_limit` with `Retry-After` (FR-011)
- [X] T017 [P] Test: the stream emits `ready`, then a frame per published batch, and nothing at all during a quiet cycle (SC-001)
- [X] T018 [P] Test: the frame carries only the changed rows, and its size does not grow with how many downloads exist (FR-004, SC-003)
- [X] T019 [P] Test: a session revoked mid-stream ends the stream with `event: error` (Credential-Safety)
- [X] T020 [P] Test: an admin flag withdrawn mid-stream ends the stream rather than continuing to send everyone's downloads
- [X] T020a [P] Test: an idle stream emits a heartbeat comment and stays open past the proxy's read timeout (FR-007)
- [X] T020b [P] Test: no request handler lists jobs or reads pod output for the stream — the orchestrator is asked by the reconciler alone, however many are connected (FR-010, SC-004)
- [X] T021 Implement `handleYtdlStream` in `server/internal/api/ytdl_stream.go`, its own limiter, and route it in `server/internal/api/router.go`

## Phase 5: The client (US1, US2)

- [X] T022 Add `streamYtdl` to `src/services/api.ts`, reusing `parseSSEFrame`
- [X] T023 [P] Test in `src/composables/__tests__/ytdl-merge.spec.ts`: merge rules — replace a held row, insert a `created` top-level row at the top, ignore an unknown row, drop a removed one
- [X] T024 Extract those rules as a pure `mergeYtdlUpdate` in `src/services/ytdl-merge.ts` so they carry a coverage floor
- [X] T025 Rewire `src/composables/useYtdl.ts`: fetch on `ready`, connect / fallback / backoff as `useTasks` does, poll only while the stream is down
- [X] T026 Hold a group's items in `useYtdl` and have `src/components/YtdlGroupModal.vue` read them live instead of refreshing itself every five seconds (FR-003)
- [X] T027 Keep the detail sheet live from the same state rather than its own 5s timer

## Phase 6: End to end

- [X] T028 e2e in `e2e/stateful/ytdl-live.spec.ts`: with the list open and no polling, driving a download through its states updates the row (US1)
- [X] T029 e2e: with a playlist sheet open, one track changing updates that row without the sheet re-reading its contents (US2)
- [X] T030 e2e: with the stream unavailable, the list still updates by polling (FR-008, SC-005)

## Notes from implementation

- **T008 was wrong, and the e2e suite is what said so.** The diff originally
  cleared its fingerprints whenever nobody was watching and re-seeded on the
  first cycle after somebody connected, publishing nothing on that cycle. That
  silently swallowed anything that happened between a client fetching its list
  and that seed — a progress reading landing in exactly that window was never
  announced, because from the next cycle on it matched. The server unit tests
  passed; the browser showed a bar that never appeared.
  The map is now kept across unwatched periods, and the first publish of a
  process announces its rows as `changed` rather than `created` so a restart
  cannot reorder anybody's list. Both are covered by tests confirmed failing
  against the old behaviour.
- **The open group sheet fills itself if it was opened too early.** Merging
  alone cannot populate an empty sheet, so a sheet opened while a playlist was
  still being expanded would have stayed empty for good.

## Phase 7: Gates

- [X] T031 `go build ./...`, `go vet ./...`, `go test ./...`
- [X] T032 `npm run build`, `npm run test:unit:coverage`
- [X] T033 `npm run test:e2e`
- [X] T034 `make roadmap`, status → in-review, version bump (planned band → minor is for `0001+`; this is ad-hoc `1001+`, so patch)
