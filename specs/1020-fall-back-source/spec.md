# Feature Specification: Alternate Domain Fallback for a Download Source

**Feature Branch**: `feat/1020-fall-back-source`

**Created**: 2026-09-03

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: User description: "The main domain of a source can be out of service, and they
provide an alternative domain to be used when that happens — currently https://zhomis.info/ for
zarfilm, and that alternative can change too. Put it in the source settings and fall back to it
when the main domain fails."

## Context

Sources of this kind get blocked or taken offline periodically and publish a mirror to reach
them at. When the main domain is down, SynoDL currently reports the source as unreachable and
the user simply loses that half of Discover, even though the catalog is still perfectly
available at another address.

Verified on 2026-09-03, the mirror is a genuine equivalent, not a landing page:

- it serves the same site (identical WordPress theme) and the **same session cookie
  authenticates** on it (`"u":"1057599"`, `"logged":"1"`);
- it serves the full catalog, and its own page links use the mirror's host;
- download links resolved through it still point at the same storage host.

One difference worth encoding: it redirects `/all-movie/page/1/` to `/all-movie/`, so a
fallback must follow redirects rather than treat a 301 as a failure.

Note this corrects an earlier finding: spec 0007's research recorded `zhomis.info` as a
dns-prefetch hint and deliberately excluded it from the allowlist. It is prefetched precisely
*because* it is the mirror.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Browsing keeps working when the main domain is down (Priority: P1)

A user opens Discover while a source's main domain is unreachable. Instead of losing that
source, they see its results as usual, fetched from the alternate domain.

**Why this priority**: This is the entire point. Without it, a domain outage — which is a
routine event for these sites, not an exceptional one — silently removes a source the operator
is paying for.

**Independent Test**: Make the main domain unreachable while the alternate stays up; Discover
still shows that source's titles, and sending a download still works.

**Acceptance Scenarios**:

1. **Given** a source with an alternate domain configured and its main domain unreachable,
   **When** a user browses Discover, **Then** that source's results appear, fetched from the
   alternate domain, with no error shown.
2. **Given** the same, **When** the user opens a title and sends a download, **Then** it
   resolves and reaches Download Station.
3. **Given** the main domain is working again, **When** a later request is made, **Then** the
   main domain is used again without the operator doing anything.
4. **Given** a source with NO alternate configured and its main domain unreachable, **When** a
   user browses, **Then** the behavior is exactly today's: that source is reported as
   unavailable and the others still render.
5. **Given** both domains are unreachable, **When** a user browses, **Then** the source is
   reported unavailable once, not once per domain.

### User Story 2 - An operator can set and change the alternate domain (Priority: P1)

An administrator sets a source's alternate domain when adding or editing it, and changes it
later when the site publishes a new one — without waiting for a SynoDL release.

**Why this priority**: The alternate domain changes on the site's schedule, not ours. If it
required a code change, the feature would be useless exactly when it is needed.

**Independent Test**: Set an alternate domain on a configured source, change it, and clear it;
each takes effect on the next request.

**Acceptance Scenarios**:

1. **Given** an administrator is adding or editing a source, **When** they view its settings,
   **Then** an alternate-domain field is offered, pre-filled with the mirror SynoDL currently
   knows about for that source kind.
2. **Given** an administrator enters an alternate domain, **When** they save, **Then** it is
   validated and stored, and used for fallback from then on.
3. **Given** an administrator clears the field, **When** they save, **Then** the source falls
   back only to the mirror the driver itself knows, or not at all if it knows none.
4. **Given** an administrator enters something that is not a usable `https` address, **When**
   they save, **Then** it is rejected with a plain-language reason and nothing is stored.

### Edge Cases

- **A slow main domain, not a dead one.** Fallback is bounded by the same per-source timeout,
  so trying both cannot double the time a user waits beyond that budget.
- **An expired session.** Being logged out is NOT a domain outage; failing over would just
  fail identically on the mirror and hide the real cause. Only availability failures fail over.
- **The mirror redirects.** Following redirects is required (the observed mirror 301s page-1
  archive URLs).
- **The mirror's own links use the mirror's host.** Titles browsed through it must remain
  addressable, and must not be treated as off-site.
