# Feature Specification: Posters and owned markers survive a source outage

**Feature Branch**: `fix/2008-posters-and-owned`

**Created**: 2026-09-05

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: User report: "Title posters from zarfilm have stopped showing up" / "OWNED labels are not showing up either"

## Context

One download source's main domain went down. Two apparently unrelated things
broke with it, and both failed silently.

**Posters.** When a source's main domain is unavailable the operator's configured
mirror takes over, and browsing keeps working — that part is by design. But the
pages the mirror serves name their images on the mirror's own host, and the image
proxy only ever knew the driver's declared image hosts. Every poster from that
source was rejected outright: the deployment logged 127 image requests answered
400 in microseconds, too fast to have attempted a fetch.

**Owned markers.** The library snapshot — which is instance-wide and shared — was
built on the *request* context, after the search had already run. A slow source
makes clients give up; the deployment logged clients hanging up mid-response.
When one did, the NAS listing was cancelled, produced an empty snapshot, and that
empty snapshot was cached for the full five-minute TTL. For those five minutes
nobody saw an owned marker on anything, because one phone had stopped listening.

The two compounded: the outage that made posters fail is the same outage that
made clients hang up.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Posters keep working while a source is on its mirror (Priority: P1)

A user browses a source whose main domain is down. Titles still list, because the
mirror serves them — but every poster is a blank placeholder.

**Independent Test**: With a mirror configured, a poster hosted on the mirror
loads.

**Acceptance Scenarios**:

1. **Given** a source with a configured mirror, **When** a poster is hosted on
   that mirror, **Then** it loads.
2. **Given** the same, **When** a poster is hosted on a subdomain of the mirror,
   **Then** it loads.
3. **Given** any source, **When** an unrelated host is requested, **Then** it is
   still refused — the proxy is unauthenticated and must stay bounded.
4. **Given** no mirror is configured, **Then** the proxy accepts exactly what it
   accepted before.

### User Story 2 - One client giving up cannot blank everyone's markers (Priority: P1)

A user's phone gives up on a slow search. Every user of the instance then sees no
owned markers at all for the next five minutes.

**Independent Test**: Ask for the snapshot with an already-cancelled caller; it is
still built from what the NAS reports.

**Acceptance Scenarios**:

1. **Given** the caller has already gone, **When** the snapshot is wanted,
   **Then** it is still built and reflects the NAS.
2. **Given** a reading that failed, **When** the NAS recovers, **Then** the next
   reading is attempted promptly rather than after the full cache lifetime.
3. **Given** a reading that succeeded, **Then** it is reused for its full
   lifetime — the retry must not turn into a NAS listing per search.

### Edge Cases

- A mirror host must widen only the image proxy, never a source's outbound reach.
- A wedged NAS must not hold the snapshot lock indefinitely.
- "The NAS reports nothing" and "the NAS could not be asked" must be told apart:
  only the second is worth retrying promptly.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The image proxy MUST accept images hosted on the operator-configured
  mirror of a configured source.
- **FR-002**: The image proxy MUST continue to refuse every other host, and MUST
  never accept a host a site nominates for itself.
- **FR-003**: Building the library snapshot MUST NOT depend on the caller still
  being connected.
- **FR-004**: Building the snapshot MUST be bounded in time.
- **FR-005**: A failed reading MUST be retried substantially sooner than a
  successful one is refreshed.
- **FR-006**: A successful reading MUST still be reused for its full lifetime.
- **FR-007**: Neither failure may become visible as a wrong claim about the
  user's library: an unreadable NAS still shows no marker rather than "absent".

## Success Criteria *(mandatory)*

- **SC-001**: With a source on its mirror, its posters render.
- **SC-002**: The image proxy accepts no host it did not accept before, other than
  configured mirrors.
- **SC-003**: A cancelled caller does not empty the shared snapshot.
- **SC-004**: After a NAS blip, markers return within seconds, not minutes.
- **SC-005**: Browsing costs no more NAS listings than before.

## Assumptions

- A mirror the operator typed in is exactly as trusted as the source's own host;
  it is already trusted for fetching pages.
- The snapshot is instance-wide by design, so its build must not be owned by
  whichever request happened to trigger it.

## Credential-Safety Impact

- No DSM allowlist change and no new NAS call; the same listing, on a context that
  outlives the request.
- The image proxy widens only to hosts an operator explicitly configured for a
  source they own. It cannot be widened by a site.
- Nothing new is logged. NAS folder and file names still never leave the server.
