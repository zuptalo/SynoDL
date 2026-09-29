"""Repair a SynoDL music library: one copy per song, clean names, real metadata.

An OPERATOR tool, not part of the synodl server (spec 1052). It runs as a
short-lived Job in the pinned worker image, mounting exactly one library, and it
is deliberately two steps: `plan` reads and writes only a reviewable plan;
`apply` carries out exactly that plan. See specs/1052-repair-music-library/.
"""

VERSION = "0.1.0"
