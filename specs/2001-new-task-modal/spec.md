# Feature Specification: New-task modal shows a false "Could not reach the server" after the task is created

**Feature Branch**: `fix/2001-new-task-modal`

**Created**: 2026-07-27

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Operator bug report: "adding new tasks actually works, but the new-task window stays on the
screen saying 'Could not reach the server!'"

## Bug Summary

Adding a download succeeds on the NAS, but the New Task modal stays open showing the error
**"Could not reach the server."** — so the user thinks it failed, closes and retries, and creates
duplicate tasks.

## Root Cause

`POST /v1/tasks` returns **`201 Created` with an empty body** (`server/internal/api/task_handlers.go`
— both the URL and torrent-upload paths call `w.WriteHeader(http.StatusCreated)` with no body). The
client HTTP wrapper `request()` in `src/services/api.ts` special-cases only `204 No Content`; for any
other 2xx it calls `await resp.json()`. Parsing an **empty** body as JSON throws a `SyntaxError`, which
is not an `ApiError`, so `messageForError()` returns its non-ApiError default, *"Could not reach the
server."*, and `NewTaskModal.submit()` never reaches `emit('created')` — the modal stays open.

pause/resume/delete are unaffected because they return `204`, which `request()` already handles. This
create-vs-action inconsistency is why swipes work but Add appears to fail.

## Why existing tests missed it

`e2e/add-task.spec.ts` asserts only that the new task **rows appear** — which they do, via the 3s
poll — and never asserts the modal was dismissed or that no error is shown. The stuck modal therefore
passed unnoticed.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Adding a download confirms success (Priority: P1)

As someone adding a download, when the NAS accepts it, the New Task window closes and I see it in the
list — with no error message.

**Independent Test**: Add a valid URL (or upload a `.torrent`); assert the modal is dismissed, no error
note is shown, and the task appears.

**Acceptance Scenarios**:

1. **Given** the New Task modal is open with a valid URL, **When** I tap Add task and the NAS accepts
   it, **Then** the modal closes, no error is shown, and the task appears in the list.
2. **Given** a `.torrent` upload the NAS accepts, **When** I tap Add task, **Then** the modal closes
   with no error.
3. **Given** the NAS genuinely rejects the request (e.g. oversized torrent → 413), **When** I tap Add
   task, **Then** the specific error is still shown and the modal stays open (unchanged behavior).

### Edge Cases

- Any successful response with an empty body (e.g. `201`, or a future empty `200`) MUST NOT be treated
  as a failure.
- A genuine network failure (fetch rejects) MUST still surface "Could not reach the server."
- A non-empty JSON success body MUST still parse and return normally.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The client HTTP wrapper MUST treat any successful (2xx) response with an empty body as a
  success returning no value — not only `204`.
- **FR-002**: On a successful create, the New Task modal MUST close, show no error, and the created
  task MUST appear in the list.
- **FR-003**: Genuine failures MUST keep their current behavior: HTTP error responses map to their
  typed messages; a true network failure maps to "Could not reach the server."; the modal stays open
  and shows the message.
- **FR-004**: A regression test MUST reproduce the bug (fail before the fix) by asserting the modal is
  dismissed with no error after a successful create.

## Credential-Safety Impact *(constitution-required)*

- **What crosses the proxy**: nothing new — this is purely client-side response-body handling.
- **What is forwarded to the NAS**: unchanged.
- **What could appear in logs/errors**: unchanged (route + outcome). No secrets involved.
- **Why nothing sensitive is retained**: no server change; no persistence; no new data handled.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After a successful create, the modal closes and no error is shown, 100% of the time.
- **SC-002**: Genuine errors (413 oversized torrent; real network failure) still show their correct
  messages — no regression.
- **SC-003**: All CI gates remain green.

## Assumptions

- The server continues to return `201` with an empty body for create; the fix is client-side (the more
  robust place — it also covers any future empty-body success). No server change is required, though
  returning `204` for consistency with the action endpoints would also be acceptable and is noted as a
  non-blocking follow-up, not part of this hotfix.
