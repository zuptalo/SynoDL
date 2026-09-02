# Credential-Safety & Outbound-Boundary Checklist: Multiple Download Sources

**Purpose**: Validate that the *requirements* around credentials, secrets and the outbound
boundary are complete, unambiguous and consistent — before implementation. Mandatory per
`CLAUDE.md` for any spec touching the credential boundary or the outbound allowlist.
**Created**: 2026-09-03
**Feature**: [spec.md](../spec.md) | **Depth**: release gate | **Audience**: reviewer (PR)

These are unit tests for the English, not for the code: each item asks whether something is
*specified* well enough to implement and review, not whether it works.

---

## Cross-Source Secret Isolation

- [ ] CHK001 - Is it explicitly required that one source's session material is never transmitted to another source's hosts? [Completeness, Spec §Credential-Safety Impact]
- [ ] CHK002 - Does the spec state which component owns the decision of *which* credentials accompany a given outbound request, rather than leaving it implicit? [Clarity, Plan §R5]
- [ ] CHK003 - Are the requirements clear that a driver must not be able to read another driver's session material, or only that it must not *send* it? [Ambiguity, Plan §R5]
- [ ] CHK004 - Is there a stated requirement covering the case where two configured sources share a `kind` — that they still hold separate, non-interchangeable session material? [Coverage, Gap, Spec §FR-001]
- [ ] CHK005 - Are the requirements explicit about what happens if a source's stored session is mis-assigned or corrupt, rather than silently sent to the wrong host? [Edge Case, Gap]

## Write-Only Secret Custody

- [ ] CHK006 - Is "write-only" defined precisely enough to be testable — naming every channel (route response, error payload, log line, metric, panic, admin view) it applies to? [Measurability, Spec §Credential-Safety Impact]
- [ ] CHK007 - Do the requirements state what an admin sees *instead* of a stored session value when reviewing a configured source? [Completeness, Gap, Contract §GET /v1/source/providers]
- [ ] CHK008 - Is the partial-update behavior specified — that omitted session fields retain their stored value without those values ever being read back to the client? [Clarity, Contract §PUT /v1/source/providers]
- [ ] CHK009 - Are requirements defined for what a verification failure may reveal, bounding it to a category rather than an upstream body? [Completeness, Spec §FR-019]
- [ ] CHK010 - Is there a stated requirement that `last_error` persists only a category and never upstream text, a URL, or a secret? [Clarity, Data Model §source_providers]

## Signed Download Links as Secrets

- [ ] CHK011 - Do the requirements state explicitly that a signed download link is itself a secret, with the reason (it embeds the account id and grants unauthenticated access until expiry)? [Completeness, Spec §Credential-Safety Impact]
- [ ] CHK012 - Is the prohibition on logging signed links stated as strongly as the prohibition on logging session material, or is it weaker? [Consistency, Spec §SC-010]
- [ ] CHK013 - Are requirements defined for whether a resolved link may appear in a client-facing response at all, and if so for how long it may be held? [Gap, Ambiguity]
- [ ] CHK014 - Is the ~18-hour link TTL captured as a requirement (resolve at send time) rather than only as a research observation? [Traceability, Spec §FR-022]
- [ ] CHK015 - Are requirements defined for what happens if a link expires between resolution and the NAS acting on it? [Edge Case, Gap]

## The NAS Boundary

- [ ] CHK016 - Is it stated as a requirement — not merely an observation — that nothing from any source's session crosses to the NAS? [Completeness, Spec §FR-023]
- [ ] CHK017 - Is FR-023 written so it can be objectively verified, i.e. as a property of the outbound NAS request rather than as a property of the link? [Measurability, Spec §FR-023]
- [ ] CHK018 - Are the requirements consistent between FR-023 (nothing crosses) and SC-006 (download completes without a shared session)? Do they assert the same thing or two different things? [Consistency, Spec §FR-023, §SC-006]
- [ ] CHK019 - Is the address-binding uncertainty recorded as an explicit assumption with a stated resolution path, rather than as an unqualified claim that links are portable? [Assumption, Spec §Edge Cases]
- [ ] CHK020 - Are requirements defined for how an address-bound-link failure would be reported to an operator, distinctly from a generic source error? [Gap, Exception Flow, Spec §Edge Cases]

## Outbound Allowlist Integrity

