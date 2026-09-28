"""The planner: decide, for every file, what SHOULD happen. Pure; never writes.

`build_plan` takes what the library IS (an Inventory) and what the sources said
(lookup Results) and returns a Plan: an ordered list of actions, each with a
reason a person can read. Nothing here touches the library, and that is the
point — the dry run is this function plus a report, so "the dry run changes
nothing" is true by construction rather than by care.

The rules it encodes, each from the spec:

  * A song is its YouTube video id; one copy survives (largest, then earliest,
    then path). Files with no id are kept and reported, never merged.
  * A playlist is a TITLE. Every folder of that title, under any artist, feeds one
    playlist file. Singles is not a playlist; an album folder is both.
  * With a confident match a track files under its LEAD artist and the source's
    album, with a track number. Without one it keeps its uploader folder — cleaned
    of channel markers — and goes to Singles, unless its old folder was already a
    real album, in which case it keeps that. Nothing moves to another artist on a
    guess.
  * A path is only ever produced by sanitize_name, and two things wanting one path
    are a conflict: neither moves.
"""

from __future__ import annotations

import hashlib
import os
import posixpath
import subprocess
import time
from dataclasses import dataclass

from . import identity, names, playlists
from .inventory import Inventory, Track
from .sources import Result

SINGLES = "Singles"
TRASH = ".trash"
RANK = {"convert": 0, "retag": 1, "move": 2, "sidecar": 3, "cover": 4, "rename_bin": 5,
        "playlist": 6, "merge_dir": 7, "orphan_nfo": 8, "trash": 9, "conflict": 10}
COVER_BYTES_ESTIMATE = 600 * 1024


class RealFs:
    """The little IO the planner needs beyond the inventory. Read-only."""

    def __init__(self, library: str):
        self.library = library

    def stat(self, rel):
        st = os.stat(os.path.join(self.library, rel))
        return st.st_size, st.st_mtime_ns

    def head(self, rel, n):
        with open(os.path.join(self.library, rel), "rb") as f:
            return f.read(n)

    def read_text(self, rel):
        try:
            with open(os.path.join(self.library, rel), encoding="utf-8", errors="replace") as f:
                return f.read()
        except OSError:
            return None

    def has_audio(self, rel):
        try:
            out = subprocess.run(
                ["ffprobe", "-v", "error", "-select_streams", "a", "-show_entries", "stream=codec_type",
                 "-of", "csv=p=0", os.path.join(self.library, rel)],
                capture_output=True, text=True, timeout=60)
            return "audio" in out.stdout
        except Exception:
            return False


@dataclass
class Plan:
    id: str
    created: str
    library: str
    library_fingerprint: str
    totals: dict
    actions: list
    skipped: list
    unidentified: list
    version: int = 1

    def to_dict(self) -> dict:
        return {"version": self.version, "id": self.id, "created": self.created, "library": self.library,
                "library_fingerprint": self.library_fingerprint, "totals": self.totals,
                "actions": self.actions, "skipped": self.skipped, "unidentified": self.unidentified}

    @property
    def changes(self) -> list:
        """Actions that alter the library. A conflict is reported, not carried out."""
        return [a for a in self.actions if a["kind"] != "conflict"]


def needs_lookup(track: Track) -> bool:
    """Should this track be resolved against the sources on this run?

    Unsettled: always (the cache answers most of them for free). Settled: only when
    its earlier lookup never completed (FR-013) — a song already answered is not
    asked about again.
    """
    return (not track.settled) or track.repair_status == "not_looked_up"


def parse_kept(track: Track) -> names.Parsed:
    """Artist, title and featured artists for a track — what a lookup asks about.

    The tag artist is cleaned of channel markers FIRST, so "10ccVEVO" can agree with
    a title that says "10cc - I'm Not in Love".
    """
    return names.split_artist_title(track.tag_title or _stem(track.relpath),
                                    names.clean_artist_folder(track.tag_artist or track.folder_artist))


def _stem(rel: str) -> str:
    return posixpath.splitext(posixpath.basename(rel))[0]


