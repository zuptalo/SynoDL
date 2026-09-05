# Feature Specification: Recover the version of downloads made before we recorded it

**Feature Branch**: `feat/0010b-recover-past-versions`

**Created**: 2026-09-05

**Status**: in-review

**Input**: User report: "No existing download labels yet!"

## Context

Recording which version was sent (spec 0010) answers "which of these do I already
have" — but only for downloads made after it shipped. On a real instance that
left 65 existing downloads, and every one of them unlabelled.

The file names on the NAS cannot help: the library has been renamed for a media
server, and the release information is gone from them.

The names we recorded when each download was created still have it. Measured over
303 of them, 302 carry a resolution, and the encoder sits immediately before the
site's own brand tag, which is the last token on almost every one. That is not the
shape the existing release parser expects, which is why nothing was recovered from
them.

## Requirements *(mandatory)*

- **FR-001**: Where a download has no recorded version, the version MUST be
  recovered from the file name recorded when it was created.
- **FR-002**: Recovery MUST take the resolution and the encoder, the encoder being
  the token preceding the site's trailing tag.
- **FR-003**: A recovered version MUST mark an option only when BOTH the
  resolution and the encoder agree, so a token taken from the wrong position marks
  nothing.
- **FR-004**: A recorded version from an actual send MUST take precedence over a
  recovered one.
- **FR-005**: A download whose recorded name yields neither MUST mark nothing.
- **FR-006**: Recovery MUST NOT read the NAS.

## Success Criteria *(mandatory)*

- **SC-001**: Downloads made before the version was recorded are labelled.
- **SC-002**: No option is marked on one half of a match.
- **SC-003**: A renamed library does not affect the outcome.

## Assumptions

- Taking the encoder by position is a heuristic, and safe as one: because both
  halves must agree before anything is marked, a token taken from the wrong place
  matches no option at all.

## Addendum — matching by folder rather than catalog id

Recovering the version only helps if the download record is found in the first
place, and it was being found by catalog id. That fails twice over:

- ids recorded before sources could be told apart carry no source at all — 47 of
  65 records on a real instance — and so never equal the qualified id a title is
  viewed under;
- the same film downloaded from one source is a different id on the other, so
  browsing the second source finds nothing.

Records are now matched by the folder they went to, which is the same folder
whichever source is being browsed, and which is already how a title is resolved
against the NAS.

- **FR-007**: A recorded download MUST be matched to a title by its destination
  folder, not by the catalog entry it came from.
- **FR-008**: A series' season folders MUST match their title's folder.
- **FR-009**: A folder that merely shares a prefix with another MUST NOT match it.

## Credential-Safety Impact

- No NAS call, no DSM allowlist change, no credential involved. It reads a column
  this instance already wrote, and adds nothing to any log.
