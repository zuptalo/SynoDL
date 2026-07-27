# Feature Specification: Copy the download link and re-download from the detail view

**Feature Branch**: `feat/1007-copy-redownload`

**Created**: 2026-07-27

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Operator request: "See and copy the download link for tasks in any state, and easily
re-download failed or finished ones from the detail view."

## User Scenarios

- **US1 (P1)** Tapping a task shows its **source link** (URL/magnet) in the detail sheet, with a **Copy**
  button, in any state.
- **US2 (P1)** For a **finished or failed** task that has a link, a **Re-download** action re-adds it to
  the NAS with the same destination.

## Functional Requirements

- **FR-001** The task DTO MUST carry the source `uri` (from DSM `additional=detail`). It is exposed to
  the authenticated client for display; it is never logged (credential-safety is about logging, not UI).
- **FR-002** The detail sheet MUST show the link (when present) with a Copy action.
- **FR-003** For a finished/errored task with a link, a Re-download action MUST create a new task from
  that link with the task's destination.

## Testing

Server: the mock emits `detail.uri`; the syno client maps it. E2E: a finished task's link is shown, Copy
is clickable, and Re-download re-adds the task (list grows to two).
