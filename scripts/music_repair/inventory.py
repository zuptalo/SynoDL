"""Inventory: walk the library and describe what is in it. Read-only.

One pass. Tags are read with mutagen straight from the ID3 header — no audio
decode, no subprocess per file — because the library is 5,800 files on NFS.

What this deliberately does NOT do:

  * follow symbolic links (FR-021). A link inside the library that points outside
    it is a way for a walk, and then a move, to leave the library.
  * look inside `.repair/`, `.trash/` or `Playlists/`: those are the repair's own
    output, and re-reading them would make a second run see its first one.
"""

from __future__ import annotations

import os
import re
import time
import unicodedata
from dataclasses import dataclass, field

# The repair's own folders. Skipped at the TOP level only: an artist folder that
# happens to be called "Playlists" is a real folder (the planner renames it).
_OWN_DIRS = (".repair", ".trash", "Playlists")

_VIDEO_ID_RE = re.compile(r"[?&]v=([A-Za-z0-9_-]{11})(?![A-Za-z0-9_-])")

_KINDS = {
    ".webm": "webm", ".bin": "bin", ".nfo": "nfo",
    ".jpg": "image", ".jpeg": "image", ".png": "image", ".webp": "image",
}


@dataclass
class Track:
    relpath: str
    size: int
    mtime_ns: int
    duration_s: float = 0.0
    video_id: str | None = None
    tag_title: str = ""
    tag_artist: str = ""
    tag_album: str = ""
    folder_artist: str = ""
    folder_playlist: str | None = None
    settled: bool = False
    # What the repair concluded when it settled the file: matched | no_match |
    # not_looked_up. Lets a later run retry ONLY the lookups that never completed
    # (FR-013) without re-asking about songs already answered.
    repair_status: str | None = None
    lyrics: str | None = None


@dataclass
class Inventory:
    library: str
    started_ns: int = 0
    tracks: list[Track] = field(default_factory=list)
    other: dict[str, list[str]] = field(default_factory=dict)
    colon_dirs: list[str] = field(default_factory=list)
    symlinks: list[str] = field(default_factory=list)
    unreadable: list[tuple[str, str]] = field(default_factory=list)


def video_id_from(text: str) -> str | None:
    m = _VIDEO_ID_RE.search(text or "")
    return m.group(1) if m else None


def _first(tags, *keys) -> str:
    for k in keys:
        frame = tags.get(k)
        if frame is not None and getattr(frame, "text", None):
            return str(frame.text[0])
    return ""


def read_track(library: str, rel: str, folder_artist: str, folder_playlist: str | None) -> Track:
    from mutagen.id3 import ID3
    from mutagen.mp3 import MP3

    path = os.path.join(library, rel)
    st = os.stat(path)
    t = Track(relpath=rel, size=st.st_size, mtime_ns=st.st_mtime_ns,
              folder_artist=folder_artist, folder_playlist=folder_playlist)
    tags = ID3(path)
    t.tag_title = _first(tags, "TIT2")
    t.tag_artist = _first(tags, "TPE1", "TPE2")
    t.tag_album = _first(tags, "TALB")
    # yt-dlp writes the watch URL as TXXX:purl AND TXXX:comment (the ID3v1-style
    # COMM copy is truncated, so it is the last resort, not the first).
    for key in ("TXXX:purl", "TXXX:comment"):
        t.video_id = video_id_from(_first(tags, key))
        if t.video_id:
            break
    if not t.video_id:
        for frame in tags.getall("COMM"):
            t.video_id = video_id_from(" ".join(map(str, frame.text)))
            if t.video_id:
                break
    t.settled = "TXXX:SYNODL_REPAIR" in tags
    t.repair_status = _first(tags, "TXXX:SYNODL_STATUS") or None
    try:
        t.duration_s = float(MP3(path).info.length)
    except Exception:  # a file with tags but unreadable frames still has an identity
        t.duration_s = 0.0
    return t


def scan(library: str, *, limit: int | None = None) -> Inventory:
    """Describe everything in `library`. Never writes."""
    inv = Inventory(library=library, started_ns=time.time_ns())
    audio: dict[str, Track] = {}
    lyrics: list[str] = []

    def add_other(kind: str, rel: str):
        inv.other.setdefault(kind, []).append(rel)

    top = sorted(e.name for e in os.scandir(library))
    artists_seen = 0
    for name in top:
        full = os.path.join(library, name)
        if os.path.islink(full):
            inv.symlinks.append(name)
            continue
        if os.path.isfile(full):
            if name == ".ytdlp-archive.txt":
                add_other("archive", name)
            elif name != ".DS_Store":
                add_other("stray", name)
            continue
        if name in _OWN_DIRS:
            continue
        artists_seen += 1
        if limit is not None and artists_seen > limit:
            break
        _walk(inv, library, name, audio, lyrics, add_other)

    # A lyrics file belongs to the audio with the same basename in the same folder.
    for rel in lyrics:
        twin = audio.get(rel[: -len(".lrc")] + ".mp3")
        if twin is not None:
            twin.lyrics = rel
        else:
            add_other("lrc_orphan", rel)

    for key in inv.other:
        inv.other[key].sort()
    inv.tracks.sort(key=lambda t: t.relpath)
    inv.colon_dirs.sort()
    return inv


def _walk(inv, library, rel_dir, audio, lyrics, add_other):
    abs_dir = os.path.join(library, rel_dir)
    parts = rel_dir.split("/")
    if ":" in parts[-1]:
        inv.colon_dirs.append(rel_dir)
    try:
        entries = sorted(os.scandir(abs_dir), key=lambda e: e.name)
    except OSError as exc:
        inv.unreadable.append((rel_dir, str(exc)))
        return
    for e in entries:
        rel = f"{rel_dir}/{e.name}"
        if e.is_symlink():
            inv.symlinks.append(rel)
        elif e.is_dir(follow_symlinks=False):
            _walk(inv, library, rel, audio, lyrics, add_other)
        elif e.is_file(follow_symlinks=False):
            _file(inv, library, rel, e.name, audio, lyrics, add_other)


def _file(inv, library, rel, name, audio, lyrics, add_other):
    lower = name.lower()
    ext = os.path.splitext(lower)[1]
    if ext == ".mp3":
        parts = rel.split("/")
        artist = unicodedata.normalize("NFC", parts[0])
        playlist = parts[1] if len(parts) == 3 else None
        try:
            t = read_track(library, rel, artist, playlist)
        except Exception as exc:
            inv.unreadable.append((rel, type(exc).__name__))
            return
        inv.tracks.append(t)
        audio[rel] = t
    elif ext == ".lrc":
        lyrics.append(rel)
    elif name == ".DS_Store":
        return
    else:
        add_other(_KINDS.get(ext, "other"), rel)
