# Quickstart: running the repair from Settings

1. Sign in as an admin. Settings → **Music library repair**.
2. **Check library.** It starts a worker; the modal shows the stage and a progress bar and
   can be closed and reopened. The first pass over a large library takes hours (the
   metadata sources allow about one request a second); later passes are minutes because
   answers are cached on the share.
3. Read the result: duplicates, moves, retags, playlists, conflicts, songs matched / not
   matched, space needed against free, and what was left alone. The complete plan is in
   the `.repair` folder of your music library on the NAS.
4. **Apply this plan.** Tick "I have taken a snapshot of the music share", then confirm.
   It runs and shows progress. A check is good for 24 hours.
5. **Undo** puts back what the last apply changed. **History** lists every run.

From the command line, `scripts/music-repair.sh` still works. A run started there is seen
here as "a repair is running from the command line" and blocks a second run.

Dev / e2e: set `MUSIC_REPAIR_IMAGE` (there is no server pod to read the image from). The mock
cluster (`make mockk8s`) runs the Jobs' lifecycle; drive it with `/__mock/jobs/{name}/emit`
and `/succeed`.
