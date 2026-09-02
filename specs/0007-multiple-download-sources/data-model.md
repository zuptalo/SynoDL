# Data Model: Multiple Download Sources

**Feature**: 0007-multiple-download-sources | **Date**: 2026-09-03

Migrations are an ordered list applied by count (`server/internal/store/schema.go`), so this
feature appends new entries and never edits an existing one.

---

## Stored entities

### `source_providers` (existing table, new columns)

Already keyed by `id AUTOINCREMENT` — the table was always shaped for many rows; only the
accessors assumed one. Two columns are added:

| Column | Type | Default | Why |
|---|---|---|---|
| `sort_order` | INTEGER | `0` | Stable, operator-controlled ordering for the source dropdown and for round-robin interleaving, so the combined list does not reshuffle between requests |
| `last_error` | TEXT | `''` | Last failure *category* for admin display (`needs_refresh`, `unsubscribed`, `unreachable`). A category only — never an upstream body, URL, or secret |

`state` gains one value: `unsubscribed` — session is valid but the account cannot download
(FR-019). Existing values (`not_configured`, `active`, `needs_refresh`) are unchanged.

**Migration**:

```sql
ALTER TABLE source_providers ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;
ALTER TABLE source_providers ADD COLUMN last_error  TEXT    NOT NULL DEFAULT '';
```

An existing single row keeps its id and gets `sort_order = 0`, becoming the first list entry
with no data change and no re-pasting of session material (FR-004).

### `source_provider_secrets` (existing table, unchanged shape)

Already keyed `provider_id … REFERENCES source_providers(id) ON DELETE CASCADE`, so multiple
sources need no schema change and deleting a source already destroys its sealed session.

What changes is the **sealed payload's shape**: from the fixed 30nama-shaped struct to a
declared key/value bag (R5). Old blobs are JSON objects with the current field names, so
reading them into the bag by key is lossless and no operator re-pastes.

```
old: {"cf_clearance":"…","c_api_key":"…","c_token":"…","user_agent":"…", …}
new: {"fields":{"cf_clearance":"…","c_api_key":"…","c_token":"…"},"user_agent":"…"}
```

Still sealed with the at-rest cipher, still write-only, still never serialized to any client.

### `source_prefs` (existing table, new column)

| Column | Type | Default | Why |
|---|---|---|---|
| `selected_source` | TEXT | `''` | The user's Discover source selection. `''` means "All sources" — so every existing row already means the right thing, and the default is correct for new users too (FR-006, FR-008) |

```sql
ALTER TABLE source_prefs ADD COLUMN selected_source TEXT NOT NULL DEFAULT '';
```

Stored as text, not an integer FK, so a removed source degrades to "All sources" rather than
breaking a constraint (edge case: source removed while a user has it selected). The value is
validated against configured providers on read; an unknown value is treated as `''`.

`preferred_quality`, `filters`, `sort`, `sort_order` are untouched and stay global across
sources (per the clarification session).

---

## In-memory / wire entities

### `source.Session` (reshaped)

```
Session {
  Fields    map[string]string   // provider-declared keys; the driver names them
  UserAgent string
}
```

Replaces the five 30nama-specific fields. `Client.Do` no longer knows any provider's header
names; each driver builds its own headers and cookies. This is both the factoring that lets
zarfilm use a cookie and a containment measure — a driver can no longer see another driver's
secrets.

### `source.SessionField` (new)

What a driver declares so the admin UI can render the right form and label it honestly:

```
SessionField { Key, Label, Help string; Secret, Required bool }
```

Added to the `Provider` interface as `SessionFields() []SessionField`. For zarfilm this is
where the elevated-sensitivity warning from the Credential-Safety Impact section is surfaced
at the point of paste.

### `source.CatalogTitle` (new field)

```
SourceID   int64   // which configured source produced this
SourceName string  // its display name, for the combined-mode label (FR-012a)
```

### `source.SearchResult` (new field)

```
Degraded []DegradedSource   // sources that failed this query; never fails the whole request
DegradedSource { SourceID int64; Name string; Reason string }
```

`Reason` is a category (`needs_refresh`, `unsubscribed`, `unreachable`, `timeout`) — never an
upstream message.

### Title identifier (wire format)

`<providerID>:<providerTitleID>` — split on the **first** colon only, because zarfilm's ids are
URL paths that contain slashes (`series/the-loyalty-game`) though never a colon. The provider
portion is validated against the caller's configured providers on every request.

---

## Entity relationships

```
operator_config ─┐
                 │
users ───────────┼──< source_prefs (1:1, selected_source → a source or "all")
                 │
source_providers ─< source_provider_secrets  (1:1, cascade delete)
       │
       └──< (runtime only) CatalogTitle / QualityOption — never persisted;
             the source remains the source of truth (constitution III)
```

Download tasks and catalog contents are still never persisted. `source_downloads` (title
metadata keyed by destination folder) is unchanged — it records what was sent, not a catalog.

---

## Validation rules

- A source's `kind` must resolve to a registered driver; an unknown kind renders the source
  disabled and reports it, rather than failing startup.
- Two sources may share a `kind` (FR-001) — required for testing US1 without a second driver.
- `sort_order` need not be unique; ties break by `id` so ordering is always total and stable.
- Deleting a source cascades its secret and must clear it from any user's `selected_source`
  on read rather than by rewriting rows.
- Every outbound host is checked against the *driver-declared* allowlist for the source being
  called — never a union across sources, or one source's compromise would widen another's
  reach.

---

## State transitions

```
not_configured ──paste session──> (verify)
                                    ├─ ok ──────────────> active
                                    ├─ logged out ──────> needs_refresh
                                    ├─ no subscription ─> unsubscribed
                                    └─ unreachable ─────> needs_refresh (+ last_error)

active ──runtime 401 / logged-out page──> needs_refresh
active ──title page shows paywall rows──> unsubscribed
unsubscribed ──subscription renewed, next verify──> active
any ──admin disables──> (enabled=0, state preserved)
any ──admin deletes──> row + sealed secret destroyed
```

`unsubscribed` is deliberately distinct from `needs_refresh`: telling an operator to re-paste a
session that is working perfectly would send them in circles (FR-019).
