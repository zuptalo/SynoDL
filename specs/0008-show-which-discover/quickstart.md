# Quickstart: exercising ownership, seasons and episodes

Phase 1 for [plan.md](./plan.md). No real NAS is needed — `synomock` stands in for DSM.

## Boot the stack

```sh
make start                # mock DSM :8291 (TLS), synodl :8280, Vite :5273
```

Sign in with the mock account `admin` / `secret`. The dev build carries the `sourcemock`
tag, so Discover browses the in-repo fake sites with no credentials.

## Seed a library worth testing

`POST /__mock/library` seeds folders **and files** (the file support is a precondition of
this plan — see plan.md Complexity Tracking). Reset only the folder tree, never
`/__mock/reset`, which also clears sessions and tasks.

```sh
curl -sk -X POST https://localhost:8291/__mock/library -d '{
  "reset": true,
  "tree": {
    "/tv-show/Attack on Titan (2013)/Season 01": ["s01e01.mkv","s01e02.mkv","s01e03.mkv"],
    "/tv-show/Attack on Titan (2013)/Season 02": ["s02e01.mkv"],
    "/tv-show/Attack on Titan (2013)/Season 00": ["season.nfo"],
    "/movie/Dune (2021)":                        ["Dune.2021.mkv"],
    "/movie/Arrival (2016)":                     ["poster.jpg","Arrival.srt"]
  }
}'
```

Those five entries are the whole feature in miniature:

| Fixture | Expected | Requirement |
|---|---|---|
| `Dune (2021)` holding a video | **owned** | FR-001 |
| `Arrival (2016)` holding only artwork and a subtitle | **absent** — the folder exists and is non-empty, and still is not evidence | FR-001a |
| `Attack on Titan` seasons 1–2 | present, episodes `[1,2,3]` and `[1]` | FR-014, FR-016 |
| `Season 00` holding only `season.nfo` | **not** present | FR-001a, US2 scenario 1c |
| Any season | never described as complete or as "n of m" | FR-016a |

`Season 00` is the real case this amendment came from: it existed on the operator's NAS
holding nothing but `season.nfo`, and 0.3.0 marked the title owned.

## Check each behaviour

1. **Grid marker** — open Discover. `Dune (2021)` is marked; `Arrival (2016)` is not.
2. **Season and episode detail** — open Attack on Titan. Seasons 1 and 2 show as present
   with their episode numbers; season 0 and seasons 3+ show nothing.
3. **Downloading beats owned** — send any title, then return to Discover before it
   finishes. It reads *downloading*, not owned (FR-001b), and is hidden by the hide-owned
   control (FR-019a).
4. **Nothing claimed before it is checked** — a title never verified carries no marker
   rather than an "absent" one (FR-010c).
5. **Degrades silently** — stop the mock (`make stop`, leave Vite running) and reload
   Discover. No markers, no error, browsing and sending unchanged (FR-009).
6. **Cost** — with the server log open, load a page of results and count
   `SYNO.FileStation.List` calls. Only titles matching a folder name should produce one;
   a page of non-matches must produce none (FR-010b).

Step 6 is the one worth actually measuring. It is the difference between this design and
the eager scan rejected in the plan, and nothing in the UI will reveal a regression in it.

## Gates before calling it done

```sh
npm run build && npm run test:unit:coverage
cd server && go build ./... && go vet ./... && go test ./...
CHROMIUM_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" npm run test:e2e
```
