# Quickstart: Save YouTube music and music videos to the library

 **Spec**: [spec.md](./spec.md) | **Date**: 2026-09-06

## For the operator

1. **Mount the two libraries.** Create `ReadWriteMany` PVCs for the NAS shares —
   `music` and `music-video`. RWX is required: concurrent workers mount the same
   library, and the existing `synodl-data` PVC (`local-path`, RWO) is not a model
   for these. The `synodl` Deployment must **not** mount them.
2. **Grant the server permission to run workers.** Apply the ServiceAccount,
   namespaced `Role`, and `RoleBinding` from `deploy/k8s/30-rbac.yaml`. Confirm it
   is a `Role` and not a `ClusterRole`, and that it grants no access to secrets or
   `pods/exec`.
3. **Pin the worker image.** Set it to an explicit tag, never `:latest`. Treat it
   like any other dependency: an outdated extractor stops working against the
   source, so bump it deliberately alongside the supply-chain review rather than
   letting it float.
4. **Set the library ownership.** Point the worker's uid/gid at whatever owns the
   shares, so files arrive readable by Plex without permission repair.
5. **Point Plex at them.** `music` as a Music library; `music-video` as its own
   library. Plex has no first-class music-video library type — Other Videos suits
   the `Artist/Album/Title` shape these produce. Worth confirming against your
   Plex version.

**Not running in Kubernetes?** The feature reports itself unavailable and the
rest of SynoDL is unaffected. Compose and bare-container deployments keep working
exactly as before.

## For a developer

`make start` boots the mock orchestrator (`:8295`) alongside the mock DSM, so the
whole flow works on a laptop with no cluster and nothing is ever downloaded.
Drive a download through its states with the control endpoints:

```sh
curl -X POST localhost:8295/__mock/jobs/<name>/start
curl -X POST localhost:8295/__mock/jobs/<name>/succeed
curl -X POST localhost:8295/__mock/jobs/<name>/fail
```

The e2e stack boots its own mock on `:8296`. Neither touches your `make start`
stack, matching the existing two-stack harness.

## Verifying the real thing

Against a real cluster, the acceptance evidence is on disk. A music download
should produce, under the music library:

```text
<Artist>/<Album|Singles>/<verbatim YouTube title>.mp3
<Artist>/<Album|Singles>/<verbatim YouTube title>.lrc
```

and the audio file's tags should carry a non-empty album and album-artist — that
is the difference between Plex shelving it properly and dumping it under
*[Unknown Album]*, and it is the single easiest thing to regress.

A music-video download should produce `.mp4` + `.en.srt` in the other library,
and probing the video should show its streams copied from the source rather than
re-encoded.
