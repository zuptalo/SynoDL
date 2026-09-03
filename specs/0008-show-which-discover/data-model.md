# Data Model: ownership, seasons and episodes

Phase 1 for [plan.md](./plan.md). **Nothing here is persisted.** Every entity lives in
memory for at most the 5-minute retention of FR-010; the NAS remains the source of truth
(Principle III: "Download tasks themselves are never persisted").

## OwnershipState

The single value the client renders. Replaces the boolean `inLibrary` shipped in 0.3.0.

| Value | Meaning | Established by |
|---|---|---|
| `unknown` | Not yet verified. **Carries no marker** (FR-010c) | default |
| `absent` | No matching folder, or the folder holds no video | layer 1 or layer 2 |
| `owned` | At least one video file beneath the title folder (FR-001) | layer 2 |
| `downloading` | An active task is writing into the title folder (FR-001b) | task list |

`downloading` takes precedence over `owned`: a title being extended by a running
download is still something the user should wait for rather than send again (FR-019a).

State is never inferred from a folder's existence, its emptiness, or from a title having
been sent — FR-008 was corrected precisely because sending is not evidence.

## Parent *(unchanged)*

The distinct movie/TV parent folders across enabled sources (FR-007). Already implemented
as `library.Parent{Path, Movies, TV}`.

## NameIndex — layer 1 *(largely unchanged)*

Directory names under each parent, normalised by the existing `library.Key`, mapping a
comparison key to candidate folders.

| Field | Type | Notes |
|---|---|---|
| `byKey` | `map[string][]Entry` | `Entry{Path, Name, Year}` |
| `builtAt` | `time.Time` | 5-minute retention (FR-010) |

**Its meaning changes even though its shape does not.** It no longer answers "does the user
have this"; it answers "is there a folder that *could* be this". A miss is conclusive
(`absent`, no NAS call). A hit is only a candidate and must be verified.

## FolderEvidence — layer 2 *(new)*

What one title folder actually contains. Cached per folder path, built lazily and only for
folders backing a title in the current response (FR-010b).

| Field | Type | Notes |
|---|---|---|
| `Path` | `string` | absolute NAS path of the title folder |
| `HasVideo` | `bool` | true if any video file at this level or in a season subfolder |
| `Seasons` | `map[int]SeasonPresence` | empty for a movie, or a series stored flat with unreadable names |
| `CheckedAt` | `time.Time` | 5-minute retention, same as layer 1 |

`HasVideo` is deliberately separate from `len(Seasons) > 0`: a series whose file names carry
no readable `SxxEyy` still has video and is still owned (FR-016b).

## SeasonPresence *(new)*

| Field | Type | Notes |
|---|---|---|
| `Season` | `int` | season number; `0` is a valid specials season |
| `Episodes` | `[]int` | sorted, de-duplicated episode numbers read from file names |
| `VideoFiles` | `int` | count of video files in the season, including unparseable names |

**There is no `Total` and no `Complete` field, deliberately.** FR-016a forbids describing a
season as complete or as a fraction: the catalog's episode count cannot be relied on, and
asserting it would repeat the over-claiming that FR-001a exists to prevent. A season with
`VideoFiles > 0` and an empty `Episodes` is valid and means "present, numbering unreadable".

## Relationships

```
Parent 1─* NameIndex.Entry          (layer 1, one listing per parent)
NameIndex.Entry 1─1 FolderEvidence  (layer 2, one listing per matched folder, lazy)
FolderEvidence 1─* SeasonPresence
OwnershipState ← FolderEvidence + active tasks
```

## Validation rules

- A catalog title and a folder both carrying a year MUST agree on it (FR-005, unchanged).
- Video is decided by extension, using the same table that governs uploads (research §1).
- Episode numbers come from the files, never from the catalog (FR-016).
- A folder whose listing fails is `unknown`, never `absent` — a failed read must not be
  reported as "you do not have this" (FR-009, FR-010c).

## Credential-Safety Impact

- **Stored**: nothing. Both layers are in-memory and per-instance, discarded on restart and
  on the invalidations of FR-008/FR-008a.
- **Crosses to the NAS**: one additional `SYNO.FileStation.List` call per matched folder,
  with `filetype` set. No new API; the operator's single stored connection as always.
- **Could appear in logs**: nothing new. Folder and file names are NOT logged — the existing
  rule stands, and this feature reads more names than before, which is exactly why it must
  stay unlogged. Log the route and outcome only.
- **Client exposure**: a user learns only whether a title they are *already browsing* is
  present, and for a series which seasons and episodes exist. FR-025a still forbids a
  client naming an arbitrary path, so this cannot be turned into a filesystem browser.
