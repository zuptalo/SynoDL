# Quickstart — Seeing the library markers locally

**Feature**: 0008 — Show which Discover titles you already have

No real NAS and no real download-source credentials are needed. `make start` runs the stateful
server against the mock DSM, and the dev build carries the `sourcemock` tag so Discover browses
the in-repo fake sites.

## 1. Boot the stack

```sh
make start      # mock DSM :8291 (TLS) + synodl :8280 + Vite :5273
```

Open http://localhost:5273 and sign in. On a fresh `.devdata/` the setup wizard runs first;
otherwise use the account you created. The mock NAS account is `admin` / `secret`.

Confirm a download source is configured (Settings → Download sources) and note its **movies** and
**TV** parent folders — the mock's fixture tree offers `movie` and `tv-show`.

## 2. Seed a fake library on the mock NAS

The mock ships a directory-only fixture tree
(`""` → `home, movie, music, music-video, rated-video, tv-show`; `/tv-show` → `Friends, The Wire`;
`/movie` → `4K, Kids`) and, before this feature, had no way to add files. `POST /__mock/library`
seeds folders and files together:

```sh
curl -sk -X POST https://localhost:8291/__mock/library -d '{
  "folders": {
    "/movie":            ["Dune 2021", "It 2017"],
    "/tv-show":          ["Friends 1994 - 2004"],
    "/tv-show/Friends 1994 - 2004": ["Season 1", "Season 2"]
  },
  "files": {
    "/tv-show/Friends 1994 - 2004/Season 1": ["Friends.S01E01.1080p.mkv", "Friends.S01E02.1080p.mkv"],
    "/tv-show/Friends 1994 - 2004/Season 2": ["Friends.S02E01.1080p.mkv"]
  }
}'
```

Use folder names that match titles the mock catalog actually returns — browse Discover first and
copy a title verbatim. `POST /__mock/reset` restores the plain fixture tree.

## 3. Walk the three surfaces

**The marker (User Story 1)**

1. Open Discover. The seeded titles carry an "already have it" marker on the poster; their
   neighbours do not.
2. Check the near-miss cases: a folder named `Dune (2021)` must still match `Dune 2021`, and
   `It 2017` must **not** mark a 1990 `It` in the catalog.

**Season detail (User Story 2)**

3. Open the seeded series. Its download options mark seasons 1 and 2 as already present and leave
   later seasons unmarked.
4. Re-seed the same series in the **flat** layout — episode files directly in the title folder,
   no `Season N` subfolders — and confirm the seasons are still detected.

**Guardrails (User Story 3)**

5. Choose a season you already have and send it: an Ionic alert asks you to confirm. Cancel — the
   task list is unchanged and no allowance is consumed.
6. Send it again and accept: the download proceeds exactly as before.
7. In the filter sheet, enable **hide what I have**: the seeded titles leave the grid, and the
   grid stays full rather than sparse as you scroll.
8. Reload the page — the toggle is still on (it is stored per user, server-side).

## 4. Walk the failure paths — these are requirements, not edge cases

**A send is recognised immediately (FR-008)**

9. Send a title you do **not** have, then return to Discover. It is marked on the next catalog
   request, without waiting out the 5-minute cache.

**Silent degradation (FR-009)**

10. Stop the mock DSM (`make stop`, or kill the `synomock` process) and reload Discover.
    Browsing, searching, and the filter sheet all still work; nothing is marked; **no error is
    shown**. Opening a title still lists its download options, with no season markers.

**Staleness bound (FR-010a)**

11. With the stack running, seed a new folder and refresh Discover: it may not appear at once —
    the snapshot is reused for up to 5 minutes. It must appear within that window with no user
    action. Sending anything invalidates the snapshot and makes it appear at once.

## 5. Gates before calling it done

```sh
npm run build                 # typecheck + build
npm run test:unit:coverage    # vitest + coverage floors
cd server && go build ./... && go vet ./... && go test ./...
npm run test:e2e              # Playwright; on macOS 12 set CHROMIUM_PATH to system Chrome
```

The e2e work for this feature lives in `e2e/stateful/library.spec.ts` and runs against the
stateful stack (Vite :5275, synodl :8283, mock DSM :8294), which is booted by
`e2e/global-setup.ts` independently of `make start`.
