"""Identity: which files are the same song, and which copy survives.

A song is its YouTube video id (FR-004) and nothing else. File hashes cannot do
this job — the same song in two playlist folders differs in size, because the
embedded album tag differs — and file names cannot either: two different songs
can share a title. A file with no readable id is kept and reported; guessing that
it is a copy of something is exactly the mistake this rule exists to prevent.
"""

from __future__ import annotations

from dataclasses import dataclass, field

from .inventory import Track


@dataclass
class Song:
    key: str
    kept: Track
    copies: list[Track] = field(default_factory=list)      # everything except `kept`
    playlists: list[str] = field(default_factory=list)     # titles, from UNSETTLED copies
    all_copies: list[Track] = field(default_factory=list)  # kept + copies


def choose_kept(tracks: list[Track]) -> Track:
    """The copy that survives (FR-005): a stated, deterministic rule.

    An already-settled copy wins outright — it is where a previous run put it.
    Otherwise the LARGEST file (the one least likely to be truncated), then the
    earliest modified time, then the path, so the same library gives the same
    answer on every run whatever order it was listed in.
    """
    return min(tracks, key=lambda t: (not t.settled, -t.size, t.mtime_ns, t.relpath))


def group(tracks: list[Track]) -> tuple[list[Song], list[Track]]:
    by_id: dict[str, list[Track]] = {}
    unidentified: list[Track] = []
    for t in tracks:
        if t.video_id:
            by_id.setdefault(t.video_id, []).append(t)
        else:
            unidentified.append(t)
    songs = []
    for vid in sorted(by_id):
        copies = by_id[vid]
        kept = choose_kept(copies)
        # Playlist membership is read from folders that are still old-layout
        # folders. A settled file's folder is an album or Singles, not a playlist.
        titles = sorted({c.folder_playlist for c in copies
                         if not c.settled and c.folder_playlist})
        songs.append(Song(key=vid, kept=kept,
                          copies=sorted((c for c in copies if c is not kept), key=lambda c: c.relpath),
                          playlists=titles,
                          all_copies=sorted(copies, key=lambda c: c.relpath)))
    unidentified.sort(key=lambda t: t.relpath)
    return songs, unidentified
