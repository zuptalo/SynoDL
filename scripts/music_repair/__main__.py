"""Command line: `python3 -m music_repair plan|apply|restore` (see contracts/cli.md).

Run as a Kubernetes Job by scripts/music-repair.sh; also runnable against any
directory, which is how it is tested. Exit codes: 0 ok · 2 bad arguments ·
3 a step failed · 4 refused (no such plan, not enough space, another run holds the
lock, or a plan that names something outside the library).
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import secrets
import shutil
import signal
import sys
import time

from . import VERSION, applier, events, identity, inventory, planner, report, sources

OK, FAILED, REFUSED = 0, 3, 4


def _atomic_write(path: str, text: str):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    tmp = path + ".tmp"
    with open(tmp, "w", encoding="utf-8") as f:
        f.write(text)
        f.flush()
        os.fsync(f.fileno())
    os.replace(tmp, path)


def _paths(args):
    library = os.path.abspath(args.library)
    return library, os.path.abspath(args.repair_dir) if args.repair_dir else os.path.join(library, ".repair")


def cmd_plan(args, transport, fetch_cover) -> int:
    library, repair = _paths(args)
    if not os.path.isdir(library):
        print(f"no such library: {library}", file=sys.stderr)
        events.result("check", ok=False, reason="rejected")
        return REFUSED
    lock = applier.Lock(repair)
    try:
        lock.acquire()
    except applier.Locked as exc:
        print(f"refused: {exc}", file=sys.stderr)
        events.result("check", ok=False, reason="locked")
        return REFUSED
    try:
        print(f"[plan] scanning {library}", flush=True)
        events.progress("scan", 0, 0)
        inv = inventory.scan(library, limit=args.limit)
        print(f"[plan] {len(inv.tracks)} tracks, {sum(len(v) for v in inv.other.values())} other files", flush=True)
        events.progress("scan", len(inv.tracks), len(inv.tracks))
        songs, _ = identity.group(inv.tracks)
        results = {}
        if not args.no_lookup:
            cache = sources.Cache(os.path.join(repair, "cache.json"))
            lookup = sources.Lookup(transport=transport or sources.http_transport, cache=cache)
            todo = [s for s in songs if planner.needs_lookup(s.kept)]
            events.progress("lookup", 0, len(todo))
            try:
                for i, s in enumerate(todo, 1):
                    p = planner.parse_kept(s.kept)
                    results[s.key] = lookup.resolve(key=s.key, lead=p.lead, title=p.title,
                                                    duration_s=s.kept.duration_s, featured=p.featured,
                                                    alt_leads=p.leads[1:])
                    if i % 25 == 0 or i == len(todo):
                        print(f"[plan] looked up {i}/{len(todo)}", flush=True)
                        events.progress("lookup", i, len(todo))
            finally:
                cache.flush()      # whatever ended the loop, the answers already found are kept
        plan_id = time.strftime("%Y%m%dT%H%M%SZ", time.gmtime()) + "-" + secrets.token_hex(3)
        events.progress("plan", 0, 0)
        plan = planner.build_plan(inv, results, plan_id=plan_id)
        doc = plan.to_dict()
        try:
            free = shutil.disk_usage(library).free
        except OSError:
            free = None
        _atomic_write(os.path.join(repair, f"plan-{plan_id}.json"), json.dumps(doc, ensure_ascii=False, indent=1))
        md_path = os.path.join(repair, f"plan-{plan_id}.md")
        _atomic_write(md_path, report.render_md(doc, free))
        t = plan.totals
        print(f"[plan] plan id {plan_id}")
        print(f"[plan] {len(plan.changes)} changes: {t['duplicates_to_trash']} duplicates set aside, {t['moves']} moves, "
              f"{t['retags']} retags, {t['playlists']} playlists, {t['conversions']} conversions, "
              f"{t['conflicts']} conflicts")
        print(f"[plan] metadata: {t['matched']} matched, {t['no_match']} no confident match, "
              f"{t['not_looked_up']} not looked up")
        if not plan.changes:
            print("[plan] nothing to do")
        print(f"[plan] read {md_path}")
        rel = os.path.relpath(md_path, library)
        events.result("check", ok=True, plan_id=plan_id, plan_file="" if rel.startswith("..") else rel,
                      section={"check": events.summarise_plan(doc, free)})
        return OK
    finally:
        lock.release()


def _load_plan(repair: str, plan_id: str):
    path = os.path.join(repair, f"plan-{plan_id}.json")
    if not os.path.isfile(path):
        return None
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def cmd_apply(args, fetch_cover) -> int:
    library, repair = _paths(args)
    plan = _load_plan(repair, args.plan)
    if plan is None:
        print(f"refused: no plan {args.plan} in {repair}", file=sys.stderr)
        events.result("apply", ok=False, reason="no_plan")
        return REFUSED
    try:
        events.progress("apply", 0, len(plan.get("actions", [])))
        res = applier.apply(plan, library, repair, fetch_cover=fetch_cover,
                            progress=lambda i, n: events.progress("apply", i, n))
    except (applier.PlanRejected, applier.NotEnoughSpace, applier.Locked) as exc:
        print(f"refused: {exc}", file=sys.stderr)
        reason = {applier.PlanRejected: "rejected", applier.NotEnoughSpace: "no_space",
                  applier.Locked: "locked"}[type(exc)]
        events.result("apply", ok=False, reason=reason, plan_id=args.plan)
        return REFUSED
    c = res.counts()
    print(f"[apply] done {c['done']}, skipped {c['skipped']}, failed {c['failed']}, already done {c['already_done']}")
    for s in res.skipped[:20]:
        print(f"[apply] skipped {s['kind']} {s.get('src') or ''}: {s['note']}")
    for f in res.failed[:20]:
        print(f"[apply] FAILED {f['kind']} {f.get('src') or ''}: {f['note']}")
    events.result("apply", ok=not res.failed, reason="failed_steps" if res.failed else "", plan_id=args.plan,
                  section={"apply": events.summarise_apply(res)})
    return FAILED if res.failed else OK


def cmd_restore(args) -> int:
    library, repair = _paths(args)
    try:
        events.progress("restore", 0, 0)
        res = applier.restore(args.plan, library, repair)
    except (applier.PlanRejected, applier.Locked) as exc:
        print(f"refused: {exc}", file=sys.stderr)
        events.result("undo", ok=False, reason="locked" if isinstance(exc, applier.Locked) else "no_plan",
                      plan_id=args.plan)
        return REFUSED
    c = res.counts()
    print(f"[restore] restored {c['done']}, skipped {c['skipped']}")
    for s in res.skipped[:20]:
        print(f"[restore] skipped {s['kind']} {s.get('src') or ''}: {s['note']}")
    events.result("undo", ok=True, plan_id=args.plan, section={"undo": events.summarise_restore(res)})
    return OK


def build_parser():
    p = argparse.ArgumentParser(prog="music_repair", description=__doc__.splitlines()[0])
    p.add_argument("--version", action="version", version=VERSION)
    sub = p.add_subparsers(dest="command", required=True)

    def common(sp):
        sp.add_argument("--library", required=True, help="the music library directory")
        sp.add_argument("--repair-dir", help="where plans/results live (default <library>/.repair)")
        sp.add_argument("--verbose", action="store_true")

    sp = sub.add_parser("plan", help="read the library and write a reviewable plan; changes nothing else")
    common(sp)
    sp.add_argument("--no-lookup", action="store_true", help="skip all network lookups")
    sp.add_argument("--limit", type=int, help="plan only the first N artist folders")
    sp = sub.add_parser("apply", help="carry out exactly one reviewed plan")
    common(sp)
    sp.add_argument("--plan", required=True, help="the plan id printed by `plan`")
    sp = sub.add_parser("restore", help="undo an applied plan")
    common(sp)
    sp.add_argument("--plan", required=True)
    return p


def _terminate(signum, frame):
    raise SystemExit(128 + signum)


def main(argv=None, *, transport=None, fetch_cover=None) -> int:
    args = build_parser().parse_args(argv)
    logging.basicConfig(level=logging.DEBUG if args.verbose else logging.INFO, format="%(message)s")
    # Kubernetes ends a Job with SIGTERM (its deadline, `kubectl delete`, a node
    # drain). Python's default action skips `finally` blocks, which would leave
    # .repair/lock behind and lose the lookups not yet flushed. Turning it into
    # SystemExit makes every finally run.
    try:
        previous = signal.signal(signal.SIGTERM, _terminate)
    except ValueError:      # not the main thread (e.g. a test runner)
        previous = None
    try:
        if args.command == "plan":
            return cmd_plan(args, transport, fetch_cover)
        if args.command == "apply":
            return cmd_apply(args, fetch_cover)
        return cmd_restore(args)
    finally:
        if previous is not None:
            signal.signal(signal.SIGTERM, previous)


if __name__ == "__main__":
    sys.exit(main())
