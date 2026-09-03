#!/usr/bin/env python3
"""Tidy a Synology media library into the Plex/Jellyfin folder convention.

    movie/Dune.2021.1080p.BluRay.x264-RARBG   ->  movie/Dune (2021)
    tv-show/Friends S01-S10 COMPLETE          ->  tv-show/Friends (1994)

WHAT THIS IS NOT
----------------
This is an operator tool. It is NOT part of the synodl server, it is not
imported by it, and it does not widen SynoDL's DSM allowlist: the server still
speaks only SYNO.FileStation.List, SYNO.FileStation.CreateFolder and the
DownloadStation task APIs. This script talks to DSM directly, as you, because
renaming a folder is something SynoDL deliberately cannot do.

SAFETY
------
  * Dry-run is the DEFAULT. Nothing changes without --apply.
  * Every applied change is written to an undo file, replayable with --undo.
  * A rename whose target already exists is REFUSED, never merged.
  * Moves never overwrite: DSM is asked with overwrite=false, and the result is
    polled, because CopyMove is asynchronous and reports failure after the fact.
  * Nothing is ever deleted. A file that cannot be attributed with confidence is
    listed and LEFT WHERE IT IS — a human placing it costs less than the tool
    filing it under the wrong title.
  * Only the immediate children of each parent are considered — no recursion.
  * Undoing a move returns the files but leaves the (now empty) folder behind,
    rather than deleting anything.

WHAT IT DOES
------------
  1. Renames title folders to "Title (Year)".
  2. Gathers files dumped straight into the parent — a video plus its subtitles,
     artwork and .nfo — into a folder per title, and TV episodes into
     "Show (Year)/Season NN". Disable with --no-group-loose.

USAGE
-----
    export NAS_PASSWORD='...'          # never pass a password on argv: it is
                                       # visible to every process via ps
    python3 scripts/library_tidy.py --nas https://10.0.1.2:1511 --user synodl \
        --movies /movie --tv /tv-show

    # read the plan, then:
    python3 scripts/library_tidy.py ... --apply
    python3 scripts/library_tidy.py ... --undo library-tidy-undo-<stamp>.json

Run the planner's tests with:  python3 -m unittest scripts.library_tidy_test
"""

from __future__ import annotations

import argparse
import json
import os
import re
import ssl
import sys
import time
import urllib.parse
import urllib.request
from dataclasses import dataclass, field

# --------------------------------------------------------------------------
# The planner: pure functions, no I/O. This is where every edge case lives, so
# it is kept free of network code and covered by library_tidy_test.py.
# --------------------------------------------------------------------------

# Tokens that describe a RELEASE rather than a title. Everything from the first
# one onward is dropped, because scene names put the title first and the
# technical description after it. Matched on whole words only, so "Her" is not
# eaten by "hdr" and "It" survives entirely.
JUNK = r"""
    2160p|1080p|720p|576p|480p|4k|uhd|fhd|hd
  | bluray|blu-?ray|bdrip|brrip|bdremux|remux|web-?dl|web-?rip|web|hdtv|pdtv
  | dvdrip|dvdscr|dvd|hdrip|hdcam|camrip|cam|telesync|ts|tc|r5|vodrip
  | x264|x265|h\.?264|h\.?265|hevc|avc|xvid|divx|10bit|8bit
  | aac|ac3|eac3|dd|ddp|dts(?:-hd)?|truehd|atmos|flac|mp3|opus|2ch|6ch|8ch
  | hdr10\+?|hdr|dovi|dv|sdr|imax|remastered|restored
  | proper|repack|internal|limited|extended|uncut|unrated|theatrical|directors?
  | dual|multi|subbed|dubbed|hardsub|softsub|softsubbed|farsi|persian|dubla|zirnevis
  | complete|completed|season|seasons|series|collection|pack|trilogy|duology
"""
JUNK_RE = re.compile(rf"(?<![^\W_])(?:{JUNK})(?![^\W_])", re.IGNORECASE | re.VERBOSE)

