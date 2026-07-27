# Feature Specification: Recognize every DSM 7 sign-in failure with its own message

**Feature Branch**: `claude/synology-nas-pwa-client-s8x3m0`

**Created**: 2026-07-27

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: User request: "Make sure the mock is implemented based on the DSM 7.3.2-86009 API specifications" + the official DSM Login Web API Guide (SYNO.API.Auth error table).

## Context: why this ad-hoc spec exists

Spec 0001 mapped the common `SYNO.API.Auth` failures (400 wrong credentials,
402 permission, 403 OTP required, 404 OTP failed). The official DSM Login Web
API Guide documents more codes the app currently lumps into wrong buckets:
**401** Disabled account (shown today as "not allowed to use Download
Station"), **406** Enforce 2FA (would fall to the generic NAS error), **407**
Blocked IP source, and **408/409/410** the expired-password family (all three
would read as a generic error even though the user's fix — change the password
in DSM — is completely different from "try again"). Aligning these keeps the
promise of spec 0001 SC-004: every failure mode reads distinctly and
actionably.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Understand exactly why sign-in failed (Priority: P1)

As a user whose sign-in fails for a DSM-side account reason (account disabled,
2FA enforcement, blocked IP, expired password), I see a message naming that
reason and what to do about it — not a generic error.

**Why this priority**: The only story; each of these states has a different
remedy in DSM, and a wrong message sends the user to reset a password that
isn't the problem.

**Independent Test**: Unit-level — every documented auth code maps to its own
kind server-side and its own message client-side; mock accounts reproduce each
state for manual dev verification.

**Acceptance Scenarios**:

1. **Given** DSM answers 401 (disabled account), **Then** the app says the
   account is disabled — distinct from the permission message.
2. **Given** DSM answers 406 (2FA enforced), **Then** the app asks for a
   verification code exactly like 403 does.
3. **Given** DSM answers 407 (blocked IP), **Then** the app says DSM has
   blocked this device's address.
4. **Given** DSM answers 408, 409, or 410 (expired-password family), **Then**
   the app says the password expired and must be changed in DSM.

### Edge Cases

- Codes must classify identically regardless of DSM version (the table is
  stable across DSM 6/7 per the guide).
- An undocumented future code still falls back to the generic NAS error.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The server MUST classify `SYNO.API.Auth` codes per the DSM Login
  Web API Guide: 400→credentials, 401→account_disabled, 402→permission,
  403→otp_required, 404→otp_invalid, 406→otp_required, 407→ip_blocked,
  408/409/410→password_expired.
- **FR-002**: The client MUST render a distinct plain-language message for
  `account_disabled`, `ip_blocked`, and `password_expired`.
- **FR-003**: The 406 path MUST behave exactly like 403 in the login flow (the
  code field appears).
- **FR-004**: The mock DSM MUST offer accounts reproducing 401, 407, and 409
  so every state is manually testable without real hardware.

## Credential-Safety Impact *(constitution-required)*

No new data crosses the proxy; this only refines the classification of
existing error codes. Typed errors still carry kind + code only — never
parameters. Nothing is stored anywhere.

## Success Criteria *(mandatory)*

- **SC-001**: 100% of the guide's documented auth error codes map to a
  non-generic, distinct user message (verified by unit tests on both sides).

## Assumptions

- Codes 405 (app portal) and OTP-device params are out of scope — the app uses
  plain sid sessions.
- Message copy directs users to DSM for remediation; deep links into DSM are
  out of scope.

## Clarifications

### Session 2026-07-27

- Q: Distinct kind for 406 vs 403? → A: No — same user action (enter/enroll a
  code); one kind keeps the client switch small. Documented in classify().
