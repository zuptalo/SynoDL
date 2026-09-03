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
  * Every applied rename is written to an undo file, replayable with --undo.
  * A rename whose target already exists is REFUSED, never merged.
  * Only folder NAMES change. No file is moved, deleted, or written.
  * Only the immediate children of each parent are considered — no recursion.

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

    def list_dirs(self, path: str) -> list[str]:
        data = self._call("entry.cgi", {
            "api": "SYNO.FileStation.List", "version": "2", "method": "list",
            "folder_path": path, "filetype": "dir", "limit": "5000",
        })
        return [f["name"] for f in data.get("files", []) if f.get("isdir")]

    def rename(self, path: str, new_name: str) -> None:
        self._call("entry.cgi", {
            "api": "SYNO.FileStation.Rename", "version": "2", "method": "rename",
            "path": path, "name": new_name,
        })


# --------------------------------------------------------------------------
# CLI
# --------------------------------------------------------------------------


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
            print(f"Reversing {len(entries)} rename(s) from {args.undo}\n")
            for e in reversed(entries):
                src = f"{e['parent']}/{e['new']}"
                print(f"  {e['new']}  ->  {e['old']}")
                if args.apply:
                    dsm.rename(src, e["old"])
            if not args.apply:
                print("\nDry run. Re-run with --apply to reverse these.")
            return 0

        applied: list[dict] = []
        for parent, is_tv in ((args.movies, False), (args.tv, True)):
            print(f"\n{parent}  ({'TV' if is_tv else 'movies'})")
            print("-" * (len(parent) + 12))
            folders = dsm.list_dirs(parent)
            if not folders:
                print("  (empty or unreadable)")
                continue
            plan = build_plan(parent, folders, is_tv=is_tv)
            print_plan(plan)
            if not args.apply:
                continue
            for c in plan.renames:
                try:
                    dsm.rename(f"{parent}/{c.old}", c.new)
                    applied.append({"parent": parent, "old": c.old, "new": c.new})
                except Exception as exc:  # keep going: one bad folder must not strand the rest
                    print(f"  FAILED   {c.old}: {exc}", file=sys.stderr)

        if args.apply and applied:
            stamp = time.strftime("%Y%m%d-%H%M%S")
            undo = f"library-tidy-undo-{stamp}.json"
            with open(undo, "w") as fh:
                json.dump(applied, fh, indent=1)
            print(f"\nRenamed {len(applied)} folder(s). To reverse:\n"
                  f"  python3 {sys.argv[0]} --nas {args.nas} --user {args.user} --undo {undo} --apply")
        elif not args.apply:
            print("\nDRY RUN — nothing was changed. Re-run with --apply to perform the renames.")
        return 0
    finally:
        dsm.logout()


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
