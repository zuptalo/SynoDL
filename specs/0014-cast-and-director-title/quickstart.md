# Quickstart: seeing spec 0014 work

## Locally, against the mocks

```sh
make start          # mock DSM + mock sites on :8291, synodl :8280, Vite :5273
```

Log in as `admin` / `secret`, add a source in Settings → pointed at the in-repo
fake site (the dev build carries the `sourcemock` tag), then open any title in
Discover and scroll past the download options.

What you should see, and which requirement it demonstrates:

| Source | What appears | Requirement |
|---|---|---|
| the JSON mock (`tn`) | Cast with character names, a director, writers | FR-001…004 |
| | One cast member with a real photo, others filled in from the mock IMDb | FR-013, FR-016 |
| | One person with no photo anywhere → initials tile | FR-031 |
| the HTML mock (`zar`) | Cast + director, no character names | FR-003, FR-011 |
| | Portraits from the site's own person pages | FR-014, FR-015 |
| both | Tapping a tile opens that person on IMDb | FR-033 |

Open a second title sharing a cast member: the tile is instant and the server
makes no outbound lookup (FR-024). Restart `synodl` and it is still instant
(FR-025).

## Proving the degradation

```sh
curl -X POST localhost:8291/__mock/imdb/down      # the fallback stops answering
```

Every unresolved face becomes an initials tile. Names, characters, links,
downloads and the rest of Discover are untouched (SC-006). Bring it back with
`/__mock/imdb/up`.

## Against the real sources

Nothing to configure — there is no API key and no setting. Point a source at the
real site with real session material as usual; the first title you open resolves
its people, the second one that shares an actor does not.

Confirm the outbound surface is what the spec says:

```sh
cd server && go test ./internal/people/ -run TestHostAllowlist -v
cd server && go test ./internal/api/ -run TestPersonPhoto -v      # id shape, 404 path, rate limit
```

## The checks that gate it

```sh
npm run build && npm run test:unit:coverage
cd server && go build ./... && go vet ./... && go test ./...
npm run test:e2e
```
