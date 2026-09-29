# Security & Worker-Orchestration Requirements Checklist: Admin-run music library repair

**Purpose**: Unit tests for the requirements — are authorisation, worker orchestration, worker-output handling, one-at-a-time and data-loss safety specified completely, clearly and measurably? Release gate; reviewer audience.
**Created**: 2026-09-29
**Feature**: [spec.md](../spec.md)

## Authorisation

- [x] CHK001 Is it stated that every operation is admin-only and refused for everyone else regardless of what the client sends? [Clarity, Spec §FR-001, §SC-002]
- [x] CHK002 Is the refusal required to be server-side, not only hidden in the UI (including the snapshot acknowledgement)? [Completeness, Spec §FR-008, contracts/api.md]
- [x] CHK003 Is the behaviour for a user who is deleted after starting a run defined? [Edge Case, Spec §Edge Cases, data-model]

## Worker orchestration (constitution Principle III)

- [x] CHK004 Is it specified that no RBAC verb, secret or service-account token is added, and is that measurable? [Measurability, Spec §FR-017, §SC-005, tasks T035]
- [x] CHK005 Is exactly one library, and the pinned worker image, required in the Job? [Clarity, Spec §FR-017, tasks T009]
- [x] CHK006 Is the server forbidden from mounting the library, and is the consequence (it cannot read plans) reflected in the requirements? [Consistency, Spec §FR-015, Edge Cases]
- [x] CHK007 Is the same-version guarantee defined, and is the mechanism's constitution reading flagged for review? [Clarity, Spec §FR-018, plan §Constitution Check]
- [x] CHK008 Are the inputs that reach the worker limited to a validated plan id as one argv element? [Completeness, research R5, tasks T009/T023]

## Worker output

- [x] CHK009 Is worker output required to be bounded, parsed into a known shape, never logged and never returned raw? [Completeness, Spec §FR-016, §FR-022]
- [x] CHK010 Are the concrete bounds (bytes read, line count, string and array caps, integer range) specified rather than left to implementation? [Ambiguity → resolved, contracts/worker-events.md, research R2]
- [x] CHK011 Is the behaviour for a missing, oversized or malformed report defined (degrade, never guess)? [Edge Case, Spec §FR-016, Edge Cases]
- [x] CHK012 Is the only free text that can reach a client bounded (≤ 20 example paths) and admin-only? [Clarity, Spec §FR-005 (clarified), Credential-Safety Impact]

## One at a time, resume, and data-loss safety

- [x] CHK013 Is single-run enforcement specified to hold under simultaneous requests and against a command-line run? [Measurability, Spec §FR-012, §FR-020, §SC-003, research R4]
- [x] CHK014 Is the snapshot acknowledgement required per request and never remembered? [Clarity, Spec §FR-008 (clarified)]
- [x] CHK015 Is the freshness rule of an applicable plan quantified and its boundary defined? [Ambiguity → resolved, Spec §FR-007 (24 h from the check finishing)]
- [x] CHK016 Is it specified which plan may be applied (only one this feature made) and what happens to a plan made elsewhere? [Completeness, Spec §Edge Cases, §Assumptions]
- [x] CHK017 Is resuming an unfinished apply distinguished from starting a new one? [Clarity, Spec §FR-010]
- [x] CHK018 Are the tool's safety properties (no deletion, no overwrite, stale skipped, reversible) required to hold unchanged? [Consistency, Spec §FR-019]
- [x] CHK019 Is a run whose Job vanished, failed or was swept required to be shown as not finished rather than completed? [Edge Case, Spec §FR-014, tasks T017]

## State and storage

- [x] CHK020 Is it stated what is stored (request, finished outcome, bounded summary) and what is derived (live state, progress)? [Clarity, Spec §FR-014, Credential-Safety Impact]
- [x] CHK021 Is the retention of history bounded and specified? [Measurability, Spec §FR-013, §Assumptions (50 runs)]
- [x] CHK022 Is the restart behaviour (mid-run, and outcome capture) specified and testable? [Coverage, Spec §SC-004, Edge Cases, tasks T017]

## Availability

- [x] CHK023 Is it specified when the feature is unavailable and that the rest of the app is then unaffected? [Coverage, Spec §FR-002, §SC-007, tasks T034]
- [x] CHK024 Are the reasons for unavailability distinguished for the operator (no cluster / no library / no image)? [Clarity, contracts/api.md]

## Notes

- 24 items; the first draft left the concrete worker-output bounds (CHK010) and the exact freshness boundary (CHK015) to implementation. Both are now numbers in the contracts / spec.
- The init container reading of "worker images are pinned third-party tags" (CHK007) is a deliberate judgement flagged for the reviewer, not an oversight.
