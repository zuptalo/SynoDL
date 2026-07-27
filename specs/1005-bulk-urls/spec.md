# Feature Specification: Bulk-paste URLs with mixed delimiters, added in batches

**Feature Branch**: `feat/1005-bulk-urls`

**Created**: 2026-07-27

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Operator request: "We should handle bulk URLs separated by space, line breaks, commas,
semicolons, or tabs; and since the downloader has a maximum number of URLs per request, batch them in
batches of 10 links and send the request."

## User Scenario

A user pastes a large list of links — separated by any mix of spaces, line breaks, commas, semicolons,
or tabs — into the new-task box. The count of detected links is shown; on Add, the links are sent to the
NAS in **batches of 10** (Download Station caps URLs per create request), with progress shown for large
pastes.

## Functional Requirements

- **FR-001** URL extraction MUST split pasted text on any mix of whitespace (spaces, tabs, line breaks),
  commas, and semicolons, keeping only accepted schemes (http/https/ftp/ftps/magnet/thunder) and
  de-duplicating in first-seen order.
- **FR-002** On Add, detected URLs MUST be sent in batches of at most 10 per create request, sequentially.
- **FR-003** For a multi-batch paste, progress MUST be shown ("Added N of M…").
- **FR-004** A single-batch paste (≤10) MUST behave exactly as before (one request).

## Out of Scope

- Server-side URL-count enforcement (the client batches; the server already accepts any count within its
  1 MB body cap).
- Per-URL success/failure reporting beyond the existing error surface.

## Testing

Pure `extractUrls` (delimiters) and `batch` (chunking) are unit-tested on the coverage allowlist. E2E:
a 12-link mixed-delimiter paste is parsed and added as two batches (all 12 land).
