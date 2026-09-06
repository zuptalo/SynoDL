# Feature Specification: Notice when a database's schema is not what it should be

**Feature Branch**: `feat/1031-schema-drift-guard`

**Created**: 2026-09-06

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: User asked for the check noted at the end of spec 2017: compare the live schema against what the migration list should have produced.

## Overview

Twice a migration has been inserted into the middle of the list, and both times
every installation already past that position recorded it as applied without ever
running it:

- **spec 2012** — a source column. `/v1/source/providers` answered 500 and the
  operator's configured sources appeared to vanish.
- **spec 2017** — the hide-owned column. Saving the Discover view answered 500 to
  a client that does not wait on it, so the toggle silently never persisted, for
  months.

Both were found by their symptoms. This adds the check that finds them by
looking.

## What can and cannot be detected, and by what

This matters, because it is easy to build a check that appears to cover the whole
class and does not:

| Situation | Found by |
|---|---|
| A migration inserted mid-list **from now on** | `TestMigrationsAreAppendOnly` — the golden checksums shift and it fails. Verified. |
| A migration inserted mid-list **before those checksums existed** | Nothing in the list. From today the insertion looks like it was always there. Only a REAL database reveals it. |
| A migration that is not safely re-runnable, or order-dependent | Replaying the list from every stopping point. |

So the missing piece is the middle row, and it needs a live database. That is
what runs at start-up.

## User Scenarios & Testing

### User Story 1 - A drifted database repairs itself (Priority: P1)

An operator upgrades an instance whose database is missing a column a mid-list
migration should have added. It starts, adds the column, and works.

**Acceptance**:
1. Given a database missing such a column, when it starts, then the column is
   added and the feature behind it works.
2. Given a healthy database, when it starts, then nothing is written and nothing
   is reported.
3. Given a database missing a whole table, when it starts, then that is reported
   and the table is NOT invented.

### User Story 2 - The list stays self-consistent (Priority: P2)

**Acceptance**:
1. Given any stopping point in the migration list, when the remaining migrations
   run, then the schema matches one built from the list in full.

### Edge Cases

- A drifted database must still start. It is already serving; refusing to open
  it turns a partial problem into a total outage.
- The repair must not run on a healthy database, or every boot is a write and a
  warning nobody can act on.
- What is reported is table and column names from the source — never anything
  from a request, a NAS folder, or a credential.

## Requirements

### Functional Requirements

- **FR-001**: On start-up the live schema MUST be compared against the schema the
  migration list produces from nothing.
- **FR-002**: A missing COLUMN MUST be added, by re-running the `ADD COLUMN`
  statement from the list.
- **FR-003**: A missing TABLE or INDEX MUST be reported and MUST NOT be created
  automatically. Creating a table implies deciding what belongs in it, and a
  database missing one has a problem an automatic repair should not paper over.
- **FR-004**: Drift MUST NOT prevent the instance from starting.
- **FR-005**: A healthy database MUST produce no writes and no report.
- **FR-006**: Replaying the list from any stopping point MUST reach the same
  schema as building it in full.
- **FR-007**: Only schema identifiers may be logged.

## Success Criteria

- **SC-001**: A database missing a column a mid-list migration skipped is usable
  after one restart, with no operator action.
- **SC-002**: A healthy start-up logs nothing about the schema.
- **SC-003**: The added start-up cost is a few milliseconds.

## Assumptions

- Comparison is by NAME — tables, columns, indexes — not by type or constraint.
  A column added by `ALTER` cannot carry every constraint the same column has in
  a `CREATE`, so comparing those would report differences SQLite itself
  introduces and train everyone to ignore the check.

## Credential-Safety Impact

None. It reads `sqlite_master` and `PRAGMA table_info` on the instance's own
database and may add a column the list already defines. Nothing is read from a
request or the NAS, and only schema identifiers are logged.
