# Feature Specification: Remove rows the cascades should have taken with them

**Feature Branch**: `fix/2019-orphaned-rows-sweep`

**Created**: 2026-09-06

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Follow-up to spec 0011's store fix: what did the broken pragma leave behind?

## Overview

Until spec 0011, `PRAGMA foreign_keys` was applied with a single `Exec` after
opening. `*sql.DB` is a POOL and that pragma is per-connection, so every
connection but one ran with foreign keys OFF — and `ON DELETE CASCADE` silently
did nothing on those.

Ten tables depend on that cascade. Deleting a user or a download source left
their dependent rows behind.

## What was actually left behind

Measured on the reporting instance:

| table | orphaned |
|---|---|
| `source_provider_secrets` | **2 of 4** |
| every user-owned table (sessions, grants, history, prefs, …) | 0 |

The user tables are clean, and the risk there was smaller than first feared:
`users.id` is `AUTOINCREMENT`, so an id is never reissued and no new account can
inherit a deleted one's rows.

`source_provider_secrets` is the one that matters. It holds the **encrypted
session credentials** for a download source, and `DeleteProvider` removes only
the provider row and trusts the cascade for the rest. So an operator who deleted
a source still had its credentials stored — encrypted, unreachable (nothing can
address a provider id that no longer exists), but kept well past the point they
asked for it to be gone.

## Why a migration alone was not enough

The sweep was first written as an appended migration. On the reporting instance
its version was recorded **while the statements had no effect** — the same
"recorded as applied, never ran" shape as spec 2012 and spec 2017, seen a third
time and this time not reproducible.

A cleanup that silently does not happen is worse than none, because everyone
believes it did. So the sweep is also driven by `PRAGMA foreign_key_check`,
which asks the database what is actually broken rather than what should have
been fixed. That needs no version, no flag and no memory of having run, cannot
act on a healthy database, and cannot remove a row that is still referenced.

## User Scenarios & Testing

### User Story 1 - Deleting a source removes its credentials (Priority: P1)

**Acceptance**:
1. Given a database with secrets belonging to a deleted source, when it starts,
   then those rows are gone and live sources keep theirs.
2. Given a healthy database, when it starts, then nothing is deleted and nothing
   is reported.
3. Given the cleanup's migration was recorded but never took effect, then the
   rows are still removed.

### Edge Cases

- A column declared `SET NULL` is cleared, not deleted: a task or a download
  outlives the account that started it.
- A foreign key declared `NO ACTION` / `RESTRICT` is left alone and reported —
  the schema does not say to remove it.

## Requirements

- **FR-001**: Rows whose parent no longer exists MUST be removed where the schema
  declares `CASCADE`.
- **FR-002**: Columns declared `SET NULL` MUST be cleared, and their rows kept.
- **FR-003**: Any other action MUST be left alone and reported.
- **FR-004**: A healthy database MUST see no writes.
- **FR-005**: The cleanup MUST NOT depend on a migration having run.
- **FR-006**: The per-user download history is removed only where its user is
  gone — which is what its own `CASCADE` declares. It is not otherwise touched.

## Success Criteria

- **SC-001**: `PRAGMA foreign_key_check` reports nothing after start-up.
- **SC-002**: The credentials of a deleted source are gone.
- **SC-003**: A live source keeps exactly its own.

## Credential-Safety Impact

Positive: this REMOVES stored credential material that outlived the source it
belonged to. Nothing is read from a request or the NAS, and only table and
column names are logged — never a row's contents.
