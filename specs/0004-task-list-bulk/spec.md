# Feature Specification: Task-list bulk actions, selection mode, app badge, and newest-first sort

**Feature Branch**: `feat/0004-task-list-bulk`

**Created**: 2026-07-27

**Status**: in-progress
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Operator request: an overflow menu with bulk actions (select mode, clear finished, pause/
resume/delete all), a selection mode driven from the FAB, an app-icon notification badge, and a
default sort that puts newly-added downloads at the top.

## User Scenarios & Testing *(mandatory)*

### US1 — Newly-added downloads appear at the top (P1)
A download I just added shows at the top of the list by default.
**Acceptance**: 1) With the default sort, the most recently created task is first. 2) When two tasks
share (or lack) a creation time, the one with the higher NAS id (added later) still sorts first.

### US2 — Bulk actions from an overflow menu (P1)
From a "…" menu at the top-left I can enter **Select mode**, **Clear finished**, **Pause all**,
**Resume all**, or **Delete all** — with a confirmation before any destructive action.

### US3 — Selection mode + bulk action on the selection (P1)
In select mode I tap rows to pick tasks; the **+** button becomes a **checkmark** (disabled until I
select at least one). Tapping the enabled checkmark opens an action menu — **Clear finished**,
**Pause**, **Resume**, **Delete** the selection — whose header shows **how many tasks** are affected;
destructive actions confirm first. I can cancel selection mode.

### US4 — App-icon badge for new notifications (P2)
When a Web Push notification arrives, my installed app icon shows a badge; it clears when I view the app.

### Edge Cases
- A bulk action with nothing eligible is a no-op / disabled.
- Selection persists across live refreshes; tasks that vanish drop out of the selection.
- A cancelled destructive confirmation changes nothing.
- Badging API absent (most desktop browsers, un-installed PWA) → silent no-op.

## Requirements *(mandatory)*
- **FR-001**: The default task sort MUST place the most-recently-created task first, using creation time
  then NAS id (numeric) as a descending tie-break so a just-added task with an equal/absent creation
  time is still first.
- **FR-002**: The Tasks header MUST provide an overflow ("…") menu with: Select, Clear finished, Pause
  all, Resume all, Delete all.
- **FR-003**: Selection mode MUST let the user toggle task selection by tapping rows, show a running
  selection count, and provide a cancel/exit control.
- **FR-004**: In selection mode the create FAB MUST become a checkmark that is disabled with zero
  selected and enabled with one or more; activating it MUST open an action menu (Clear finished, Pause,
  Resume, Delete) whose header states the number of tasks affected.
- **FR-005**: Every destructive bulk action (Delete, Delete all, Clear finished) MUST require an
  explicit confirmation stating the count before it runs.
- **FR-006**: Bulk actions MUST operate via the existing pause/resume/delete task APIs (id lists); no
  new server endpoint is required. "Clear finished" deletes the finished tasks.
- **FR-007**: On receiving a push, the service worker MUST set the app-icon badge where the Badging API
  exists; the app MUST clear it when it becomes visible/focused; absence of the API MUST be a silent no-op.

## Credential-Safety Impact *(constitution-required)*
No change to what crosses the proxy or is stored. Bulk actions reuse the existing task endpoints; the
badge uses only client-side counts. No secrets; nothing new logged or persisted server-side.

## Success Criteria *(mandatory)*
- **SC-001**: A newly added download is visibly first under the default sort, 100% of the time.
- **SC-002**: A user can clear finished / pause / resume / delete many tasks in one action, with a
  count-stating confirmation for destructive ones.
- **SC-003**: The selection-mode FAB checkmark is disabled at zero selected, enabled otherwise.
- **SC-004**: An opted-in device shows an app-icon badge on a new push and clears it on view (supported platforms).

## Assumptions
- The Badging API (`navigator.setAppBadge`/`clearAppBadge`) is used as a progressive enhancement
  (Chromium + Safari 17+ installed PWAs); no fallback where unavailable.
- "Clear finished" deletes tasks whose status is `finished` (Download Station's "clear completed");
  the NAS files are untouched — only the task entries go.
- Bulk pause/resume target only eligible tasks (pausable / paused).
