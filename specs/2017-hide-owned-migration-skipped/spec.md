# Feature Specification: "Hide what I have" never saved, and every view save answered 500

**Feature Branch**: `fix/2017-hide-owned-migration-skipped`

**Created**: 2026-09-06

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Found while verifying an unrelated change: `PUT /v1/source/view` answered 500 on the reporting instance.

## Overview

The migration adding `source_prefs.hide_owned` was inserted into the **middle**
of the migration list, at position 22. An installation records how many
migrations it has applied, so any database already past position 22 counted that
migration as applied without ever running it — and the column was never added.

This is the same mistake spec 2012 was written about, made a second time. The
append-only guard added by that spec did not catch it: the guard pins the list as
it already stood, so it prevents future insertions but blesses the one already
there.

Confirmed on the reporting instance: schema version 32, and `source_prefs` has no
`hide_owned` column.

## Why it stayed hidden

Saving the Discover view does two writes. The first — filters, sort, order,
selected source — succeeds. The second, the hide-owned toggle, fails on the
missing column and the handler answers 500. The client does not wait on that
response (`void saveView()`), so:

- filters and sort appeared to save, because they did
- "hide what I have" silently never persisted
- every view save returned 500 to nobody in particular

## User Scenarios & Testing

### User Story 1 - The toggle sticks (Priority: P1)

A user turns on "hide what I have", reopens Discover, and it is still on.

**Acceptance**:
1. Given an instance whose database skipped the migration, when it starts, then
   the column is added.
2. Given the column is present, when the user sets the toggle, then it persists
   and the request succeeds.
3. Given a database that already HAS the column, when it starts, then the repair
   is a no-op and start-up is unaffected.

## Requirements

- **FR-001**: The missing column MUST be added on start-up for an installation
  that skipped it.
- **FR-002**: The repair MUST be appended, never inserted, and MUST be a no-op
  where the column already exists.
- **FR-003**: Saving the Discover view MUST succeed.
- **FR-004**: No data may be lost: the repair adds a column with a default and
  touches nothing else.

## Success Criteria

- **SC-001**: `PUT /v1/source/view` answers 204 on the reporting instance.
- **SC-002**: The hide-owned toggle round-trips.
- **SC-003**: An installation that already has the column starts unchanged.

## Credential-Safety Impact

None. One column with a default is added to a table that already exists; no data
is read, moved, or logged.

## Note for the future

The append-only guard cannot find a migration that was mis-inserted BEFORE the
guard existed. Both instances so far were found by their symptoms, not by the
test. A guard that compares the live schema against what the list should have
produced would catch the class rather than the instances — noted, not built here.