# Release groups trail the name after a dash: "-RARBG", "-YTS.MX", "- YIFY".
GROUP_RE = re.compile(r"[\s._-]+-\s*[A-Za-z0-9]{2,12}(?:\.[A-Za-z]{2,4})?\s*$")

# A season span the folder name announces: "S01-S10", "Season 1-5", "S01".
SEASON_RE = re.compile(
    r"(?<![^\W_])(?:s\d{1,2}(?:\s*[-–~]\s*s?\d{1,2})?|seasons?\s*\d{1,2}(?:\s*[-–~]\s*\d{1,2})?)(?![^\W_])",
    re.IGNORECASE,
)

# A bracketed aside anywhere: "[1080p]", "(BluRay)", "{Farsi}".
BRACKET_RE = re.compile(r"[\[\{(][^\[\]{}()]*[\]\})]")

YEAR_RE = re.compile(r"(?<!\d)((?:19|20)\d{2})(?!\d)")

# A run of years, which is how a series' life is written: "1994 - 2004", or
# "2022 -" while it is still going. Plex keys a show on its FIRST air year, and
# this is the exact shape SynoDL itself writes, so it is the single most common
# rename this tool performs.
YEAR_RANGE_RE = re.compile(r"(?<!\d)((?:19|20)\d{2})\s*[-–—~]\s*((?:19|20)\d{2})?(?!\d)")


def _spaces(s: str) -> str:
    """Scene names join words with dots or underscores; make them words again.

    Path separators are folded to spaces too. DSM will not hand us a folder name
    containing one, but a proposed name that did would be a path-traversal bug
    rather than a rename, so it is removed at the source instead of being relied
    on not to occur.
    """
    for ch in (".", "_", "/", "\\"):
        s = s.replace(ch, " ")
    return re.sub(r"\s+", " ", s).strip()


def extract_year(name: str) -> tuple[str, int | None]:
    """Split the release year off a folder name.

    Returns (name-without-that-year, year). The LAST plausible year wins when a
    title itself contains one — "Blade Runner 2049 2017" is the 2017 film, not a
    film called "Blade Runner" from 2049. A name that is ONLY a year keeps it as
    the title, because "1917" and "2012" are real films.
    """
    if _spaces(YEAR_RE.sub("", name)).strip(" -–.[](){}") == "":
        return name, None  # the year IS the title, e.g. "1917" or "2012"

    # A range is checked first and wins outright: in "Friends 1994 - 2004" the
    # last year is when the show ENDED, and taking it would file the show under
    # the wrong year in every scraper.
    if (r := YEAR_RANGE_RE.search(name)) is not None:
        return name[: r.start()] + " " + name[r.end():], int(r.group(1))

    years = list(YEAR_RE.finditer(name))
    if not years:
        return name, None
    m = years[-1]
    return name[: m.start()] + " " + name[m.end():], int(m.group(1))


def clean_title(raw: str, *, is_tv: bool) -> str:
    """Reduce a messy folder name to just the title."""
    s = _spaces(raw)
    s = BRACKET_RE.sub(" ", s)
    s = GROUP_RE.sub(" ", s)
    if is_tv:
        s = SEASON_RE.sub(" ", s)
    # Everything from the first release token onward is description, not title.
    m = JUNK_RE.search(s)
    if m and _spaces(s[: m.start()]):
        s = s[: m.start()]
    s = _spaces(s).strip(" -–—.,")
    # Re-casing is deliberately timid, because it is the one change here with no
    # objective right answer. An ALL-CAPS name is left alone: "WALL-E", "UP" and
    # "M*A*S*H" are titles, not shouting, and a scraper matches case-insensitively
    # anyway — so re-casing them risks a wrong rename to fix nothing. A
    # multi-word all-lowercase name ("the dark knight") is the one case where the
    # intent is unambiguous. A single lowercase word is left alone too, because
    # "icarly" might be exactly how it is meant to look.
    if s and s.islower() and len(s.split()) > 1:
        # Capitalise each alphabetic run, not each space-separated word, so
        # "spider-man" becomes "Spider-Man" rather than "Spider-man".
        s = re.sub(r"[a-z]+", lambda m: m.group().capitalize(), s)
    return s


