# Research: YouTube sign-in for download workers

## R1 — Why anonymous workers are refused
Verified 2026-09-29: from the cluster and from an unrelated laptop container alike, the pinned yt-dlp (2026.08.19) **and** the nightly (2026.09.27) got "Sign in to confirm you're not a bot" for every client mode (`default`, `tv`, `web_safari`, `mweb`, `android_vr`). A cookies file made from a signed-in browser's `Cookie` request header (24 cookies, 2.9 KB, converted to Netscape format with all values on `.youtube.com`) resolved the same video at once. Host-network mode changed nothing (same public address). Conclusion: the client is treated as automated; a signed-in session is what clears it.

## R2 — Delivery options for a secret into a worker pod
| Option | Verdict |
|---|---|
| Value in Job env / args / annotation / ConfigMap | Rejected: readable by anyone who can read Jobs (FR-016). |
| Per-worker Kubernetes Secret | Rejected: `30-rbac.yaml` grants no `secrets`, deliberately; `create` alone leaves the Secret alive until the Job TTL (SC-007) and `delete` on Secrets could delete `synodl-secrets`, which holds `SECRETS_KEY`. Would be a constitution-level change. |
| Operator-created Secret mounted in workers | Rejected: no paste-in-UI, and the server could not update it. |
| **One-time pull bound to the pod** | **Chosen**: no RBAC change; the Job carries only a grant that is useless without being the right pod, once. |

## R3 — Binding the pull to the pod
The server already has `pods get/list`. The grant names a Job; the fetch lists pods with label `job-name=<job>` and requires `r.RemoteAddr` (host part, raw) to equal `status.podIP`. `httpx.clientIP` honours `X-Forwarded-For` for the login rate limiter, so it MUST NOT be used here. Traefik (the published ingress) adds `X-Forwarded-For`; any request carrying it is refused, so nothing coming through the ingress can redeem a grant, and an in-cluster client with a different address cannot either. No NetworkPolicy exists in `deploy/`, so the pod → Service path is open. The Service is `synodl:8080`.

## R4 — yt-dlp behaviour verified in the pinned image
- `--cookies` on a missing file: Python `FileNotFoundError` traceback → download fails. So the init step must always leave a valid file (fail open).
- A header-only file (`# Netscape HTTP Cookie File`) is accepted and runs anonymously.
- A read-only valid cookie file is accepted without error, but the jar is written back on exit when it changes, so the volume is left writable.

## R5 — Scratch volume
`emptyDir{medium: Memory, sizeLimit: 1Mi}` is a tmpfs owned by the pod; kubelet removes it when the pod ends, so no copy outlives the worker (SC-007) and nothing touches disk.

## R6 — Which failures mean "the sign-in stopped working"
Only the bot-check text (`not a bot`) — the other refusals (403/429) are not evidence about the session. The classification already exists (`failure.go`); it gains a variant applied when the Job carries the sign-in label.

## R7 — Cookie domains
A signed-in export contains `.youtube.com` and `.google.com` cookies; the extractor reads `.youtube.com`. Keep `youtube.com` and `google.com` and subdomains; drop everything else. A pasted header has no domain information, so all its cookies are filed under `.youtube.com` with the secure flag set for `__Secure-`/`__Host-` names and the known login names.

## Open items carried to tasks (none block)
- Mock cluster must report a pod IP and pods by `job-name` for the e2e worker-side checks; the endpoint's IP binding is unit-tested with fakes.
