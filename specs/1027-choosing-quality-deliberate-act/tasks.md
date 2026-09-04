# Tasks: Choosing a quality is a deliberate act

**Spec**: [spec.md](./spec.md) · **Plan**: [plan.md](./plan.md)

## Phase 1: User Story 1 — nothing is chosen until you choose it (P1)

- [x] T001 [US1] Stop pre-picking an option on load in `src/components/SourceTitleModal.vue`; open on the user's preferred quality **tier** instead, so the stored preference keeps its value without making the choice (FR-001, FR-002)
- [x] T002 [US1] Clear the pick when switching tier if it belongs to another tier (FR-004)
- [x] T003 [US1] Clear the pick when another season is opened if it belongs to another season — the case where the send button was armed with an invisible option (FR-004)

## Phase 2: User Story 2 — the next step comes to you (P1)

- [x] T004 [US2] Add an anchor between the option list and what follows it, and scroll to it when a pick is made, so a season pack lands on its episode selection and a movie on its send button (FR-005, FR-006, FR-007)

## Phase 3: User Story 3 — only warn about real repeats (P2)

- [x] T005 [US3] Decide the repeat warning by what is being sent — the season for a pack, the title otherwise — keeping the warning for a title still arriving (FR-008, FR-009)

## Phase 4: User Story 4 — one confirmation (P3)

- [x] T006 [US4] Remove the toast on a successful send and its now-unused import; the button becomes the live status control for the download just created (FR-010, FR-011)

## Phase 5: Cross-cutting

- [x] T007 Add `e2e/stateful/title-send.spec.ts` covering the whole flow, which had **no** coverage: nothing picked on open and sending unavailable; picking arms the button and reveals the episodes; opening another season drops a hidden pick; a season you lack sends with no warning and no toast; a season you have still warns and cancelling sends nothing; a movie has no episode picker
- [x] T008 Run the gates: `npm run build`, `npm run test:unit:coverage`, `cd server && go build ./... && go vet ./... && go test ./...`, `npm run test:e2e`
- [x] T009 Set the spec `**Status**:` to `in-review`, run `make roadmap`, commit, push, open the PR, merge, confirm k3s picks it up

## Dependencies

T001 → T002/T003 (the invariant only matters once nothing is pre-picked). T007 depends on all of them.

## Added during implementation

- [x] T010 Fix the season-level repeat warning, found by driving the running app:
  a series counts as owned when ANY season is present, so sending season 3 of a
  show with seasons 1-2 on the NAS warned "you already have this"
- [x] T011 Make merged branches actually get deleted: `.github/workflows/auto-merge.yml`
  merged without `--delete-branch`, and the repo's `delete_branch_on_merge`
  setting does not fire for a merge the workflow performs with its own token, so
  every merged branch survived. Four stale remote branches removed by hand after
  verifying each PR's merge commit was an ancestor of `main`.
