# Credential-Safety & Worker-Orchestration Checklist: YouTube downloads you can watch, keep, and retry

**Purpose**: Constitution Principle III gate. Validates that the REQUIREMENTS in spec 0013 are complete, unambiguous and consistent where they touch stored data, worker orchestration credentials, worker inputs, ownership, and what may leave the server. This tests the writing, not the implementation.
**Created**: 2026-09-08
**Feature**: [spec.md](../spec.md)
**Depth**: Formal gate — required by the constitution (Gate sequencing) because this spec touches worker/cluster credentials and the worker input allowlist.
**Audience**: Maintainer reviewing the spec before `/speckit-plan` completes.

## The widened worker-output read

- [x] CHK001 Is the *amount* of worker output SynoDL will read bounded by a requirement, rather than left open? [Gap — an unbounded read of worker output is the one new unbounded allocation this feature introduces] **→ spec updated:** FR-013e added — the read is bounded in size.
- [x] CHK002 Is it specified that reading worker output is confined to workers SynoDL itself created, and how that confinement is established? [Clarity, Credential-Safety Impact §1] **→ already covered:** Credential-Safety Impact §1 — scoped to workloads SynoDL created.
- [x] CHK003 Are the permissions this feature adds enumerated positively — what is granted — rather than only negatively as what is not granted? [Completeness, Credential-Safety Impact §1] **→ already covered:** Stated positively as reading what a worker prints about its own work.
- [x] CHK004 Is there a requirement that no cluster-wide permission is introduced, stated in the spec itself rather than inherited from the constitution? [Gap] **→ already covered:** Credential-Safety Impact §1 — confined to SynoDL’s own namespace.
- [x] CHK005 Is the frequency or cost of reading worker output constrained, given the parallel limit and the polling the client already does? [Gap, Spec §FR-010] **→ spec updated:** FR-013f added — bounded in frequency, and not scaling with viewers.
- [x] CHK006 Is it specified that companion-file and language facts must be captured while the worker's output is still readable, before the orchestrator sweeps it? [Gap, Spec §FR-011 — the fact is durable but its source is not] **→ spec updated:** FR-013g added — outliving facts captured while the output is still readable.
- [x] CHK007 Are requirements defined for what a download reports when its worker produced output that cannot be parsed, as distinct from output that could not be read at all? [Coverage, Spec §FR-013] **→ already covered:** FR-013 covers both cases with one behaviour, which is the right answer.
- [x] CHK008 Is "progress" defined precisely enough to be measurable — what it is a proportion of, for an item fetched as two separate streams? [Measurability, Spec §FR-012] **→ already covered:** FR-012 is the measurable property; how it is computed is plan-level.
- [x] CHK009 Is it specified that the *expansion* worker's output is subject to the same handling rules as a download worker's, given it carries many titles and links at once? [Gap, Spec §FR-016] **→ spec updated:** FR-013h added — all worker output, expansion included, is untrusted input.

## New durable state

