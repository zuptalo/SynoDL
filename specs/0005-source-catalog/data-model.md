# Phase 1 Data Model: Download-source catalog

All persistent state lives in the single existing SQLite volume via **one append-only migration (0005)**.
Catalog listings/titles/qualities are **fetched live and never persisted** (like download tasks).

## New tables (migration 0005)

### `source_providers` — non-secret provider config + status
| Column | Type | Notes |
|---|---|---|
| `id` | INTEGER PK | v1 typically a single row |
| `kind` | TEXT NOT NULL | driver key (e.g. `thirtynama`); selects the `internal/source` provider |
| `display_name` | TEXT NOT NULL | shown in admin UI |
| `api_hosts` | TEXT NOT NULL | newline/JSON list of allowed API hosts (outbound allowlist) |
| `download_hosts` | TEXT NOT NULL | allowed signed-download hosts (may be a suffix/pattern set) |
| `movies_parent` | TEXT NOT NULL DEFAULT '' | admin-designated parent folder for movie subfolders |
| `tv_parent` | TEXT NOT NULL DEFAULT '' | parent for series/anime (provisioned for the fast-follow) |
| `enabled` | INTEGER NOT NULL DEFAULT 0 | off by default |
| `state` | TEXT NOT NULL DEFAULT 'not_configured' | `not_configured` / `active` / `needs_refresh` |
| `last_verified_at` | INTEGER NOT NULL DEFAULT 0 | epoch of last successful sample call |
| `created_at` | INTEGER NOT NULL DEFAULT 0 | |
| `updated_at` | INTEGER NOT NULL DEFAULT 0 | |

*No secret values here — safe to return to admins as status.*

### `source_provider_secrets` — encrypted, write-only session material
| Column | Type | Notes |
|---|---|---|
| `provider_id` | INTEGER PK REFERENCES source_providers(id) ON DELETE CASCADE | |
| `session_enc` | BLOB NOT NULL | `Cipher.Seal(json)` of `{cf_clearance, c_api_key, c_token, user_agent, c_platform, c_app_version, extra...}` |
| `updated_at` | INTEGER NOT NULL DEFAULT 0 | |

**Never returned to any client.** Decrypted only in-process to build outbound requests.

### `source_prefs` — per-user preferred quality
| Column | Type | Notes |
|---|---|---|
| `user_id` | INTEGER PK REFERENCES users(id) ON DELETE CASCADE | |
| `preferred_quality` | TEXT NOT NULL DEFAULT '' | matched against a title's offered quality labels |
| `updated_at` | INTEGER NOT NULL DEFAULT 0 | |

## Transient (in-memory, not persisted) entities

- **CatalogTitle**: `id`, `type` (movie/series/anime), `title`, `posterUrl`, `imdbId`, `imdbScore`,
  `providerScore`, `flags` (freeDownload, comingSoon…), facets. From search results.
- **QualityOption**: `id` (opaque, provider-scoped), `label`, `size`, `resolution`, `encoder?`, `hardsub?`.
  From the title-download response. The signed URL it resolves to is fetched at send time and never stored.

## State transitions — `source_providers.state`

```
not_configured ──(admin PUT session, verify OK)──▶ active
active ──(provider call → challenge / token-unauth / IP-mismatch)──▶ needs_refresh
needs_refresh ──(admin PUT fresh session, verify OK)──▶ active
(any) ──(admin DELETE)──▶ not_configured
```

## Validation rules

- Admin-only writes to `source_providers` / `source_provider_secrets` (FR-001).
- `PUT session` MUST perform a sample provider call and only persist on success (FR-004); on failure store
  nothing and keep prior state.
- Reads of provider config MUST project **only** non-secret columns (FR-002/005).
- `send` destination = `<movies_parent>/<sanitized title>`; MUST pass `authz.AllowedForCreate(isAdmin,
  grants, dest)` before any NAS call (FR-015); reuse existing subfolder if present (FR-016).
- Outbound requests MUST target only `api_hosts` / `download_hosts` (FR-008).
- `preferred_quality` is advisory: auto-select when a title offers an exact/normalized match, else manual pick.

## Client-side (IndexedDB)

- No new object store required for v1 (server is source of truth). If a preferred-quality cache is added for
  snappy UI, it MUST bump `DB_VERSION` with a forward migration (Principle IV) and treat the server value as
  authoritative.