def _fingerprint(tracks: list[Track]) -> str:
    h = hashlib.sha256()
    for t in sorted(tracks, key=lambda t: t.relpath):
        h.update(f"{t.relpath}\t{t.size}\t{t.mtime_ns}\n".encode())
    return h.hexdigest()[:16]


def build_plan(inv: Inventory, results: dict[str, Result], *, plan_id: str, fs=None,
               created: str | None = None) -> Plan:
    return _Builder(inv, results, plan_id, fs or RealFs(inv.library),
                    created or time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())).build()


class _Builder:
    def __init__(self, inv, results, plan_id, fs, created):
        self.inv, self.results, self.id, self.fs, self.created = inv, results, plan_id, fs, created
        self.actions: list[dict] = []
        self.skipped: list[dict] = []
        self.staying: set[str] = set()          # audio that will still be where it is
        self.taken: dict[str, str] = {}         # destination -> what wants it
        self.existing: set[str] = set()
        self.totals = {k: 0 for k in (
            "tracks", "songs", "unidentified", "duplicates_to_trash", "moves", "retags", "covers", "playlists",
            "conversions", "conflicts", "orphan_nfo", "bytes_reclaimed", "bytes_needed", "matched", "no_match",
            "not_looked_up", "to_singles", "album_known", "already_settled", "name_clashes")}

    # ---- helpers ---------------------------------------------------------

    def add(self, kind, reason, *, src=None, dst=None, track=None, **extra):
        a = {"kind": kind, "reason": reason}
        if src is not None:
            a["src"] = src
            if track is not None:
                a["size"], a["mtime_ns"] = track.size, track.mtime_ns
            else:
                try:
                    a["size"], a["mtime_ns"] = self.fs.stat(src)
                except OSError:
                    pass
        if dst is not None:
            a["dst"] = dst
        a.update(extra)
        self.actions.append(a)
        return a

    def skip(self, rel, reason):
        self.skipped.append({"relpath": rel, "reason": reason})

    def trash(self, rel, reason, track=None):
        return self.add("trash", reason, src=rel, dst=f"{TRASH}/{self.id}/{rel}", track=track)

    # ---- naming ----------------------------------------------------------

    def _canonical_artists(self):
        folders = {t.folder_artist for t in self.inv.tracks} | {r.split("/")[0] for r in self.existing if "/" in r}
        self.canon: dict[str, str] = {}
        # Prefer a folder that is already clean, so `10cc` and `10ccVEVO` become one.
        for f in sorted(folders, key=lambda f: (names.clean_artist_folder(f) != f, f)):
            c = names.clean_artist_folder(f)
            self.canon.setdefault(names.fold(c), c)

    def artist_dir(self, display: str) -> str:
        display = names.clean_artist_folder(display)
        c = self.canon.get(names.fold(display)) or display
        out = names.sanitize_name(c) or "Unknown Artist"
        # `Playlists/` is ours. An artist of that name must not share the folder.
        return "Playlists (artist)" if out.casefold() == "playlists" else out

    def _folder_artists(self):
        self.folder_artists: dict[tuple, list[str]] = {}
        for t in self.inv.tracks:
            if t.folder_playlist:
                self.folder_artists.setdefault((t.folder_artist, t.folder_playlist), []).append(
                    names.clean_artist_folder(t.tag_artist or t.folder_artist))

    def album_from_folders(self, copies: list[Track]) -> str | None:
        for c in copies:
            if c.folder_playlist:
                a = names.album_of_folder(c.folder_playlist, self.folder_artists.get(
                    (c.folder_artist, c.folder_playlist), []))
                if a:
                    return a
        return None

    def twin_of(self, artist: str, folder: str) -> str | None:
        """The clean sibling of a folder whose name differs only in punctuation."""
        for d in self.dirs:
            head, _, name = d.rpartition("/")
            if head == artist and name != folder and names.fold(name) == names.fold(folder):
                return name
        return None

    # ---- the build -------------------------------------------------------

    def build(self) -> Plan:
        inv = self.inv
        self.existing = {t.relpath for t in inv.tracks} | {t.lyrics for t in inv.tracks if t.lyrics}
        for paths in inv.other.values():
            self.existing.update(paths)
        self.dirs = {posixpath.dirname(p) for p in self.existing if "/" in p}
        self._canonical_artists()
        self._folder_artists()
        self.totals["tracks"] = len(inv.tracks)

        songs, unidentified = identity.group(inv.tracks)
        self.totals["songs"], self.totals["unidentified"] = len(songs), len(unidentified)
        self.pl: dict[str, dict] = {}       # playlist key -> {title, entries}
        for t in unidentified:
            self.skip(t.relpath, "no video id (kept where it is, never merged on a guess)")
            self.staying.add(t.relpath)
            self._join_playlists(t, [t], playlists.Entry(t.tag_artist or t.folder_artist, t.tag_title or _stem(t.relpath),
                                                         t.relpath, t.duration_s))
        for rel, why in inv.unreadable:
            self.skip(rel, f"unreadable ({why})")
            self.staying.add(rel)
        for rel in inv.symlinks:
            self.skip(rel, "symbolic link (not followed)")

        self._songs(songs)
        self._webm()
        self._bins()
        self._playlists()
        self._colon_dirs()
        self._artist_files()
        self._orphan_nfo()
        return self._finish(unidentified)

    # ---- songs -----------------------------------------------------------

    def _songs(self, songs):
        planned = []
        for s in songs:
            kept = s.kept
            res = self.results.get(s.key)
            parsed = parse_kept(kept)
            replan = (not kept.settled) or kept.repair_status == "not_looked_up"
            if kept.settled and not replan:
                self.totals["already_settled"] += 1
            match = res.match if res and res.status == "matched" else None
            if replan:
                # No result at all (a plan made with --no-lookup) is "not looked up",
                # which a later run retries — never silently "no match".
                status = res.status if res is not None else "not_looked_up"
                self.totals[status if status in ("matched", "no_match", "not_looked_up") else "no_match"] += 1
            planned.append((s, kept, res, match, parsed, replan))

        # Destination of each kept file, then name clashes across the whole plan.
        dest: dict[str, str | None] = {}
        for s, kept, res, match, parsed, replan in planned:
            if not replan:
                dest[s.key] = None
                continue
            if kept.settled and (res is None or res.status != "matched"):
                dest[s.key] = None            # a settled file never moves back on a non-answer
                continue
            dest[s.key] = self._destination(s, kept, match, parsed)
        src_of = {s.key: kept.relpath for s, kept, *_ in planned}
        wanted: dict[str, list[str]] = {}
        for key in sorted(dest):
            if dest[key]:
                wanted.setdefault(dest[key], []).append(key)

        # Two different videos of one title — "Easy On Me (Official Video)" and
        # "(Official Lyric Video)" — are two songs to us (identity is the video id)
        # and want one file name. Both are KEPT: the one that is already there, or
        # else the first by video id, keeps the plain name, and the others carry
        # their video id in the file name. The TITLE tag stays clean. Only when even
        # that name is taken does anything become a conflict, and then nothing moves.
        clash: dict[str, str] = {}
        conflicted: set[str] = set()
        taken = set(self.existing) | set(wanted)
        for d, keys in sorted(wanted.items()):
            occupied = d in self.existing and any(src_of[k] != d for k in keys)
            if len(keys) == 1 and not occupied:
                continue
            owner = next((k for k in keys if src_of[k] == d), None)
            if owner is None and not occupied:
                owner = keys[0]
            for k in keys:
                if k == owner:
                    continue
                named = f"{d[: -len('.mp3')]} [{k}].mp3"
                if named in taken:
                    conflicted.add(k)
                    continue
                taken.add(named)
                dest[k] = named
                others = [x for x in keys if x != k]
                clash[k] = ("same title as " + (f"video {others[0]}" if others else "a file already there")
                            + "; both are kept, this one named with its video id")
        for d in dest.values():
            if d:
                self.taken[d] = "a song"

        for s, kept, res, match, parsed, replan in planned:
            self._one_song(s, kept, res, match, parsed, replan, dest[s.key], s.key in conflicted,
                           clash.get(s.key), wanted)

    def _destination(self, s, kept, match, parsed) -> str:
        album_folder = self.album_from_folders(s.all_copies)
        if match:
            adir = self.artist_dir(match.artist)
            album = match.album or album_folder
            no = match.track_no if match.album else None
        else:
            adir = self.artist_dir(kept.folder_artist)
            album, no = album_folder, None
        album_dir = names.sanitize_name(album) if album else ""
        album_dir = album_dir or SINGLES
        name = names.sanitize_name(parsed.title) or _stem(kept.relpath)
        prefix = f"{int(no):02d} - " if isinstance(no, int) and no > 0 else ""
        return f"{adir}/{album_dir}/{prefix}{name}.mp3"

    def _one_song(self, s, kept, res, match, parsed, replan, dst, conflict, clash, wanted):
        final = kept.relpath
        if conflict:
            self.add("conflict", f"{dst} and its video-id name are both taken; nothing was moved",
                     src=kept.relpath, dst=dst, track=kept)
            self.totals["conflicts"] += 1
            self.staying.add(kept.relpath)
        elif replan:
            status = res.status if res else "not_looked_up"
            if dst is None:
                # A settled file whose lookup had not finished. Only its status
                # can change, and only when the lookup now finished with a "no".
                if res and res.status == "no_match":
                    self.add("retag", "lookup finished: no confident match", src=kept.relpath, track=kept,
                             tags={"repair": self.id, "repair_status": "no_match"})
                    self.totals["retags"] += 1
                self.staying.add(kept.relpath)
            else:
                tags = self._tags(kept, parsed, match, dst, status)
                why = self._why(match, res)
                if clash:
                    why += f"; {clash}"
                    self.totals["name_clashes"] += 1
                self.add("retag", why, src=kept.relpath, track=kept, tags=tags)
                self.totals["retags"] += 1
                if dst != kept.relpath:
                    self.add("move", why, src=kept.relpath, dst=dst, track=kept)
                    self.totals["moves"] += 1
                    final = dst
                else:
                    self.staying.add(kept.relpath)
                if dst.split("/")[1] == SINGLES:
                    self.totals["to_singles"] += 1
                elif match and match.album:
                    self.totals["album_known"] += 1
                if match and match.cover_url and match.album:
                    self.add("cover", f"cover art for {match.album} ({match.source})", src=None, dst=None,
                             audio=final, folder=posixpath.dirname(final), cover_url=match.cover_url)
                    self.totals["covers"] += 1
        else:
            self.staying.add(kept.relpath)

        # Lyrics follow the kept audio; every other copy's sidecar is set aside.
        lyrics = kept.lyrics
        chosen = lyrics or next((c.lyrics for c in s.copies if c.lyrics), None)
        target = final[: -len(".mp3")] + ".lrc"
        for c in s.all_copies:
            if c.lyrics and c.lyrics != chosen:
                self.trash(c.lyrics, f"lyrics of a duplicate ({s.key})")
        if chosen and chosen != target:
            self.add("sidecar", "lyrics follow the kept audio", src=chosen, dst=target, audio=final)
        for c in s.copies:
            self.trash(c.relpath, f"duplicate of {kept.relpath} (video {s.key})", track=c)
            self.totals["duplicates_to_trash"] += 1
            self.totals["bytes_reclaimed"] += c.size

        entry = playlists.Entry(parsed.lead, parsed.title, final, kept.duration_s)
        self._join_playlists(kept, s.all_copies, entry)

    def _why(self, match, res):
        if match:
            return (f"matched by {match.source} (artist and title equal, length within "
                    f"{abs(match.delta_s):.1f}s)")
        if res and res.status == "not_looked_up":
            return "no source could be reached; filed without a match and retried on the next run"
        if res and res.status == "no_match":
            near = f"; nearest length off by {abs(res.nearest_delta_s):.0f}s" if res.nearest_delta_s is not None else ""
            return "no confident match" + near
        return "not looked up"

    def _tags(self, kept, parsed, match, dst, status):
        album_folder = None
        if not match:
            album_folder = dst.split("/")[1]
        tags = {"title": parsed.title, "original_title": kept.tag_title or _stem(kept.relpath),
                "repair": self.id, "repair_status": status if status in ("matched", "no_match", "not_looked_up") else "no_match"}
        if match:
            tags["artist"] = match.artist
            tags["albumartist"] = match.artist
            tags["album"] = match.album or dst.split("/")[1]
            if match.track_no and match.album:
                tags["track"] = str(match.track_no)
            if match.year:
                tags["date"] = str(match.year)
            if match.mbid_recording:
                tags["musicbrainz_recording"] = match.mbid_recording
            if match.mbid_release:
                tags["musicbrainz_release"] = match.mbid_release
        else:
            tags["artist"] = parsed.lead or self.artist_dir(kept.folder_artist)
            tags["albumartist"] = dst.split("/")[0] if dst else self.artist_dir(kept.folder_artist)
            tags["album"] = album_folder or SINGLES
        if parsed.featured:
            tags["featured"] = "; ".join(parsed.featured)
        return tags

    # ---- playlists -------------------------------------------------------

    def _join_playlists(self, track, copies, entry):
        titles = set()
        for c in copies:
            # Singles is where unmatched tracks are FILED, not a playlist anyone made.
            if c.settled or not c.folder_playlist or c.folder_playlist.casefold() == SINGLES.casefold():
                continue
            t = c.folder_playlist
            twin = self.twin_of(c.folder_artist, t) if ":" in t else None
            titles.add(twin or t)
        for t in titles:
            slot = self.pl.setdefault(playlists.key(t), {"title": t, "entries": {}})
            slot["title"] = min(slot["title"], t)
            slot["entries"][entry.relpath] = entry

    def _playlists(self):
        files = playlists.filenames([p["title"] for p in self.pl.values()])
        for slot in sorted(self.pl.values(), key=lambda p: p["title"]):
            entries = sorted(slot["entries"].values(), key=lambda e: (names.fold(e.artist), names.fold(e.title)))
            # A playlist that already lists every one of these is not a change. Without
            # this an unidentified track left in its old folder would re-propose
            # itself on every run, and "run twice, the second finds nothing" (SC-004)
            # would be false.
            existing = self.fs.read_text(f"{playlists.PLAYLIST_DIR}/{files[slot['title']]}")
            if existing is not None and {e.relpath for e in entries} <= {e.relpath for e in playlists.parse(existing)}:
                continue
            self.add("playlist", f"{len(entries)} songs from folders titled '{slot['title']}'",
                     dst=f"{playlists.PLAYLIST_DIR}/{files[slot['title']]}", title=slot["title"],
                     entries=[{"artist": e.artist, "title": e.title, "relpath": e.relpath,
                               "duration_s": round(e.duration_s, 1)} for e in entries])
            self.totals["playlists"] += 1

    # ---- leftovers -------------------------------------------------------

    def _webm(self):
        for rel in self.inv.other.get("webm", []):
            if not self.fs.has_audio(rel):
                self.skip(rel, "no audio stream")
                continue
            parts = rel.split("/")
            folder = parts[1] if len(parts) == 3 else None
            artists = self.folder_artists.get((parts[0], folder), []) if folder else []
            album = names.album_of_folder(folder, artists) if folder else None
            parsed = names.split_artist_title(_stem(rel), names.clean_artist_folder(parts[0]))
            adir = self.artist_dir(parts[0])
            dst = f"{adir}/{names.sanitize_name(album) if album else SINGLES}/{names.sanitize_name(parsed.title)}.mp3"
            if dst in self.existing or dst in self.taken:
                self.skip(rel, f"cannot convert: {dst} already exists")
                continue
            self.taken[dst] = rel
            self.add("convert", "webm carrying audio becomes an mp3 like any other track", src=rel, dst=dst,
                     tags={"title": parsed.title, "artist": parsed.lead, "albumartist": adir,
                           "album": album or SINGLES, "original_title": _stem(rel), "repair": self.id,
                           "repair_status": "no_match"})
            self.trash(rel, "converted to mp3")
            self.totals["conversions"] += 1
            try:
                self.totals["bytes_needed"] += self.fs.stat(rel)[0]
            except OSError:
                pass

    _MAGIC = ((b"\x89PNG\r\n\x1a\n", ".png"), (b"\xff\xd8\xff", ".jpg"), (b"GIF8", ".gif"))

    def _bins(self):
        for rel in self.inv.other.get("bin", []):
            head = self.fs.head(rel, 16)
            ext = next((e for m, e in self._MAGIC if head.startswith(m)), None)
            if ext is None and head[:4] == b"RIFF" and head[8:12] == b"WEBP":
                ext = ".webp"
            dst = posixpath.splitext(rel)[0] + (ext or "")
            if ext and dst not in self.existing:
                self.add("rename_bin", f"a {ext[1:]} image with the wrong extension", src=rel, dst=dst)
            else:
                self.trash(rel, "not an image" if not ext else f"{dst} already exists")

    def _colon_dirs(self):
        for d in self.inv.colon_dirs:
            artist, _, name = d.partition("/")
            twin = self.twin_of(artist, name)
            self.add("merge_dir", "a folder name the mount cannot list" +
                     (f"; its tracks are filed like its twin '{twin}'" if twin else "; its tracks are filed by album"),
                     src=d, dst=f"{artist}/{twin}" if twin else None)

    def _artist_files(self):
        """When an artist folder is renamed, its nfo and artwork go with it."""
        by_artist: dict[str, list[str]] = {}
        for kind in ("nfo", "image"):
            for rel in self.inv.other.get(kind, []):
                if rel.count("/") == 1:
                    by_artist.setdefault(rel.split("/")[0], []).append(rel)
        for artist, files in sorted(by_artist.items()):
            target = self.artist_dir(artist)
            if target == artist or any(s.startswith(artist + "/") for s in self.staying):
                continue
            for rel in sorted(files):
                dst = f"{target}/{posixpath.basename(rel)}"
                if dst in self.existing or dst in self.taken:
                    self.trash(rel, f"{target} already has its own {posixpath.basename(rel)}")
                else:
                    self.taken[dst] = rel
                    self.add("move", f"artist folder '{artist}' becomes '{target}'", src=rel, dst=dst)

    def _orphan_nfo(self):
        """A media-server nfo whose audio is gone is an orphan; nothing else is (FR-016)."""
        already = {a.get("src") for a in self.actions if a["kind"] in ("trash", "move")}
        for rel in self.inv.other.get("nfo", []):
            if rel.count("/") != 2 or rel in already:
                continue
            folder = posixpath.dirname(rel)
            if any(posixpath.dirname(s) == folder for s in self.staying):
                continue
            self.add("orphan_nfo", "its tracks were moved or set aside; the media server rewrites it on its next scan",
                     src=rel, dst=f"{TRASH}/{self.id}/{rel}")
            self.totals["orphan_nfo"] += 1

    # ---- finish ----------------------------------------------------------

    def _finish(self, unidentified):
        self.actions.sort(key=lambda a: (RANK[a["kind"]], a.get("src") or a.get("dst") or "", a.get("dst") or ""))
        for i, a in enumerate(self.actions, 1):
            a["id"] = f"a{i:06d}"
        ordered = []
        for a in self.actions:
            ordered.append({"id": a.pop("id"), **a})
        self.totals["bytes_needed"] += self.totals["covers"] * COVER_BYTES_ESTIMATE + 50_000 * self.totals["playlists"]
        return Plan(id=self.id, created=self.created, library=self.inv.library,
                    library_fingerprint=_fingerprint(self.inv.tracks), totals=self.totals, actions=ordered,
                    skipped=sorted(self.skipped, key=lambda s: s["relpath"]),
                    unidentified=[t.relpath for t in unidentified])
