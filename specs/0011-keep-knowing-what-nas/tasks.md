# Tasks: Keep knowing what is on the NAS

**Spec**: [spec.md](./spec.md) · **Plan**: [plan.md](./plan.md)

Tests come before the code they cover (Principle II).

## Phase 1 — Foundational (blocks every story)

- [ ] T001 Append the `library_folders` and `library_evidence` migrations to `server/internal/store/schema.go`
- [ ] T002 Extend the pinned migration list in `server/internal/store/migrations_golden_test.go`
- [ ] T003 [P] Write `server/internal/store/library_repos_test.go`: folders round-trip, wholesale replace drops a removed parent, evidence round-trip, prune drops a vanished folder, stale ordering oldest-first
- [ ] T004 Implement `server/internal/store/library_repos.go` to make T003 pass

## Phase 2 — US1 + US2: markers survive a restart and a NAS blip (P1)

- [ ] T005 [US1][US2] Add failing tests to `server/internal/api/library_test.go`: cold cache serves the stored index with no NAS read; a NAS failure falls back to the stored index instead of blanking
- [ ] T006 [US1][US2] Write the index through to the store on a successful build, in `server/internal/api/library.go`
- [ ] T007 [US1][US2] Load from the store on a cold cache and on a failed build, in `server/internal/api/library.go`

## Phase 3 — US3: opening a title does not wait on the NAS (P2)

- [ ] T008 [US3] Add failing tests: a stored-but-stale folder reading is returned immediately; a NAS failure with a stored reading returns the stored reading
- [ ] T009 [US3] Write folder evidence through to the store, and read it back stale-while-revalidate, in `server/internal/api/library.go`

## Phase 4 — US4: a finished download shows up on its own (P2)

- [ ] T010 [US4] Add a failing test in `server/internal/push/watcher_test.go` that the finish callback fires once with the destination
- [ ] T011 [US4] Add `OnFinished` to `server/internal/push/watcher.go` and fire it on the finished transition
- [ ] T012 [US4] Add failing tests in `server/internal/api/library_scan_test.go`: a cycle is bounded; an enqueued folder jumps the queue; prune removes a vanished folder
- [ ] T013 [US4] Implement `server/internal/api/library_scan.go` (`RunLibraryScan`, `scanOnce`, `RefreshFolder`)
- [ ] T014 [US4] Enqueue the destination on a successful send in `server/internal/api/source_handlers.go`
- [ ] T015 [US4] Wire the scanner and the finish callback in `server/cmd/synodl/main.go`

## Phase 5 — Polish

- [ ] T016 [P] Extend `e2e/stateful/` to assert markers appear from a scan without the client opening the title first
- [ ] T017 Run every gate: `npm run build`, `npm run test:unit:coverage`, `go build`/`vet`/`test`, `npm run test:e2e`
- [ ] T018 `make roadmap`, bump the version, commit

## Dependencies

Phase 1 blocks everything. Phases 2 and 3 are independent of each other once
Phase 1 lands. Phase 4 depends on Phase 1 (the store) and reads better after
Phase 3 (the write-through it schedules).
