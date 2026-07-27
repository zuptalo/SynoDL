# Requirements Checklist: 0001-connect-tasks-mvp

**Purpose**: spec self-validation (content quality / completeness / readiness).

## Content Quality

- [x] CHK001 Spec speaks in user outcomes (WHAT/WHY), not implementation, in
      every user story [Spec §User Scenarios]
- [x] CHK002 Every story carries priority, why, independent test, and
      Given/When/Then scenarios [Spec §US1–US5]
- [x] CHK003 No unresolved `[NEEDS CLARIFICATION]` markers remain — all
      resolved in §Clarifications with documented defaults

## Requirement Completeness

- [x] CHK004 Every acceptance scenario traces to at least one FR
      [Spec §FR-001..018]
- [x] CHK005 Auth failure taxonomy enumerated and testable [FR-004, SC-004]
- [x] CHK006 Upload size cap specified with server behavior (413) [FR-012]
- [x] CHK007 Filter sheet options enumerate the exact sort keys and twelve DSM
      statuses from the reference app [FR-016]
- [x] CHK008 Persistence scope named (on-device, settings store) [FR-002,
      FR-017]

## Feature Readiness

- [x] CHK009 Success criteria measurable and technology-agnostic
      [Spec §SC-001..005]
- [x] CHK010 Out-of-scope items explicit (select-files, auto-relogin,
      create-paused, Live Activity, multi-NAS) [Spec §Assumptions,
      §Clarifications]
- [x] CHK011 Credential-Safety Impact section present and complete
      (constitution III) [Spec §Credential-Safety Impact]
