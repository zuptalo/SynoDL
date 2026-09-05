# Feature Specification: Never strand the app on a source that is down

**Feature Branch**: `fix/2010-never-strand-app`

**Created**: 2026-09-05

**Status**: in-review

**Input**: User report: "When a source that is down is selected when app opens, it doesn't allow switching to the healthy one, let's automatically switch to the healthy one even if the previous session the selected source was the one that is not available anymore"

## Context

Discover restores the source the user was last looking at. When that source has
stopped answering since, the first search fails and Discover shows the
full-screen "the download source needs refreshing" state.

That state replaces the toolbar — and with it the source picker. There is then no
way to reach a source that IS working: the app is stuck on the one thing that
cannot work, and the only way out is to fix the broken source.

It became easy to hit once a failing session was reported honestly rather than as
an empty catalog (spec 2009): the dead end was always there, but a source that
failed silently used to land the user on an empty grid they could still navigate
away from.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Reach the working source (Priority: P1)

A user opens the app. The source they last browsed is down; another is fine.
Today they see a dead end. After this change they see the working source's
catalog, with a notice naming the one that is missing.

**Independent Test**: Store a selection pointing at a source that then stops
answering, open Discover, and confirm the catalog appears rather than the
full-screen refresh prompt.

**Acceptance Scenarios**:

1. **Given** the stored selection names a source that is down and another source
   is healthy, **When** Discover opens, **Then** the healthy source's results are
   shown.
2. **Given** the same, **Then** the view is on all sources, so the picker is
   reachable again.
3. **Given** the same, **Then** the notice names the source that could not answer.
4. **Given** every source is down, **Then** the refresh prompt still shows —
   falling back must not disguise a total outage as something else.

### User Story 2 - Keep what the user chose (Priority: P2)

Falling back is a way out of a dead end, not a decision about what the user
prefers.

**Independent Test**: After a fallback, the stored selection still names the
source the user picked.

**Acceptance Scenarios**:

1. **Given** a fallback has happened, **Then** the stored selection is unchanged.
2. **Given** the source recovers, **When** the app is opened again, **Then** the
   user is back on the source they chose.

### Edge Cases

- The fallback must not loop when the fallback view also fails.
- A source failing for a reason that is not about availability must not silently
  change the view.
- A fallback triggered by a stale search must not fight a newer one.

## Requirements *(mandatory)*

- **FR-001**: When a search for a single selected source fails because that source
  is unavailable or needs refreshing, the view MUST fall back to all sources and
  search again.
- **FR-002**: The fallback MUST NOT occur when no single source is selected, which
  also guarantees it cannot loop.
- **FR-003**: When the fallback view also fails, the honest failure state MUST be
  shown.
- **FR-004**: The fallback MUST NOT overwrite the user's stored selection.
- **FR-005**: The source that could not answer MUST be named on screen.

## Success Criteria *(mandatory)*

- **SC-001**: A user whose last-viewed source is down still reaches a working
  catalog on open, without touching settings.
- **SC-002**: The source picker is always reachable when any source can answer.
- **SC-003**: A total outage still reports itself as one.
- **SC-004**: A recovered source is still the user's selection.

## Credential-Safety Impact

- Client-only. No API, DSM allowlist, NAS call, persistence or logging change; the
  fallback deliberately does NOT write the stored preference.
