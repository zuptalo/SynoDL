# Credential-Safety & NAS-Boundary Checklist: Upload a file straight into your library

**Purpose**: Requirements-quality gate mandated by constitution Principle III. This spec adds the
first DSM API that writes file CONTENT to the NAS, so the requirements must be complete and
unambiguous before any code exists.
**Created**: 2026-09-03
**Feature**: [spec.md](../spec.md)
**Depth**: Formal gate · **Audience**: PR reviewer

`[x]` is satisfied by the requirements as written. `[ ]` is a gap this review found, to be closed
before implementation.

## The new write capability

- [x] CHK001 - Is the exact API being added named, and its capability stated plainly? [Clarity, §Credential-Safety Impact]
- [x] CHK002 - Is it stated that this is a step up from creating empty folders to writing content? [Completeness]
- [x] CHK003 - Is the justification recorded, including why no lesser mechanism suffices? [Traceability]
- [x] CHK004 - Is it required that the server persists no part of the file? [Clarity, §FR-015]

## Where a file can be placed

- [x] CHK005 - Is it required that a client cannot supply a destination path? [Clarity, §FR-002]
- [x] CHK006 - Are the permitted parents bounded to SynoDL's configured folders? [Completeness, §FR-001]
- [x] CHK007 - Are per-user folder grants required on the FINAL path? [Completeness, §FR-003]
- [x] CHK008 - Is the "no parents configured" case specified rather than left to a default? [Coverage, §FR-004]
- [x] CHK009 - Is the uploaded FILE NAME required to be a single path segment? FR-013 says the original name is preserved, but a name containing separators or `..` would be preserved straight into the path sent to the NAS. The title is validated; the file name is not. [Gap, §FR-013] — **RESOLVED** as FR-013a: a file name carrying a separator, `..`, or a control character is REJECTED, never repaired.
- [x] CHK010 - Is there any restriction on WHAT may be uploaded? Nothing limits the capability to media and sidecar files, so the endpoint doubles as a general-purpose "write any file to the NAS" tool — a much broader capability than the request, and one an operator has not knowingly granted. [Gap] — **RESOLVED** as FR-013b: only media and sidecar types are accepted.

## Limits and abuse

- [x] CHK011 - Is a size cap required, and enforced mid-stream rather than only up front? [Completeness, §FR-020]
- [x] CHK012 - Is the cap surfaced to the user rather than only enforced? [Clarity, §FR-021]
- [x] CHK013 - Is the RATE of uploads bounded? A signed-in client may issue uploads back to back, making the endpoint a way to fill the operator's volume as fast as the link allows. Nothing in the spec bounds this. [Gap] — **RESOLVED** as FR-021a: uploads are rate-bounded per user.
- [x] CHK014 - Is it required that a dropped connection frees server resources? [Coverage, §FR-019]
- [x] CHK015 - Is overwriting forbidden rather than left to chance? [Clarity, §FR-012]

## Separation from downloads

- [x] CHK016 - Is it required that an upload does not consume the download allowance? [Clarity, §FR-023]
- [x] CHK017 - Is it required that an upload does not enter download statistics or task ownership? [Completeness, §FR-023]

## Disclosure

- [x] CHK018 - Are file names required to stay out of logs, errors, and metrics? [Completeness, §FR-022]
- [x] CHK019 - Is authentication stated as a requirement in its own right? Grants imply a signed-in user, but "an upload requires an authenticated session" is nowhere written, so a reviewer cannot check it against a requirement. [Gap] — **RESOLVED** as FR-022a: an upload requires an authenticated session.

## Failure and recovery

- [x] CHK020 - Is a partial or failed upload required to leave nothing behind? [Coverage, §SC-005]
- [x] CHK021 - Is a per-file outcome required when several are uploaded? [Completeness, §FR-014]
- [x] CHK022 - Are errors required to be legible rather than numeric? [Clarity, §FR-018]
- [x] CHK023 - Are concurrent uploads creating the same folder addressed? [Coverage, §Edge Cases]

## Requirement quality overall

- [x] CHK024 - Does the spec carry a Credential-Safety Impact section? [Completeness]
- [x] CHK025 - Is every boundary requirement objectively checkable rather than an adjective? [Measurability]
- [x] CHK026 - Are the assumptions underpinning the safety argument stated where they can be challenged? [Assumption]

## Notes

**Four gaps were found (CHK009, CHK010, CHK013, CHK019) and all four are now closed** in spec.md
before any code was written. What each was:

- **CHK009 (file name is a path)** is the sharpest. The title is sanitised into a single segment,
  but the FILE name is explicitly preserved — and it is a client-supplied string that becomes part
  of a path on the NAS. It needs the same single-segment guarantee as the title.
- **CHK010 (no file-type restriction)** turns a media-upload feature into an arbitrary-file-write
  capability. Restricting to media and sidecar types keeps the granted capability the size of the
  request.
- **CHK013 (no rate bound)** makes the endpoint a way to fill a volume; the same omission was
  found on spec 0008's lookup and closed there.
- **CHK019 (authentication unstated)** is a completeness fix, not a hole in the design.
