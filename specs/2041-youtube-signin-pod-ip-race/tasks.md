# Tasks: YouTube workers wait for their pod address before fetching the saved sign-in

**Input**: [spec.md](./spec.md)

**Tests**: REQUIRED.

- [x] T001 Add a bounded init-container retry while preserving fail-open behavior (FR-001).
- [x] T002 Preserve the grant across address refusals and add safe reason-specific diagnostics (FR-002, FR-003).
- [x] T003 Add regression tests for the retry script and delayed `status.podIP` visibility (FR-004).
- [x] T004 Verify on the home cluster: first redemption is `address_pending`, the next succeeds with `signin fetch ok`.
- [x] T005 Gate: full `go test ./...`, `go vet ./...`, and production Docker build.
- [ ] T006 Merge; confirm `:latest` rolls out; restore the deployment from the temporary local debug image; mark shipped.
