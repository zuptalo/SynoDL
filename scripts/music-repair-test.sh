#!/usr/bin/env bash
# Runs the music-repair unit tests inside the PINNED worker image — the exact
# runtime the repair Job uses (Python, mutagen, ffmpeg). Running them anywhere
# else would let the tag, cover and conversion tests skip and pass green while
# testing nothing, so MUSIC_REPAIR_REQUIRE_DEPS=1 turns a missing dependency
# into a failure.
#
#   scripts/music-repair-test.sh                 # all tests
#   scripts/music-repair-test.sh -k names        # extra args go to unittest
set -euo pipefail
cd "$(dirname "$0")/.."
# The manifest is an install-time template now, so its literal value is
# ${YTDL_IMAGE}. Keep one real default in install.sh and read that when neither
# caller override is present. This makes local and CI tests exercise the same
# pinned image a default installation receives without duplicating the pin.
DEFAULT_IMAGE="$(sed -n 's/^: "${YTDL_IMAGE:=\([^}]*\)}"$/\1/p' deploy/k8s/install.sh)"
IMAGE="${MUSIC_REPAIR_IMAGE:-${YTDL_IMAGE:-$DEFAULT_IMAGE}}"
[ -n "$IMAGE" ] || { echo "could not resolve the pinned music-repair image" >&2; exit 2; }
exec docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD:/repo" -w /repo/scripts \
  -e MUSIC_REPAIR_REQUIRE_DEPS=1 -e PYTHONDONTWRITEBYTECODE=1 -e HOME=/tmp \
  --entrypoint python3 "$IMAGE" \
  -m unittest discover -s music_repair -p 'test_*.py' "$@"
