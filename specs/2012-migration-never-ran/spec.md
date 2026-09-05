# Feature Specification: A migration added in the middle never runs

**Feature Branch**: `fix/2012-migration-never-ran`

**Created**: 2026-09-05

**Status**: in-review

**Input**: User report: "The configurations are gone and there is no way to add any!"

## Context

0.10.0 added a column and shipped code that selects it. The migration adding it
was placed in the MIDDLE of the migration list instead of at the end.

An installation records how many migrations it has applied. Anything sitting
below that mark is never run — so the column was never created, while the code
selecting it shipped anyway. Every request for the source list answered 500: the
sources looked deleted and none could be added.

Two further consequences, both from the same insertion:

- It shifted the migration after it down by one, so installations already past
  that point ran it a second time. It was an idempotent UPDATE, which is the only
  reason nothing was corrupted.
- Those installations recorded a version for that re-run, so their counter and
  the list disagree, and simply moving the migration to the end is not enough to
  make it land on them.

No data was lost. The sources were in the database throughout.

## Requirements *(mandatory)*

- **FR-001**: The column MUST be created on an installation that upgraded through
  the mistaken release, and on a fresh one.
- **FR-002**: An installation created ON the mistaken release already has the
  column; applying the corrected migration MUST NOT fail it. Adding a column that
  is already present MUST be treated as already done, never as a reason to refuse
  to start.
- **FR-003**: Existing data MUST be untouched.
- **FR-004**: Migrations MUST be append-only, and a change to any already-shipped
  migration MUST fail the build rather than reach an installation.
- **FR-005**: A test that exercises a specific shipped migration MUST identify it
  explicitly, not by position, so appending never silently stops exercising it.

## Success Criteria *(mandatory)*

- **SC-001**: The affected installation lists its sources again.
- **SC-002**: No configured source is lost.
- **SC-003**: An installation created on the mistaken release still starts.
- **SC-004**: Editing or inserting into migration history fails a test.

## Credential-Safety Impact

- No allowlist, NAS, credential or logging change. Schema only; stored material is
  untouched and was never at risk.
