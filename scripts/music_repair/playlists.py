"""Playlists: the .m3u8 files that keep "what was in that playlist" answerable.

The old layout stored a playlist as a FOLDER of copies. After deduplication the
songs live once, so membership has to live somewhere else — and a playlist file
is what a media server, and every player, already understands.

A playlist is identified by its TITLE alone (clarified): the same title under
fifty artists is ONE playlist. Its order is by artist then title, because the
original order is not recoverable from folders and must not be invented.

Entries are paths RELATIVE to the playlist file, so the library still resolves
when it is mounted somewhere else (FR-006). And because a title is untrusted
input — it comes off a video — nothing taken from one may add a line, an entry
or a directive: control characters are replaced before anything is written.
"""

from __future__ import annotations

import posixpath
import re
from dataclasses import dataclass

from . import names

_CONTROL = re.compile(r"[\x00-\x1f\x7f  ]")
PLAYLIST_DIR = "Playlists"


@dataclass(frozen=True)
class Entry:
    artist: str
    title: str
    relpath: str        # library-relative path of the audio
    duration_s: float = 0.0


def key(title: str) -> str:
    """Two titles are one playlist if they differ only in case or whitespace."""
    return " ".join((title or "").split()).casefold()


def _clean(s: str) -> str:
    return _CONTROL.sub(" ", s or "").strip()


def _sort_key(e: Entry):
    return (names.fold(e.artist), names.fold(e.title), e.relpath)


def _line(e: Entry) -> list[str]:
    rel = posixpath.relpath(_clean(e.relpath), PLAYLIST_DIR)
    return [f"#EXTINF:{int(round(e.duration_s or 0))},{_clean(e.artist)} - {_clean(e.title)}", rel]


def render(entries: list[Entry]) -> str:
    seen, ordered = set(), []
    for e in sorted(entries, key=_sort_key):
        if e.relpath in seen:
            continue
        seen.add(e.relpath)
        ordered.append(e)
    lines = ["#EXTM3U"]
    for e in ordered:
        lines.extend(_line(e))
    return "\n".join(lines) + "\n"


def parse(text: str) -> list[Entry]:
    """Read back what render wrote. Anything else is treated as empty."""
    out, pending = [], None
    for raw in (text or "").splitlines():
        line = raw.strip()
        if line.startswith("#EXTINF:"):
            m = re.match(r"#EXTINF:(-?\d+),(.*)$", line)
            pending = m.groups() if m else None
        elif line and not line.startswith("#"):
            rel = posixpath.normpath(posixpath.join(PLAYLIST_DIR, line))
            if not names.safe_rel(rel):
                pending = None
                continue
            secs, label = pending if pending else ("0", "")
            artist, _, title = label.partition(" - ")
            out.append(Entry(artist=artist, title=title or posixpath.basename(rel), relpath=rel,
                             duration_s=float(secs)))
            pending = None
    return out


def merge(existing_text: str, entries: list[Entry]) -> str:
    """Union with an existing playlist, so a later run only ever ADDS."""
    old = parse(existing_text)
    have = {e.relpath for e in entries}
    return render(entries + [e for e in old if e.relpath not in have])


def filenames(titles: list[str]) -> dict[str, str]:
    """title → 'X.m3u8', sanitised and collision-free.

    Two titles that sanitise to the same file name (they differ only in a
    character the mount cannot hold) get a numeric suffix rather than one
    silently overwriting the other.
    """
    out, used = {}, set()
    for title in sorted(titles):
        base = names.sanitize_name(title) or "Playlist"
        name, n = base, 1
        while name.casefold() in used:
            n += 1
            name = f"{base} ({n})"
        used.add(name.casefold())
        out[title] = name + ".m3u8"
    return out
