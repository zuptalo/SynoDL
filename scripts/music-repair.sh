#!/usr/bin/env bash
# Repair the music library, safely, as a one-shot Kubernetes Job (spec 1052).
#
#   scripts/music-repair.sh plan              # read the library, write a reviewable plan. Changes nothing else.
#   scripts/music-repair.sh apply <plan-id>   # carry out exactly that plan
#   scripts/music-repair.sh restore <plan-id> # undo an applied plan
#   scripts/music-repair.sh status [<plan-id>]# what the last Job said
#
# Read docs/MUSIC-LIBRARY-REPAIR.md first, and snapshot the music share on the NAS
# before the first `apply`: .trash protects against a wrong plan, not a lost volume.
#
# Environment: NAMESPACE (default synodl), CONTEXT (default: kubectl's current).
# Exit codes: 0 ok · 2 bad arguments · 3 the Job failed · 4 refused.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
NAMESPACE="${NAMESPACE:-synodl}"
KUBECTL=(kubectl)
[ -n "${CONTEXT:-}" ] && KUBECTL+=(--context "$CONTEXT")
KUBECTL+=(-n "$NAMESPACE")

usage() { sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//'; exit 2; }

cmd="${1:-}"
[ -n "$cmd" ] || usage
shift || true

case "$cmd" in
  plan)                     [ $# -eq 0 ] || usage ;;
  apply|restore)            [ $# -eq 1 ] || { echo "$cmd needs a plan id (printed by 'plan')" >&2; exit 2; } ;;
  status)                   [ $# -le 1 ] || usage ;;
  -h|--help|help)           usage ;;
  *)                        echo "unknown command: $cmd" >&2; usage ;;
esac
PLAN_ID="${1:-}"

# A plan id is printed by `plan` and passed back here, so it is validated before it
# goes anywhere near a manifest or a command line.
if [ -n "$PLAN_ID" ] && ! [[ "$PLAN_ID" =~ ^[0-9]{8}T[0-9]{6}Z-[0-9a-f]{6}$ ]]; then
  echo "that is not a plan id: $PLAN_ID" >&2
  exit 2
fi

# The image and the user come from the LIVE config, so the repair can never run a
# different worker image, or as a different user, than the downloads do.
cfg() { "${KUBECTL[@]}" get configmap synodl-config -o "jsonpath={.data.$1}"; }

render() {  # render <job-name> <configmap> <json-args>
  local image uid gid claim
  # The MUSIC_REPAIR_* overrides exist so the manifest can be rendered and tested with no cluster.
  image="${MUSIC_REPAIR_IMAGE:-$(cfg YTDL_IMAGE)}"; uid="${MUSIC_REPAIR_UID:-$(cfg YTDL_UID)}"
  gid="${MUSIC_REPAIR_GID:-$(cfg YTDL_GID)}"; claim="${MUSIC_REPAIR_CLAIM:-$(cfg YTDL_MUSIC_CLAIM)}"
  [ -n "$image" ] && [ -n "$uid" ] && [ -n "$gid" ] && [ -n "$claim" ] \
    || { echo "synodl-config is missing YTDL_IMAGE / YTDL_UID / YTDL_GID / YTDL_MUSIC_CLAIM" >&2; exit 4; }
  sed -e "s|__JOB_NAME__|$1|g" -e "s|__NAMESPACE__|$NAMESPACE|g" -e "s|__IMAGE__|$image|g" \
      -e "s|__UID__|$uid|g" -e "s|__GID__|$gid|g" -e "s|__CLAIM__|$claim|g" \
      -e "s|__CONFIGMAP__|$2|g" -e "s|__ARGS__|$3|g" "$HERE/../deploy/k8s/music-repair-job.yaml.tpl"
}

