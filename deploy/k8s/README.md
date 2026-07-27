# SynoDL on k3s (with auto-deploy)

Deploys SynoDL to a k3s cluster and keeps it current: **Keel** rolls the
Deployment whenever a new production image is published to
`ghcr.io/zuptalo/synodl:latest`. Unlike the Ring deployment, there is **no
database, no secret, and no volume** — SynoDL is a **stateless, credential-free
proxy** (constitution Principle III). The only server-side setting is the single
NAS URL it forwards to.

```
                                     ┌──────────── namespace: synodl ──────────┐
  client ──HTTPS──►  edge Traefik ──HTTP──►  k3s Traefik ──► synodl (1 replica) │
  (browser/PWA)      (terminates TLS   :80    Host-route       :8080 PWA + /v1  │
                      for the host)    (web)                   stateless proxy  │
                                                                     │          │
   Keel (ns: keel, shared) ── polls GHCR ──────────────────────────►│ redeploy │
                                                              on a new :latest  │
                                     └──────────────────── forwards to ─────────┘
                                                            SYNO_URL (your NAS)
```

## Why this shape (and how it differs from Ring)

- **SynoDL terminates no TLS of its own.** It is a plain HTTP server on `:8080`
  serving the PWA at `/` and the API at `/v1` + `/healthz`. So TLS is terminated
  **at the edge** (the proxy that fronts your cluster), which forwards cleartext
  HTTP into k3s. Ring, by contrast, needs raw TLS on :443 (for its embedded
  TURN/SFU), so Ring uses SNI **passthrough**; SynoDL does not.
- **Stateless — nothing to back up.** No Postgres, no PVC, no `Secret`. The NAS
  session id lives on the client and rides each request in `X-Syno-Sid`. Losing
  the pod loses nothing.
- **Routing on the `web` (:80) entrypoint**, not `websecure`: this k3s Traefik has
  no ACME cert-resolver, so terminating TLS in-cluster would only serve Traefik's
  self-signed default cert. Keep TLS at the edge.
- **Pull-based updates (Keel).** Nothing needs inbound access to your cluster and
  no kubeconfig lives in CI. Keel (already installed cluster-wide, shared with
  Ring) polls GHCR and redeploys on a new digest.

## Prerequisites

- A running k3s cluster with its **bundled Traefik** (default).
- An **edge/reverse proxy** that fronts the cluster and can terminate TLS for the
  app host (e.g. your Synology/Traefik box). It forwards HTTP to the k3s node's
  `:80`, preserving the `Host` header.
- The NAS reachable **from inside the cluster** at `SYNO_URL` (a LAN IP or a name
  the k3s node resolves).
- `kubectl` pointed at the cluster, plus `envsubst` (from `gettext`) on the
  machine you run the installer from.

## Install

```sh
cd deploy/k8s
APP_HOST=synodl.example.com \
SYNO_URL=https://nas.local:5001 \
SYNO_TLS_INSECURE=true \
./install.sh
# SYNO_TLS_INSECURE defaults to false; set true only for a self-signed DSM cert.
```

The installer is **idempotent**: no secrets are generated, so re-running simply
reconciles config (e.g. a changed `SYNO_URL`) and rolls the Deployment.

## Edge Traefik: what to forward

Terminate TLS for the app host at your edge proxy and forward HTTP to the k3s
node's `:80` (the `web` entrypoint), preserving the `Host` header. Traefik
dynamic-config example (adjust `certResolver` and the node IP to yours):

```yaml
http:
  routers:
    synodl:
      rule: "Host(`synodl.example.com`)"
      entryPoints: [websecure]
      tls:
        certResolver: letsencrypt      # your existing resolver
      service: synodl
  services:
    synodl:
      loadBalancer:
        passHostHeader: true           # k3s route matches on Host — keep it
        servers:
          - url: "http://<k3s-node-ip>:80"
```

> If you later add an ACME cert-resolver to the k3s Traefik itself and prefer it
> to terminate TLS, move `20-ingressroute.yaml` to the `websecure` entrypoint and
> add a `tls: { certResolver: <name> }` block; then the edge can SNI-passthrough
> :443 instead. Not required for the setup above.

## How auto-deploy works

The `synodl` Deployment is annotated for Keel:

```yaml
keel.sh/policy: force          # redeploy when the tag's digest changes
keel.sh/trigger: poll
keel.sh/pollSchedule: "@every 2m"
```

Keel polls `ghcr.io/zuptalo/synodl:latest` every ~2 min; when a new production
release moves `:latest`, Keel forces a rolling redeploy. (To track the rolling
dev build instead, point the image tag in `10-synodl.yaml` at `:develop`.) Keel
is installed once cluster-wide (shared with Ring); this folder does not reinstall
it.

## Verify

```sh
kubectl -n synodl rollout status deploy/synodl
kubectl -n synodl logs deploy/synodl | head        # "synodl starting version=..."
# In-cluster health (no state, so ready ~immediately):
kubectl -n synodl exec deploy/synodl -- wget -qO- http://127.0.0.1:8080/healthz
# End to end, once DNS + edge TLS are live:
curl -fsS https://synodl.example.com/healthz && echo OK
```

Then open `https://synodl.example.com`, install the PWA, and log in with your DSM
account. The `sid` is stored on the client; the server keeps nothing.

## Files

| File | What |
|---|---|
| `00-namespace.yaml` | `synodl` namespace |
| `10-synodl.yaml` | ConfigMap (ENV/PORT/ALLOWED_ORIGINS/SYNO_URL/SYNO_TLS_INSECURE) + Deployment (Keel-annotated, stateless) + Service |
| `20-ingressroute.yaml` | Traefik HTTP IngressRoute on the `web` entrypoint (Host → synodl:8080) |
| `install.sh` | idempotent installer (substitute settings, apply, roll) |

No `Secret` and no Postgres manifest exist here **by design** — the proxy is
stateless and credential-free. The host-specific values (`APP_HOST`, `SYNO_URL`,
`SYNO_TLS_INSECURE`) are substituted at apply time and are **not** committed.
