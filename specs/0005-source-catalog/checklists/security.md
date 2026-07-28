# Checklist: Credential Boundary & Outbound Allowlist (Principle III)

**Purpose**: Validate that the specification's requirements around custodial secrets and the new outbound
surface are complete, clear, consistent, and measurable — before implementation.
**Created**: 2026-07-28
**Feature**: [spec.md](../spec.md) · **Plan**: [plan.md](../plan.md)
**Focus**: security / credential-safety (Constitution Principle III) · **Depth**: formal release gate ·
**Audience**: reviewer (PR)

## Secret Storage & Encryption

- [ ] CHK001 - Is the complete set of provider "session material" fields enumerated in requirements (clearance cookie, api key, auth token, User-Agent, platform/app-version)? [Completeness, Spec §FR-002, data-model.md]
- [ ] CHK002 - Is "encrypted at rest" specified with the concrete mechanism (existing `Cipher` / `SECRETS_KEY`) rather than left generic? [Clarity, Spec §Credential-Safety Impact]
- [ ] CHK003 - Are requirements explicit that only the non-secret provider status columns may be persisted in plaintext, and which those are? [Clarity, data-model.md `source_providers`]
- [ ] CHK004 - Is the behavior on missing/rotated `SECRETS_KEY` addressed (unrecoverable-by-design, consistent with the NAS-password precedent)? [Gap]

## Write-Only / Never-Returned

- [ ] CHK005 - Is it unambiguously required that session material is never returned to any client on any endpoint (including admin reads)? [Completeness, Spec §FR-002/FR-005]
- [ ] CHK006 - Are the exact non-secret fields that `GET /v1/source/status` may return enumerated, so "status only" is testable? [Measurability, contracts §status]
- [ ] CHK007 - Is there a requirement that a re-`PUT` (refresh) does not require the client to re-read prior secrets? [Consistency, Spec §US4]

## Log / Error / Metric / URL Leakage

- [ ] CHK008 - Do requirements prohibit secrets (cookie, `c-token`, api key, UA) in logs, error payloads, metrics, and panics — not just logs? [Completeness, Spec §FR-003]
- [ ] CHK009 - Is it required that full signed download URLs are treated as secret and never logged (they embed a signature)? [Coverage, Spec §Credential-Safety Impact, R4]
- [ ] CHK010 - Is it required that no secret ever appears in a URL or query string (only headers/body)? [Clarity, Spec §FR-003]
- [ ] CHK011 - Are verification-failure error reasons required to be categorical (e.g. "challenge"/"invalid_token") rather than echoing provider responses that might contain values? [Ambiguity, contracts §PUT session]

## Verify-Before-Store & Admin-Only

- [ ] CHK012 - Is "verify before store" specified precisely (a sample provider call must succeed; nothing persists on failure; prior state preserved)? [Clarity, Spec §FR-004]
- [ ] CHK013 - Is admin-only enforcement required at the server (not merely hidden in the UI) for configure/refresh/remove? [Consistency, Spec §FR-001]
- [ ] CHK014 - Is it specified that non-admins have zero configuration surface and cannot enumerate provider secrets anywhere? [Coverage, Spec §US1 AC4, SC-005]

## Outbound Allowlist (No Open Proxy)

- [ ] CHK015 - Is the outbound target bounded to hosts declared in the provider config, with client-supplied target hosts explicitly forbidden? [Completeness, Spec §FR-008]
- [ ] CHK016 - Are the host-match semantics for `download_hosts` defined (exact host vs. suffix/pattern) so the allowlist is testable? [Ambiguity, data-model.md — flagged A1 in analyze]
- [ ] CHK017 - Is it required that the DSM allowlist (`internal/syno`) is unchanged — no new NAS APIs introduced by this feature? [Consistency, Spec §Constitution Check]
- [ ] CHK018 - Is the new outbound surface required to be off by default and active only after admin configuration? [Completeness, Spec §FR-006]

## Least Privilege on Send (Folder Grants)

- [ ] CHK019 - Is per-user folder-grant validation on the send destination required before any NAS call, reusing the existing authz path? [Completeness, Spec §FR-015, R8]
- [ ] CHK020 - Is the refusal behavior for a destination outside a non-admin's grants specified with a clear, non-leaking error? [Clarity, contracts §send 403]

## Signed Links — Freshness, No Caching

- [ ] CHK021 - Is it required that signed download links are generated at send time and never cached/persisted? [Completeness, Spec §FR-014]
- [ ] CHK022 - Are the failure modes for an expired or wrong-IP link specified (reported, not silently retried; no empty subfolder left as sole trace)? [Coverage, Spec §Edge Cases, contracts §send 502]

## Session/IP Expiry — Graceful, No Leakage

- [ ] CHK023 - Is a single client-facing "needs refreshing" signal defined for all expiry causes (clearance, token, IP), with optional best-effort layer detail? [Consistency, Spec §FR-018/FR-019, contracts §409]
- [ ] CHK024 - Is it required that an IP mismatch surfaces as needs-refresh rather than a hang/timeout that could leak internal detail? [Clarity, Spec §FR-019]
- [ ] CHK025 - Are the distinct clearance-vs-token expiry states required to be distinguishable to the admin without exposing secret values? [Measurability, Spec §Edge Cases]

## Measurability & Traceability

- [ ] CHK026 - Is "zero secrets in client responses and logs" stated as an objectively verifiable criterion (inspection/tests), not a general aspiration? [Measurability, Spec §SC-004]
- [ ] CHK027 - Does the spec include a Credential-Safety Impact answering what is stored/protected, what crosses to provider and NAS, and what could leak? [Completeness, Constitution Principle III]
- [ ] CHK028 - Is the widening of the outbound surface recorded as a justified exception with a simpler-alternative rejection rationale? [Traceability, plan §Complexity Tracking]

## Notes

- This checklist gates the **credential boundary** for spec 0005 (Principle III). Its items validate that the
  *requirements* are complete/clear/consistent/measurable; the corresponding *implementation* verification is
  covered by tasks T003, T006, T007, T011, T022, T024, T036 (redaction audit) and T039 (gates).
- Open ambiguity to resolve in implementation: **CHK016** host-match semantics (analyze finding A1) — pin
  during T007.
