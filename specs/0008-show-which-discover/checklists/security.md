# Credential-Safety & NAS-Boundary Checklist: Show which Discover titles you already have

**Purpose**: Requirements-quality gate mandated by constitution Principle III — this spec widens
what SynoDL reads from the NAS, so the requirements themselves must be complete, unambiguous, and
free of conflicts before any code is written.
**Created**: 2026-09-03
**Feature**: [spec.md](../spec.md) · [plan.md](../plan.md) · [contracts/library-api.md](../contracts/library-api.md)
**Depth**: Formal gate (constitution-mandated) · **Audience**: PR reviewer

Items marked `[x]` are satisfied by the requirements as written. Items marked `[ ]` are genuine
gaps found by this review and MUST be resolved in the spec/plan/contract before implementation.

## The widened NAS read

- [x] CHK001 - Is the exact extent of the widened read stated (from directory names to file names inside a browsed title's folder)? [Clarity, Spec §Credential-Safety Impact]
- [x] CHK002 - Is it explicitly recorded that no new DSM API is added to the allowlist, with the already-allowlisted API named? [Completeness, Spec §Credential-Safety Impact; Research §R4]
- [x] CHK003 - Are the folders the read may touch bounded to the operator's configured parents, rather than left as "the NAS"? [Clarity, Spec §FR-001, §FR-003, §FR-007]
- [x] CHK004 - Is the justification for needing file names (rather than directory names alone) documented, with the cheaper alternative and its rejection? [Traceability, Plan §Complexity Tracking; Research §R8]
- [x] CHK005 - Is there a requirement that the client-supplied title in `GET /v1/library/title` cannot be used to read a folder outside the configured parent? The contract accepts an arbitrary `title` string that becomes part of a path; no requirement forbids `../` or absolute-path input. [Gap, Contract §2] — **RESOLVED** as FR-025a + Contract §2 "Rejecting, not sanitising": escape attempts are refused with `400`, never repaired and answered.
- [x] CHK006 - Is a bound specified on how often the widened read may be triggered? Every opened title causes a NAS listing, and no requirement limits the rate, so the endpoint is an unbounded NAS-load amplifier. `POST /v1/session` is already rate-limited for the analogous reason. [Gap, Contract §2] — **RESOLVED** as FR-025b + Contract §2 "Rate limiting": bounded per user via the existing limiter; `429` is treated by the client as a failed lookup.
- [x] CHK007 - Is the read confined to the single operator-configured NAS, with no client-supplied target host? [Consistency with Principle III, Spec §Credential-Safety Impact]

## Persistence — what may be stored

- [x] CHK008 - Is it stated unambiguously that nothing about the NAS's contents is persisted? [Clarity, Spec §Credential-Safety Impact]
- [x] CHK009 - Is the complete set of durable changes enumerated, so a reviewer can confirm the claim rather than trust it? [Measurability, Data Model §4, §5]
- [x] CHK010 - Is the lifetime of the in-memory snapshot bounded and quantified rather than described as "cached"? [Clarity, Spec §FR-010, §FR-010a]
- [x] CHK011 - Is the single-store constraint respected (no second datastore introduced)? [Consistency with Principle III, Data Model §4]
- [x] CHK012 - Is it specified that the one persisted value carries no secret and defaults to today's behaviour? [Completeness, Data Model §4]

## Least privilege — the instance-wide signal

- [x] CHK013 - Is the decision to make the signal instance-wide rather than per-user stated explicitly, rather than left implicit in the design? [Clarity, Spec §Credential-Safety Impact; Plan §Complexity Tracking]
- [x] CHK014 - Is the resulting information disclosure characterised precisely (a user without a folder grant may learn a title exists there)? [Completeness, Spec §Credential-Safety Impact]
- [x] CHK015 - Is the per-user alternative documented as considered and rejected, with reasons? [Traceability, Plan §Complexity Tracking]
- [x] CHK016 - Is there a requirement that the signal grants no new *capability*, only knowledge? [Clarity, Spec §FR-027]
- [x] CHK017 - Are the requirements consistent with the existing parental-control feature? A user with a content rating cap has their catalog filtered server-side, but `GET /v1/library/title` accepts any title, so a restricted user could learn whether a title they may not browse exists on the NAS. No requirement addresses this interaction. [Conflict, Spec §FR-025 vs. existing content-rating behaviour] — **RESOLVED** as FR-025c + Contract §2 "Content rating": held to the same line as fetching a title's details, which is itself unrestricted today. The pre-existing gap is now recorded in Out of Scope rather than silently inherited.

## Permission boundary — sending is unchanged

- [x] CHK018 - Is it required that existing per-user folder permissions continue to govern sending, unchanged by this feature? [Completeness, Spec §FR-027]
- [x] CHK019 - Is it clear that the ownership marker does not become a route to download into a folder the user lacks a grant on? [Clarity, Spec §FR-027; Edge Cases]
- [x] CHK020 - Are the confirmation and hide-owned behaviours specified so they cannot bypass or weaken an existing check? [Consistency, Spec §FR-019 – §FR-024]

## Logging, errors, and observability

- [x] CHK021 - Does the no-logging requirement cover *panics*? FR-026 names logs, error payloads, and metrics, but the constitution's formulation also includes panics, and a panic mid-listing is exactly where a folder name would escape. [Completeness, Spec §FR-026] — **RESOLVED**: FR-026 amended to include panics, matching the constitution's own wording.
- [x] CHK022 - Is it specified that read failures degrade silently rather than surfacing an error that could carry a path? [Coverage, Spec §FR-009, §FR-017; Contract §2]
- [x] CHK023 - Is the client-visible response shape defined so it cannot carry a folder path? [Clarity, Contract §2 — path deliberately absent, Spec §FR-025]
- [x] CHK024 - Is the distinction between "not present" and "could not look" specified as deliberately indistinguishable to the caller? [Clarity, Contract §2]

## Failure, recovery, and lifecycle scenarios

- [x] CHK025 - Are requirements defined for an unreachable NAS, a missing parent, and an unreadable parent — as three cases, not one? [Coverage, Spec §FR-009; Edge Cases]
- [x] CHK026 - Are requirements defined for the empty/zero state (no source configured, no parents set)? [Coverage, Spec §Assumptions; Contract §2 — 409]
- [x] CHK027 - Are requirements defined for snapshot invalidation when the operator *changes a source's parent folders* or deletes a source? FR-008 covers invalidation after a send only, so a parent change could leave a stale snapshot answering for folders no longer configured. [Gap, Spec §FR-008] — **RESOLVED** as FR-008a: a parent-folder change, or adding/disabling/removing a source, discards the retained reading.
- [x] CHK028 - Is the staleness window quantified and its user-visible consequence stated, rather than left as "eventually consistent"? [Measurability, Spec §FR-010a, §SC-003a]
- [x] CHK029 - Are requirements defined for concurrent access to the snapshot (multiple users browsing while it rebuilds)? [Coverage, Data Model §1 — single mutex-guarded cache]

## Requirement quality overall

- [x] CHK030 - Does the spec carry a Credential-Safety Impact section as Principle III requires? [Completeness]
- [x] CHK031 - Is every security-relevant requirement objectively verifiable, rather than relying on adjectives like "safe" or "minimal"? [Measurability, Spec §FR-025 – §FR-027]
- [x] CHK032 - Is the false-positive risk (a title wrongly marked as owned) identified and given a concrete mitigating rule rather than a general intention? [Measurability, Spec §FR-005; Plan §Risks]
- [x] CHK033 - Are the assumptions that underpin the safety argument stated where a reviewer can challenge them? [Assumption, Spec §Assumptions]

## Notes

**Five gaps were found (CHK005, CHK006, CHK017, CHK021, CHK027) and all five are now closed** in
spec.md, plan.md, and contracts/library-api.md. All were requirement-level omissions rather than
design flaws. What each was, and how it was resolved:

- **CHK005 (path escape)** mattered most: a client-supplied title becomes part of a NAS path. Now
  **rejected rather than sanitised** (FR-025a) — repairing hostile input and answering for whatever
  folder the repair produced is precisely the failure this boundary exists to prevent.
- **CHK017 (parental-control interaction)** turned out to expose a *pre-existing* gap rather than a
  new one: content rating narrows catalog search but not retrieval of a single title by id. FR-025c
  holds this feature to that same existing line, and the underlying gap is now recorded in the
  spec's Out of Scope with the reason it needs its own spec.
- **CHK006 (rate limiting)** would have let one authenticated user amplify load against the
  operator's own NAS, since every opened title costs a listing. Now bounded (FR-025b).
- **CHK021 (panics)** was a one-word completeness fix aligning FR-026 with the constitution.
- **CHK027 (invalidation on configuration change)** is a correctness gap with a safety edge — a
  stale reading could answer for a folder the operator has since stopped using (FR-008a).

**Residual risk accepted**: the instance-wide signal (CHK013–CHK016) remains a deliberate
least-privilege trade-off, justified in plan.md under Complexity Tracking rather than resolved.

## Session 2026-09-04 — the widened FileStation read

Added when ownership moved from folder names to file contents. `SYNO.FileStation.List` is
already allowlisted, but it is now called with `filetype=file`, so the server reads **file
names inside a browsed title's folder** where it previously read only directory names.

### Requirement Completeness

- [ ] CHK017 - Is it stated that no new `SYNO.*` API is added, and that the change is a widened *return* from an already-allowlisted one? [Completeness, Plan §Constitution Check]
- [ ] CHK018 - Are the requirements explicit that file names are read but never persisted? [Completeness, Data Model §Credential-Safety Impact]
- [ ] CHK019 - Is there a requirement covering what happens when a title folder listing fails, distinct from the folder being absent? [Coverage, Spec §FR-009, §FR-010c]
- [ ] CHK020 - Are the requirements for the per-user lookup budget quantified, or only asserted to exist? [Measurability, Spec §FR-025b]

### Requirement Clarity

- [ ] CHK021 - Is "video file" defined by a stated rule rather than left to implementation judgement? [Clarity, Research §1]
- [ ] CHK022 - Is it unambiguous that a client cannot influence WHICH path is listed, only which catalog title is asked about? [Ambiguity, Spec §FR-025a]
- [ ] CHK023 - Does the spec state what a season's episode list may and may not imply, so "absent from the list" is not read as "does not exist"? [Clarity, Spec §FR-016a, §FR-016b]

### Consistency

- [ ] CHK024 - Do the logging requirements still forbid folder AND file names now that more names are read? [Consistency, Constitution III]
- [ ] CHK025 - Is the 5-minute retention stated consistently for both index layers, or does one imply a different freshness? [Consistency, Spec §FR-010]
- [ ] CHK026 - Does the ownership signal expose strictly less than the folder listing it derives from? [Consistency, Spec §FR-025]

### Edge Cases

- [ ] CHK027 - Are requirements defined for a folder holding only non-video files, distinguishing it from an empty one? [Edge Case, Spec §FR-001a]
- [ ] CHK028 - Is the case of a video file being written by an active download addressed, rather than left to read as owned? [Edge Case, Spec §FR-001b]
- [ ] CHK029 - Are requirements stated for a season folder whose episode numbering cannot be parsed? [Edge Case, Spec §FR-016b]
- [ ] CHK030 - Is there a requirement bounding how many folder listings a single catalog response may trigger? [Gap, Spec §FR-010b]

### Notes

CHK020 and CHK030 are the two most likely to be found wanting: both concern bounding cost
and abuse, and both are currently stated qualitatively. If either fails, the remedy is a
number in the spec, not a note in the plan.
