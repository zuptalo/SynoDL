"""Names: cleaning what YouTube called a track, and making a name safe to file.

Everything here is pure. It is where the rules live, so it is where the tests
are (test_names.py), and it shares its sanitising table (names_cases.json) with
the Go recipe's tests so the two cannot disagree about what a safe name is.

Two different jobs share this file and must not be confused:

  * CLEANING decides what a track or artist is CALLED — "In Da Club (Official
    Music Video)" is the song "In Da Club".
  * SANITISING decides what may appear in a PATH. Every name derived from a tag,
    a title or a playlist is untrusted input (FR-010): a source that calls
    itself "../../../etc" is not injecting a command, it is choosing a
    directory, exactly the hole spec 0013 closed on the download side.
"""

from __future__ import annotations

import re
import unicodedata
from dataclasses import dataclass, field

# Same bound as server/internal/ytdl/sanitize.go: a name is one component of a
# path that already has a library root and a file name in it.
MAX_NAME_LENGTH = 120

# Characters the mount cannot list. Substituted with their fullwidth lookalikes,
# which is exactly what yt-dlp itself does, so a name we write is spelled the
# way the recipe would have spelled it. '/' and '\' are different: they would
# change the NUMBER of path components, so — like the Go rule — they become a
# space rather than a lookalike.
_RESERVED = {
    ":": "：", "*": "＊", "?": "？", '"': "＂",
    "<": "＜", ">": "＞", "|": "｜",
}


def sanitize_name(name: str) -> str:
    """Make a source-derived name safe to use as ONE path component.

    Returns "" when nothing usable survives; callers treat that as "no name".
    Mirrors ytdl.SanitizeName, plus the mount-hostile characters.
    """
    name = unicodedata.normalize("NFC", name or "").strip()
    if not name:
        return ""
    out = []
    for ch in name:
        if ch in "/\\" or ch == "\x00" or ord(ch) < 0x20 or ord(ch) == 0x7F or ch.isspace():
            out.append(" ")
        else:
            out.append(_RESERVED.get(ch, ch))
    # A token made only of dots carries no name, whatever it means to the
    # filesystem; dropping it outright is simpler and more honest than trimming.
    kept = [t for t in "".join(out).split() if t.strip(".") != ""]
    clean = " ".join(kept)
    if not clean:
        return ""
    clean = clean.lstrip(".").rstrip(". ")
    if len(clean.encode("utf-8")) > MAX_NAME_LENGTH:
        runes = list(clean)
        while len("".join(runes).encode("utf-8")) > MAX_NAME_LENGTH:
            runes.pop()
        clean = "".join(runes).rstrip(". ")
    return clean.strip()


def safe_rel(rel: str) -> bool:
    """True when `rel` is a plain library-relative path (FR-021).

    A plan is an input like any other: an edited or corrupted one must not be
    able to name anything outside the mounted library.
    """
    if not isinstance(rel, str) or not rel or "\x00" in rel or rel.startswith("/"):
        return False
    return all(part not in ("", ".", "..") for part in rel.split("/"))


def fold(s: str) -> str:
    """A comparison key: NFKC (fullwidth → ASCII), no accents, casefolded, words only.

    NEVER used for a filename — only to decide two spellings are the same one.
    """
    s = unicodedata.normalize("NFKC", s or "")
    s = "".join(c for c in unicodedata.normalize("NFD", s) if not unicodedata.combining(c))
    s = re.sub(r"[^\w]+", " ", s.casefold(), flags=re.UNICODE).replace("_", " ")
    return " ".join(s.split())


# ---- titles ---------------------------------------------------------------

# A bracketed aside is removed ONLY when every word in it is noise. "(Live)",
# "(Remix)" and "(Acoustic Version)" name a different RECORDING and stay — the
# lookup would otherwise match the studio cut against a live video.
_NOISE = frozenset("""
    official music video videoclip clip lyric lyrics audio visualizer visualiser
    hd hq 4k 8k uhd 1080p 720p remastered remaster in new song mv full
    oficial officiel ufficiale
""".split())

_GROUP_RE = re.compile(r"\(([^()]*)\)|\[([^\[\]]*)\]")
_FEAT_GROUP_RE = re.compile(r"^(?:feat\.?|ft\.?|featuring)\s+(.+)$", re.IGNORECASE)
_FEAT_TAIL_RE = re.compile(r"\s+(?:feat\.?|ft\.?|featuring)\s+(.+)$", re.IGNORECASE)
_FEAT_SPLIT_RE = re.compile(r"\s*(?:,|&|\band\b)\s*", re.IGNORECASE)
_SEP_RE = re.compile(r"\s+[-–—―]\s+")
_QUOTES = ('"', "＂", "“", "”")


def _names(s: str) -> list[str]:
    return [p.strip() for p in _FEAT_SPLIT_RE.split(s) if p.strip()]