def target_name(folder: str, *, is_tv: bool) -> str:
    """The Plex/Jellyfin name for a folder: 'Title (Year)', or 'Title'."""
    without_year, year = extract_year(folder)
    title = clean_title(without_year, is_tv=is_tv)
    if not title:
        return folder.strip()  # nothing survived; leave it alone
    return f"{title} ({year})" if year else title


# --------------------------------------------------------------------------
# Loose files: a title dumped straight into the parent, usually with its
# subtitles, artwork and .nfo beside it. Renaming folders does nothing for these,
# so they are gathered into a folder per title.
# --------------------------------------------------------------------------

# A video is what makes a group real. Everything else is a sidecar, and a sidecar
# alone proves nothing: a lone .srt might belong to a film stored elsewhere, or
# to one that is simply missing.
VIDEO_EXT = {"mkv", "mp4", "avi", "m4v", "mov", "wmv", "mpg", "mpeg", "m2ts",
             "ts", "iso", "divx", "flv", "webm", "vob", "rmvb"}
SIDECAR_EXT = {"srt", "sub", "idx", "ass", "ssa", "vtt", "smi", "sup",
               "jpg", "jpeg", "png", "webp", "tbn", "bmp",
               "nfo", "xml", "json", "txt", "md5", "sfv"}

# Sidecars named for their ROLE rather than their title. Attributing these by
# name would file one film's artwork inside another's folder, so they are always
# left where they are for a human to place.
GENERIC_STEMS = {"poster", "folder", "cover", "fanart", "banner", "backdrop",
                 "thumb", "movie", "video", "artwork", "logo", "clearart",
                 "landscape", "disc", "season", "show", "series", "default"}

# "S01E02", "s1.e2", "1x05" — bounded on non-alphanumerics rather than \b,
# because scene names separate with underscores and \b treats "_" as a word
# character, so it would never fire between "_" and "S".
EPISODE_RE = re.compile(
    r"(?<![^\W_])(?:s(\d{1,2})[^\w]?e(\d{1,3})|(\d{1,2})x(\d{1,3}))(?![^\W_])",
    re.IGNORECASE,
)


# Camera and phone dumps: "VID_20190104_120000.mp4", "IMG_0421.mov",
# "PXL_20230518_101122.mp4", "GOPR0043.mp4". These carry no title at all, and a
# folder called "VID 20190104 120000" is worse than leaving the file alone.
DEVICE_RE = re.compile(
    r"^(?:vid|img|mov|dsc|dscn|pxl|dji|gopr|gh\d|mvi|p\d{7})[\s._-]*\d",
    re.IGNORECASE,
)


def looks_unattributable(title: str, stem: str) -> bool:
    """True when a name carries no title worth filing under.

    Being wrong in this direction is cheap — the file stays put and a human looks
    at it. Being wrong the other way files somebody's holiday video inside a
    feature film's folder.
    """
    if len(title) < 2 or not any(ch.isalpha() for ch in title):
        return True
    if DEVICE_RE.match(stem.strip()):
        return True
    # A run of six or more digits is a timestamp or a camera counter, never part
    # of a title (the longest real one is a four-digit year).
    if re.search(r"\d{6,}", title):
        return True
    return False


def split_ext(name: str) -> tuple[str, str]:
    stem, dot, ext = name.rpartition(".")
    if not dot or not ext.isalnum() or len(ext) > 5:
        return name, ""
    return stem, ext.lower()


def strip_language_suffix(stem: str) -> str:
    """Drop a subtitle's trailing language/flag tag.

    "Dune.2021.en.srt" and "Dune.2021.forced.srt" must land with "Dune.2021.mkv",
    so the tag has to come off before the two are compared.
    """
    tags = r"(?:en|eng|english|fa|far|fas|persian|farsi|ar|fr|de|es|it|nl|tr|ru|" \
           r"forced|sdh|hi|cc|default|full)"
    return re.sub(rf"(?:[.\s_-]{tags})+$", "", stem, flags=re.IGNORECASE)


