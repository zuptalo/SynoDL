# Credential-Safety Checklist: 0001-connect-tasks-mvp

**Purpose**: Principle III gate — this spec touches DSM credentials, OTP codes,
source credentials, and the session id. Completed during T018.

- [ ] CHK001 Login request body (account/password/otp) is never logged by
      client or server; httpx.Log emits method/path/status/duration only
      [Spec §Credential-Safety Impact; server/internal/httpx/middleware.go]
- [ ] CHK002 The sid is transported only via the `X-Syno-Sid` header and
      persisted only in the client's IndexedDB `settings` store [FR-002]
- [ ] CHK003 The server keeps no session, credential, or task state — no new
      server-side storage introduced by this spec [Constitution III]
- [ ] CHK004 New-task source credentials + unzip password are request-scoped:
      forwarded to the NAS, never written to idb, never in error payloads
      [FR-014]
- [ ] CHK005 Task URIs never appear in server logs or typed errors
      [server/internal/syno/errors.go doc contract]
- [ ] CHK006 No DSM API beyond the existing allowlist (Auth, Task, Statistic,
      FileStation.List) is called [server/internal/syno/http.go]
- [ ] CHK007 Login rate limiting per client IP is active on `POST /v1/session`
      [FR-005; server/internal/api/router.go]
- [ ] CHK008 Logout clears local state even when the NAS is unreachable
      [US1 scenario 5; useSession.logout]
- [ ] CHK009 The expired-session path clears the sid exactly once and cannot
      loop (no re-login storms from concurrent 401s) [Edge cases]
