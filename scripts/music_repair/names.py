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

# Words that mark a group as VIDEO METADATA when it starts with one ("Official
# Video By Someone", "Official video, 2022"), whatever else it says. Anything that
# names a different recording — a remix, a live take — keeps the group.
_STRONG = frozenset("official lyric lyrics visualizer visualiser videoclip oficial officiel ufficiale".split())
_KEEP = frozenset("remix live acoustic instrumental edit mix cover demo session unplugged extended radio".split())
_EXTRA = frozenset("explicit clean dirty".split())
# Unambiguous on their own at the end of a title ("... For HD"), unlike "Video"/"Audio".
_LONE = frozenset("hd hq 4k 8k uhd 1080p 720p".split())

_GROUP_RE = re.compile(r"\(([^()]*)\)|\[([^\[\]]*)\]|\*([^*]*)\*")
_FEAT_GROUP_RE = re.compile(r"^(?:feat\.?|ft\.?|featuring)\s+(.+)$", re.IGNORECASE)
# A bare trailing "feat. X". It must not contain a dash or a bracket: in
# "Dom Dolla feat. Daya - Dreamin (Anyma Remix)" the feat belongs to the ARTIST
# half, and an unanchored match once swallowed the whole title after it.
_FEAT_TAIL_RE = re.compile(r"\s+(?:feat\.?|ft\.?|featuring)\s+([^-\u2013\u2014\u2015\[\](){}]+)$", re.IGNORECASE)
# yt-dlp joins credits with ", ", and titles use "&" and "x": all of them are credit lists.
_CREDIT_SPLIT_RE = re.compile(r"\s*(?:,|&|\band\b|\bx\b|\bvs\.?)\s*", re.IGNORECASE)
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
        inner = next(g for g in m.groups() if g is not None).strip()
        fm = _FEAT_GROUP_RE.match(inner)
        if fm:
            featured.extend(_names(fm.group(1)))
            return " "
        words = fold(inner).split()
        if not words or _KEEP & set(words):
            return m.group(0)
        if all(w in _NOISE for w in words) or words[0] in _STRONG \
                or ("official" in words and all(w in _NOISE or w in _EXTRA for w in words)):
            return " "
        return m.group(0)

    title = _GROUP_RE.sub(group, original)
    tail = _FEAT_TAIL_RE.search(title)
    if tail:
        featured.extend(_names(tail.group(1)))
        title = title[: tail.start()]
    title = _strip_trailing_noise(" ".join(title.split()))
    if len(title) >= 2 and title[0] in _QUOTES and title[-1] in _QUOTES:
        title = title[1:-1].strip()
    if not title or fold(title) == "":
        return original, []
    return title, featured


def _strip_trailing_noise(title: str) -> str:
    """Drop a trailing "Official Video" / "| Official Visualizer" / "HD" run.

    Only a run made entirely of noise words that ALSO contains something
    unambiguous — "official", "lyrics", "visualizer" — or is nothing but "HD"/"4K",
    so a title that merely ends in "Video" or "Audio" is left alone.
    """
    words = list(re.finditer(r"\S+", title))
    i = len(words)
    while i > 0:
        raw = words[i - 1].group()
        if any(c in raw for c in "()[]{}"):
            break                       # "HD)" is the end of a bracket, not the word HD
        w = fold(raw)
        if w in ("",) or w in _NOISE or w in _LONE:
            i -= 1
        else:
            break
    run = [fold(m.group()) for m in words[i:]]
    run = [w for w in run if w]
    if not run or i == 0:
        return title
    if not (_STRONG & set(run) or set(run) <= _LONE or "music video" in " ".join(run)):
        return title
    cut = title[: words[i].start()] if i < len(words) else title
    return cut.rstrip(" |-\u2013\u2014\u2015").rstrip() or title


@dataclass
class Parsed:
    lead: str
    title: str
    featured: list[str] = field(default_factory=list)
    # The lead as written, then its first credit when it is a credit list.
    # MusicBrainz models "Anyma & CamelPhat" as [Anyma, CamelPhat], so a lookup
    # under the whole name finds nothing while the first credit does.
    leads: list[str] = field(default_factory=list)


def _leads(lead: str) -> list[str]:
    if not lead:
        return []
    parts = [p.strip() for p in _CREDIT_SPLIT_RE.split(lead) if p.strip()]
    out = [lead]
    if len(parts) > 1 and fold(parts[0]) != fold(lead):
        out.append(parts[0])
    return out


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
        # The tag may itself be a credit list ("Anyma, Sphere"): the prefix agrees
        # with it if ANY credit is named in the prefix, or the prefix is inside the tag.
        credits = [fold(c) for c in _CREDIT_SPLIT_RE.split(tag) if fold(c)]
        trusted = not tag or fold(left_lead) == ft or (ft != "" and f" {ft} " in f" {fl} ") \
            or (fl != "" and f" {fl} " in f" {ft} ") or any(f" {c} " in f" {fl} " for c in credits)
        if trusted and left_lead:
            title, feat = clean_title(right)
            return Parsed(left_lead, title, left_feat + feat, _leads(left_lead))
    title, feat = clean_title(raw)
    return Parsed(tag, title, feat, _leads(tag))


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