- [ ] CHK021 - Is the distinction between the *per-driver* catalog allowlist and the *union* image-proxy allowlist stated explicitly, with the reason the union must not apply to catalog calls? [Clarity, Contract §Image proxy]
- [ ] CHK022 - Are the requirements clear that allowlists remain provider-declared and are never influenced by client input or operator free-text? [Completeness, Spec §FR-024]
- [ ] CHK023 - Is domain-suffix matching specified precisely enough to exclude a lookalike registration (e.g. that a suffix match anchors on a dot boundary)? [Clarity, Ambiguity, Plan §R7]
- [ ] CHK024 - Is it a stated requirement that the dns-prefetch host observed on title pages is *excluded* from the allowlist, so a later reader does not "helpfully" add it? [Completeness, Plan §R7]
- [ ] CHK025 - Is the test-only allowlist override required to be impossible to enable in a production build, and is "impossible" defined (not an env var, not a config flag)? [Measurability, Spec §FR-025, Plan §R8]
- [ ] CHK026 - Are requirements defined for what happens when a driver is asked to call a host outside its own allowlist — fail closed, and never fall back to another source's list? [Coverage, Spec §FR-024]

## Operator Disclosure & Informed Consent

- [ ] CHK027 - Is the requirement to disclose the cookie's elevated blast radius tied to a specific moment (the point of paste), rather than to documentation generally? [Clarity, Spec §Credential-Safety Impact]
- [ ] CHK028 - Are the requirements specific about *what* must be disclosed — full account access, the embedded account id in links, and how to revoke? [Completeness, Spec §Credential-Safety Impact]
- [ ] CHK029 - Is the distinction between the existing scoped token and the new full-account cookie stated in requirements, so the two sources are not treated as equivalent risk? [Consistency, Spec §Credential-Safety Impact]
- [ ] CHK030 - Are requirements defined for how an operator revokes or invalidates material they believe is exposed? [Gap, Recovery Flow]

## Migration of Sealed Material

- [ ] CHK031 - Is it a stated requirement that migration is lossless — no operator re-pastes and no field is silently dropped? [Completeness, Spec §FR-004]
- [ ] CHK032 - Are requirements defined for the failure path if an old sealed blob cannot be read into the new shape (fail loudly vs. degrade to not-configured)? [Gap, Exception Flow]
- [ ] CHK033 - Is there a requirement that migration never writes decrypted material anywhere, including temporarily? [Gap, Completeness]
- [ ] CHK034 - Are rollback requirements defined if the migration ships and must be reverted with sessions already re-shaped? [Gap, Recovery Flow]

## Input Handling on Source-Qualified Identifiers

- [ ] CHK035 - Are the requirements explicit that the provider portion of a title id is validated against the caller's configured providers on every request? [Completeness, Contract §GET /v1/source/title/{id}]
- [ ] CHK036 - Since zarfilm ids are URL *paths*, are requirements defined constraining what a driver may do with an id — specifically that it cannot escape its own site or alter the target host? [Gap, Security, Plan §R7]
- [ ] CHK037 - Is the first-colon split rule stated unambiguously, including behavior for a malformed id with no colon or an empty provider portion? [Clarity, Edge Case, Plan §R2]
- [ ] CHK038 - Are requirements defined for whether an id referencing a disabled or deleted source is an error or a silent miss? [Ambiguity, Gap]

## Fan-Out & Abuse

- [ ] CHK039 - Does the spec acknowledge that a single combined search now causes N upstream requests, and state any requirement bounding that amplification? [Gap, Non-Functional]
- [ ] CHK040 - Are per-source timeout requirements quantified, or only described as "a per-source timeout"? [Measurability, Plan §R3]
- [ ] CHK041 - Are requirements defined for whether combined fan-out is rate-limited per user, given the existing login rate limit sets a precedent for protecting upstreams? [Gap, Consistency]
- [ ] CHK042 - Are requirements defined for repeated fan-out against a source already known to be failing — i.e. is there a backoff or circuit-breaking expectation, or is every request retried? [Gap, Non-Functional]

---

## Notes

Items marked `[Gap]` indicate something the requirements do **not** currently say. A gap is not
automatically a defect — it may be correctly out of scope — but each must be answered
explicitly before implementation rather than discovered during it.

The highest-risk clusters, in the order they would hurt:

1. **Cross-source isolation (CHK001–005)** — today `Client.Do` sends every provider header and
   the clearance cookie to whatever host it is called with. Adding a second source turns that
   from harmless-but-untidy into a real leak path. The plan fixes it (R5); these items check
   the *requirement* is stated, not just the design.
2. **Fan-out amplification (CHK039–042)** — genuinely absent from the spec. One client request
   becoming N upstream requests is a new property of this feature and nothing bounds it.
3. **Signed links as secrets (CHK011–015)** — the spec states it, but the requirement needs to
   be as enforceable as the session-material one, since links are far more likely to end up in
   a debug log.
