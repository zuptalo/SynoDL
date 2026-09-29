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
import shutil

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


def _rewrite(path: str, mutate):
    """Apply `mutate(tags)` to a COPY of the file, then swap it in atomically.

    mutagen rewrites a file IN PLACE, and when the new tags outgrow the padding
    that means shifting the whole audio stream — a kill part-way leaves a file
    that is neither the old one nor the new one. A copy in the same folder and an
    os.replace() means the file is always one or the other, complete. The cost is
    one extra read and write of the file; for a library repair that runs once,
    that is the right trade. The temp file is hidden and removed on any failure.
    """
    st = os.stat(path)
    tmp = os.path.join(os.path.dirname(path), f".{os.path.basename(path)}.repair-tmp")
    shutil.copy2(path, tmp)
    try:
        tags = _load(tmp)
        result = mutate(tags)
        tags.save(tmp, v2_version=3)   # 2.3: what the library already uses, and what every player reads
        os.utime(tmp, ns=(st.st_atime_ns, st.st_mtime_ns))
        os.replace(tmp, path)
    except BaseException:
        try:
            os.unlink(tmp)
        except OSError:
            pass
        raise
    return result


def read_tags(path: str, keys) -> dict:
    """The current value of each key, without writing anything."""
    tags = _load(path)
    return {k: _read(tags, k) for k in keys}


def repair_marker(path: str):
    """The plan id this file was last repaired by, or None."""
    return read_tags(path, ["repair"])["repair"]


def write_tags(path: str, tags_to_write: dict) -> dict:
    """Set the given tags; return {key: previous value or None} for each."""
    unknown = set(tags_to_write) - set(_TEXT) - set(_TXXX) - {"musicbrainz_recording"}
    if unknown:
        raise ValueError(f"unknown tag keys: {sorted(unknown)}")
    def mutate(tags):
        previous = {k: _read(tags, k) for k in tags_to_write}
        for k, v in tags_to_write.items():
            _put(tags, k, v)
        return previous
    return _rewrite(path, mutate)


def restore_tags(path: str, previous: dict):
    def mutate(tags):
        for k, v in previous.items():
            _put(tags, k, v)
    _rewrite(path, mutate)


def embed_cover(path: str, data: bytes, mime: str):
    """Embed `data` as the front cover; return the (data, mime) it replaced, or None."""
    def mutate(tags):
        old = tags.getall("APIC")
        previous = (old[0].data, old[0].mime) if old else None
        tags.delall("APIC")
        tags.add(APIC(encoding=3, mime=mime, type=3, desc="Cover", data=data))
        return previous
    return _rewrite(path, mutate)


def read_cover(path: str):
    """The embedded cover as (data, mime), or None."""
    old = _load(path).getall("APIC")
    return (old[0].data, old[0].mime) if old else None


def restore_cover(path: str, previous):
    def mutate(tags):
        tags.delall("APIC")
        if previous:
            tags.add(APIC(encoding=3, mime=previous[1], type=3, desc="Cover", data=previous[0]))
    _rewrite(path, mutate)
