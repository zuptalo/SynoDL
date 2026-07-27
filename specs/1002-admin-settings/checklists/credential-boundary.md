# Checklist: Credential Boundary — Admin NAS-config editing (spec 1002)

**Purpose**: Confirm the view/edit/test-connection endpoints preserve the custodial credential-safety
invariant (constitution Principle III) before shipping.

## Secrets never leave the server

- [x] `GET /v1/nas/config` returns the non-secret projection only — no `nasPassword`, no
      `nas_password_enc`, no sid. (Tested: `TestGetNASConfigNeverLeaksPassword`.)
- [x] Edit accepts a blank password meaning "keep stored"; the stored secret is never echoed back to set
      it. (Tested: `TestUpdateNASConfigPublicURLOnly`, blank-password re-test in `TestTestNASConnection`.)
- [x] `POST /v1/nas/test` persists nothing (login → logout → discard sid).
- [x] No credentials, OTP, sid, or full task URIs are logged by any new handler.

## Authorization

- [x] All three endpoints (`GET`/`PUT /v1/nas/config`, `POST /v1/nas/test`) are `requireAdmin` —
      403 for a non-admin, 401 unauthenticated. (Tested: `TestNASConfigEndpointsRequireAdmin`.)

## Proxy boundary intact

- [x] No new DSM API added to the allowlist; test/verify reuse the existing `Login`/`Logout`.
- [x] A rejected connection edit rolls back to the previous working config (no stranded instance).
      (Tested: `TestUpdateNASConfigBadCredentialsRollsBack`.)
- [x] `SYNO_URL` is not required in stateful mode; the NAS comes from the encrypted store.

## Encryption at rest

- [x] The NAS password continues to live only in the encrypted `nas_password_enc` column
      (`SaveOperatorConfig` seals it); no plaintext at rest, no new unencrypted secret columns.
- [x] `SECRETS_KEY` handling is unchanged (no rotation/re-key introduced).
