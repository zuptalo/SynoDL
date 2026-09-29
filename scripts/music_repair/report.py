"""The human-readable plan (plan-<id>.md), rendered from the plan alone.

The operator reads THIS before deciding to apply, so it leads with what matters:
how much changes, what could not be decided, and how big the gap is where a song
was not matched. It is generated from the JSON, never the other way round —
apply reads only the JSON, so nothing typed into the .md can change what runs.
"""

from __future__ import annotations

from collections import Counter

SAMPLE = 20


def _mb(n):
    return f"{n / 1_000_000:,.1f} MB"


def render_md(plan: dict, free_bytes: int | None = None) -> str:
    t, actions = plan["totals"], plan["actions"]
    kinds = Counter(a["kind"] for a in actions)
    out = [f"# Music library repair plan `{plan['id']}`", "",
           f"Created {plan['created']} for `{plan['library']}`. **Nothing has been changed.** "
           f"Apply exactly this plan with `scripts/music-repair.sh apply {plan['id']}`.", ""]

    changes = sum(v for k, v in kinds.items() if k != "conflict")
    if changes == 0:
        out += ["## Nothing to do", "", "The library already matches what the repair would produce.", ""]

    out += ["## Totals", "", "| What | Count |", "|---|---:|"]
    rows = [("Tracks scanned", t["tracks"]), ("Distinct songs (by video id)", t["songs"]),
            ("Duplicates set aside in `.trash`", t["duplicates_to_trash"]),
            ("Space reclaimed once `.trash` is emptied", _mb(t["bytes_reclaimed"])),
            ("Files moved", t["moves"]), ("Files retagged", t["retags"]), ("Covers", t["covers"]),
            ("Playlist files written or extended", t["playlists"]), ("Converted to mp3", t["conversions"]),
            ("Orphaned media-server nfo files set aside", t["orphan_nfo"]),
            ("Same title, different videos (both kept; the later one's file name carries its video id)", t["name_clashes"]),
            ("Conflicts (nothing moved)", t["conflicts"]), ("No video id (kept, reported)", t["unidentified"])]
    out += [f"| {k} | {v} |" for k, v in rows]

    looked = t["matched"] + t["no_match"] + t["not_looked_up"]
    out += ["", "## Metadata", "", "| Outcome | Songs |", "|---|---:|",
            f"| Confident match (album, track number, year, ids, cover) | {t['matched']} |",
            f"| No confident match (filed in `Singles` or its existing album folder) | {t['no_match']} |",
            f"| Not looked up (a source was unreachable, or the plan was made offline) | {t['not_looked_up']} |",
            f"| Filed in `Singles` | {t['to_singles']} |", f"| Filed in a known album | {t['album_known']} |", ""]
    if t["no_match"]:
        out += ["A song is matched only when artist and title are equal AND its length is within 3 seconds of the "
                "file's. A video is often longer than the album cut, so many popular songs stay unmatched; the "
                "nearest length found is listed below.", ""]

    out += ["## Space", "", f"Needed by this plan: about {_mb(t['bytes_needed'])}."]
    if free_bytes is not None:
        verdict = "enough" if free_bytes >= t["bytes_needed"] * 1.05 else "**NOT ENOUGH — apply will refuse**"
        out.append(f"Free on the volume: {_mb(free_bytes)} ({verdict}).")
    out.append("")

    out += ["## Actions by kind", "", "| Kind | Count |", "|---|---:|"]
    out += [f"| {k} | {v} |" for k, v in sorted(kinds.items())]

    conflicts = [a for a in actions if a["kind"] == "conflict"]
    if conflicts:
        out += ["", f"## Conflicts ({len(conflicts)}) — neither file was moved", ""]
        out += [f"- `{a['src']}` → `{a['dst']}`: {a['reason']}" for a in conflicts[:200]]

    if plan["skipped"]:
        out += ["", f"## Left alone and reported ({len(plan['skipped'])})", ""]
        out += [f"- `{s['relpath']}` — {s['reason']}" for s in plan["skipped"][:200]]
        if len(plan["skipped"]) > 200:
            out.append(f"- … and {len(plan['skipped']) - 200} more (see the JSON)")

    nomatch = [a for a in actions if a["kind"] == "retag" and a["reason"].startswith("no confident match")]
    if nomatch:
        out += ["", f"## No confident match ({len(nomatch)}) — first 50", ""]
        out += [f"- `{a['src']}` — {a['reason']}" for a in nomatch[:50]]

    trash = sorted((a for a in actions if a["kind"] == "trash" and a["src"].endswith(".mp3")),
                   key=lambda a: -a.get("size", 0))
    if trash:
        out += ["", "## Largest duplicates set aside", ""]
        out += [f"- {_mb(a.get('size', 0))} — `{a['src']}`" for a in trash[:25]]

    out += ["", f"## Samples ({SAMPLE} per kind)", ""]
    for kind in sorted(kinds):
        if kind in ("conflict", "playlist"):
            continue
        out += [f"### {kind}", ""]
        for a in [x for x in actions if x["kind"] == kind][:SAMPLE]:
            line = f"- `{a.get('src', a.get('audio', ''))}`"
            if a.get("dst"):
                line += f" → `{a['dst']}`"
            out.append(f"{line} — {a['reason']}")
        out.append("")
    pls = [a for a in actions if a["kind"] == "playlist"]
    if pls:
        out += ["### playlists", ""]
        out += [f"- `{a['dst']}` — {a['reason']}" for a in pls[:SAMPLE]]
        out.append("")
    return "\n".join(out) + "\n"
