"""Tag I/O: the only place that writes ID3. Everything here is reversible.

Every write returns the PREVIOUS value of each thing it changed, because a repair
that rewrites files in place is only safe if the old values can be put back
(FR-007, SC-010): `.trash` protects what is removed, this protects what is edited.

What is deliberately never touched: the video-id frames (TXXX:purl / :comment).
They are how a file is found back at its source and how a later run recognises it.
Files keep their modified time, so "how old is this download" survives a retag.
"""

from __future__ import annotations

import os

from mutagen.id3 import APIC, ID3, TALB, TDRC, TIT2, TPE1, TPE2, TRCK, TXXX, UFID, ID3NoHeaderError

_TEXT = {"title": TIT2, "artist": TPE1, "albumartist": TPE2, "album": TALB, "track": TRCK, "date": TDRC}
_TXXX = {
    "musicbrainz_release": "MusicBrainz Album Id",    # the names Picard writes, which Jellyfin/Plex read
    "featured": "SYNODL_FEATURED_ARTISTS",
    "original_title": "SYNODL_ORIGINAL_TITLE",
    "repair": "SYNODL_REPAIR",
    "repair_status": "SYNODL_STATUS",
}
_UFID_OWNER = "http://musicbrainz.org"


def _read(tags: ID3, key: str):
    if key in _TEXT:
        frame = tags.get(_TEXT[key].__name__)
        return str(frame.text[0]) if frame is not None and frame.text else None
    if key == "musicbrainz_recording":
        frame = tags.get("UFID:" + _UFID_OWNER)
        return frame.data.decode("utf-8", "replace") if frame is not None else None
    frame = tags.get("TXXX:" + _TXXX[key])
    return str(frame.text[0]) if frame is not None and frame.text else None


def _put(tags: ID3, key: str, value):
    if key in _TEXT:
        cls = _TEXT[key]
        tags.delall(cls.__name__)
        if value is not None:
            tags.add(cls(encoding=3, text=str(value)))
    elif key == "musicbrainz_recording":
        tags.delall("UFID:" + _UFID_OWNER)
        if value is not None:
            tags.add(UFID(owner=_UFID_OWNER, data=str(value).encode("utf-8")))
    else:
        tags.delall("TXXX:" + _TXXX[key])
        if value is not None:
            tags.add(TXXX(encoding=3, desc=_TXXX[key], text=str(value)))


def _load(path: str) -> ID3:
    try:
        return ID3(path)
    except ID3NoHeaderError:   # a file just converted has no tag block yet
        return ID3()


def _save(path: str, tags: ID3):
    st = os.stat(path)
    tags.save(path, v2_version=3)   # 2.3: what the library already uses, and what every player reads
    os.utime(path, ns=(st.st_atime_ns, st.st_mtime_ns))


def write_tags(path: str, tags_to_write: dict) -> dict:
    """Set the given tags; return {key: previous value or None} for each."""
    unknown = set(tags_to_write) - set(_TEXT) - set(_TXXX) - {"musicbrainz_recording"}
    if unknown:
        raise ValueError(f"unknown tag keys: {sorted(unknown)}")
    tags = _load(path)
    previous = {k: _read(tags, k) for k in tags_to_write}
    for k, v in tags_to_write.items():
        _put(tags, k, v)
    _save(path, tags)
    return previous


def restore_tags(path: str, previous: dict):
    tags = _load(path)
    for k, v in previous.items():
        _put(tags, k, v)
    _save(path, tags)


def embed_cover(path: str, data: bytes, mime: str):
    """Embed `data` as the front cover; return the (data, mime) it replaced, or None."""
    tags = _load(path)
    old = tags.getall("APIC")
    previous = (old[0].data, old[0].mime) if old else None
    tags.delall("APIC")
    tags.add(APIC(encoding=3, mime=mime, type=3, desc="Cover", data=data))
    _save(path, tags)
    return previous


def restore_cover(path: str, previous):
    tags = _load(path)
    tags.delall("APIC")
    if previous:
        tags.add(APIC(encoding=3, mime=previous[1], type=3, desc="Cover", data=previous[0]))
    _save(path, tags)
