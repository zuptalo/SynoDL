# Feature Specification: Two addresses per source, each with its own sign-in

**Feature Branch**: `feat/0009-two-addresses-per`

**Created**: 2026-09-05

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: User request: "if you think it is better to make the alternative url the main or if you think we should have both configured in the source as backup plan, let's make that happen, we can allow the user to provide one or both of them in the configuration and we can switch between them when one becomes unavailable just like today"

## Context

A source of this kind gets blocked periodically and publishes a second address.
Today one of the two is fixed in the driver and the other is an operator setting,
and there is exactly **one** set of credentials for both.

An outage showed why that is not enough. The main domain went down, the mirror
took over, and the stored session was not valid there — so the catalog was served
as a login page, which the app could only report as "nothing found". The session
could not be fixed either, because there was nowhere to put credentials that
belong to the second address.

Neither address is really "the" address: whichever is up is the one that matters.
So both become operator settings, each with its own optional sign-in.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Configure either address, or both (Priority: P1)

An operator adding a source can give the address they use, the alternate one, or
both. Leaving one blank is normal, not an error.

**Independent Test**: Configure a source with only an alternate address and browse
it; configure another with both and browse it.

**Acceptance Scenarios**:

1. **Given** only one address is given, **When** the source is browsed, **Then**
   that address is used.
2. **Given** neither is given, **When** the source is browsed, **Then** the
   driver's own built-in address is used, exactly as today.
3. **Given** both are given, **When** one stops answering, **Then** the other is
   used without the operator changing anything.
4. **Given** both are given, **Then** requests reach only those two addresses —
   configuring an address MUST NOT widen where any other source may be reached.

### User Story 2 - Sign in to each address separately (Priority: P1)

Credentials for one address are not necessarily valid on the other: a challenge
cookie is issued per domain, and a site's login cookie is tied to the address it
was issued for. An operator must be able to paste material for each.

**Independent Test**: Store distinct material for the two addresses and confirm
each address is called with its own.

**Acceptance Scenarios**:

1. **Given** material is stored for both addresses, **When** a request goes to one
   of them, **Then** it carries that address's material and not the other's.
2. **Given** material is stored for only one address, **When** a request goes to
   the other, **Then** it carries the material that was given, so a single set
   keeps working exactly as it does today.
3. **Given** material is replaced for one address, **Then** the other's is
   untouched.
4. **Given** an operator views the source, **Then** they can see WHETHER each
   address has material stored, and never the material itself.

### User Story 3 - Keep working through a change (Priority: P2)

An operator who configured a source before this existed must not have to do
anything.

**Acceptance Scenarios**:

1. **Given** a source configured before this change, **When** it is browsed,
   **Then** it behaves exactly as before, with the material already stored.
2. **Given** the same, **When** the operator opens it, **Then** the addresses it
   is actually using are shown.

### Edge Cases

- The same address given twice must be treated as one address.
- An address that is not a valid absolute URL must be refused when saved, not when
  browsed.
- An address whose host is already another source's must not grant this source
  access to that source's material, nor the reverse.
- Removing an address must remove the material stored for it.

## Requirements *(mandatory)*

- **FR-001**: A source MUST accept an optional primary address and an optional
  alternate address.
- **FR-002**: When neither is given, the driver's built-in address MUST be used.
- **FR-003**: Both configured addresses MUST be tried, preferring the one that
  last answered, exactly as the alternate is used today.
- **FR-004**: Each address MUST be able to carry its own sign-in material.
- **FR-005**: A request MUST carry the material belonging to the address it is
  sent to.
- **FR-006**: Where an address has no material of its own, the source's other
  material MUST be used, so one set keeps working.
- **FR-007**: Stored material MUST NEVER be returned to any client; only whether
  it is present.
- **FR-008**: A configured address MUST widen the outbound allowlist for THAT
  source only.
- **FR-009**: Images served from either configured address MUST load.
- **FR-010**: An invalid address MUST be rejected when saved.
- **FR-011**: A source configured before this change MUST keep working untouched.

## Key Entities

- **Address** — one place a source can be reached, and the sign-in material that
  belongs to it.

## Success Criteria *(mandatory)*

- **SC-001**: A source reachable at only its alternate address is fully usable.
- **SC-002**: An outage of one address is invisible to the user beyond a notice.
- **SC-003**: Credentials for one address are never sent to the other when that
  address has its own.
- **SC-004**: No stored credential value is ever readable through the API.
- **SC-005**: Existing sources keep working with no operator action.

## Assumptions

- Two addresses is the shape the problem actually has; a general list would add
  configuration for a case nobody has.
- An operator-typed address is as trusted as the driver's own, for requests and
  for images alike — it is already trusted for fetching pages.

## Credential-Safety Impact

- Material stays encrypted at rest under the same key and is still never returned
  by any endpoint; the API reports only presence.
- Per-address material makes the boundary TIGHTER: credentials for one address
  are no longer sent to a different host by default once that host has its own.
- Each source's allowlist is widened only by its own addresses (FR-008), which the
  existing per-source allowlist already enforces and which gains a test here.
- No new DSM API, no NAS call, no new logging. Addresses are configuration, not
  user content.
