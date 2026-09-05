# Feature Specification: Mark the version you downloaded, from what we sent

**Feature Branch**: `feat/0010-mark-version-you`

**Created**: 2026-09-05

**Status**: in-review

**Input**: User request: "make sure the downloaded version in owned it is marked correctly for series and movies"

## Context

Marking which version of a title is already on the NAS was built on reading the
file names: the resolution and the release group are taken from them and compared
against each download option.

Measured against a real library, that cannot work. Of 63 episode files in one
series folder, **none** yielded a resolution or a release group. The library has
been renamed for a media server, which is the ordinary thing to do — and that
renaming removes exactly the information the matching depends on. It is not
recoverable by better parsing; it is not in the file any more.

So the season header could say a season was on the NAS while no option said which
version, and there was no way for it to.

There is one record that survives renaming: what SynoDL itself sent. It was not
being kept.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See which version you took (Priority: P1)

A user opens a title they downloaded through SynoDL. The option they chose is
marked, whatever the files have since been renamed to.

**Acceptance Scenarios**:

1. **Given** a version was sent for a title, **When** the title is opened,
   **Then** that option is marked.
2. **Given** the same, **Then** no other option for that title is marked.
3. **Given** a series, **When** a version was sent for one season, **Then** only
   that season's matching option is marked.
4. **Given** a movie, **Then** the option sent for it is marked.
5. **Given** the files have since been renamed, **Then** the marking is unchanged
   — it does not depend on them.

### User Story 2 - Never claim the wrong version (Priority: P1)

**Acceptance Scenarios**:

1. **Given** the source has reordered its options so an identifier now refers to a
   different one, **When** the title is opened, **Then** the option that matches
   how the sent one described itself is marked, and the one that merely inherited
   the identifier is not.
2. **Given** nothing was sent and the files identify nothing, **Then** no option
   is marked and the season still reports what it holds.
3. **Given** content that arrived by some other route and whose files DO identify
   a release, **Then** matching on the files still applies.

### Edge Cases

- A download recorded before this existed has no version stored; it must mark
  nothing rather than guess.
- Two options that describe themselves identically are indistinguishable, and
  both may be marked.
- Re-sending a different version of the same season must leave the record
  describing what was actually sent last.

## Requirements *(mandatory)*

- **FR-001**: The version chosen for a download MUST be recorded when it is sent.
- **FR-002**: An option MUST be marked when it is the version recorded as sent for
  that title, and for a series, that season.
- **FR-003**: Where a recorded version describes itself, that description MUST
  decide the match; an identifier alone MUST NOT.
- **FR-004**: A record with no description MUST fall back to the identifier, there
  being nothing else.
- **FR-005**: A record from before this feature MUST mark nothing.
- **FR-006**: Matching on file names MUST remain for content that arrived by other
  routes.
- **FR-007**: Marking MUST NOT depend on the files keeping their original names.
- **FR-008**: The recorded description MUST NOT decide what is fetched; the
  download MUST still be resolved from the option's identifier against the source.

## Success Criteria *(mandatory)*

- **SC-001**: A title downloaded through SynoDL shows which version, for series
  and movies alike.
- **SC-002**: Renaming the files does not change what is marked.
- **SC-003**: A reordered catalog never marks the wrong version.
- **SC-004**: Nothing is marked when nothing is known.

## Assumptions

- Renaming a library for a media server is normal, and losing release information
  to it is expected rather than a fault to correct.
- The description a source gives an option is stable enough to match on between a
  send and a later viewing, and is more stable than a positional identifier.

## Credential-Safety Impact

- No DSM allowlist change, no NAS call added, no credential involved.
- What is stored is catalog description — the wording a source already publishes
  for an option — alongside the download record that already exists. Nothing about
  NAS contents is added, and nothing new is logged.
