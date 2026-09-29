# Quickstart: repairing the library

```sh
# 0. Snapshot the music share on the NAS first (Snapshot Replication). .trash protects
#    against a wrong plan, not against a lost volume.

# 1. Make the plan. Changes nothing. Takes ~2 h the first time (MusicBrainz = 1 req/s).
scripts/music-repair.sh plan
#   -> prints  plan id 20260928T201500Z-a1b2c3   and the totals

# 2. Read it. On the share:  .repair/plan-<id>.md   (and plan-<id>.json)

# 3. Apply exactly that plan.
scripts/music-repair.sh apply 20260928T201500Z-a1b2c3

# 4. Check, then let the media server rescan. To look at what was set aside:  .trash/<id>/
#    Changed your mind?  scripts/music-repair.sh restore 20260928T201500Z-a1b2c3

# Run again any time: it finds nothing to do, or only your newly downloaded files.
```

Local, offline sanity run (never writes to the library):

```sh
python3 -m music_repair plan --library /Volumes/music --repair-dir /tmp/repair --no-lookup
```

Tests: `python3 -m unittest discover -s scripts/music_repair -p 'test_*.py'`
