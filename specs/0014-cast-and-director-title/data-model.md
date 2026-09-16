# Phase 1 Data Model: Who made it — cast and director on a title

## 1. Wire / domain types (`server/internal/source`)

### `Person`

One name on a tile. Everything but `name` is optional, because every source is
missing something.

| Field | JSON | Notes |
|---|---|---|
| `Name` | `name` | As the source publishes it. Never normalised, never transliterated. |
| `Character` | `character,omitempty` | Only 30nama publishes this (`as`). |
| `IMDbID` | `imdbId,omitempty` | `nm…`. The tile links out on this, and the photo endpoint is keyed by it. |
| `PhotoURL` | `photoUrl,omitempty` | A **source-hosted** photo, already known to be real (not a stand-in). Empty means "ask the photo endpoint, or show initials". The client proxies it through the existing `/v1/source/image`. |
| `Ref` | `-` | The source's own handle (`actor/kurt-russell`). **Never serialised** — a `json:"-"` tag, load-bearing the same way `QualityOption.ReleaseName`'s is: it is an internal join key, and a wire tag enforces that permanently rather than relying on every future handler to remember. |

### `TitleDetail` additions

| Field | JSON | Notes |
|---|---|---|
| `Cast` | `cast,omitempty` | `[]Person`, in the source's billing order (FR-002). |
| `Directors` | `directors,omitempty` | `[]Person` |
| `Creators` | `creators,omitempty` | `[]Person` |
| `Writers` | `writers,omitempty` | `[]Person` |
| `Year` | `year,omitempty` | `string`, same reasoning as `CatalogTitle.Year` (a series carries a range). Populated by FR-036. |

`omitempty` throughout: a title with no people serialises byte-identically to
today's, and the client's "render nothing" (FR-005) is the absence of the field
rather than an empty array it has to test for.

**Caps.** At most 20 cast and 10 per crew role, applied by the driver (FR-007).
A normal billed cast is 3–6.

### `PersonResolver` (optional driver capability)

```go
// Implemented by a driver whose title pages NAME people without identifying
// them. Called only for a Person that arrived with a Ref and no IMDbID.
type PersonResolver interface {
    ResolvePerson(ctx context.Context, c *Client, cfg Config, s Session, ref string) (Person, error)
}
```

Type-asserted at the call site, so 30nama (which identifies people inline) is
unaffected and implements nothing.

---

## 2. Persistence (`server/internal/store`, migration 39)

Two tables. Both hold **derived public facts**: no secret, no user id, no title
id, nothing about who looked at what (FR-030). Neither is encrypted, and neither
may ever hold anything that would deserve to be.

```sql
-- 0039 — remembered faces (spec 0014)
CREATE TABLE IF NOT EXISTS person_photos (
    imdb_id    TEXT PRIMARY KEY,           -- "nm0000621"
    photo_url  TEXT NOT NULL DEFAULT '',   -- '' == IMDb has no photograph of them
    checked_at INTEGER NOT NULL DEFAULT 0  -- unix seconds
);
CREATE INDEX IF NOT EXISTS idx_person_photos_checked ON person_photos (checked_at);

CREATE TABLE IF NOT EXISTS source_people (
    source_kind TEXT NOT NULL,             -- "zarfilm"
    ref         TEXT NOT NULL,             -- "actor/kurt-russell"
    imdb_id     TEXT NOT NULL DEFAULT '',
    photo_url   TEXT NOT NULL DEFAULT '',  -- source-hosted portrait, '' when none
    name        TEXT NOT NULL DEFAULT '',
    checked_at  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (source_kind, ref)
);
CREATE INDEX IF NOT EXISTS idx_source_people_checked ON source_people (checked_at);
```

`IF NOT EXISTS` throughout, because the spec 1031 drift repair rewinds
`schema_migrations` and replays — a migration that cannot run twice turns that
repair into a boot failure.

### Lifetimes

| Row | Fresh for | Why |
|---|---|---|
| `person_photos` with a URL | 30 days | Publicity stills change rarely; this is the row that saves the lookups. |
| `person_photos` with `''` | 3 days | A person IMDb has no photo of today may have one next week, and a *block* also lands here — so a blocked instance recovers on its own without re-scraping every view. |
| `source_people` | 30 days | A person's identity does not change; only their portrait might. |

Expiry is read-time (`checked_at + ttl < now` ⇒ treat as a miss and re-resolve),
so no sweeper job exists.

### Bounding (FR-029)

On write, every 256th insert: delete rows past their TTL, then delete the oldest
beyond 20 000 rows per table. Both tables are pure cache — a delete costs a
lookup, never data.

---

## 3. In-memory layer (`server/internal/people`)

```
lookup(imdbID) ─▶ LRU (4096 entries, ~10 min) ─▶ SQLite ─▶ single-flight ─▶ IMDb
```

- **LRU** absorbs the "one sheet asks for six faces, three of them the same
  person as the last sheet" case without touching SQLite.
- **Single-flight**: a `map[string]chan struct{}` of in-flight ids. Concurrent
  requests for the same unresolved person wait on one lookup (FR-028). Hand-rolled
  rather than `golang.org/x/sync` — see research R5.
- **Concurrency cap**: a buffered channel of 4 tokens gates every outbound IMDb
  lookup instance-wide (FR-021).
- **Byte cache**: the served image bytes go in a dedicated 32 MB `imageCache` (the
  existing LRU type), separate from `posterCache` so a busy Discover grid cannot
  evict every face and vice versa.

---

## 4. Client types (`src/services/source.ts` / component props)

```ts
export interface Person {
  name: string;
  character?: string;
  imdbId?: string;
  photoUrl?: string;
}
```

`TitleDetail` gains `cast?`, `directors?`, `creators?`, `writers?`, `year?`.

Two pure helpers, both under the vitest coverage floor (Principle II):

- `imdbPersonUrl(id)` in `src/services/imdb-link.ts` — strict `^nm\d+$`, `''`
  otherwise, exactly like the existing `imdbUrl()`. It is provider data being
  interpolated into an `href`.
- `initials(name)` in `src/services/person.ts` — up to two leading letters of the
  name as published, working on non-Latin scripts, `''` for a name with no
  letters (the tile then shows a neutral glyph rather than an empty circle).

Image source resolution, in order, per tile:

1. `photoUrl` → `/v1/source/image?u=…` (existing proxy, source-hosted).
2. else `imdbId` → `/v1/source/person/{imdbId}/photo` (new endpoint).
3. else, or on the `<img>`'s `error` event → initials tile (FR-031, FR-032).