def clean_title(raw: str) -> tuple[str, list[str]]:
    """Strip video noise from a title; lift featured artists out of it.

    Returns (title, featured). Never returns an empty title: a title that is
    nothing BUT noise is left as it was, because guessing is worse than noise.
    """
    original = unicodedata.normalize("NFC", raw or "").strip()
    featured: list[str] = []

    def group(m: re.Match) -> str:
        inner = (m.group(1) if m.group(1) is not None else m.group(2)).strip()
        fm = _FEAT_GROUP_RE.match(inner)
        if fm:
            featured.extend(_names(fm.group(1)))
            return " "
        words = fold(inner).split()
        if words and all(w in _NOISE for w in words):
            return " "
        return m.group(0)

    title = _GROUP_RE.sub(group, original)
    tail = _FEAT_TAIL_RE.search(title)
    if tail:
        featured.extend(_names(tail.group(1)))
        title = title[: tail.start()]
    title = " ".join(title.split())
    if len(title) >= 2 and title[0] in _QUOTES and title[-1] in _QUOTES:
        title = title[1:-1].strip()
    if not title or fold(title) == "":
        return original, []
    return title, featured


@dataclass
class Parsed:
    lead: str
    title: str
    featured: list[str] = field(default_factory=list)


def split_artist_title(raw_title: str, tag_artist: str) -> Parsed:
    """Separate "Artist - Title (noise)" into its parts.

    The prefix is trusted as the artist only where it agrees with the tag: it
    names the tag artist, or is one of a "Lead feat. Guest" credit that contains
    it. A title such as "Live - Tonight" is a title, not an artist. This is
    idempotent: an already-clean title has no prefix and the tag stands.
    """
    raw = unicodedata.normalize("NFC", raw_title or "").strip()
    tag = (tag_artist or "").strip()
    parts = _SEP_RE.split(raw, maxsplit=1)
    if len(parts) == 2:
        left, right = parts[0].strip(), parts[1].strip()
        left_feat: list[str] = []
        left_lead = left
        m = _FEAT_TAIL_RE.search(left)
        if m:
            left_lead = left[: m.start()].strip()
            left_feat = _names(m.group(1))
        fl, ft = fold(left), fold(tag)
        trusted = not tag or fold(left_lead) == ft or (ft != "" and f" {ft} " in f" {fl} ") \
            or (fl != "" and f" {fl} " in f" {ft} ")
        if trusted and left_lead:
            title, feat = clean_title(right)
            return Parsed(left_lead, title, left_feat + feat)
    title, feat = clean_title(raw)
    return Parsed(tag, title, feat)


# ---- artist folders -------------------------------------------------------

# Whole-token SUFFIXES only, in this exact list. A name that merely contains the
# word — "42 dugg Music", "Music Brokers", "Official Nonsense Band" — is a name.
_CHANNEL_SUFFIXES = (
    re.compile(r"\s+official\s+youtube\s+channel$", re.IGNORECASE),
    re.compile(r"\s+official\s+channel$", re.IGNORECASE),
    re.compile(r"\s*-\s*topic$", re.IGNORECASE),
    re.compile(r"\s+official$", re.IGNORECASE),
    re.compile(r"\s*vevo$", re.IGNORECASE),
)


def clean_artist_folder(name: str) -> str:
    """The uploader's folder name with channel markers removed; never empty."""
    original = unicodedata.normalize("NFC", name or "").strip()
    current = original
    changed = True
    while changed:
        changed = False
        for pat in _CHANNEL_SUFFIXES:
            stripped = pat.sub("", current).strip()
            if stripped != current and stripped:
                current, changed = stripped, True
    return current or original


_ALBUM_FOLDER_RE = re.compile(r"^(.+?)(?:\s+[-–—]\s+|:\s+)(.+)$")
_ALBUM_NOISE_RE = re.compile(r"\s+(?:official\s+album\s+playlist|official\s+playlist|full\s+album)$",
                             re.IGNORECASE)


def album_of_folder(folder: str, track_artists: list[str]) -> str | None:
    """The album a folder already is, or None when it is only a playlist.

    A folder named "<Artist> - <X>" whose tracks are ALL by that artist is a real
    album — the old recipe filed YouTube's album playlists this way (Coldplay has
    fifteen). Its tracks keep it rather than being demoted to "Singles". Anything
    else — "Old TikTok Songs That We Forgot About" — is a playlist.
    """
    m = _ALBUM_FOLDER_RE.match(unicodedata.normalize("NFC", folder or "").strip())
    if not m:
        return None
    prefix, album = fold(m.group(1)), _ALBUM_NOISE_RE.sub("", m.group(2)).strip()
    folds = {fold(a) for a in track_artists if a}
    if not album or not prefix or folds != {prefix}:
        return None
    return album