def title_key(name: str, *, is_tv: bool) -> tuple[str, int | None, str | None]:
    """Reduce a file or folder name to (comparison key, year, season).

    The key is what decides whether two names are the same title, so it is folded
    to letters and digits only — the same rule the server's matcher uses.
    """
    season = None
    base = name
    if is_tv and (m := EPISODE_RE.search(base)):
        season = int(m.group(1) or m.group(3))
        base = base[: m.start()]
    without_year, year = extract_year(base)
    title = clean_title(without_year, is_tv=is_tv)
    key = re.sub(r"[^0-9a-z]+", "", title.casefold())
    return key, year, (f"Season {season:02d}" if season is not None else None)


@dataclass
class Move:
    files: list[str]
    dest: str                 # folder name under the parent
    create: bool              # True when that folder does not exist yet
    season: str | None = None  # subfolder, e.g. "Season 01"


def plan_loose_files(
    files: list[str], existing_dirs: list[str], *, is_tv: bool
) -> tuple[list[Move], list[tuple[str, str]]]:
    """Group loose files at a parent's root into a folder per title.

    Returns (moves, unattributed). Anything that cannot be attributed with
    confidence is returned as unattributed and left exactly where it is — the
    cost of leaving a file alone is that a human looks at it, while the cost of
    guessing is a file filed under the wrong title.
    """
    # Where a title already has a folder, use ITS name, whatever spelling it has,
    # so loose extras land inside it rather than in a near-duplicate beside it.
    dir_by_key: dict[str, str] = {}
    for d in existing_dirs:
        k, _, _ = title_key(d, is_tv=is_tv)
        if k:
            dir_by_key.setdefault(k, d)

    anchors: dict[tuple[str, str | None], dict] = {}
    pending: list[tuple[str, str, int | None, str | None]] = []
    unattributed: list[tuple[str, str]] = []

    for name in files:
        stem, ext = split_ext(name)
        if ext in VIDEO_EXT:
            key, year, season = title_key(stem, is_tv=is_tv)
            if not key or key in GENERIC_STEMS or looks_unattributable(key, stem):
                unattributed.append((name, "no recognisable title"))
                continue
            slot = anchors.setdefault(
                (key, season), {"files": [], "year": year, "key": key, "season": season}
            )
            slot["files"].append(name)
            if slot["year"] is None:
                slot["year"] = year
        elif ext in SIDECAR_EXT:
            clean = strip_language_suffix(stem)
            key, year, season = title_key(clean, is_tv=is_tv)
            if not key or key in GENERIC_STEMS or looks_unattributable(key, clean):
                unattributed.append((name, "sidecar with no title of its own"))
                continue
            pending.append((name, key, year, season))
        else:
            unattributed.append((name, "not a media or sidecar file"))

    # A sidecar joins a group only if that group actually exists.
    for name, key, _year, season in pending:
        if (key, season) in anchors:
            anchors[(key, season)]["files"].append(name)
        elif is_tv and season is None and any(k == key for k, _ in anchors):
            for (k, s) in anchors:
                if k == key:
                    anchors[(k, s)]["files"].append(name)
                    break
        else:
            unattributed.append((name, "no matching video file here"))

    moves: list[Move] = []
    for slot in anchors.values():
        # Two files landing on one name would collide on the way in; the planner
        # must not pick a winner, so the whole group is handed back.
        seen: set[str] = set()
        collided = False
        for f in slot["files"]:
            if f in seen:
                collided = True
            seen.add(f)
        if collided:
            for f in slot["files"]:
                unattributed.append((f, "two files would collide in the destination"))
            continue

        existing = dir_by_key.get(slot["key"])
        if existing:
            dest, create = existing, False
        else:
            dest = target_name(_rebuild_name(slot, is_tv), is_tv=is_tv)
            create = True
            if not dest:
                for f in slot["files"]:
                    unattributed.append((f, "could not name a destination folder"))
                continue
        moves.append(Move(files=sorted(slot["files"]), dest=dest, create=create,
                          season=slot["season"]))
    moves.sort(key=lambda m: (m.dest, m.season or ""))
    return moves, unattributed


