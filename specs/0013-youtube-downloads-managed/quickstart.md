# Quickstart: working on spec 0013

## Run it

```sh
make start      # mock DSM :8291 (TLS) + mock Jobs API :8295 + synodl :8280 + Vite :5273
```

App on http://localhost:5273, sign in as `admin` / `secret`. A second account is
needed for the ownership work (US1) — create one in Settings as admin.

No cluster and no NAS are required, and that is a constraint rather than a
convenience: `internal/k8smock` must gain pod listing, pod logs, and controls
that emit progress lines, lyrics lines and expansion entries, or none of this
feature is exercisable locally (R13).

## The gates

```sh
npm run build                 # typecheck + build (there is no separate lint)
npm run test:unit:coverage    # floors are a ratchet — may rise, never regress
cd server && go build ./... && go vet ./... && go test ./...
npm run test:e2e              # builds and boots its own stacks
```

E2E runs locally on this machine with `CHROMIUM_PATH` pointed at system Chrome.

## Where the decisions live

| Decision | File |
|---|---|
| The verified download recipe | `server/internal/ytdl/command.go` |
| Host allowlist + scope classification | `server/internal/ytdl/url.go` |
| Job assembly, labels, state mapping | `server/internal/ytdl/job.go` |
| Migrations (append-only, must be re-runnable) | `server/internal/store/schema.go` |
| The RBAC that must stay minimal | `deploy/k8s/30-rbac.yaml` |

## Three things to prove, not assume

These are the open items from `research.md`. Each has a fallback already written
into the spec, so none can block — but each must be answered by observation, not
by reading code.

1. **Progress fields on the audio path** (R2). The audio path downloads and then
   post-processes; confirm the progress template actually yields usable values
   through that sequence, and that the download reports sensibly during
   extraction rather than appearing stuck at 100%.

2. **That a value injected via metadata parsing is sanitised by the extractor**
   (R6). This does not change what to build — server-side sanitising is required
   regardless (FR-038a) — but it tells you whether the extractor is a second
   layer or no layer. Test with a group name containing path separators and
   parent-directory references and look at where the file actually lands.

3. **mp4 cover art against the pinned image** (R9). `jauderho/yt-dlp:2026.08.19`
   may or may not carry the helper the extractor needs to embed art in mp4, and
   when it is missing the extractor warns and continues — the download succeeds
   and the art is silently absent. Run the music-video path, open the file, and
   look. If art is absent, write a sidecar; FR-034 accepts either.

## Things that will bite

- **A migration must run twice.** The spec 1031 drift repair rewinds
  `schema_migrations` and replays. `IF NOT EXISTS` everywhere, `INSERT OR IGNORE`
  for the backfill. A migration that cannot run twice turns the repair into a
  boot failure.
- **Records outlive their account.** `ON DELETE SET NULL`, matching the existing
  convention — not CASCADE. This corrected the spec, not the code (R10).
- **404, never 403,** for another user's download. A 403 confirms it exists.
- **The URL stays the last argv element**, exactly once, after `--`. Nothing
  concatenates it. There is already a test that fails if this is broken; the new
  group-name argument is subject to the same rule.
- **Failure before success, everywhere.** Reporting an unsuccessful download as
  completed is the one outcome 0012 FR-018 forbids and this spec keeps.
- **Release-note subjects** for `feat`/`fix` are shown verbatim to users. No
  spec numbers, no FR references, no `US3`.
- **`make roadmap`** after changing the spec's `Status:` line, or CI's guard
  fails.
