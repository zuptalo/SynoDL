# Credential-Safety / Custodial-State Checklist (Principle III)

**Feature**: 1016 — Tasks view posters + catalog linkage. Triggered because the change adds
persisted columns (stored-data trigger). No credential-boundary or allowlist surface changes.

- [x] CHK001 — New stored fields are non-sensitive: a public catalog poster URL (already rendered by
  Discover) and the catalog title id. No credentials, session id, OTP, or signed download URI stored.
- [x] CHK002 — No secret or signed URL is logged. Poster URL / catalog id are ordinary metadata; the
  existing "never log sids/credentials/full task URIs" rule is unaffected (we add no logging of URIs).
- [x] CHK003 — Data resides in the existing SQLite volume, in the existing `source_downloads` table
  (spec 1013 pattern). No new volume, store, or table.
- [x] CHK004 — No new outbound API call; the DSM/source allowlist is unchanged. Poster/id ride the
  existing `POST /v1/source/send` request (already authenticated to the app), captured from data the
  client already holds.
- [x] CHK005 — Migrations are append-only and default empty; existing rows and older clients keep
  working (backward compatible), no destructive change.
- [x] CHK006 — The poster image is loaded by the client directly from the public catalog CDN (as in
  Discover); the server does not proxy or fetch it, so no new server egress or SSRF surface.
- [x] CHK007 — "Open in Discover" passes the catalog title in-app via shared state (not a URL query),
  so no poster/plot/id data leaks into browser history or the address bar.
