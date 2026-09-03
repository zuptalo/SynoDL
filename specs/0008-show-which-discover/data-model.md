# Phase 1 — Data Model

**Feature**: 0008 — Show which Discover titles you already have

Two things are worth stating up front, because they bound everything below:

1. **Nothing about the NAS's contents is persisted.** The library snapshot is in-memory only,
   rebuilt on demand (R5). There is no table for it and no migration that carries it.
2. **The only durable change is one boolean per user.**

---

## 1. In-memory entities (server)

### `library.Entry` — one thing observed on the NAS

| Field | Type | Notes |
|---|---|---|
| `Name` | `string` | The folder's name exactly as DSM reported it |
| `Path` | `string` | Share-relative path, e.g. `movie/Dune 2021` |
| `Key` | `string` | Normalised comparison key (see below) |
| `Year` | `string` | Start year parsed off the name; `""` when the name carries none |

### `library.Index` — one snapshot of the configured parents

| Field | Type | Notes |
|---|---|---|
| `byKey` | `map[string][]Entry` | Several entries may share a key (collisions are real — see spec Edge Cases) |
| `BuiltAt` | `time.Time` | Drives the 5-minute TTL (FR-010) |
| `Empty` | `bool` | True when the build failed; makes "we know nothing" explicit rather than indistinguishable from "nothing is there" |

Behaviour:

- `Key(name) (key, year string)` — split the trailing year/range off (the forms enumerated by
  `src/services/title-year.ts`), drop a leading article and bracketed extras, lowercase, then
  keep only `unicode.IsLetter` / `unicode.IsDigit` runes. **Never an ASCII filter** — that would
  collapse every Persian title to the empty string (R2).
- `Lookup(catalogTitle, mediaType) (Entry, bool)` — key equality, then the year-agreement rule:
  if both sides carry a year the start years must match; if either lacks one, the key match
  stands (FR-005).
- An index built from a failed read is `Empty` and matches nothing (FR-009).

### `library.SeasonPresence` — what is inside one title folder

| Field | Type | Notes |
|---|---|---|
| `Season` | `int` | Season number present on disk |
| `Files` | `int` | Episode files counted; `0` means "not counted", never "empty" (FR-016) |

Derived from a single listing of the title folder, covering both layouts (R3):
- **Nested** — subdirectories named `Season 1`, `S01`, `Series 1`.
- **Flat** — episode files carrying `S01E05`, parsed with the same non-alphanumeric-bounded form
  used by `src/services/task-title.ts` and `server/internal/tasktitle/tasktitle.go`.

### Snapshot cache (`api/library.go`)

Held on `Deps` behind a `sync.Mutex`: the current `*library.Index` plus its build time. Read
path: reuse if younger than 5 minutes, otherwise rebuild. Invalidated outright after a successful
`handleSourceSend` (FR-008). Process-local — it does not survive a restart, and does not need to.

---

## 2. Wire shapes

### `source.CatalogTitle` — one added field

```go
// InLibrary reports that a folder for this title already exists under the
// configured parent. Set by the API layer from the library snapshot; drivers
// never populate it — the same arrangement as SourceID/SourceName.
InLibrary bool `json:"inLibrary,omitempty"`
```

Mirrored on the client `CatalogTitle` interface in `src/services/api.ts`. `omitempty` keeps the
payload unchanged for titles that are not present, which is most of them.

### `GET /v1/library/title` response

See [contracts/library-api.md](./contracts/library-api.md). Summary shape:

```json
{ "inLibrary": true,
  "seasons": [ { "season": 1, "files": 24 }, { "season": 2, "files": 0 } ] }
```

**No folder path is returned** (FR-025): the client needs to know *whether* and *which seasons*,
never *where*. Returning the path would widen the exposure for no functional gain.

### `/v1/source/prefs` — one added field

`hideOwned: boolean`, alongside the existing preferred-quality and view state.

---

## 3. New `syno` client surface

```go
// Entry is one FileStation item — a directory or a file.
type Entry struct {
    Name  string `json:"name"`
    Path  string `json:"path"`
    IsDir bool   `json:"isDir"`
}

// ListEntries lists BOTH directories and files under an absolute path, using the
// already-allowlisted SYNO.FileStation.List / "list" with no filetype filter
// (DSM defaults to all). One round-trip serves both season layouts.
ListEntries(ctx context.Context, sid, path string) ([]Entry, error)
```

Added to the `syno.Client` interface, the `HTTPClient`, and the fake in
`server/internal/api/fake_test.go`. `ListFolder` (`filetype=dir`) is unchanged and still serves
the destination picker and the parent scan.

---

## 4. The one migration

Appended to `migrations` in `server/internal/store/schema.go`. The array currently holds **21
entries**, so this becomes entry 22 (`schema_migrations.version = 22`); the `// 00NN` comment
numbers are logical groups and already run ahead of the indices, so this is group **0019**.

```go
// 0019 — remember whether a user hides titles they already have from Discover
// (spec 0008). 0 = show everything, which is exactly today's behaviour, so every
// existing row and every new account already means the right thing.
`ALTER TABLE source_prefs ADD COLUMN hide_owned INTEGER NOT NULL DEFAULT 0;`,
```

Additive, defaulted, and on the table that already holds the rest of the per-user Discover view
state (R7). Never edit a shipped migration — this is appended.

`store_test.go:TestOpenRunsMigrations` asserts `SchemaVersion() == len(migrations)`, so it keeps
passing without change; no new table means nothing to add to its table-existence list.

**Accessors**: extended through the existing `GetSourceViewFull` / `SaveSourceViewFull` pair
(`server/internal/store/source_repos.go:336, 353`) rather than a new pair, keeping one read and
one upsert for the whole Discover view.

---

## 5. What deliberately has no data model

- **No IndexedDB change.** `src/db/idb.ts` stays at `DB_VERSION = 1`; the hide-owned flag is
  server-side so it follows the user across devices (FR-024). Principle IV's bump requirement is
  therefore not triggered.
- **No table of library contents.** See R5 — the NAS is the source of truth.
- **No per-user snapshot.** The index is instance-wide; the reasoning and its least-privilege
  tension are tabled in [plan.md](./plan.md) under Complexity Tracking.
