# SynoDL

**A mobile-first PWA client for Synology Download Station** — shipped as one small,
stateless container that serves an installable PWA and its Go proxy. The proxy is
**credential-free**: your DSM password is forwarded once to your NAS at login, the
session id lives only in your browser, and the server persists nothing at all.

- 📱 **Installable PWA** — Vue 3 + Ionic, dark-first, made for phones (the DSM web UI isn't).
- ⬇️ **Download Station, properly mobile** — live task list, add by URL/magnet/.torrent, pause/resume/delete, destination folder picker.
- 🔐 **Stateless & credential-free** — no database, no volumes, no stored secrets; only an allowlisted subset of the DSM API is ever forwarded.
- 🗂️ **One image** — PWA at `/`, API at `/v1`, on the same origin.
- 🪪 **AGPL-3.0**, self-hostable. Not affiliated with Synology Inc.

## Quick start

```sh
docker run -d --name synodl \
  -p 8080:8080 \
  -e SYNO_URL=https://your-nas:5001 \
  zuptalo/synodl:latest
```

Open http://localhost:8080 and log in with your DSM account (2FA supported).
`SYNO_TLS_INSECURE=true` accepts a self-signed NAS certificate (deliberate
opt-in). Full configuration and development docs: see the
[GitHub repository](https://github.com/zuptalo/synodl).

## Tags

| Tag | What it is |
| --- | --- |
| `latest`, `X.Y.Z`, `X.Y` | Production releases. |
| `X.Y.Z-rc.N` | Immutable release candidates (never moves `latest`). |
| `develop`, `develop-<sha>` | Rolling development build. |

Images are multi-arch (`linux/amd64` + `linux/arm64`) and also published to
`ghcr.io/zuptalo/synodl`.
