# Security Requirements Quality Checklist: YouTube sign-in for download workers

**Purpose**: unit tests for the credential-boundary requirements
**Created**: 2026-09-29
**Feature**: [spec.md](../spec.md)

## Credential custody
- [x] CHK001 Is it stated where the sign-in is stored and how it is protected at rest? [Spec FR-013, Plan §Credential-Safety]
- [x] CHK002 Is every place a cookie value could surface (API, log, error, notification, history, Job, output) enumerated as forbidden? [Spec FR-014, FR-015]
- [x] CHK003 Is there a requirement for what happens when the key can no longer decrypt the row? [Spec Edge Cases]
- [x] CHK004 Are the API's read-back fields limited to non-secret metadata, and is that list closed? [Contract api.md]

## Delivery to the worker
- [x] CHK005 Is "not readable from the Job/pod description" specified as a testable property? [Spec FR-016, SC-002]
- [x] CHK006 Is the lifetime of every copy bounded by the worker's, whichever way it ends? [Spec SC-007, Plan §Teardown]
- [x] CHK007 Is the grant's scope (one Job, once, ten minutes, that pod's address) fully specified, including the forwarded-header case? [Plan §Binding, Research R3]
- [x] CHK008 Is the failure behaviour specified (fail open) and consistent with the "never breaks a working download" requirement? [Spec Clarifications, contract worker-init.md]
- [x] CHK009 Is the residual risk of plain HTTP inside the cluster stated rather than implied? [Plan §Credential-Safety]

## Permissions and scope
- [x] CHK010 Is "no widening of cluster permissions, no Secrets" a requirement with a check (RBAC file unchanged)? [Spec FR-017, tasks T021]
- [x] CHK011 Is "only YouTube hosts get a grant" specified and tested for every other source? [Spec FR-010, SC-003]
- [x] CHK012 Is admin-only access specified for read, write, remove and status? [Spec FR-001, FR-007]
- [x] CHK013 Is the stateless build's absence of the feature specified? [Spec FR-008]

## Input handling
- [x] CHK014 Are size, count and character bounds on the paste specified? [Spec FR-005, Clarifications]
- [x] CHK015 Do validation messages avoid repeating pasted text, as a requirement? [Spec FR-005]
- [x] CHK016 Is domain filtering specified so unrelated cookies in a pasted file are dropped? [Spec FR-006]

## Outcomes and abuse
- [x] CHK017 Is it specified that a refused download is distinguishable from an ordinary failure without leaking anything? [Spec FR-019]
- [x] CHK018 Is retry behaviour bounded so a saved sign-in cannot cause faster retrying against YouTube? [Spec FR-021]
- [x] CHK019 Is there a requirement covering concurrent workers with the same sign-in? [Spec Edge Cases]

## Gaps noted (not blocking)
- The single-node, trusted pod network assumption (CHK009) would need revisiting on a multi-node cluster with untrusted neighbours; recorded, not solved here.
- A rotated session is not captured back from a worker (spec Assumptions).
