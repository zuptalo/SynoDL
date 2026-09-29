"""Events: how a run tells the server what it is doing (spec 1053).

The operator tool keeps printing its human-readable lines. When it is run by the
server from Settings, the server cannot open the plan on the share — it never
mounts the library — so the tool ALSO prints a few fixed, machine-readable lines,
and the server reads only those (contracts/worker-events.md).

The design rule is that a reader on the other side must never have to trust this
output, and this side does not ask it to:

  * one line per event, prefixed `@@synodl `, JSON, with no way for a string to
    end the line or start another event (json.dumps escapes newlines, and control
    characters are removed anyway);
  * a fixed set of phases, kinds and refusal reasons — an unknown one is a bug
    here, not something to pass along;
  * every number clamped, every string length-capped, every list capped, so a
    summary has a known worst-case size that fits the line cap;
  * reasons are GENERALISED before they are counted ("cannot convert: A/b.mp3
    already exists" is "an mp3 of that name already exists"), so a count stays a
    count and a path does not ride along inside one.

The receiving side re-checks all of it. Both ends enforcing the same bounds is
deliberate: the worker is third-party-adjacent code reading a library that
contains other people's file names.
"""

from __future__ import annotations

import json
import re
import sys
from collections import Counter

PREFIX = "@@synodl "
MAX_PROGRESS_LINE = 4096
MAX_RESULT_LINE = 16384
INT_MAX = 10**15

PHASES = ("scan", "lookup", "plan", "apply", "restore")
KINDS = ("check", "apply", "undo")
REASONS = ("", "locked", "no_plan", "no_space", "rejected", "failed_steps")
PLAN_ID = re.compile(r"^\d{8}T\d{6}Z-[0-9a-f]{6}$")

MAX_REASONS = 12
MAX_CHECK_EXAMPLES = 20
MAX_OUTCOME_EXAMPLES = 10
PATH_CAP = 200
REASON_CAP = 120

_CONTROL = re.compile(r"[\x00-\x1f\x7f  ]")


def _clean(s, cap: int) -> str:
    return _CONTROL.sub(" ", str(s if s is not None else "")).strip()[:cap]


def _int(n) -> int:
    try:
        return max(0, min(int(n), INT_MAX))
    except (TypeError, ValueError):
        return 0


def _out(out):
    return out if out is not None else sys.stdout


def emit(obj: dict, *, cap: int, out=None):
    line = PREFIX + json.dumps(obj, ensure_ascii=False, separators=(",", ":"))
    if len(line.encode("utf-8")) > cap:
        # Never send an over-long line: the reader would drop the whole event.
        # Shed the optional lists first, then refuse.
        for key in ("check", "apply", "undo"):
            for lst in ("examples", "failedExamples", "skippedExamples", "skippedByReason", "byReason"):
                sec = obj.get(key)
                if isinstance(sec, dict):
                    if lst in sec:
                        sec[lst] = []
                    la = sec.get("leftAlone")
                    if isinstance(la, dict) and lst in la:
                        la[lst] = []
        line = PREFIX + json.dumps(obj, ensure_ascii=False, separators=(",", ":"))
        if len(line.encode("utf-8")) > cap:
            raise ValueError("event does not fit its line cap")
    o = _out(out)
    o.write(line + "\n")
    o.flush()


def progress(phase: str, done: int, total: int, *, out=None):
    if phase not in PHASES:
        raise ValueError(f"unknown phase {phase!r}")
    emit({"event": "progress", "phase": phase, "done": _int(done), "total": _int(total)},
         cap=MAX_PROGRESS_LINE, out=out)


def result(kind: str, *, ok: bool, reason: str = "", plan_id: str = "", plan_file: str = "",
           section: dict | None = None, out=None):
    if kind not in KINDS:
        raise ValueError(f"unknown kind {kind!r}")
    if reason not in REASONS:
        raise ValueError(f"unknown reason {reason!r}")
    obj = {"event": "result", "kind": kind, "ok": bool(ok)}
    if reason:
        obj["reason"] = reason
    if plan_id and PLAN_ID.match(plan_id):
        obj["planId"] = plan_id
    if plan_file:
        obj["planFile"] = _clean(plan_file, PATH_CAP)
    if section:
        obj.update(section)
    emit(obj, cap=MAX_RESULT_LINE, out=out)


# ---- summaries ------------------------------------------------------------

_GENERAL = (
    ("cannot convert", "an mp3 of that name already exists"),
    ("unreadable", "unreadable"),
    ("symbolic link", "symbolic link (not followed)"),
    ("no video id", "no video id"),
    ("no audio stream", "no audio stream"),
    ("destination exists", "destination exists"),
    ("original location is occupied", "original location is occupied"),
    ("source changed since", "source changed since the plan was made"),
)


def _norm_reason(reason) -> str:
    r = _clean(reason, 300)
    if r.startswith("cover: "):
        r = r[len("cover: "):]
    for head, general in _GENERAL:
        if r.startswith(head):
            return general
    return _clean(re.split(r"\s+\(|:\s", r, maxsplit=1)[0], REASON_CAP)


def _by_reason(reasons) -> list[dict]:
    counts = Counter(reasons)
    ranked = sorted(counts.items(), key=lambda kv: (-kv[1], kv[0]))[:MAX_REASONS]
    return [{"reason": r, "count": _int(c)} for r, c in ranked]


def summarise_plan(plan: dict, free_bytes) -> dict:
    t = plan.get("totals", {})
    skipped = plan.get("skipped", [])
    return {
        "tracks": _int(t.get("tracks")), "songs": _int(t.get("songs")),
        "duplicates": _int(t.get("duplicates_to_trash")), "moves": _int(t.get("moves")),
        "retags": _int(t.get("retags")), "covers": _int(t.get("covers")), "playlists": _int(t.get("playlists")),
        "conflicts": _int(t.get("conflicts")), "nameClashes": _int(t.get("name_clashes")),
        "matched": _int(t.get("matched")), "noMatch": _int(t.get("no_match")),
        "notLookedUp": _int(t.get("not_looked_up")), "toSingles": _int(t.get("to_singles")),
        "albumKnown": _int(t.get("album_known")), "orphanNfo": _int(t.get("orphan_nfo")),
        "bytesReclaimed": _int(t.get("bytes_reclaimed")), "bytesNeeded": _int(t.get("bytes_needed")),
        "freeBytes": _int(free_bytes),
        "leftAlone": {
            "total": _int(len(skipped)),
            "byReason": _by_reason(_norm_reason(s.get("reason")) for s in skipped),
            "examples": [{"path": _clean(s.get("relpath"), PATH_CAP), "reason": _norm_reason(s.get("reason"))}
                         for s in skipped[:MAX_CHECK_EXAMPLES]],
        },
    }


def _examples(items) -> list[dict]:
    return [{"path": _clean(i.get("src"), PATH_CAP), "note": _norm_reason(i.get("note"))}
            for i in items[:MAX_OUTCOME_EXAMPLES]]


def summarise_apply(res) -> dict:
    return {"done": _int(len(res.done)), "skipped": _int(len(res.skipped)), "failed": _int(len(res.failed)),
            "alreadyDone": _int(res.already),
            "skippedByReason": _by_reason(_norm_reason(s.get("note")) for s in res.skipped),
            "failedExamples": _examples(res.failed)}


def summarise_restore(res) -> dict:
    return {"restored": _int(len(res.done)), "skipped": _int(len(res.skipped)),
            "skippedExamples": _examples(res.skipped)}