def _rebuild_name(slot: dict, is_tv: bool) -> str:
    """A representative name for the group, used to derive the folder name."""
    sample = slot["files"][0]
    stem, _ = split_ext(sample)
    if is_tv and (m := EPISODE_RE.search(stem)):
        stem = stem[: m.start()]
    return stem


@dataclass
class Change:
    parent: str
    old: str
    new: str
    action: str  # rename | keep | conflict | unsafe
    note: str = ""


@dataclass
class Plan:
    changes: list[Change] = field(default_factory=list)

    @property
    def renames(self) -> list[Change]:
        return [c for c in self.changes if c.action == "rename"]

    @property
    def problems(self) -> list[Change]:
        return [c for c in self.changes if c.action in ("conflict", "unsafe")]


def build_plan(parent: str, folders: list[str], *, is_tv: bool) -> Plan:
    """Decide what each folder under one parent should be called."""
    plan = Plan()
    existing = {f.casefold() for f in folders}
    claimed: dict[str, str] = {}
    for old in folders:
        new = target_name(old, is_tv=is_tv)
        if new == old:
            plan.changes.append(Change(parent, old, new, "keep"))
            continue
        if not new or "/" in new or "\\" in new:
            plan.changes.append(Change(parent, old, old, "unsafe", "produced an unusable name"))
            continue
        # Renaming onto an existing folder would merge two libraries' worth of
        # files, or fail halfway. Report it and let a human decide.
        if new.casefold() in existing and new.casefold() != old.casefold():
            plan.changes.append(Change(parent, old, new, "conflict", "target folder already exists"))
            continue
        if new.casefold() in claimed:
            plan.changes.append(
                Change(parent, old, new, "conflict", f"also the target of {claimed[new.casefold()]!r}")
            )
            continue
        claimed[new.casefold()] = old
        plan.changes.append(Change(parent, old, new, "rename"))
    return plan


# --------------------------------------------------------------------------
# DSM client: the only part that touches the network.
# --------------------------------------------------------------------------


