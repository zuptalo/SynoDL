# Security Requirements Checklist: Who made it — cast and director on a title

**Purpose**: Validate that the requirements governing this feature's credential and
outbound boundary are complete, unambiguous and measurable — BEFORE implementation.
Required by the constitution's gate sequencing: this spec adds a persisted table
**and** two new outbound hosts, both Principle III surfaces.

**Created**: 2026-09-16
**Feature**: [spec.md](../spec.md) · [plan.md](../plan.md) · [contracts/person-photo.md](../contracts/person-photo.md)
**Depth**: formal release gate · **Audience**: reviewer (PR)

This is a test of the *requirements*, not of the code. An unchecked item means the
spec needs changing, not that an implementation is wrong.

## The outbound allowlist

- [x] CHK001 - Is the complete set of newly-reachable hosts enumerated, rather than described by category? [Completeness, Spec §FR-018, Contract §Outbound allowlist]
- [x] CHK002 - Is the requirement that this allowlist is SEPARATE from the existing source image proxy's stated, with its reason? [Clarity, Spec §FR-018; Plan §Constitution Check]
- [x] CHK003 - Is it stated that the allowlist is fixed at build time and cannot be influenced by operator configuration, a source, or a client? [Completeness, Spec §FR-018]
- [x] CHK004 - Are the mock/dev redirects required to be a BUILD-TIME capability absent from a release binary, rather than a runtime setting? [Clarity, Spec §FR-035; Research §R6]
- [x] CHK005 - **[RESOLVED — FR-018a added]** Is there a requirement that a URL discovered inside third-party content is itself re-validated against the allowlist before being fetched? [Gap → Spec §FR-018a]
- [x] CHK006 - **[RESOLVED — FR-018b added]** Is redirect-following behaviour specified, so an allowlisted host cannot hand the fetch to one that is not? [Gap → Spec §FR-018b]

## What a client can steer

- [x] CHK007 - Is the exact shape of the only client-supplied value that reaches an outbound request specified, and is it a shape that cannot express a host? [Clarity, Spec §FR-019]
- [x] CHK008 - Is it required that validation happens BEFORE any outbound request exists, rather than before the response is returned? [Clarity, Spec §FR-019]
- [x] CHK009 - **[RESOLVED — FR-014a added]** Is it stated that the source-session-bearing person lookups are server-initiated only and never addressable by a client? [Gap → Spec §FR-014a]
- [x] CHK010 - Are requirements defined for what a caller may LEARN from the endpoint, and is that assessed as public? [Completeness, Spec §Credential-Safety Impact]
- [x] CHK011 - Is the decision to leave the endpoint unauthenticated justified against the alternative, rather than assumed? [Traceability, Spec §Credential-Safety Impact; Research §R4]

## Amplification and exhaustion bounds

- [x] CHK012 - Is a bound on concurrent outbound lookups specified, and is it instance-wide rather than per-request? [Measurability, Spec §FR-021]
- [x] CHK013 - Is a per-caller rate limit required on the endpoint that can trigger a third-party lookup? [Completeness, Spec §FR-021]
- [x] CHK014 - Is the number of bytes read from an upstream page bounded, with a stated stopping condition? [Measurability, Spec §FR-020]
- [x] CHK015 - Is the number of outbound lookups a single title can cause bounded? [Coverage, Spec §FR-007, §FR-014; Edge Cases]
- [x] CHK016 - Is it required that concurrent demand for the same unresolved person collapses to one lookup? [Completeness, Spec §FR-028]
- [x] CHK017 - Are timeouts specified for each outbound leg? [Measurability, Contract §Outbound allowlist]
- [x] CHK018 - **[RESOLVED — FR-030a added]** Are bounds specified on the SIZE of values accepted from a source before they are stored or rendered? [Gap → Spec §FR-030a]

## What the new table may hold

