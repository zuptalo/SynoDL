# Feature Specification: A DATA_DIR with punctuation in it opens the wrong database

**Feature Branch**: `fix/2016-data-dir-special-chars`

**Created**: 2026-09-05

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Follow-up to spec 0011's store change.

## Overview

Spec 0011 moved SQLite's pragmas into the connection string so they reach every
pooled connection. That string was built by concatenation:

```go
return "file:" + dsn + "?" + q.Encode()
```

The driver hands a `file:` DSN to SQLite's own URI parser, which reads `?` as the
start of the query, `#` as a fragment, and `%xx` as an escape. SynoDL is
self-hosted and `DATA_DIR` is whatever the operator sets, so a perfectly ordinary
path breaks it:

| DATA_DIR contains | what SQLite is asked to open |
|---|---|
| `/srv/a?b/synodl.db` | `/srv/a` — the rest read as query parameters |
| `/srv/a#b/synodl.db` | `/srv/a` — the rest read as a fragment, pragmas lost |
| `/srv/100%/synodl.db` | a path with `%2f`-style escapes decoded into something else |

The operator sees the service fail to start, or — worse — start against an empty
database at a path they did not choose.

## User Scenarios & Testing

### User Story 1 - My data directory is where I put it (Priority: P1)

An operator sets `DATA_DIR` to a path containing a space, `#`, `?`, `%` or `&`.
SynoDL creates and opens its database at exactly that path, with its settings
applied.

**Acceptance**:
1. Given a `DATA_DIR` containing any of those characters, when the service
   starts, then the database file exists at that exact path.
2. And the connection still carries the configured pragmas.

## Requirements

- **FR-001**: The database path MUST be escaped for SQLite's URI parser rather
  than concatenated into it.
- **FR-002**: The pragmas MUST still be applied for every such path.
- **FR-003**: A path that is already a `file:` URI, or `:memory:`, MUST be left
  alone.

## Success Criteria

- **SC-001**: A store opens at a path containing a space, `#`, `?`, `%` or `&`,
  and the file lands at that exact path.
- **SC-002**: `busy_timeout` is non-zero on a connection opened under such a path.

## Credential-Safety Impact

None. No new data is read, stored, or logged; this changes only how a path the
operator already configured is encoded.
