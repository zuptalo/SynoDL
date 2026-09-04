# Feature Specification: Foreground-aware notifications, Tasks search bar, fewer tabs

**Feature Branch**: `feat/1013-tasks-search-notif`

**Created**: 2026-07-28

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Operator request: "Only send notifications if a user has opted in AND is not online. If the
user is in the app but not on the Tasks tab, an in-app notification is fine. Also remove the Search and
RSS tabs, and add a search bar at the top of the Tasks tab instead."

## Functional Requirements

- **FR-001** A SYSTEM (OS) push notification MUST only appear when no app window is in the foreground.
  When a window is visible, the service worker forwards the push to the page instead of showing a system
  notification (and sets no badge).
- **FR-002** In the foreground, the page MUST show an in-app notification (toast) UNLESS the user is on
  the Tasks tab, where the change is already visible live (then: nothing).
- **FR-003** Opt-in and per-user preferences (spec 1004) still gate whether a push is sent at all.
- **FR-004** The Search and RSS tabs are removed; the tab bar is Tasks, Browser, Settings. Old
  `/tabs/search` and `/tabs/rss` paths redirect to Tasks.
- **FR-005** The Tasks tab gains an always-visible search bar that filters the download list by name
  (case-insensitive, partial), reusing the existing filter term.

## Testing

E2E: the tab bar excludes Search/RSS; the Tasks search bar filters the list by name. The service-worker
foreground routing is verified by build + review (a push into a foregrounded SW can't be staged in the
Playwright harness).
