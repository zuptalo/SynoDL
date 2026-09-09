# Feature Specification: Reading a worker's output, actually permitted by the API

**Feature Branch**: `fix/2023-podlog-accept`

**Created**: 2026-09-09

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: On the live deployment, every playlist expansion failed with "could
not read what that link contains", while the enumeration worker's output was
present and readable by hand.

## Overview

Reading a pod's log sent `Accept: text/plain`, which is the obvious header for a
response whose body IS plain text. The API server answers **406** to it:

```
only the following media types are accepted: application/json,
application/yaml, application/vnd.kubernetes.protobuf
```

Content negotiation there is on the API's own media types, not on what a
particular subresource happens to return. `kubectl` sends no `text/plain` either.

So every read of a worker's output failed, on every deployment, since the feature
shipped. Expansion reported "could not read what that link contains" and progress
never had a reading to report — the same symptom the missing `pods/log` grant
produced, which is why the grant looked like the whole story.

The unit test did not catch it because its `httptest` fake ignored the `Accept`
header entirely: the fake was more permissive than the thing it stood in for.

## User Scenarios & Testing

### User Story 1 - A playlist expands (Priority: P1)

Someone submits a playlist and it becomes the downloads it contains.

**Independent Test**: read a real pod's log through the client and get its bytes.

**Acceptance Scenarios**:

1. **Given** an enumeration worker that has finished, **When** the server reads
   its output, **Then** the read succeeds and the entries are parsed.
2. **Given** a running download, **When** the server reads its output, **Then**
   progress is reported.

---

### Edge Cases

- **A fake that is more permissive than the real server.** The failure was
  invisible in tests because the stand-in accepted a request the real API
  refuses. Any header the real server negotiates on must be negotiated in the
  fake too.

## Requirements

### Functional Requirements

- **FR-001**: Reading a worker's output MUST NOT send an `Accept` header the API
  server refuses.
- **FR-002**: The fake API server used in tests MUST reproduce the real server's
  content negotiation, so a request the real one rejects fails in tests too.

## Success Criteria

### Measurable Outcomes

- **SC-001**: Reading a worker's output succeeds against a real cluster.
- **SC-002**: Re-introducing the offending header makes the test suite fail.

## Credential-Safety Impact

- No change to what is stored, logged, sent to a worker, or granted. This is a
  request header on a call that was already permitted.

## Assumptions

- Groups already failed by this bug are not migrated: they hold no items and
  wrote no files, so dismissing and re-adding is correct.
