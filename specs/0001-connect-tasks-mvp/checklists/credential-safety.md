# Credential-Safety Checklist: 0001-connect-tasks-mvp

**Purpose**: Principle III gate — this spec touches DSM credentials, OTP codes,
source credentials, and the session id. Completed during T018.

- [x] CHK001 Login request body (account/password/otp) is never logged by
      client or server; httpx.Log emits method/path/status/duration only
      [Spec §Credential-Safety Impact; server/internal/httpx/middleware.go]
- [x] CHK002 The sid is transported only via the `X-Syno-Sid` header and
      persisted only in the client's IndexedDB `settings` store [FR-002]
- [x] CHK003 The server keeps no session, credential, or task state — no new
      server-side storage introduced by this spec [Constitution III]
- [x] CHK004 New-task source credentials + unzip password are request-scoped:
      forwarded to the NAS, never written to idb, never in error payloads
      [FR-014]
- [x] CHK005 Task URIs never appear in server logs or typed errors
      [server/internal/syno/errors.go doc contract]
- [x] CHK006 No DSM API beyond the existing allowlist (Auth, Task, Statistic,
      FileStation.List) is called [server/internal/syno/http.go]
- [x] CHK007 Login rate limiting per client IP is active on `POST /v1/session`
      [FR-005; server/internal/api/router.go]
- [x] CHK008 Logout clears local state even when the NAS is unreachable
      [US1 scenario 5; useSession.logout]
- [x] CHK009 The expired-session path clears the sid exactly once and cannot
      loop (no re-login storms from concurrent 401s) [Edge cases]
