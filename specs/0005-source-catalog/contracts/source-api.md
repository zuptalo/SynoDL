# Phase 1 Contracts: Source-catalog API (`/v1/source/*`)

All routes are registered under the existing stateful block (`if d.Stateful`) in `router.go`. Auth wrappers
reuse `requireUser` / `requireAdmin`. **No provider secret is ever present in any response.** Errors use the
existing typed JSON error shape; session/clearance failures map to a dedicated `needs_refresh` signal so the
client can render the refresh state (not a generic 500).

## Status & admin config

### `GET /v1/source/status` — (requireUser)
Returns non-secret provider status so the Browser tab and Settings can render.
```json
{ "configured": true, "enabled": true, "state": "active",
  "providerName": "30nama", "lastVerifiedAt": 1785312000,
  "canManage": false }
```
- `state`: `not_configured` | `active` | `needs_refresh`.
- `canManage`: true only for admins (drives the config affordance).

### `PUT /v1/source/session` — (requireAdmin)
Submit/refresh session material. Server verifies with a sample call **before** storing.
```json
// request
{ "kind": "30nama", "displayName": "30nama",
  "moviesParent": "/movies", "tvParent": "/tv",
  "session": { "cfClearance": "…", "cApiKey": "…", "cToken": "…",
               "userAgent": "…", "cPlatform": "…", "cAppVersion": "…" } }
// 200
{ "state": "active", "lastVerifiedAt": 1785312000 }
// 4xx (nothing stored) — no secret echoed
{ "error": "provider_verify_failed", "reason": "challenge" | "invalid_token" | "ip_mismatch" }
```

### `DELETE /v1/source/session` — (requireAdmin)
Removes the provider config + secrets → `state: not_configured`.

## Catalog (requireUser)

### `POST /v1/source/search`
```json
// request
{ "query": "matrix", "page": 1,
  "filters": { "type": "movie", "quality": "4K", "genre": ["3357"],
               "language": "en", "country": "US" } }
// 200
{ "page": 1, "pages": 12,
  "items": [ { "id": "217561", "type": "movie", "title": "Soul 2020",
               "posterUrl": "https://…/cover.jpg", "imdbId": "tt2948372",
               "imdbScore": 8.0, "providerScore": 8.8,
               "flags": { "comingSoon": false, "freeDownload": false } } ] }
// 409 when session expired
{ "error": "source_needs_refresh", "layer": "clearance" | "token" | "ip" }
```

### `GET /v1/source/title/{id}` — (requireUser)
Movie detail + available qualities (v1: movies; series/anime return `sendable:false`).
```json
{ "id": "217561", "type": "movie", "title": "Soul 2020", "sendable": true,
  "qualities": [ { "id": "q_512085", "label": "x265 BluRay REMUX 2160p",
                   "size": "37.55 GB", "resolution": "3840x2160",
                   "encoder": "AR644", "hardsub": false } ] }
```
- The signed download URL is **not** included here; it is resolved server-side at send time (R4/FR-014).

### `POST /v1/source/send` — (requireUser)
Resolve the chosen quality to a signed link and create the task.
```json
// request
{ "titleId": "217561", "qualityId": "q_512085" }
// 200
{ "destination": "/movies/Soul 2020", "created": true, "taskAdded": true }
// 403 — destination outside caller's folder grants
{ "error": "destination_forbidden" }
// 409 — session expired
{ "error": "source_needs_refresh", "layer": "…" }
// 502 — provider/link/NAS failure (no empty folder left as sole trace)
{ "error": "send_failed", "reason": "link_expired" | "download_host_unreachable" | "nas_error" }
```
Server steps: resolve signed link (movies) → validate `dest = <moviesParent>/<sanitized title>` via
`authz.AllowedForCreate` → `CreateFolder` (reuse if exists) → `CreateTaskURIs([link], {destination})`.

## Preferences (requireUser)

### `GET /v1/source/prefs` → `{ "preferredQuality": "1080p" }`
### `PUT /v1/source/prefs` — `{ "preferredQuality": "1080p" }` → normalized echo.

## Cross-cutting contract rules
- Every handler: secrets never serialized; logs carry route + outcome only (redaction test required).
- Outbound provider calls restricted to the configured `api_hosts` / `download_hosts`.
- `source_needs_refresh` (HTTP 409) is the single client-facing expiry signal; `layer` is best-effort detail.
- All routes are absent (404) in legacy stateless mode (no `SECRETS_KEY`); the client degrades the Browser tab
  to the existing "unavailable" state.
