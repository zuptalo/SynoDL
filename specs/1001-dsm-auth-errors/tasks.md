# Tasks: Recognize every DSM 7 sign-in failure with its own message

**Tests**: REQUIRED — constitution Principle II (Red → Green). Ad-hoc band;
unit-level coverage on both sides of the contract.

- [x] T001 [US1] RED: extend `server/internal/syno/http_test.go` TestClassify
      with 401/406/407/408/409/410 expectations per the guide's table.
- [x] T002 [US1] GREEN: add `KindAccountDisabled`, `KindIPBlocked`,
      `KindPasswordExpired` to `server/internal/syno/errors.go`; extend
      `classify()`; map the new kinds in `writeSynoError`
      (`server/internal/api/router.go`) — 401-family → 401, blocked/disabled →
      403.
- [x] T003 [US1] RED: extend `src/services/syno-errors.test.ts` with the three
      new codes' distinct messages (and keep the all-distinct invariant test).
- [x] T004 [US1] GREEN: extend `src/services/syno-errors.ts`.
- [x] T005 [US1] Mock accounts for manual dev parity (FR-004): `disabled`
      (401), `blocked` (407), `expired` (409) in `internal/synomock` + test.
- [x] T006 Gates: unit + go suites green; Status → in-review; `make roadmap`.
