# Feature Specification: Say when a source session is not valid where it is being asked

**Feature Branch**: `fix/2009-say-when-source`

**Created**: 2026-09-05

**Status**: in-review

**Input**: User report: "In all sources it says zarfilm is not responding but it is hard to spot, when switching to zarfilm only it says download source needs refresh, after swaping tabs once it shows the 30nama content while zarfilm is selected"

## Context

A source whose main domain is down is reached at the operator's mirror. A session
captured on the main domain is not necessarily valid there — and when it is not,
the site does not answer with an error. It answers with a **login page, status
200**. Parsed, that is a page with no results, which is indistinguishable from an
empty archive, so the source reported success with nothing in it.

Three things followed, all of them confusing rather than wrong-looking:

1. Nothing told the user their session was the problem.
2. When a search failed, the results already on screen were kept — deliberately,
   so a refresh does not flash empty. But if the user had switched source in
   between, one source's catalog stayed on screen under another source's name.
3. The one line that explained the missing half of the catalog was grey note text
   the same size and weight as everything around it.

## Requirements *(mandatory)*

- **FR-001**: A listing that returns nothing AND does not report a logged-in
  session MUST be reported as needing a refreshed session, not as an empty result.
- **FR-002**: A listing that IS logged in and simply has no results MUST remain an
  empty result — running out of catalog is not a broken session.
- **FR-003**: Results MUST never remain on screen attributed to a source they did
  not come from; when the selected source changes and the new search fails, the
  stale results MUST be cleared.
- **FR-004**: Results MAY still be kept through a failure when the selection has
  not changed, so refreshing does not flash an empty grid.
- **FR-005**: A source that could not contribute MUST be reported distinctly
  enough to be noticed against the surrounding text.

## Success Criteria *(mandatory)*

- **SC-001**: A session invalid on the answering host produces "needs refreshing",
  not an empty catalog.
- **SC-002**: Exhausting the catalog still produces an empty result.
- **SC-003**: No source's results are ever shown under another source's name.
- **SC-004**: The degraded notice is visually distinct from body text.

## Assumptions

- One session per source is stored, and it is sent to whichever host is answering.
  Whether it is valid on both is the site's business, not ours — but the app must
  say so when it is not.

## Credential-Safety Impact

- No allowlist, NAS, persistence or logging change. The detection reads a flag the
  page already carries and reports it as an existing typed error.
