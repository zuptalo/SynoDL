# Upgrading & operating a SynoDL instance

This is the operator's runbook for keeping a self-hosted SynoDL up to date. It
assumes you already have an instance running (see the [README](../README.md) for
first-time setup).

## How upgrades work

The SynoDL container is **stateless and credential-free**. It stores nothing:
no database, no volumes, no secrets. Your session and app settings live in your
browser's storage; your downloads live on the NAS. The container holds nothing
you need to preserve.

That makes an upgrade trivial: **pull a newer image and restart the container.**
There is no migration, no backup, no build step, and no state to lose.

Two halves update independently and that is by design:

- **Server (`synodl`)** updates the instant the new container is running.
- **Client (the PWA)** is served by that same container, but installed PWAs cache
  the app shell. SynoDL builds with `registerType: 'prompt'`, so a client that
  already has the app open does **not** silently reload onto the new version — it
  detects the new deploy (it compares its build version against the server's
  `/v1/config`) and surfaces an in-app "update available" prompt. The new version
  applies when the user accepts. This is intentional: no surprise reloads while
  you're queuing downloads.

## Choosing an image tag

`ghcr.io/zuptalo/synodl` (mirrored at `docker.io/zuptalo/synodl`) is published
under several tags; which one you pin decides how upgrades reach you:

| Tag             | What it is                                  | Use it when…                                         |
| --------------- | ------------------------------------------- | ---------------------------------------------------- |
| `:latest`       | Rolling build of `main` — every green merge. | You auto-deploy from main (e.g. k3s polling) and want each merge live. |
| `:main-<sha>`   | One specific merge (immutable).             | You pin deployments to exact merges and roll back by sha. |
| `:X.Y.Z`        | One specific tagged release (immutable).    | You want **deliberate**, pinned upgrades and easy rollback. |
| `:X.Y`          | Newest patch within a minor line.           | You want patch updates but not minor/major jumps.    |
| `:X.Y.Z-rc.N`   | A **release candidate** (pre-release).      | You're helping test an upcoming release.             |

Pinning a specific `:X.Y.Z` (or `:main-<sha>`) is the safest posture for
production: redeploys are reproducible and a rollback is just re-pinning the
previous number. `:latest` floats with every merge to main — ideal for an
auto-updating home deployment, since only fully-green merges ever publish.

## Upgrading

```sh
docker compose pull && docker compose up -d
# or, plain docker:
docker pull ghcr.io/zuptalo/synodl:latest && docker restart synodl
```

## Rolling back

Because the container is stateless, a rollback is just running the previous
tag — nothing else to restore:

```sh
docker run -d --name synodl -p 8080:8080 -e SYNO_URL=… ghcr.io/zuptalo/synodl:X.Y.Z
```

If a broken client shell is stuck in an installed PWA, accepting the update
prompt (or clearing site data as a last resort) resolves it — the server always
serves the current build's assets and honestly 404s superseded ones.