- [x] CHK010 Is it explicitly stated whether download records are secrets requiring encryption at rest, or user data that is not? [Ambiguity — Principle III mandates encryption for secrets; a reviewer must not have to infer that these are outside it] **→ already covered:** Credential-Safety Impact §4 — user-chosen public URLs, explicitly not secrets.
- [x] CHK011 Are requirements defined for what happens when unbounded history exhausts the single state volume? [Gap, Spec §FR-006 — "no bound" and "one volume" are in tension and the spec resolves neither] **→ spec updated:** FR-006b added — a record that cannot be stored never fails a download that saved.
- [x] CHK012 Are schema migration requirements stated, including what must happen if the migration fails part-way? [Gap — the store gains tables and prior specs 2012 and 1031 exist precisely because this went wrong before] **→ spec updated:** FR-006c added — a partial schema change leaves data intact and is reportable.
- [x] CHK013 Is it specified what becomes of a user's download records, queued items, and running workers when that user is deleted? [Gap, Coverage] **→ spec updated:** FR-006d added — records removed, queue cleared, running workers left to finish.
- [x] CHK014 Is the boundary between "the request and its finished outcome" (durable) and "in-flight worker state" (derived) drawn precisely enough to be checked against Principle III? [Clarity, Relationship to Spec 0012 §"stand"] **→ already covered:** Relationship to Spec 0012, "decisions that stand".
- [x] CHK015 Is a queued download's record — which has no worker at all — explicitly reconciled with the "never mirrored" rule rather than left for a reviewer to argue? [Consistency, Credential-Safety Impact §5] **→ already covered:** Credential-Safety Impact §5 — a queued download has no worker, so it mirrors nothing.
- [x] CHK016 Are requirements defined for how progress is held between polls, and is it stated that this holding is not a mirror of worker state? [Gap, Spec §FR-010] **→ spec updated:** FR-013g covers what is held and why holding it is not a mirror.
- [x] CHK017 Is it specified that no second datastore is introduced, in the spec rather than only in the constitution? [Traceability, Credential-Safety Impact §4] **→ already covered:** Credential-Safety Impact §4 — no second datastore.
- [x] CHK018 Are retention requirements for a *dismissed* record specified — removed outright, or tombstoned so a re-run can tell "never had it" from "had it and forgot it"? [Ambiguity, Spec §FR-020a — FR-020a depends on this distinction and does not define it] **→ already covered:** FR-020a settles it: dismissed means not held, so the record goes outright.

## User-influenced values reaching a worker

- [x] CHK019 Is it specified that a playlist or channel name injected as album metadata cannot escape the mounted library when it is used as a folder name? [Gap, Spec §FR-036 / §FR-038 — FR-038 addresses shell interpolation but NOT path traversal, and the album value becomes a directory] **→ spec updated:** FR-038a added — the highest-risk finding. A source-derived name is a FOLDER name; discrete-argument passing prevents command injection and does nothing about a path. Both are now required.
- [x] CHK020 Are constraints defined on the length and character set of any source-derived value passed to a worker? [Gap, Spec §FR-038] **→ spec updated:** FR-038b added — length and character constraints on source-derived values.
- [x] CHK021 Is it stated which values reaching the worker are user-influenced, as an explicit list, so a reviewer can check each against the argv rule? [Completeness, Credential-Safety Impact §3 — the spec names the link and the playlist name; is that the whole set?] **→ already covered:** FR-038c now enumerates the set explicitly.
- [x] CHK022 Are requirements defined for what a worker receives when a playlist or channel publishes no usable name at all? [Coverage, Gap, Spec §FR-036] **→ already covered:** FR-036 provides for a generic fallback behind the source-derived one.
- [x] CHK023 Is it specified that expansion output — item identifiers and titles arriving from an external source — is treated as untrusted input before any of it becomes a worker argument? [Gap, Spec §FR-014] **→ spec updated:** FR-013h added.
- [x] CHK024 Is the host allowlist stated to apply to every URL that reaches a worker, including ones derived from expansion rather than typed by a user? [Gap, Spec §FR-014 — 0012 validated only what the user submitted] **→ spec updated:** FR-016a added — the host allowlist applies to links produced by expansion, not only to links a user typed.
- [x] CHK025 Are requirements defined for how an expanded item is identified for the purposes of FR-020's "already holds" check, unambiguously enough that two spellings of one link cannot defeat it? [Clarity, Spec §FR-020] **→ spec updated:** FR-016b added — stable identity derived from the link, not its text.

## Ownership enforcement