# `--dry-run` prints the manifest and touches nothing (used by the tests).
if [ "${MUSIC_REPAIR_DRY_RUN:-}" = "1" ]; then
  case "$cmd" in
    plan)    ARGS='["plan","--library","/library"]' ;;
    apply)   ARGS="[\"apply\",\"--library\",\"/library\",\"--plan\",\"$PLAN_ID\"]" ;;
    restore) ARGS="[\"restore\",\"--library\",\"/library\",\"--plan\",\"$PLAN_ID\"]" ;;
    *)       echo "nothing to render for $cmd" >&2; exit 2 ;;
  esac
  render "music-repair-dry" "music-repair-code" "$ARGS"
  exit 0
fi

stamp="$(date -u +%Y%m%d%H%M%S)"
job="music-repair-$cmd-$stamp"
code="music-repair-code-$stamp"

case "$cmd" in
  status)
    last="$("${KUBECTL[@]}" get jobs -l app.kubernetes.io/name=synodl-music-repair --sort-by=.metadata.creationTimestamp -o name | tail -1)"
    [ -n "$last" ] || { echo "no music-repair Job found" >&2; exit 4; }
    "${KUBECTL[@]}" get "$last"
    "${KUBECTL[@]}" logs "$last" --tail=40
    exit 0 ;;
  plan)    ARGS='["plan","--library","/library"]' ;;
  apply)   ARGS="[\"apply\",\"--library\",\"/library\",\"--plan\",\"$PLAN_ID\"]" ;;
  restore) ARGS="[\"restore\",\"--library\",\"/library\",\"--plan\",\"$PLAN_ID\"]" ;;
esac

# The code ships as a ConfigMap: exactly what is in this checkout, no new image.
files=("$HERE"/music_repair/[a-z]*.py "$HERE"/music_repair/__init__.py "$HERE"/music_repair/__main__.py "$HERE"/music_repair/names_cases.json)
args=()
for f in "${files[@]}"; do
  case "$(basename "$f")" in test_*|fixtures.py) continue ;; esac
  args+=(--from-file="$(basename "$f")=$f")
done
"${KUBECTL[@]}" create configmap "$code" "${args[@]}" >/dev/null

# Until the Job exists the ConfigMap is ours to clean up. Once it does, the Job OWNS it
# (below): deleting it on exit — Ctrl-C, or the give-up at the end — would pull the code out
# from under a Job that is still pending or running.
cleanup() { "${KUBECTL[@]}" delete configmap "$code" --ignore-not-found >/dev/null 2>&1 || true; }
trap cleanup EXIT

render "$job" "$code" "$ARGS" | "${KUBECTL[@]}" apply -f - >/dev/null
uid="$("${KUBECTL[@]}" get "job/$job" -o jsonpath='{.metadata.uid}')"
if ! "${KUBECTL[@]}" patch configmap "$code" --type merge -p \
     "{\"metadata\":{\"ownerReferences\":[{\"apiVersion\":\"batch/v1\",\"kind\":\"Job\",\"name\":\"$job\",\"uid\":\"$uid\"}]}}" >/dev/null; then
  "${KUBECTL[@]}" delete "job/$job" --ignore-not-found >/dev/null 2>&1 || true   # fail closed: no Job without its code
  echo "could not attach the code to the Job; nothing was left running" >&2
  exit 3
fi
trap - EXIT   # the ConfigMap now lives and dies with the Job (ttlSecondsAfterFinished)
echo "started Job $job — following its log (Ctrl-C stops watching, not the Job)"
"${KUBECTL[@]}" wait --for=condition=ready pod -l job-name="$job" --timeout=180s >/dev/null 2>&1 || true
"${KUBECTL[@]}" logs -f "job/$job" || true

# Wait for a terminal state, then report it as the exit code.
for _ in $(seq 1 600); do
  if "${KUBECTL[@]}" get "job/$job" -o jsonpath='{.status.succeeded}' | grep -q 1; then exit 0; fi
  if "${KUBECTL[@]}" get "job/$job" -o jsonpath='{.status.failed}' | grep -q 1; then
    echo "the Job failed — read the log above (exit 3 = a step failed, 4 = refused)" >&2; exit 3
  fi
  sleep 5
done
echo "gave up waiting; the Job may still be running: scripts/music-repair.sh status" >&2
exit 3