class DSM:
    def __init__(self, base: str, insecure: bool = False):
        self.base = base.rstrip("/")
        self.sid: str | None = None
        self.ctx = ssl._create_unverified_context() if insecure else None

    def _call(self, cgi: str, params: dict) -> dict:
        if self.sid:
            params = {**params, "_sid": self.sid}
        url = f"{self.base}/webapi/{cgi}?" + urllib.parse.urlencode(params)
        with urllib.request.urlopen(url, context=self.ctx, timeout=60) as r:
            out = json.load(r)
        if not out.get("success"):
            code = out.get("error", {}).get("code")
            raise RuntimeError(f"DSM {cgi} {params.get('method')} failed (error {code})")
        return out.get("data", {})

    def login(self, user: str, password: str, otp: str = "") -> None:
        p = {
            "api": "SYNO.API.Auth", "version": "6", "method": "login",
            "account": user, "passwd": password, "session": "FileStation", "format": "sid",
        }
        if otp:
            p["otp_code"] = otp
        self.sid = self._call("auth.cgi", p)["sid"]

    def logout(self) -> None:
        if not self.sid:
            return
        try:
            self._call("auth.cgi", {"api": "SYNO.API.Auth", "version": "6",
                                    "method": "logout", "session": "FileStation"})
        except Exception:
            pass
        self.sid = None

    def _list(self, path: str, filetype: str) -> list[dict]:
        """List one folder, paging until DSM stops returning entries."""
        out: list[dict] = []
        offset = 0
        while True:
            data = self._call("entry.cgi", {
                "api": "SYNO.FileStation.List", "version": "2", "method": "list",
                "folder_path": path, "filetype": filetype,
                "limit": "1000", "offset": str(offset),
            })
            batch = data.get("files", [])
            out.extend(batch)
            if len(batch) < 1000:
                return out
            offset += len(batch)

    def list_dirs(self, path: str) -> list[str]:
        return [f["name"] for f in self._list(path, "dir") if f.get("isdir")]

    def list_files(self, path: str) -> list[str]:
        """Loose FILES at a folder's root — the ones a dir-only listing misses."""
        return [f["name"] for f in self._list(path, "file") if not f.get("isdir")]

    def create_folder(self, parent: str, name: str) -> None:
        self._call("entry.cgi", {
            "api": "SYNO.FileStation.CreateFolder", "version": "2", "method": "create",
            "folder_path": parent, "name": name, "force_parent": "false",
        })

    def move(self, paths: list[str], dest_folder: str) -> None:
        """Move files, refusing to overwrite anything already at the destination.

        CopyMove is asynchronous: it hands back a task id and the move continues
        on the NAS, so the result has to be polled. Returning without polling
        would report success for a move that later failed.
        """
        data = self._call("entry.cgi", {
            "api": "SYNO.FileStation.CopyMove", "version": "3", "method": "start",
            "path": ",".join(paths), "dest_folder_path": dest_folder,
            "remove_src": "true", "overwrite": "false",
        })
        taskid = data.get("taskid")
        if not taskid:
            raise RuntimeError("CopyMove returned no task id")
        deadline = time.time() + 1800
        while time.time() < deadline:
            st = self._call("entry.cgi", {
                "api": "SYNO.FileStation.CopyMove", "version": "3",
                "method": "status", "taskid": taskid,
            })
            if st.get("finished"):
                if st.get("errors"):
                    raise RuntimeError(f"move reported errors: {st['errors']}")
                return
            time.sleep(1)
        raise RuntimeError("move did not finish within 30 minutes")

    def rename(self, path: str, new_name: str) -> None:
        self._call("entry.cgi", {
            "api": "SYNO.FileStation.Rename", "version": "2", "method": "rename",
            "path": path, "name": new_name,
        })


# --------------------------------------------------------------------------
# CLI
# --------------------------------------------------------------------------


def print_moves(moves: list[Move], loose: list[tuple[str, str]]) -> None:
    if not moves and not loose:
        return
    print("\n  Loose files at the root of this folder:")
    for m in moves:
        target = f"{m.dest}/{m.season}" if m.season else m.dest
        mark = "  (new folder)" if m.create else ""
        print(f"    -> {target}{mark}")
        for f in m.files:
            print(f"         {f}")
    if loose:
        print("\n    LEFT ALONE — these need a human:")
        for name, why in loose:
            print(f"         {name}   [{why}]")
    total = sum(len(m.files) for m in moves)
    print(f"\n  {total} file(s) into {len(moves)} folder(s), {len(loose)} left alone")