- [x] CHK026 Is the admin's positive ability to act on another user's download stated directly, rather than implied by the negative in FR-008? [Clarity, Spec §FR-008] **→ spec updated:** FR-009a added — the admin permission is stated positively.
- [x] CHK027 Are ownership requirements defined for a *group* as well as an item — specifically whether dismissing a group can cancel another user's queued work? [Gap, Spec §FR-005a / §FR-008] **→ spec updated:** FR-009b added — group dismissal is owner-or-admin and binds an admin to the same rules.
- [x] CHK028 Is ownership specified for the artwork proxy, or is it stated why that endpoint is exempt? [Gap — the proxy takes a caller-supplied URL and is not covered by FR-007 or FR-008] **→ spec updated:** FR-009d added — the artwork proxy requires a signed-in user.
- [x] CHK029 Are requirements stated for what a refused request reveals, so a refusal cannot itself confirm that a download exists? [Clarity, Spec §FR-008 — "MUST NOT disclose that download's existence" is asserted; is the observable behaviour specified?] **→ already covered:** FR-008 states the property; the observable form is plan-level.
- [x] CHK030 Is it specified whether the queue and its fair-share ordering expose anything about other users' downloads to a non-admin? [Gap, Spec §FR-022b — fairness is inherently cross-user and the spec does not say what a user may see of it] **→ spec updated:** FR-009c added — the queue reveals nothing cross-user to a non-admin.
- [x] CHK031 Are ownership requirements consistent between listing, detail, retry, dismissal, and notification? [Consistency, Spec §FR-007, §FR-008, §FR-025d] **→ already covered:** FR-007, FR-008, FR-009a–d and FR-025d align.

## What must never reach a log, an error payload, or a client

- [x] CHK032 Is the set of values that must never be logged enumerated for the values this spec newly introduces — worker output, expansion results, progress, saved paths, playlist names? [Completeness, Credential-Safety Impact §2] **→ spec updated:** FR-032a added — the never-logged set enumerated, and the safe-to-expose set with it.
- [x] CHK033 Is the "plain language, no internals" rule stated for every new failure path, or only for a download's failure reason? [Coverage, Spec §FR-032 — expansion failure, queue admission failure and retry failure are not covered] **→ spec updated:** FR-032 widened to every failure this feature can produce.
- [x] CHK034 Is it specified what a client receives when the orchestrator or the worker-output read fails, distinct from what it receives when a download fails? [Ambiguity, Spec §FR-013] **→ spec updated:** FR-032b added — infrastructure failure reported distinctly from a failed download.
- [x] CHK035 Are requirements defined ensuring the orchestrator credential cannot appear in any new error surface introduced by this spec? [Traceability, Credential-Safety Impact §1] **→ already covered:** Inherited from spec 0012 and restated in Credential-Safety Impact §1.

## The constitution amendment itself

- [x] CHK036 Does the spec state what the amended Principle III text must *permit*, rather than only that an amendment is needed? [Gap, Credential-Safety Impact §"Constitution impact"] **→ already covered:** Credential-Safety Impact enumerates (a), (b) and (c).
- [x] CHK037 Is it specified that the amendment must land with or before the implementation, rather than after it? [Gap — Principle I makes code without an approved constitutional basis a defect] **→ spec updated:** Constitution impact now requires the amendment before or with the first dependent task.
- [x] CHK038 Is the expected semantic-version bump of the constitution stated, so the amendment PR can be checked against it? [Gap, Governance] **→ spec updated:** Constitution impact now states MINOR, 2.1.0 → 2.2.0, with the reasoning.

## Result

**38 of 38 evaluated. 16 already covered, 22 required a spec change — all now resolved.**

The spec gained 18 functional requirements as a direct result of this gate
(FR-006b–d, FR-009a–d, FR-013e–h, FR-016a–b, FR-032a–b, FR-038a–c), plus two
success criteria (SC-007 widened, SC-007a) and two edge cases.

## Notes

- Check items off as completed: `[x]`
- An item that passes needs no spec change. An item that fails should be resolved by editing `spec.md`, not by annotating this file.
- **CHK019 is the highest-risk item in this checklist.** FR-038 forbids assembling a source-derived value into a command string, which addresses shell injection — but the same value is used as a *directory name* under the mounted library, and the spec contains no requirement preventing it from escaping that library. Path traversal and shell injection are different problems with different mitigations; the spec currently answers only one.
- CHK006, CHK011, CHK013 and CHK024 are each a case where a decision already taken in the Clarifications session has a consequence the requirements do not yet carry.
- This checklist was generated before `plan.md` exists, so every item is answerable from `spec.md` alone. Items are about whether the requirements are written well enough to implement and review — not about whether an implementation behaves.
