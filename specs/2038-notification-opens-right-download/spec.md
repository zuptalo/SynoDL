# Feature Specification: Tapping a YouTube notification opens that download

**Feature Branch**: `fix/2038-notification-opens-right-download`

**Created**: 2026-09-27

**Status**: in-progress
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "Tapping a notification about a task, doesn't take me to that task correctly" — screenshot: the NAS "Task details" sheet saying "This task is no longer available."

## Overview

Every push notification carried a bare `taskId`, and a tap always opened
`/tabs/tasks?task=<id>` — the NAS task sheet. A YouTube download's
notification (spec 0013) carries that download's request id, which is no NAS
task, so the sheet reported it gone. The same happened through the in-app
"View" toast.

## Requirements

- **FR-001**: A YouTube download's notification MUST say so (`kind: "download"`);
  a NAS task's stays as it was.
- **FR-002**: Tapping one — as an OS notification or the in-app toast — MUST
  open `/tabs/tasks?download=<id>`, which opens the download's own sheet: the
  playlist sheet for a group, the download sheet for a single.
- **FR-003**: It MUST work when the download is not in the list the page
  holds (a group past the page, or the list still loading): a group is fetched
  by id to stand in for its row; a single's sheet fetches itself.
