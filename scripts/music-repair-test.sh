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
IMAGE="${MUSIC_REPAIR_IMAGE:-$(grep -E '^\s*YTDL_IMAGE:' deploy/k8s/10-synodl.yaml | head -1 | sed -E 's/.*"([^"]+)".*/\1/')}"
exec docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD/scripts:/s" -w /s \
  -e MUSIC_REPAIR_REQUIRE_DEPS=1 -e PYTHONDONTWRITEBYTECODE=1 -e HOME=/tmp \
  --entrypoint python3 "$IMAGE" \
  -m unittest discover -s music_repair -p 'test_*.py' "$@"
