#!/usr/bin/env bash
# Idempotent installer for SynoDL on a k3s cluster. SynoDL is a stateless,
# credential-free proxy — there is NO database, NO secret to generate, and NO PVC.
# The installer just substitutes the operator settings into the manifests and
# applies them; re-running is safe and simply reconciles config changes.
#
#   APP_HOST=synodl.example.com SYNO_URL=https://nas.local:5001 ./install.sh
#
# Required env:
#   APP_HOST    public hostname the PWA is served on (also the CORS origin).
#   SYNO_URL    the single NAS this proxy forwards to, reachable FROM THE CLUSTER
#               (a LAN IP or a name the k3s node resolves), e.g.
#               https://192.168.1.10:5001  or  https://nas.example.com:5001
#
# Optional env:
#   SYNO_TLS_INSECURE   "true" to skip TLS verification of the NAS cert (set this
#                       for a self-signed / private-CA DSM cert). Default "false".
#   KUBECTL             kubectl binary (default kubectl).
#
# This installer manages only the k3s side. TLS is terminated by the edge proxy
# that fronts the cluster: point an HTTP router for $APP_HOST at the k3s node's
# :80 (the `web` entrypoint), preserving the Host header. See README.md.
#
# Keel (the cluster-wide image auto-updater) is assumed already installed — it is
# shared with the Ring deployment. This installer does NOT (re)install it; the
# synodl Deployment is merely annotated so the existing Keel picks it up.
set -euo pipefail

: "${APP_HOST:?set APP_HOST, e.g. APP_HOST=synodl.example.com}"
: "${SYNO_URL:?set SYNO_URL, e.g. SYNO_URL=https://nas.local:5001 (reachable from the cluster)}"
: "${SYNO_TLS_INSECURE:=false}"
KUBECTL="${KUBECTL:-kubectl}"
DIR="$(cd "$(dirname "$0")" && pwd)"
export APP_HOST SYNO_URL SYNO_TLS_INSECURE

command -v envsubst >/dev/null || { echo "need 'envsubst' (gettext). install it and re-run." >&2; exit 1; }

echo "==> namespace"
"$KUBECTL" create namespace synodl --dry-run=client -o yaml | "$KUBECTL" apply -f - >/dev/null

# SECRETS_KEY (spec 0003): generated exactly once and kept stable forever — it
# encrypts the stored NAS credentials + the VAPID private key at rest. Its
# presence switches the server to STATEFUL mode (setup wizard + SynoDL accounts).
# Re-running never rotates it (that would make the stored secrets unrecoverable).
# Set STATELESS=true to skip it and stay on the legacy SYNO_URL path.
if [ "${STATELESS:-}" = "true" ]; then
  echo "==> STATELESS=true — not creating SECRETS_KEY; legacy mode"
elif "$KUBECTL" -n synodl get secret synodl-secrets >/dev/null 2>&1; then
  echo "==> synodl-secrets exists; keeping SECRETS_KEY as-is (stateful)"
else
  echo "==> generating synodl-secrets (SECRETS_KEY) — BACK THIS UP with the volume"
  "$KUBECTL" -n synodl create secret generic synodl-secrets \
    --from-literal=SECRETS_KEY="$(openssl rand -hex 32)"
fi

echo "==> applying manifests for APP_HOST=$APP_HOST SYNO_URL=$SYNO_URL SYNO_TLS_INSECURE=$SYNO_TLS_INSECURE"
for f in 00-namespace 10-synodl 20-ingressroute; do
  envsubst '${APP_HOST} ${SYNO_URL} ${SYNO_TLS_INSECURE}' < "$DIR/${f}.yaml" | "$KUBECTL" apply -f -
done

# A ConfigMap change does not restart the pod on its own (envFrom is read at boot).
# Roll the Deployment so a re-run that changes SYNO_URL / origins takes effect now.
"$KUBECTL" -n synodl rollout restart deploy/synodl >/dev/null 2>&1 || true

echo "==> waiting for synodl"
"$KUBECTL" -n synodl rollout status deploy/synodl --timeout=120s || \
  echo "!!  synodl not ready yet; check: $KUBECTL -n synodl logs deploy/synodl"

cat <<EOF

Done. Next:
  1. Point DNS for ${APP_HOST} at your edge proxy, and set that edge proxy to
     TERMINATE TLS for ${APP_HOST} and forward HTTP to the k3s node's :80,
     preserving the Host header (see README.md for the Traefik snippet).
  2. Watch it come up:   $KUBECTL -n synodl rollout status deploy/synodl
  3. Open https://${APP_HOST}, install the PWA, and log in with your DSM account.

New production releases (:latest) auto-deploy within ~2 minutes via the shared Keel.
To track the rolling dev build instead, edit 10-synodl.yaml: image tag -> :develop.
EOF
