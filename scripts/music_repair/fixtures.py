"""Builds a small library in the OLD layout for tests: Artist/<playlist>/<title>.mp3.

Real (tiny) mp3s are generated with ffmpeg and tagged with mutagen so tag
round-trips are tested against the real libraries, not mocks. Whether a missing
dependency skips or fails is the caller's choice: MUSIC_REPAIR_REQUIRE_DEPS=1
(set by scripts/music-repair-test.sh and CI) makes it a failure, because a skip
here would let the whole tag/cover/convert surface pass green untested.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import unittest

try:  # pragma: no cover - environment dependent
    from mutagen.id3 import COMM, ID3, TALB, TIT2, TPE1, TPE2, TXXX
except ImportError:  # pragma: no cover
    ID3 = None

HAVE_DEPS = ID3 is not None and shutil.which("ffmpeg") is not None


def require_deps(testcase_cls):
    """Skip (or FAIL, when required) a TestCase whose dependencies are absent."""
    if HAVE_DEPS:
        return testcase_cls
    if os.environ.get("MUSIC_REPAIR_REQUIRE_DEPS"):
        class _Missing(unittest.TestCase):
            def test_dependencies_present(self):
                self.fail("mutagen and ffmpeg are required (MUSIC_REPAIR_REQUIRE_DEPS=1)")
        _Missing.__name__ = testcase_cls.__name__
        return _Missing
    return unittest.skip("mutagen/ffmpeg not available")(testcase_cls)


def make_mp3(path, *, seconds=1, title="", artist="", album="", video_id=None, size_pad=0, description=None):
    """Write a real mp3 with the tags yt-dlp writes (TXXX:purl / TXXX:comment)."""
    os.makedirs(os.path.dirname(path), exist_ok=True)
    subprocess.run(
        ["ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi", "-i",
         "anullsrc=r=22050:cl=mono", "-t", str(seconds), "-q:a", "9", path],
        check=True)
    tags = ID3()
    if title:
        tags.add(TIT2(encoding=3, text=title))
    if artist:
        tags.add(TPE1(encoding=3, text=artist))
        tags.add(TPE2(encoding=3, text=artist))
    if album:
        tags.add(TALB(encoding=3, text=album))
    if video_id:
        url = f"https://www.youtube.com/watch?v={video_id}"
        tags.add(TXXX(encoding=3, desc="purl", text=url))
        tags.add(TXXX(encoding=3, desc="comment", text=url))
    if description:
        tags.add(TXXX(encoding=3, desc="description", text=description))
    elif size_pad:
        tags.add(TXXX(encoding=3, desc="description", text="x" * size_pad))
    tags.save(path, v2_version=3)
    return path


def make_webm(path, seconds=1):
    """A genuine webm with an audio stream, the kind yt-dlp leaves behind."""
    os.makedirs(os.path.dirname(path), exist_ok=True)
    subprocess.run(["ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi", "-i",
                    f"sine=frequency=440:duration={seconds}", "-c:a", "libopus", path], check=True)
    return path


def touch(path, data=b"", mtime=None):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "wb") as f:
        f.write(data)
    if mtime is not None:
        os.utime(path, (mtime, mtime))
    return path


def build_library(root):
    """The shape of the real volume, small enough to reason about.

    * `In Da Club` (video AAAAAAAAAAA) sits in THREE playlist folders across two
      artists' folders, with different sizes (the embedded album tag differs);
    * a real album folder (Coldplay - Parachutes), a Singles folder, a folder with
      a colon in its name, a .webm, a .bin, sidecars and media-server files.
    """
    def track(artist_dir, playlist, name, **kw):
        return make_mp3(os.path.join(root, artist_dir, playlist, name + ".mp3"), **kw)

    for artist in ("50 Cent", "ABBA", "Coldplay", "10ccVEVO"):
        touch(os.path.join(root, artist, "artist.nfo"), b"<artist/>")
        touch(os.path.join(root, artist, "folder.jpg"), b"\xff\xd8\xff\xe0jpg")
    for i, pl in enumerate(("Party Hits", "Dance Music", "Best Songs")):
        track("50 Cent", pl, "50 Cent - In Da Club (Official Music Video)",
              title="50 Cent - In Da Club (Official Music Video)", artist="50 Cent",
              album=pl, video_id="AAAAAAAAAAA", seconds=2, size_pad=10 * i)
        touch(os.path.join(root, "50 Cent", pl, "album.nfo"), b"<album/>")
    touch(os.path.join(root, "50 Cent", "Party Hits",
                       "50 Cent - In Da Club (Official Music Video).lrc"), b"[00:01.00]hi")
    track("50 Cent", "Party Hits", "Chief Keef Feat 50 Cent & Wiz Khalifa - Hate Bein' Sober",
          title="Chief Keef Feat 50 Cent & Wiz Khalifa - Hate Bein' Sober",
          artist="50 Cent", album="Party Hits", video_id="BBBBBBBBBBB")
    track("ABBA", "Party Hits", "ABBA - Mamma Mia (Official Music Video)",
          title="ABBA - Mamma Mia (Official Music Video)", artist="ABBA",
          album="Party Hits", video_id="CCCCCCCCCCC",
          description="SECRET-DESCRIPTION-MARKER visit https://example.com/?token=abc123")
    track("Coldplay", "Coldplay - Parachutes", "Coldplay - Yellow (Official Video)",
          title="Coldplay - Yellow (Official Video)", artist="Coldplay",
          album="Coldplay - Parachutes", video_id="DDDDDDDDDDD")
    track("Coldplay", "Coldplay: Everyday Life", "Coldplay - Orphans",
          title="Coldplay - Orphans", artist="Coldplay",
          album="Coldplay: Everyday Life", video_id="EEEEEEEEEEE")
    touch(os.path.join(root, "Coldplay", "Coldplay - Everyday Life", "album.nfo"), b"<album/>")
    track("10ccVEVO", "Singles", "10cc - I'm Not in Love",
          title="10cc - I'm Not in Love", artist="10ccVEVO", album="Singles",
          video_id="FFFFFFFFFFF")
    track("ABBA", "Party Hits", "No Id Track", title="No Id Track", artist="ABBA")
    make_webm(os.path.join(root, "Jazz", "Singles", "Cozy Cabin.webm"))
    touch(os.path.join(root, "Alec Benjamin", "logo.bin"), b"\x89PNG\r\n\x1a\nrest")
    touch(os.path.join(root, ".ytdlp-archive.txt"), b"youtube AAAAAAAAAAA\n")
    return root
