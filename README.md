# SynoDL

**A mobile-first, self-hostable web client for Synology Download Station.**

SynoDL is an installable PWA (Vue 3 + Ionic) backed by a tiny stateless Go
proxy, shipped as a single Docker container. Point it at your Synology NAS and
get a fast, phone-friendly UI for Download Station — the kind of experience the
DSM web interface never had on mobile.

> SynoDL is an independent open-source project. It is **not affiliated with or
> endorsed by Synology Inc.** Synology, DSM, and Download Station are trademarks
> of Synology Inc.

## Features

- **Tasks** — live download list with progress, speeds, peers, and ETA; pull to
  refresh; pause / resume / delete; term search plus sort and multi-select
  status filters.
- **Add downloads** — paste one or many URLs (HTTP/FTP/magnet), upload
  `.torrent` files, and pick the destination folder by browsing your NAS shares.
- **Sessions your NAS controls** — log in with your DSM account (2FA/OTP
  supported). The proxy is credential-free: your password is forwarded once to
  the NAS, the session id is stored only in your browser.
- **Installable PWA** — add to home screen, dark and light themes, prompt-based
  updates (never a silent reload).
- On the roadmap: Download Station search modules, RSS feeds, an in-app browser
  with favorites, and notifications — see [`ROADMAP.md`](ROADMAP.md).

## Quick start (self-hosting)

```sh
docker run -d --name synodl \
  -p 8080:8080 \
  -e SYNO_URL=https://your-nas:5001 \
  ghcr.io/zuptalo/synodl:latest
```

Open http://localhost:8080 and log in with your DSM account. That's the whole
deployment: one container, no database, no volumes, nothing stored server-side.

| Env var | Default | What it does |
|---|---|---|
| `SYNO_URL` | *(required)* | Base URL of your NAS's DSM, e.g. `https://nas.local:5001` |
| `SYNO_TLS_INSECURE` | `false` | Accept a self-signed NAS certificate (outbound connection only — opt in deliberately) |
| `PORT` | `8080` | HTTP listen port |
| `MAX_TORRENT_MB` | `16` | Upload size cap for `.torrent` files |

A `docker-compose.yml` with the same setup is included. Images are published
multi-arch (amd64 + arm64) to `ghcr.io/zuptalo/synodl` and
`docker.io/zuptalo/synodl`; `:latest` is the current release, `:develop` the
rolling integration build.

Because the proxy forwards your DSM login, run SynoDL over HTTPS (behind your
reverse proxy) whenever it's reachable from outside your LAN.

## Development

Requires **Go 1.26** and **Node 22**. No Docker and no real NAS needed — dev
runs against an in-repo mock DSM.

```sh
npm install
make start      # mock DSM (:8091) + Go proxy (hot reload) + Vite (:5173)
```

Log in with `admin` / `secret` (OTP account: `otpuser` / `secret`, code
`000000`). See [`CONTRIBUTING.md`](CONTRIBUTING.md) for the spec-driven
workflow (Spec Kit + TDD), test gates, and the release train, and
[`CLAUDE.md`](CLAUDE.md) for the architecture map.

### Repository administration (one-time setup)

- Create `develop` (default branch) and `main`, then run
  `scripts/setup-branch-protection.sh` to apply the protected-branch ruleset.
- Add the `DOCKERHUB_USERNAME` / `DOCKERHUB_TOKEN` secrets for the Docker Hub
  publish steps (GHCR works out of the box with `GITHUB_TOKEN`).

## License

[AGPL-3.0-only](LICENSE). If you run a modified SynoDL server over a network,
you must offer its corresponding source to its users.
