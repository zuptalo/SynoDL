# Security & Stored-Data Requirements Checklist: Per-User Download Statistics

**Purpose**: Validate that the spec's requirements around Constitution Principle III
(custodial state & credential safety) and the new `download_history` persistence are
complete, clear, consistent, and measurable — BEFORE implementation. These are unit
tests for the requirements text, not for the eventual code.
**Created**: 2026-07-30
**Feature**: [spec.md](../spec.md) · [plan.md](../plan.md) · [data-model.md](../data-model.md)

## Stored Data — What & How Protected

- [x] CHK001 Are all fields persisted by the new table enumerated in the requirements, with each field's purpose stated? [Completeness, data-model.md §download_history]
- [x] CHK002 Is it explicitly required that no credential, session id, OTP, or full task URI is stored in the new table? [Completeness, Spec §FR-004, plan.md §Credential-Safety Impact]
- [x] CHK003 Are `destination`/`task_name` (folder & file names) documented as correlation-only data, with a stated requirement that they are never emitted to logs or error payloads? [Clarity, plan.md §Credential-Safety Impact]
- [x] CHK004 Does the spec state that this table adds no new secret, so no per-column encryption is required (distinguishing it from encrypted `*_enc` columns)? [Clarity, Assumption]
- [x] CHK005 Is the single-volume/one-store rule explicitly preserved (the new table lives in the existing SQLite DB, no new datastore/volume)? [Consistency, Constitution III]

## Task-State vs. Event-Log Boundary

- [x] CHK006 Do the requirements clearly distinguish the append-only statistics/attribution log from live task state (which must remain NAS-owned and unpersisted)? [Clarity, Constitution III, plan.md §Constitution Check]
- [x] CHK007 Is the append-only property stated as a requirement (re-download creates a new record; records are never mutated except size backfill and user-delete cascade)? [Completeness, Spec §FR-006]
- [x] CHK008 Is it specified that no live status beyond completion time + final size is retained (i.e. the log is not a task mirror)? [Clarity, data-model.md §Lifecycle]

## DSM Allowlist

- [x] CHK009 Do the requirements state that no new DSM API is added and the allowlist is unchanged (the watcher reuses the existing task-list poll)? [Coverage, Constitution III/Domain Constraints]

## Per-User Data Isolation (Statistics)

- [x] CHK010 Is it explicitly required that a regular (non-admin) user can view only their own statistics, with zero cross-user leakage? [Completeness, Spec §FR-022, §SC-005]
- [x] CHK011 Are the visibility rules stated as server-enforced (not merely UI-gated), so scope cannot be widened by a crafted client request? [Clarity, Ambiguity — Spec §FR-013/§FR-022 vs. contracts §role gating]
- [x] CHK012 Is the admin/owner "see all users" capability defined using the existing role model (is_admin; owner = first user) rather than a new role tier? [Consistency, Assumptions]
- [x] CHK013 Is the behavior in legacy/stateless mode specified (statistics not applicable) so the isolation rule has no undefined gap? [Coverage, contracts §Errors 403]

## Notification Username Exposure

- [x] CHK014 Is it unambiguous that the owner's username is included ONLY for all-scope (admin/owner) subscribers, and never for a non-admin? [Clarity, Spec §FR-002/§FR-003]
- [x] CHK015 Is the self-exclusion rule specified (a subscriber is not told "added by <themselves>")? [Completeness, contracts §Notification payload, research.md §D5]
- [x] CHK016 Is the readable-title fallback for an underivable title defined so the notification body is never empty and never leaks the raw path structure unintentionally? [Edge Case, Spec §Edge Cases]

## User-Delete Cascade

- [x] CHK017 Is the requirement that a deleted user's history is removed with the account stated explicitly and tied to the existing cascade behavior? [Completeness, Spec §FR-012, Clarifications 2026-07-29]
- [x] CHK018 Is there a stated requirement covering what happens to aggregate/all-time totals after a user delete (they drop accordingly), so the deletion behavior is unambiguous? [Clarity, Spec §Edge Cases]

## Data-Model Correctness (Counts, Sizes, Scope)

- [x] CHK019 Is it explicitly required that counts include paused/canceled downloads (recorded at create time), and is this stated consistently across the count graph and the daily-limit accounting? [Consistency, Spec §FR-008/§FR-023/§FR-016, §SC-006]
- [x] CHK020 Is it explicitly required that size averages use completed downloads only, with size-less rows excluded from averages but retained in counts? [Clarity, Spec §FR-007/§FR-014/§FR-023]
- [x] CHK021 Is the "no completed data" case specified to render as not-available rather than zero or an error? [Edge Case, Spec §FR-015]
- [x] CHK022 Is the fresh-start requirement (no seeding from incomplete legacy `download_events`/`source_downloads`) stated, with the rationale that legacy data is undercounted and size-less? [Completeness, Spec §FR-011, research.md §D3]
- [x] CHK023 Is the daily-limit scope explicitly required to stay catalog-only, with a stated requirement that recording direct downloads does NOT make them count against the limit? [Consistency, Spec §FR-017/§FR-026, §FR-009]
- [x] CHK024 Is the size-backfill correlation defined precisely enough (match by destination + expected file name; unmatched ⇒ size stays null) to be objectively verifiable, including the multi-episode case? [Measurability, data-model.md §Lifecycle, research.md §D2]
- [x] CHK025 Is the "one row per file" counting unit stated and reconciled with multi-episode sends and the daily-limit unit? [Consistency, Spec §FR-008, Clarifications]

## Ambiguities & Assumptions to Resolve

- [x] CHK026 Is the source classification (`catalog` vs `direct`) defined as authoritative-from-origin (never client-supplied for catalog), removing any ambiguity about spoofed sources? [Ambiguity, Spec §FR-010, contracts §4]
- [x] CHK027 Is the direct-download category override precedence (explicit user choice > heuristic > `other`) stated unambiguously, including handling of an invalid/`auto` value without failing the download? [Clarity, Spec §FR-014/§FR-015, contracts §4]
- [x] CHK028 Is the assumption that this behavior applies only in stateful mode documented, so credential-safety reasoning has no undefined path in the legacy proxy? [Assumption, Spec §Assumptions]

## Notes

- These items test the *requirements*, not the implementation. An item passes when
  the spec/plan/data-model answers it clearly, consistently, and measurably.
- REQUIRED by Constitution Principle III (this spec stores data + alters notification
  payloads). Resolve any failing item in the spec/plan before `/speckit-implement`.
