# Quickstart: Per-User Download Statistics and Richer Notifications

How to build, run, and verify spec 0006 end-to-end against the mock DSM (no real
NAS needed).

## Run the dev stack

```sh
make start   # mock DSM (:8291) + synodl (:8280, air) + Vite (:5273)
```
Open http://localhost:5273. Log in as `admin` / `secret` (admin/owner) and, in a
second browser/profile, as a regular user (create one under Settings → Users).

## Manual verification by user story

### US1 — readable, attributed notifications
1. Subscribe to notifications on both the admin device and a regular user's device
   (Settings → Notifications). Admin keeps scope "any"; the regular user is forced
   to "own".
2. As the regular user, send a Discover download whose file name is a release-scene
   string (e.g. `X_Men_97_S01E01_1080p_WEB-DL_30NAMA.mkv`).
3. Let the mock task finish. **Expect**: the regular user's notification body reads
   `X-Men '97 · S01E01` (readable title, not the raw name); the admin's
   notification additionally reads `… · added by <regular-user>`.
4. **Expect**: a regular user never receives another user's download notification
   and never sees another username.

### US2 / US4 — statistics section (counts + averages, catalog + direct)
1. As the regular user, send a few catalog titles (a movie + a season) and add a
   couple of direct downloads from the Tasks view (a torrent/URL), using the new
   **category** picker (leave one on Auto, set one explicitly to Music).
2. Let them finish so sizes backfill.
3. Open **Settings → Statistics**.
   - **Regular user**: sees only their own figures — per-category counts and
     average sizes, split by source (Catalog / Direct / All).
   - **Admin/owner**: sees a user picker; can view any user or all combined.
4. **Expect**: counts include paused/canceled downloads; average sizes reflect
   only completed downloads; a category with no completed downloads shows "—".

### US3 — historical graph
1. In Statistics, use the bucket segment to switch **day / week / month / year /
   all-time** and the source segment (catalog / direct / all).
2. **Expect**: the SVG bar chart re-aggregates instantly with no page reload;
   empty periods show as zero-height gaps; totals stay consistent across buckets.

## Automated gates (must pass before "done")

```sh
npm run build                 # vue-tsc typecheck + vite build (client)
npm run test:unit:coverage    # vitest + coverage floors (incl. stats-buckets, title parity)
cd server && go build ./... && go vet ./... && go test ./...
npm run test:e2e              # Playwright: statistics.spec.ts + notification path
```

### What the tests cover (TDD order — see tasks.md)
- **Server unit** — `history_repos_test.go` (counts incl. canceled; averages over
  completed only; per-source/category grouping; daily grouping; cascade on user
  delete), `tasktitle_test.go` (parity with `task-title.test.ts` cases),
  `mediaclass_test.go` (folder + extension classification), `stats_handlers_test.go`
  (role gating: non-admin scoped to self, admin sees all), watcher size-backfill
  test (match by destination+name; no-match leaves size NULL).
- **Client unit** — `stats-buckets.test.ts` (day→week/month/year/all with local
  boundaries and zero-fill), `format.test.ts` reuse for `formatBytes`.
- **e2e** — seed `download_history` via the mock's `/__mock/*` control endpoints
  (or by driving real sends), then assert the Statistics summary numbers, graph
  bucketing, admin-vs-regular gating, and one readable/attributed notification.

## Seeding history in tests

The e2e harness seeds deterministic downloads through the mock DSM control
endpoints (as existing task fixtures do) and lets the watcher attribute + backfill,
or seeds `download_history` directly via a `/__mock`-adjacent test hook if a
control endpoint is added. Prefer driving real sends so attribution + backfill are
exercised end-to-end.
