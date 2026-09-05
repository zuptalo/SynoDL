# Implementation Plan: Keep knowing what is on the NAS

**Spec**: [spec.md](./spec.md)
**Branch**: `feat/0011-keep-knowing-what-nas`

## Technical Context

The reading being persisted already exists, fully modelled, in
`server/internal/api/library.go`:

- **Layer one — the name index.** `library.Build(parents, namesByParent, at)`
  builds an immutable `*library.Index` from nothing but the configured parents
  and the folder names read under each. That is the entire input, which is what
  makes it persistable without inventing a second representation: store those
  two things, rebuild the identical index from them.
- **Layer two — folder evidence.** `evidenceRec` per title folder: `hasVideo`,
  `seasons []source.SeasonPresence`, `releases map[int][]library.Release`,
  `keys map[int][]string`, `checkedAt`.

Both live in `libraryCache`, a mutex-guarded struct hung off `Deps`, and both are
dropped on restart and replaced with an empty value on any NAS failure. The
feature is entirely about where those two things live and who refreshes them.

## Constitution Check

| Principle | Status |
|---|---|
| I — Ionic/PWA first | N/A, no client change |
| II — TDD | Tests before code, per task list |
| III — Credential boundary | See spec's Credential-Safety Impact. No new DSM API; `SYNO.FileStation.List` already allowlisted for both folder and file listings. No secret is involved. New: NAS-derived facts at rest — bounded by FR-011 and removed by FR-009 |
| IV — Single store | Two new tables in the existing SQLite store; no new volume, no second store |
| V — Release-note subjects | `feat(discover): ...` user-facing copy |
| VI — Dependencies are spec decisions | No new dependency |

Migrations are **appended** to `internal/store/schema.go` and pinned by
`TestMigrationsAreAppendOnly` — the rule that spec 2012 exists to enforce.

## Phase 0 — Research

No unknowns. Three decisions, recorded here rather than in a separate file:

**Persist the index inputs, not the index.** `library.Build` is pure and cheap,
so storing `parents` + `namesByParent` and rebuilding is strictly better than
serialising a derived structure: no format to migrate when matching changes, and
the matching rules stay in one place.

**Stale-while-revalidate, not read-through.** A stored reading is served
immediately and a refresh is kicked off behind it. Read-through would put a NAS
round-trip back on the request path for every folder whose memory entry expired,
which is the cost this feature exists to remove.

**Bounded, oldest-first scan.** A library of several hundred titles cannot be
re-read in one cycle without flooding the NAS. Each cycle refreshes the parent
listings (cheap: one call per parent) and at most `scanFolderBudget` title
folders, least-recently-read first. Convergence is eventual and that is stated
in the spec.

## Phase 1 — Design

### Schema (appended)

```sql
CREATE TABLE library_folders (
  parent     TEXT    NOT NULL,          -- configured parent path, no leading '/'
  name       TEXT    NOT NULL,          -- folder name as read from the NAS
  movies     INTEGER NOT NULL DEFAULT 0,
  tv         INTEGER NOT NULL DEFAULT 0,
  scanned_at INTEGER NOT NULL,
  PRIMARY KEY (parent, name)
);

CREATE TABLE library_evidence (
  path       TEXT    PRIMARY KEY,       -- 'parent/folder', the title folder
  has_video  INTEGER NOT NULL DEFAULT 0,
  seasons    TEXT    NOT NULL DEFAULT '[]',  -- JSON [{season,episodes,videoFiles}]
  releases   TEXT    NOT NULL DEFAULT '{}',  -- JSON {season:[{resolution,group,key}]}
  file_keys  TEXT    NOT NULL DEFAULT '{}',  -- JSON {season:[key]}
  checked_at INTEGER NOT NULL
);
CREATE INDEX idx_library_evidence_checked ON library_evidence(checked_at);
```

Nothing else is written. No size, no full path beyond `parent/name`, no file
name — the release tokens and the identity key are what the existing in-memory
record already reduces a file name to, and the name itself is discarded exactly
as it is today.

### Store API — `internal/store/library_repos.go` (new)

