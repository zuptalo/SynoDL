# Feature Specification: A source that omits the resolution can still be matched

**Feature Branch**: `fix/2011-source-omits-resolution`

**Created**: 2026-09-05

**Status**: in-review

**Input**: User report: "the OWNED details are gone! We see the title as owned, but when we dive in we don't see which quality is actually available on the NAS anymore!"

## Context

Ownership became per-release: an option is marked as already downloaded only when
the copy on the NAS can be identified as that option's release, which requires
both a resolution and a release group.

One source returns an **empty `resolution` on every download it offers**, while
naming the resolution plainly in the quality label ("BluRay 1080p"). An option
carrying no resolution can never satisfy the rule, so for that source nothing was
ever marked: the season header said "on your NAS" and not one of its options said
which. It reads as the feature having broken.

## Requirements *(mandatory)*

- **FR-001**: Where a source states a resolution, that value MUST be used.
- **FR-002**: Where a source omits it but names it in the option's own
  description, the resolution MUST be read from there.
- **FR-003**: Where neither states one, the resolution MUST stay empty — a guess
  could mark an option the user does not have.
- **FR-004**: The resolution MUST be normalised so that a label saying 4K or UHD
  agrees with a release named 2160p.
- **FR-005**: Deriving a resolution MUST NOT change which option is marked when
  the release on disk cannot be identified at all.

## Success Criteria *(mandatory)*

- **SC-001**: Every option that names a resolution carries one.
- **SC-002**: An option that names none carries none.
- **SC-003**: Nothing is marked on the strength of a resolution alone.

## Assumptions

- Reading the resolution out of a label the source wrote is not a guess: it is the
  same fact, stated in the only place that source states it.

## Credential-Safety Impact

- No allowlist, NAS, persistence or logging change. A field is filled from text
  the source already returned.
