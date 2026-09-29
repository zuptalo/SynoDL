# Implementation Plan: YouTube sign-in for download workers

**Branch**: `feat/1055-youtube-cookies-download-workers` | **Date**: 2026-09-29 | **Spec**: [spec.md](spec.md)

## Summary

An admin saves one instance-wide YouTube sign-in (a pasted `Cookie` header or a cookies
file). It is stored encrypted in the single SQLite store. When a YouTube download or a
playlist/channel expansion Job is created and a sign-in exists, the Job gets an **init
step** that exchanges a **single-use grant** for the cookies over the cluster network and
writes them to **memory-backed scratch space** the downloader reads with `--cookies`.
No cluster permission changes: the server never puts the cookies in a Job, and never
creates a Secret.

## Technical Context

**Language/Version**: Go 1.27 (stdlib only), TypeScript/Vue 3 + Ionic.
**New dependencies**: none (Go or npm).
**Storage**: one new table `youtube_signin` (single row) in the existing SQLite store; the sealed blob uses the existing `store.Cipher`. Grants are in memory only.
**Testing**: Go table tests (converter, store, handlers, job assembly, reconciler), vitest for a small pure module if any, Playwright e2e on the stateful stack against the mock cluster.
**Target Platform**: k3s, single namespace, single node (the home cluster), Traefik ingress publishing port 8080.
**Constraints**: no RBAC change; no `secrets`; namespaced Role only; pinned worker image; a worker mounts exactly one media library; init step must fail OPEN.

## Constitution Check

| Principle / constraint | Status |
|---|---|
| I Spec-driven | Spec 1055, full pipeline. |
| II TDD | Tests written first per task group (tasks.md orders Red → Green). |
| III Custodial state & credentials (NON-NEGOTIABLE) | Sealed at rest under `SECRETS_KEY`, single store, never returned/logged, delivered by single-use pod-bound pull, not via Job objects, no RBAC widening. See *Credential-Safety Impact*. |
| IV Offline-first client | Settings modal reads live state from the server; no new IndexedDB store. |
| V Quality gates | build/vet/test, vitest coverage, e2e, `npm run release:minor`; `feat` subject in plain language. |
| VI Ionic-first UI | Stock Ionic modal following `MusicLibraryModal` / `MusicRepairModal`. |
| VII Traceable delivery | Issues per task group; PR closes them. |
| Domain: ephemeral workers, one library, pinned image, host allowlist, discrete argv | Unchanged; `--cookies <fixed path>` is a constant argument, never user input. |
| Domain: RBAC namespaced, no secrets | Unchanged. `30-rbac.yaml` must be byte-identical. |

Gate: **PASS**, no Complexity Tracking entries.

### Credential-Safety Impact

- **Stored**: one row, cookie text sealed with `Cipher.Seal`; plus non-secret metadata (count, which login cookies, saved-at, saved-by name, last-refused / last-ok times). Losing the key makes the row undecryptable → treated as absent.
- **In memory**: grants (SHA-256 of a 32-byte random token → job name, expiry, used flag); the plaintext only transiently while answering one fetch.
- **Crosses to the worker**: in one HTTP response to an init container, over the in-cluster network (plain HTTP, pod → Service). Accepted residual: the pod network is trusted to the same degree as the service-account and NAS-proxy traffic on this cluster (single node); not a cluster-wide claim. The response is `Cache-Control: no-store` and carries nothing else.
- **What the Job objects carry**: the grant (worthless alone), the fetch URL, a `synodl.io/signin=true` label. Never a cookie value.
- **Binding the grant**: single use; expires after 10 minutes; names exactly one Job; redeemable only when the raw connection address equals the `status.podIP` of a pod carrying that Job's `job-name` label. The `X-Forwarded-For` header is **never** consulted, and a request carrying one is refused (an outside call through the ingress carries one). This is why the endpoint is safe on the port the ingress publishes.
- **Logs / errors**: no value ever; the fetch endpoint logs only "signin fetch ok/denied" with the Job name and a fixed reason code, never the grant.
- **Teardown**: the scratch volume is `emptyDir{medium: Memory}` — removed with the pod on any exit; grants are revoked when the reconciler sees the Job end and expire on their own regardless.
- **Why safe**: admin-only; sealed at rest; metadata-only reads; the only secret-bearing path is a one-shot, pod-bound response to a workload the server itself created; the server's permissions are unchanged.

## Design

### Server

- `internal/ytsignin` (pure): `Parse(text) (Result, error)` accepting a `Cookie:` header (any case, whitespace/newlines tolerated), a Netscape file, or an upload of one; keeps only `youtube.com` / `google.com` (+ subdomains) cookies; ≤ 64 KiB, ≤ 200 cookies, name ≤ 128, value ≤ 4096, no control characters; emits canonical Netscape bytes, cookie count, login names found/missing (`SAPISID`, `__Secure-3PSID`, `LOGIN_INFO`, `SID`). Errors carry counts, never text.
- `store`: migration 0040→0041 `youtube_signin`; `SaveYoutubeSignIn`, `GetYoutubeSignIn` (metadata), `OpenYoutubeSignInCookies` (plaintext, used only by the fetch handler), `DeleteYoutubeSignIn`, `NoteYoutubeSignInOutcome(refused|ok)`.
- `api`: `GET/PUT/DELETE /v1/youtube/signin` (admin) and `GET /v1/internal/ytdl-signin` (grant-authenticated, no session). Grants live in a pointer field on `Deps` (`signinGrants`), like `ytdlProgress`.
- `ytdl`: `Options.SignIn bool` adds `--cookies /signin/cookies.txt`; `BuildJob` / `BuildExpansionJob` add the init container, the memory volume and the label when `JobConfig.SignInGrant != ""`. Non-YouTube (catalog) sources never call these builders, and the URL class is checked so only YouTube-host targets get a grant (FR-010).
- `k8s`: `EmptyDirSource{Medium, SizeLimit}`, `Pod.Status.PodIP`, container `Env` already exists.
- Reconciler: when a Job labelled `synodl.io/signin=true` ends — refused (not-a-bot) → `ReasonSignInRefused` and `last_refused_at`; succeeded → `last_ok_at`. Always revoke its grant.
- Config: `SIGNIN_FETCH_URL` override; default `http://synodl.<namespace>.svc:8080/v1/internal/ytdl-signin`.

### Init step (constant script, no user input)

Runs in the pinned worker image as the same uid. Writes a header-only Netscape file first, then tries to replace it with the fetched cookies (timeout 10 s); always exits 0 (fail open — verified: `yt-dlp --cookies` on a missing file crashes, on a header-only file runs anonymously).

### Client

`YoutubeSignInModal.vue` opened from an admin-only Settings row (`settings-youtube-signin`): status block, paste field (no autofill, no autocapitalise), file picker, Save / Remove, guidance on where the header is, sensitivity warning; the paste is held in memory only and cleared after a successful save. `YtdlDetailModal` shows the admin hint for a refused download.

## Phases

1. Pure core: converter + store (tests first).
2. Grants + fetch endpoint + admin API.
3. Job assembly (init container, memory volume, label, `--cookies`) for download and expansion; YouTube-only guard.
4. Reconciler outcomes and failure wording.
5. Client modal + Settings row + hint.
6. e2e incl. leak checks; docs; release bump; deploy verification on the home cluster.

## Complexity Tracking

None.
