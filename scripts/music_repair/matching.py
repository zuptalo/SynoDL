"""The confidence rule, and choosing which release is "the album".

A wrong album is worse than none, so the bar is strict and OBJECTIVE (spec 1052,
clarified): the artist AND the cleaned title match exactly after normalising
case and punctuation, AND the recording's length is within ±3 seconds of the
file's. A near-miss is "no match". The length is the strongest signal we hold
for a file that came from a video — it is what tells "In Da Club" the album cut
from "In Da Club" the music video.

Trade-off, stated because it will surprise: a video is often longer than the
album cut (intros, skits), so many popular songs stay in `Singles`. That is the
rule working, not failing. The plan report says how many, and by how much.
"""

from __future__ import annotations

from dataclasses import dataclass, field

from . import names

DURATION_TOLERANCE_S = 3.0

# A release-group with ANY of these secondary types is not "the album": the
# recording sits on it, but it is a compilation of other people's choices, a
# concert, a soundtrack. Filing 50 Cent under "Bravo Hits Party: 2000ER" would be
# worse than leaving him in Singles.
_PRIMARY_RANK = {"Album": 0, "EP": 1, "Single": 2}


@dataclass
class Comparison:
    confident: bool
    artist_eq: bool
    title_eq: bool
    delta_s: float | None


def compare(lead: str, title: str, duration_s: float | None,
            cand_artist: str, cand_title: str, cand_length_s: float | None) -> Comparison:
    artist_eq = names.fold(lead) != "" and names.fold(lead) == names.fold(cand_artist)
    title_eq = names.fold(names.clean_title(title)[0]) == names.fold(names.clean_title(cand_title)[0]) \
        and names.fold(title) != ""
    if not duration_s or duration_s <= 0 or cand_length_s is None:
        return Comparison(False, artist_eq, title_eq, None)
    delta = float(cand_length_s) - float(duration_s)
    # Round to the millisecond: MusicBrainz lengths are integral ms, and a value
    # that is 3.0000000001 seconds off is three seconds off.
    ok = artist_eq and title_eq and round(abs(delta), 3) <= DURATION_TOLERANCE_S
    return Comparison(ok, artist_eq, title_eq, delta)


def choose_release(releases: list[dict]) -> dict | None:
    """The release to call the album, or None when there is no honest one.

    Album beats EP beats Single (a song's own single predates the album, and a
    library shelves by album); then the earliest date; then id, so the answer is
    the same every run.
    """
    candidates = []
    for r in releases or []:
        group = r.get("release-group") or {}
        if r.get("status") not in (None, "Official"):
            continue
        if group.get("secondary-types"):
            continue
        rank = _PRIMARY_RANK.get(group.get("primary-type"))
        if rank is None:
            continue
        candidates.append((rank, r.get("date") or "9999", r.get("id") or "", r))
    if not candidates:
        return None
    candidates.sort(key=lambda c: c[:3])
    return candidates[0][3]


def year_of(date) -> int | None:
    try:
        return int(str(date)[:4])
    except (TypeError, ValueError):
        return None


@dataclass
class Match:
    """A source's confident answer for a song. Only ever built when confident."""
    source: str
    artist: str
    title: str
    length_s: float
    delta_s: float
    featured: list[str] = field(default_factory=list)
    album: str | None = None
    track_no: int | None = None
    year: int | None = None
    mbid_recording: str | None = None
    mbid_release: str | None = None
    cover_url: str | None = None

    def to_dict(self) -> dict:
        return dict(self.__dict__)

    @staticmethod
    def from_dict(d: dict) -> "Match":
        return Match(**{k: d[k] for k in Match.__dataclass_fields__ if k in d})
