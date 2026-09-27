# Tasks: YouTube downloads stop waiting forever once a few hundred have run

**Input**: [spec.md](./spec.md)

**Tests**: REQUIRED, and for a `2001+` fix the work BEGINS with a failing
regression test that reproduces the bug.

No plan/research artifacts: this changes how an existing list is fetched and
when an existing Job is deleted. No endpoint, stored value, RBAC verb or outbound
host is added (`jobs: delete` has been granted since spec 0012), so there is
nothing to design and nothing for a checklist to review.

- [x] T001 Operational: delete the 696 finished Jobs whose outcome was already recorded; leave the 4 unrecorded ones; confirm the reconciler resumes
- [x] T002 Write the failing regressions in `server/internal/k8s/{jobs,pods}_test.go`: a 900 × 6 KiB list must arrive whole and paged; an oversized body is `ErrResponseTooLarge` — confirmed failing (`k8s: decode response: unexpected end of JSON input`)
- [x] T003 Page `ListJobs`/`ListPods` via `listPaged`; name the oversized response in `do()` (FR-001, FR-002)
- [x] T004 Delete a download's Job in `captureFinished` once `captureTerminal` reports it recorded (FR-003), with tests for both halves
- [x] T005 Narrow `readWorkerOutput` and `podLogFor` to `request-id=<id>` (FR-004), with a test
- [x] T006 Edge-triggered WARN/INFO for the reconciler's list (FR-005), with a test
- [x] T007 `sweepDismissed` deletes only on `store.ErrNotFound` (FR-006)
- [x] T008 Run the gate: `go build/vet/test ./...`
- [ ] T009 Merge; confirm Keel rolls the pod and the queue drains in production
- [ ] T010 Set `**Status**: shipped` and run `make roadmap`
