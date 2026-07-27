# Credential-Safety Checklist: spec 0003 (constitution-required)

Required for any change touching Principle III (stored data, secrets, NAS
connection, user auth). Reviewed for Increment 2 (wizard + SynoDL auth); items
for later increments are marked.

## Secrets at rest
- [x] NAS password encrypted at rest (AES-256-GCM under `HKDF(SECRETS_KEY)`) — `store.Cipher`, `operator_config.nas_password_enc`.
- [x] `SECRETS_KEY` sourced from env/k8s Secret only; never written to the DB or logs; `optional:true` in the manifest so absence = legacy mode, not a crash.
- [x] Boot canary: a stored config that won't decrypt fails startup with a clear message — never a silent reset (`main.go`).
- [ ] VAPID private key encrypted at rest — Increment 4.

## Passwords & sessions
- [x] SynoDL passwords stored only as salted PBKDF2-SHA256 hashes; never reversible (`auth.HashPassword`).
- [x] Constant-time password verification (`crypto/subtle`).
- [x] Session tokens: only the SHA-256 hash is stored; the raw token is returned once and lives client-side.
- [x] Sessions carried in a header (`X-SynoDL-Session`), not a cookie ⇒ CSRF-immune.
- [x] Disabling/deleting a user invalidates sessions immediately (`SetUserEnabled` drops sessions; `UserForSession` requires `is_enabled`).
- [x] Login response is uniform on any failure (no user-enumeration / reason leak).

## Least privilege & the NAS
- [x] App users authenticate to SynoDL, never to the NAS; NAS reached only via the one stored connection (`internal/nas`).
- [x] DSM allowlist unchanged — no new `SYNO.*` API added; still `internal/syno` only.
- [x] Admin-only endpoints enforced server-side (`requireAdmin`), not just hidden in the UI (tested: non-admin → 403).
- [x] 2FA session cannot be forged: expiry surfaces `nas_reauth`; only an admin can supply a fresh code.
- [ ] Per-user folder ACLs enforced server-side (picker + task-create validation, traversal-proof) — Increment 3.

## Logs & errors
- [x] No NAS password, OTP, DSM sid, SynoDL password hash, or session token logged — handlers log route + outcome only; secrets never placed in error payloads.
- [x] Test fixtures use obvious non-secret placeholders (GitGuardian-clean).

## Persistence & deploy
- [x] Exactly one datastore: the single SQLite volume; no second store, no state elsewhere.
- [x] Setup runs once; a second attempt is rejected (can't silently overwrite the stored NAS connection); failed setup rolls back rather than leaving a broken config.
- [x] Backup story documented: the `synodl-data` volume + the `SECRETS_KEY` secret (losing the key = unrecoverable stored NAS secrets, by design).

## Notes
- Increment 2 verified via server integration/unit tests (wizard→login→tasks, guards, disabled-user lockout, 2FA re-auth) and client typecheck. e2e for the wizard runs in CI. Items marked `[ ]` are owned by Increments 3–4.
