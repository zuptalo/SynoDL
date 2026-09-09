# Feature Specification: Discover opens where you left it, and a search can be called off

**Feature Branch**: `feat/1041-discover-resume`

**Created**: 2026-09-09

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: "Could you please change the behaviour of the Discover tab so it
doesn't pull anything new by default when the app is freshly opened and just
present the last session's result if there is any, and if there is no last
session only then pull and show the available titles based on how the client's
filter and sort is set (could be the default). Let's also add the possibility for
the user to cancel the search/filtering/sort order while in progress with a very
elegant and user friendly way."

## Overview

Two complaints with one cause: Discover decides for itself when to talk to the
source, and there is no way to tell it not to.

**Opening the app always searches.** Results live only in memory, so a cold start
has nothing on screen, and the mount path reads that as "nothing yet" and fetches
page one — plus however many more pages it takes to fill the grid. The source is
rate-limited and shared, so every launch spends requests on a view the reader may
not even be there for.

**A search cannot be called off.** While one runs, every control that could
change it is disabled — the sort, the filters, the source picker, the reset
button. That is correct, since changing them mid-flight would race, but it leaves
the reader with nothing to do but wait out a request they already regret. A
filter combination that returns slowly is the exact case where they most want to
take it back.

Both are fixed by the same idea: remember the last view that actually produced
results, and make it something to return to — on a cold start, and when a search
is abandoned.

## User Scenarios & Testing

### User Story 1 - Opening the app (Priority: P1)

Someone opens the app, goes to Discover, and sees what they were looking at last
time. Nothing is fetched.

**Independent Test**: browse, close the app, reopen it, and confirm the same
titles are on screen with no search request made.

**Acceptance Scenarios**:

1. **Given** a previous session's results, **When** the app is opened fresh,
   **Then** they are shown and no search is made.
2. **Given** no previous session, **When** Discover is opened, **Then** it
   searches with whatever the filter and sort are set to, default or not.
3. **Given** restored results, **When** the reader pulls to refresh, **Then** it
   searches — asking is still one gesture away.
4. **Given** restored results, **When** the saved view has changed on another
   device, **Then** it searches, because what is on screen no longer matches what
   the view says.

---

### User Story 2 - Calling off a search (Priority: P1)

Someone changes the sort, realises it was the wrong one, and takes it back
without waiting for it to finish.

**Why this priority**: it is the second half of the request, and it is what makes
a slow source bearable rather than something to wait out.

**Independent Test**: start a search, cancel it, and confirm the previous results
and the previous sort are both back.

**Acceptance Scenarios**:

1. **Given** a search in progress, **When** it is cancelled, **Then** the request
   stops and no further pages are fetched.
2. **Given** a cancelled search, **When** the screen settles, **Then** the
   previous results are shown again.
3. **Given** a cancelled sort change, **When** the screen settles, **Then** the
   sort control reads as it did before — the screen never claims a view it is
   not showing.
4. **Given** a cancelled search, **When** the controls are looked at, **Then**
   they are usable again immediately.
5. **Given** no search running, **When** the screen is looked at, **Then** there
   is nothing offering to cancel.

---

### Edge Cases

- **A cancel that arrives as the results do.** Whichever wins, the screen and the
  controls must agree afterwards; a half-applied view is the one outcome to
  avoid.
- **A first-ever search cancelled.** There is nothing to go back to, so the
  screen says there is nothing rather than restoring emptiness as though it were
  a result.
- **Stored results from a source that has since been removed or disabled.**
  Showing another source's catalog under the wrong name is worse than fetching,
  so a restored view whose source no longer exists is discarded.
- **A very long scroll.** What is remembered has to be bounded; an unbounded
  session is not something to write to a device's storage on every search.
- **Private browsing, or storage that refuses.** Remembering is best effort:
  failing to save or read must leave Discover behaving exactly as it does today.

## Requirements

### Functional Requirements

- **FR-001**: Opening the app MUST NOT search when a previous session's results
  can be restored.
- **FR-002**: With no previous session, Discover MUST search using the filter and
  sort as they are set.
- **FR-003**: Restored results MUST come with the view that produced them, so the
  controls describe what is on screen.
- **FR-004**: A restored view whose source is no longer available MUST be
  discarded rather than shown.
- **FR-005**: Pull-to-refresh MUST still search, unchanged.
- **FR-006**: Remembering MUST be bounded in size and MUST be best effort: a
  storage failure changes nothing else.
- **FR-007**: While a search is running, a way to cancel it MUST be visible.
- **FR-008**: Cancelling MUST stop the in-flight request and any further pages.
- **FR-009**: Cancelling MUST restore both the previous results and the previous
  view, so the controls never describe a view that is not on screen.
- **FR-010**: After cancelling, every control MUST be usable again.
- **FR-011**: A cancel MUST NOT be offered when nothing is running, and MUST NOT
  shift the layout when it appears or goes.
- **FR-012**: A cancelled request MUST NOT be reported as the server being
  unreachable.

### Key Entities

- **Remembered view**: the filters, sort, order, query and source that last
  produced results, together with those results.

## Success Criteria

### Measurable Outcomes

- **SC-001**: A cold start with a previous session makes no search request.
- **SC-002**: A cold start with no previous session shows titles.
- **SC-003**: A cancelled search leaves the controls and the grid describing the
  same view.
- **SC-004**: Cancelling stops further requests to the source.
- **SC-005**: Nothing on screen moves when the cancel affordance appears.

## Credential-Safety Impact

- **No new server state, no new endpoint, no new permission.** The remembered
  view is client-side app data, which is where Principle IV already puts the
  session, settings and filter state.
- **What is stored is catalog metadata already shown on screen** — titles, years,
  poster URLs. No credential, no session material, and nothing the source does
  not publish.
- The saved view continues to be persisted server-side exactly as it is today; a
  cancel restores it, so the two ends do not drift.

## Notes from implementation

- **Two bugs, both silent, both found only by counting requests.** The remembered
  view was written straight off Vue refs, so it held reactive PROXIES —
  IndexedDB refuses to clone one, the write threw, and the best-effort catch
  swallowed it. Every launch searched exactly as before while the code looked
  correct. And Ionic fires `ionViewWillEnter` BEFORE `onMounted`, the opposite of
  what the first guard assumed, so the enter hook searched and the mount then
  restored into a grid that had already been fetched.
- **Infinite scroll counts as asking.** With `threshold="100%"` it fires the
  moment the grid is shorter than the viewport, which after restoring is a page
  request on open by another name. It waits for a real scroll instead.
- One existing test drove searches by reloading. That is exactly what this spec
  stops, so it now pulls to refresh — the gesture that still asks.

## Assumptions

- The remembered results are the reader's own view; they are not shared and not
  read by anything else.
- Restoring does not restore scroll depth. What is remembered is bounded, so a
  deep session comes back as its first stretch with more a refresh away.
- Cancelling a search means "put it back", not "stop and leave it half applied" —
  which is why the view reverts along with the results.
