# Feature Specification: A failing source says why, and says it out loud

**Feature Branch**: `fix/2032-source-failure-reason`

**Created**: 2026-09-15

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: Observed in production. Discover said *"Some results are missing:
ZarFilm isn't responding"* for hours. The site was fine — it served its whole
catalogue, 419 KB of it, to an anonymous request throughout. The actual cause was
an expired login session, and re-entering the credentials fixed it instantly.

## Overview

Two separate failures conspired to send the operator to check a website that was
never down.

**The cooling-off breaker forgets why it opened.** `breakerState` records a
failure count and a deadline, and nothing else. So once a source has failed
`coolOffThreshold` times, `SearchAll` short-circuits and reports
`ReasonUnreachable` — flat, regardless of what actually went wrong. `classify()`
had already worked out the real reason on the very first attempt and that answer
is thrown away. Since every subsequent failure re-arms the window, a source whose
session has expired reports "unreachable" for as long as the condition lasts.

**The reason never reaches a human.** The client turns anything that is not
`unsubscribed`, `needs_refresh` or `filter_unsupported` into the catch-all
"isn't responding" — which is what an operator sees. The server logs nothing at
all about source failures: only request lines. So the true reason, correctly
computed and then discarded, existed nowhere a person could read it.

Together: a known, nameable cause became an inaccurate message and no record.

## User Scenarios & Testing

### User Story 1 - The message names the real cause (Priority: P1)

**Acceptance Scenarios**:

1. **Given** a source whose session has expired, **When** it fails enough times
   to open the breaker, **Then** it still reports `needs_refresh` and the reader
   is told it needs signing in again.
2. **Given** a source that is genuinely unreachable, **When** the breaker opens,
   **Then** it reports `unreachable` as before.
3. **Given** a source that starts working, **When** it succeeds, **Then** the
   remembered reason is cleared with the rest of the breaker state.

---

### User Story 2 - The operator can find out what happened (Priority: P1)

**Acceptance Scenarios**:

1. **Given** a source fails, **When** it does, **Then** the server logs which
   source and why.
2. **Given** a log line is written, **When** it is read, **Then** it contains no
   credential, cookie, token or session material.
3. **Given** a source fails repeatedly, **When** it does, **Then** the log does
   not grow without bound from one reader refreshing a page.

---

### Edge Cases

- **A reason that arrives while the breaker is already open.** The breaker is
  skipping the call, so there is no new reason; it must keep reporting the one it
  has rather than blanking it.
- **Two sources failing for different reasons.** Each keeps its own; one must not
  overwrite the other.
- **A filter the source cannot express.** That is not a failure and must not open
  the breaker or be logged as one.

## Requirements

- **FR-001**: The breaker MUST remember the reason that opened it.
- **FR-002**: While open, a source MUST report that remembered reason rather
  than a flat `unreachable`.
- **FR-003**: A success MUST clear the remembered reason along with the counts.
- **FR-004**: Each source MUST keep its own reason.
- **FR-005**: A source failure MUST be logged server-side with the source's id,
  its name, and the reason.
- **FR-006**: No credential, cookie, token, session value or full URL may appear
  in that log line.
- **FR-007**: Logging MUST NOT turn a repeatedly failing source into a flood: a
  source logs when its reason CHANGES, not once per request.
- **FR-008**: `filter_unsupported` MUST NOT be logged as a failure, and MUST NOT
  open the breaker.

## Success Criteria

- **SC-001**: A source failing with an expired session reports `needs_refresh`
  before AND after the breaker opens.
- **SC-002**: A genuinely unreachable source still reports `unreachable`.
- **SC-003**: One failure produces one log line naming the source and reason; a
  hundred consecutive identical failures do not produce a hundred lines.

## Credential-Safety Impact

- **Reviewed.** This ADDS a log line about a source, so the constitution's
  "never log credentials, sids, OTP codes, or full task URIs" rule is directly in
  scope. The line carries an id, a display name and a fixed reason keyword from a
  closed set — no free-form error text, which is where a URL or token would most
  easily leak.

## Assumptions

- The reason keywords are already a closed set (`needs_refresh`, `unsubscribed`,
  `unreachable`, `timeout`, `filter_unsupported`), so logging the keyword rather
  than the underlying error keeps the line safe by construction.
