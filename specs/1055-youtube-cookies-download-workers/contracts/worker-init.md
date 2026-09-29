# Contract: worker init step

Container `signin`, same pinned image and uid/gid as the downloader, mounts only the memory volume at `/signin`. Constant command (no user input), values only via env:

```
set -u; umask 077
f=/signin/cookies.txt
printf '# Netscape HTTP Cookie File\n' > "$f"
wget -q -T 10 -O "$f.tmp" --header "Authorization: Bearer $SYNODL_SIGNIN_GRANT" "$SYNODL_SIGNIN_URL" \
  && [ -s "$f.tmp" ] && mv "$f.tmp" "$f"
rm -f "$f.tmp"; exit 0
```
Exit is always 0 (fail open). The downloader gets `--cookies /signin/cookies.txt` as two discrete argv elements, only when the Job carries the sign-in.