- [x] CHK019 - Is the full column set of the new persisted state enumerated, and classified as secret or non-secret? [Completeness, Spec §Credential-Safety Impact; Data model §2]
- [x] CHK020 - Is there an explicit prohibition on the table holding a user id, a title id, or anything revealing who viewed what? [Clarity, Spec §FR-030]
- [x] CHK021 - Is the absence of encryption justified by the contents, rather than by omission? [Traceability, Spec §Credential-Safety Impact]
- [x] CHK022 - Is it required that the store remains a single datastore on one volume? [Consistency, Spec §FR-025; Principle III]
- [x] CHK023 - Are growth bounds specified in a way that can be objectively measured (rows, expiry), not "reasonable"? [Measurability, Spec §FR-029; Data model §2]
- [x] CHK024 - Is the persistence decision justified AGAINST Principle III's default preference for deriving state? [Traceability, Spec §Credential-Safety Impact; Research §R5]
- [x] CHK025 - Are requirements defined for the failure mode of the schema change itself (a migration that cannot be applied)? [Coverage, Recovery; Data model §2 — `IF NOT EXISTS` + append-only migrations, per the spec 1031 drift repair]

## What reaches the third party

- [x] CHK026 - Is it specified exactly what is sent to the third party, as an enumerated list rather than "no personal data"? [Clarity, Spec §Credential-Safety Impact]
- [x] CHK027 - Is it required that the lookup is made by the server, so no user's IP or user agent reaches the third party? [Completeness, Spec §Credential-Safety Impact, §FR-016]
- [x] CHK028 - Is it required that no cookie, session, referrer or title context accompanies the lookup? [Completeness, Spec §Credential-Safety Impact]
- [x] CHK029 - Are requirements defined for the third party refusing, blocking or rate-limiting SynoDL — including that recovery is automatic? [Coverage, Exception Flow; Spec §FR-023, §SC-006; Data model §Lifetimes]

## Logs, errors, and untrusted content

- [x] CHK030 - Is there an explicit prohibition on names, characters and image URLs reaching logs, error payloads, metrics or panics? [Completeness, Spec §FR-034]
- [x] CHK031 - Is it required that an upstream failure is swallowed into an ordinary outcome rather than surfaced, so an upstream body or URL cannot leak through a message? [Clarity, Spec §FR-023]
- [x] CHK032 - Is source-published text required to be treated as untrusted when rendered, rather than assumed safe? [Completeness, Spec §FR-034]
- [x] CHK033 - Is the value used to build an outward link required to be validated strictly, consistent with how the existing title link is validated? [Consistency, Spec §FR-033; Data model §4]

## Scope boundaries

- [x] CHK034 - Is it stated that this feature touches neither the NAS, the DSM allowlist, nor worker orchestration? [Completeness, Spec §Credential-Safety Impact]
- [x] CHK035 - Is it stated that no new operator configuration, API key or secret is introduced? [Completeness, Spec §Assumptions; Research §R3]
- [x] CHK036 - Are the boundaries of the feature stated such that "no photograph" is a designed outcome and never an error condition? [Consistency, Spec §FR-023, §FR-031, §US5]

## Resolution log

Six items opened as gaps against the spec and were closed by amending it, which is
what this gate is for:

| Item | Gap found | Amendment |
|---|---|---|
| CHK005 | Nothing required a URL *found inside* an upstream page to be re-checked against the allowlist — the host rule only governed the hosts we deliberately contact. A page could therefore name an image anywhere. | **FR-018a** |
| CHK006 | Redirect behaviour was unspecified, so an allowlisted host could hand the fetch to one that is not. | **FR-018b** |
| CHK009 | FR-014 permitted person-page fetches but never said they are server-initiated only. As written, a future client-addressable version would not violate it — and that version spends the operator's source session on a client-supplied path. | **FR-014a** |
| CHK018 | Nothing bounded the SIZE of a name, character or URL taken from a source, so a hostile or broken source could put an arbitrarily large value into the store and the DOM. | **FR-030a** |
| CHK025 | Migration failure had no stated requirement; resolved by pointing at the existing append-only + `IF NOT EXISTS` mechanism in the data model rather than inventing a new rule. | documented, no new FR |
| CHK023 | "Bounded" was qualitative in the spec; the measurable numbers live in the data model. | cross-referenced, no new FR |
