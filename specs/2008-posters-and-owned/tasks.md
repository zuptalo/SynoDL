# Tasks: Posters and owned markers survive a source outage

**Spec**: [spec.md](./spec.md)

- [x] T001 [US1] Accept configured mirror hosts in the image proxy (`server/internal/api/source_image.go`), memoised briefly since a page asks for ~40 posters at once, and still refusing every other host (FR-001, FR-002)
- [x] T002 [US1] Tests: a mirror and its subdomain are accepted; an unrelated host and a lookalike are refused; with no mirror configured the proxy is unchanged
- [x] T003 [US2] Build the library snapshot on a context detached from the request, with a bound of its own (`server/internal/api/library.go`) (FR-003, FR-004)
- [x] T004 [US2] Tell "the NAS reports nothing" apart from "the NAS could not be asked", and retry only the latter promptly (FR-005, FR-006)
- [x] T005 [US2] Make the fake NAS client respect context cancellation — it did not, so a test written to catch exactly this bug passed against the broken code
- [x] T006 [US2] Tests: a cancelled caller still yields a built snapshot; a failed reading is retried promptly; a successful one keeps its full lifetime
- [x] T007 Run the gates and ship

## Note on how this was found

Both faults are silent by design, so neither appeared in a log as an error. They
were found by reading the deployment's request log — 127 image requests answered
400 in microseconds, and clients hanging up mid-response — then reproducing
against the operator's own configuration locally.