def print_plan(plan: Plan) -> None:
    width = max((len(c.old) for c in plan.changes), default=0)
    for c in plan.changes:
        if c.action == "rename":
            print(f"  RENAME   {c.old:<{width}}  ->  {c.new}")
    for c in plan.problems:
        print(f"  SKIP     {c.old:<{width}}  ->  {c.new}   [{c.note}]")
    kept = sum(1 for c in plan.changes if c.action == "keep")
    print(f"\n  {len(plan.renames)} to rename, {len(plan.problems)} skipped, {kept} already correct")


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--nas", required=True, help="https://host:port")
    ap.add_argument("--user", required=True)
    ap.add_argument("--otp", default="", help="2FA code, if the account uses one")
    ap.add_argument("--movies", default="/movie", help="absolute path to the movies parent")
    ap.add_argument("--tv", default="/tv-show", help="absolute path to the TV parent")
    ap.add_argument("--insecure", action="store_true", help="accept a self-signed NAS certificate")
    ap.add_argument("--apply", action="store_true", help="actually rename (default is a dry run)")
    ap.add_argument("--undo", metavar="FILE", help="replay an undo file, reversing an earlier --apply")
    ap.add_argument("--no-group-loose", action="store_true",
                    help="only rename folders; leave loose files at the parent root alone")
    args = ap.parse_args(argv)

    password = os.environ.get("NAS_PASSWORD")
    if not password:
        print("Set NAS_PASSWORD in the environment (never pass a password on the command line —\n"
              "it is visible to every process on the machine via ps).", file=sys.stderr)
        return 2

    dsm = DSM(args.nas, insecure=args.insecure)
    dsm.login(args.user, password, args.otp)
    try:
        if args.undo:
            with open(args.undo) as fh:
                entries = json.load(fh)
            print(f"Reversing {len(entries)} operation(s) from {args.undo}\n")
            for e in reversed(entries):
                if e.get("op") == "move":
                    print(f"  {len(e['files'])} file(s) from {e['dest']}  ->  {e['parent']}")
                    if args.apply:
                        dsm.move([f"{e['dest']}/{f}" for f in e["files"]], e["parent"])
                    continue
                print(f"  {e['new']}  ->  {e['old']}")
                if args.apply:
                    dsm.rename(f"{e['parent']}/{e['new']}", e["old"])
            if not args.apply:
                print("\nDry run. Re-run with --apply to reverse these.")
            return 0

        applied: list[dict] = []
        for parent, is_tv in ((args.movies, False), (args.tv, True)):
            print(f"\n{parent}  ({'TV' if is_tv else 'movies'})")
            print("-" * (len(parent) + 12))
            folders = dsm.list_dirs(parent)
            plan = build_plan(parent, folders, is_tv=is_tv)
            print_plan(plan)

            for c in plan.renames:
                if not args.apply:
                    continue
                try:
                    dsm.rename(f"{parent}/{c.old}", c.new)
                    applied.append({"op": "rename", "parent": parent, "old": c.old, "new": c.new})
                except Exception as exc:  # one bad folder must not strand the rest
                    print(f"  FAILED   {c.old}: {exc}", file=sys.stderr)

            if args.no_group_loose:
                continue

            # Plan the moves against the names the folders have AFTER renaming,
            # so a loose file lands in the tidied folder rather than creating a
            # duplicate next to it. In a dry run the renames have not happened,
            # so apply them on paper first.
            renamed = {c.old: c.new for c in plan.renames}
            dirs_after = [renamed.get(d, d) for d in folders]
            try:
                files = dsm.list_files(parent)
            except Exception as exc:
                print(f"  (could not list loose files: {exc})")
                continue
            moves, loose = plan_loose_files(files, dirs_after, is_tv=is_tv)
            print_moves(moves, loose)
            if not args.apply:
                continue
            for m in moves:
                dest = f"{parent}/{m.dest}"
                try:
                    if m.create:
                        dsm.create_folder(parent, m.dest)
                    if m.season:
                        dsm.create_folder(dest, m.season)
                        dest = f"{dest}/{m.season}"
                    dsm.move([f"{parent}/{f}" for f in m.files], dest)
                    applied.append({"op": "move", "parent": parent, "dest": dest,
                                    "files": m.files})
                except Exception as exc:
                    print(f"  FAILED   -> {m.dest}: {exc}", file=sys.stderr)

        if args.apply and applied:
            stamp = time.strftime("%Y%m%d-%H%M%S")
            undo = f"library-tidy-undo-{stamp}.json"
            with open(undo, "w") as fh:
                json.dump(applied, fh, indent=1)
            print(f"\nRenamed {len(applied)} folder(s). To reverse:\n"
                  f"  python3 {sys.argv[0]} --nas {args.nas} --user {args.user} --undo {undo} --apply")
        elif not args.apply:
            print("\nDRY RUN — nothing was changed. Re-run with --apply to perform the\n"
                  "renames and moves above.")
        return 0
    finally:
        dsm.logout()


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
