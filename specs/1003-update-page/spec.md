# Feature Specification: In-app update page — what's new, one-tap OK, and self-healing apply

**Feature Branch**: `feat/1003-update-page`

**Created**: 2026-07-27

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Operator request: "When I tap the update notification and the app opens, I should see an
update page that tells me what's about to update, with only an OK button to move forward, and it should
update automatically. If I exit the app mid-process, the next launch should update automatically."

## Overview

A new deploy already fans out a Web Push "A new version is available" notice (spec 0003 Inc.4) and parks
the fresh service worker in "waiting". Previously the client surfaced only a small toast with
Update/Later. This feature replaces that with a dedicated **update page** that shows the incoming
version's release notes, offers a **single OK**, applies + reloads on OK, and **self-heals** an
interrupted apply on the next launch.

## User Scenarios

### US1 — See what's updating and apply (P1)
A new version deploys; the user taps the "A new version is available" notification (or already has the
app open). A full-screen **update page** appears showing "what's new" (the incoming version's release
notes). The only control is **OK**; tapping it applies the update and reloads into the new version.

### US2 — Interrupted apply self-heals (P1)
The user taps OK but closes the app before the reload completes (or is notified and the apply is cut
off). On the **next launch**, the still-pending update is applied **automatically**, without prompting
again.

## Functional Requirements

- **FR-001** When a new service-worker version is waiting, the client MUST show a full-screen update
  page (not a toast) with the incoming version and its release notes ("what's about to update").
- **FR-002** The update page MUST present exactly one forward action — **OK** — and MUST NOT be
  dismissible by any other means (no Later, no backdrop dismiss).
- **FR-003** Tapping OK MUST apply the waiting update (SKIP_WAITING) and reload into the new version.
- **FR-004** The incoming release notes MUST come from the server's `/v1/config` (the server is already
  the new version once deployed, so this reflects the version about to load).
- **FR-005** If OK was pressed but the reload did not complete (app exited mid-process), the next launch
  MUST apply the pending update automatically, without prompting.
- **FR-006** Once the app is running the version it was updating to, the pending marker MUST be cleared
  so a later, different update prompts again rather than auto-applying.

## Notes / Constitution

This amends Principle V's update UX: updates are now mandatory-forward (OK only) and an **interrupted**
apply completes automatically on the next launch. The first encounter is still an explicit, user-visible
page (the user is notified and taps OK) — the auto-apply covers only the recovery case, so a brand-new
update is never applied without the user having seen and accepted it.

## Out of Scope

- Changing what triggers the update push (unchanged: the server's version-change fan-out).
- Background auto-update while the app is closed (browsers don't run app code then; the push +
  next-open page is the mechanism).

## Testing

The apply/auto-heal decision is a pure function (`decideUpdate`) with unit tests on the coverage
allowlist. The service-worker integration and reload are verified manually (the e2e harness cannot stage
a second deploy to produce a waiting worker).