- **Repeated outages.** The working domain is remembered briefly so every request during an
  outage does not pay for a failed attempt first.
- **A mirror that is stale or hostile.** The operator chose it, and their session material will
  be sent to it — see Credential-Safety Impact.

## Requirements *(mandatory)*

- **FR-001**: A source MUST support an optional alternate domain, settable and changeable by an
  administrator without a release.
- **FR-002**: A driver MAY declare a currently-known mirror for its kind, used as the default
  value offered to the administrator and as the fallback when they set none.
- **FR-003**: When a request to the main domain fails for an AVAILABILITY reason — connection
  refused, DNS failure, timeout, or a server-side error — the same request MUST be retried once
  against the alternate domain.
- **FR-004**: An authentication or entitlement failure MUST NOT trigger fallback: it is not a
  domain outage, and retrying would report the wrong cause.
- **FR-005**: Fallback MUST be transparent to the user: results, titles and downloads behave
  identically whichever domain served them.
- **FR-006**: Pages served by the alternate domain MUST be parsed correctly, including links
  that use the alternate host, and redirects MUST be followed.
- **FR-007**: Once the alternate has answered, it SHOULD be preferred briefly rather than
  re-attempting the main domain on every request, and the main domain MUST be retried again
  after that period so recovery needs no operator action.
- **FR-008**: The alternate domain MUST be validated as an `https` address with a plausible
  host before being stored, and rejected with a plain-language reason otherwise.
- **FR-009**: The total time spent on a source that fails over MUST remain within the existing
  per-source timeout budget.
- **FR-010**: An alternate domain MUST apply only to the source it is configured on, and MUST
  NOT widen any other source's outbound reach.

## Success Criteria *(mandatory)*

- **SC-001**: With the main domain unreachable and an alternate configured, a user can browse,
  open a title and send a download, with no visible difference from normal operation.
- **SC-002**: With the main domain restored, the source returns to using it without any
  operator action.
- **SC-003**: An operator can change the alternate domain and have it take effect without a
  SynoDL release.
- **SC-004**: A source with no alternate configured behaves exactly as it does today.
- **SC-005**: A failing-over source does not make a combined Discover query exceed its existing
  per-source time budget.

## Credential-Safety Impact

Required by constitution Principle III.

- **What changes.** A source gains one non-secret, operator-set field: an alternate base URL.
  No new secret is stored, and the existing session material is unchanged.
- **The real consideration: an operator-set outbound host.** Until now a driver's outbound
  allowlist was entirely provider-declared, and this makes one entry operator-supplied. The
  source's session material — for this provider, a full account cookie — will be sent to
  whatever host the administrator enters. That is a meaningful widening and MUST be stated
  plainly in the field's help text, not buried.
- **Why it is nonetheless acceptable.** It is *administrator* configuration, not client input,
  which is the distinction the constitution's "no client-supplied target hosts" rule draws:
  the same administrator already supplies the NAS address and the session material itself, and
  already chooses to trust this site. The rule exists to prevent an app user, or a remote page,
  steering our outbound traffic — and that remains impossible.
- **Bounding it.** The alternate applies to exactly one source, never to others (FR-010). It
  must be `https` (FR-008), so material is not sent in clear text. It cannot be set by a
  non-admin. Nothing about it is inferred from a page the source served — a site cannot
  advertise its own next mirror and have SynoDL adopt it.
- **What could appear in logs.** Nothing new. The alternate host is not a secret, but requests
  to it carry the same material as requests to the main domain and are subject to the same
  no-logging rule.

## Assumptions

- The alternate domain is a genuine mirror of the same site, sharing the same accounts and
  sessions. Verified for the current one; a future mirror that did not share sessions would
  simply fail to authenticate, and would be reported as needing a refresh.
- Only one alternate per source is needed. Sites publish one at a time.
- Download hosts are unaffected: links resolved through the mirror pointed at the same storage
  domain, which is already allowlisted.
- Fallback is for availability, not load-balancing. The main domain stays preferred.

## Out of scope

- Discovering a new mirror automatically. A site announcing its own next domain and SynoDL
  adopting it would let the site redirect our credentials at will.
- More than one alternate per source.
- Failing over between *sources* — that is what having several sources already does.
