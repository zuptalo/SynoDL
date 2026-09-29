# API contract: YouTube sign-in

All bodies JSON; errors `{ "error": "<code>", "message": "<human text, never cookie text>" }`.

## `GET /v1/youtube/signin` — admin
`200 { "available": bool, "saved": bool, "cookieCount": n, "loginCookies": ["SAPISID",…], "missingLogin": ["LOGIN_INFO",…], "savedAt": unix|null, "savedBy": "name"|"", "lastRefusedAt": unix|null, "lastOkAt": unix|null }`
`available` is false in the stateless build (the route answers `404`/absent there). Non-admin → `403`. Never any cookie value.

## `PUT /v1/youtube/signin` — admin
Body `{ "text": "<Cookie header | cookies file text>" }` (≤ 64 KiB).
`200` the same object as GET plus `"warning": "no_login_cookies"|null`.
`400 { "error": "too_few_cookies" | "unrecognised" | "too_large" | "invalid" }` — the message states counts only.

## `DELETE /v1/youtube/signin` — admin
`204`. Removes the row; workers created afterwards run anonymously.

## `GET /v1/internal/ytdl-signin` — grant-authenticated (workers only)
Header `Authorization: Bearer <grant>`. Not behind the session gate, not CORS-enabled.
`200 text/plain` Netscape cookies, `Cache-Control: no-store`.
Any failure — unknown/used/expired grant, address ≠ pod IP, no such pod, `X-Forwarded-For` present, no sign-in saved — answers the same `404` with an empty body (no oracle). A grant is consumed only by a `200`.