```go
type LibraryParent struct{ Path string; Movies, TV bool }
type LibraryFolders struct {
    Parents   []LibraryParent
    Names     map[string][]string
    ScannedAt time.Time
}
func (s *Store) SaveLibraryFolders(f LibraryFolders) error   // replaces wholesale, one tx
func (s *Store) GetLibraryFolders() (LibraryFolders, error)

type LibraryEvidence struct {
    Path      string
    HasVideo  bool
    Seasons   []SeasonPresence
    Releases  map[int][]ReleaseToken
    FileKeys  map[int][]string
    CheckedAt time.Time
}
func (s *Store) SaveLibraryEvidence(e LibraryEvidence) error
func (s *Store) GetLibraryEvidence(path string) (LibraryEvidence, bool, error)
func (s *Store) StaleLibraryEvidence(limit int) ([]string, error) // oldest checked_at first
func (s *Store) PruneLibraryEvidence(keep []string) error        // FR-009
```

`SaveLibraryFolders` replacing wholesale is what satisfies FR-009 for parents:
a parent that is no longer configured simply is not in the new set, so its rows
go. `PruneLibraryEvidence` does the same for title folders, called with the
folder set the scan just observed.

The store package must not import `internal/library` or `internal/source`
(they are leaves and would create a cycle in the wrong direction), so
`SeasonPresence` and `ReleaseToken` are plain structs declared in `store`, and
`internal/api` converts. That conversion is small and keeps the boundary clean.

### Cache changes — `internal/api/library.go`

- `libraryIndex`: on a build failure, load from the store instead of returning
  `library.Empty` (FR-004). On the very first call after start-up the memory
  cache is cold, so the store load happens before any NAS read (FR-003).
- `folderEvidence`: memory hit → return; else store hit → return it and kick a
  background refresh if it is older than `libraryTTL` (FR-010); else read the
  NAS as today. A NAS failure with a stored record returns the stored record
  (FR-004).
- A successful build/read writes through to the store.
- `invalidateLibrary` keeps dropping the memory layer. It does **not** delete
  stored rows: they are the fallback, and dropping them would reintroduce the
  blank-on-blip failure this fixes. Freshness is handled by the scan.

### Scanner — `internal/api/library_scan.go` (new)

```go
func (d Deps) RunLibraryScan(ctx context.Context, interval time.Duration)
func (d Deps) scanOnce(ctx context.Context)
func (d Deps) RefreshFolder(folder string)   // enqueue for the next cycle (FR-007/008)
```

`scanOnce` refreshes the parent listings, persists them, prunes evidence for
folders that no longer exist, then refreshes up to `scanFolderBudget` folders —
anything explicitly enqueued first, then least-recently-read. Wired in
`cmd/synodl/main.go` beside `RunSourceKeepAlive`.

### Completion hook

`push.Watcher` already observes the `finished` transition and already publishes
`ActiveDestinations`. Add an optional `OnFinished func(destination string)`
callback set from `main.go` to `deps.RefreshFolder`, so a completed download
re-reads its folder (FR-007). `handleSourceSend` calls the same on success
(FR-008), alongside the existing invalidation.

### Constants

| Name | Value | Why |
|---|---|---|
| `libraryScanInterval` | 10m | Between the 30s task poll and the 15m keep-alive; a folder added out-of-band shows up within one cycle |
| `scanFolderBudget` | 24 | Bounds a cycle at roughly the same NAS cost as one user opening a handful of titles |
| `storedEvidenceMaxAge` | 7d | Past this a stored reading is no longer served as a fallback; a folder untouched for a week is re-read before it is trusted |

## Phase 2 — Testing

- `internal/store/library_repos_test.go` — round-trip, wholesale replace drops a
  removed parent, prune drops a removed folder, stale ordering is oldest-first.
- `internal/store/migrations_golden_test.go` — extend the pinned list (append
  only; the test fails loudly if a migration is inserted).
- `internal/api/library_test.go` — start-up serves from the store with no NAS
  read; a NAS failure falls back to the stored reading rather than blanking; a
  stored-but-stale record is returned immediately and refreshed behind.
- `internal/api/library_scan_test.go` — a cycle is bounded; an enqueued folder
  jumps the queue; prune removes a vanished folder.
- `internal/push/watcher_test.go` — the finish callback fires once, with the
  destination.
- `e2e/stateful/` — seed the mock library, restart is not testable e2e, so
  assert instead that markers appear without the client having opened the title
  first, once a scan has run.

## Verification

```sh
npm run build
npm run test:unit:coverage
cd server && go build ./... && go vet ./... && go test ./...
npm run test:e2e     # CHROMIUM_PATH=<system Chrome> on this machine
```
