"""The applier: carry out EXACTLY a reviewed plan, safely, resumably, reversibly.

Everything that can lose data is handled here, so the rules are worth stating:

  * It never deletes. What leaves the library goes to `.trash/<plan id>/` with its
    original path preserved; nothing empties `.trash`.
  * It never overwrites. A destination that exists is a failure of that step, and
    the source stays where it was.
  * A plan is an input, not an authority. Every path is checked to be a plain
    library-relative one that crosses no symbolic link (FR-021) BEFORE the first
    step, and the whole plan is refused if any is not.
  * A source that changed since the plan was made is skipped and reported (FR-003).
    "Changed" is tracked PER FILE through the journal: retagging legitimately
    changes a file's size, so the step after it (the move) is compared with what
    this run left, not with the plan. That is also what lets an interrupted run
    resume without mistaking its own earlier work for somebody else's edit.
  * Every completed step is journaled with what it changed, including the previous
    value of every tag, so `restore` can undo it (SC-010) and an interrupted run
    can resume (FR-018).
  * One run at a time (FR-022). A lock left by a crashed run is REPORTED with who
    and when; it is never silently taken over.
"""

from __future__ import annotations

import hashlib
import json
import os
import posixpath
import shutil
import socket
import subprocess
import time
from dataclasses import dataclass, field

from . import names, playlists, sources, tagio

KINDS = {"convert", "retag", "move", "sidecar", "cover", "rename_bin", "playlist", "merge_dir",
         "orphan_nfo", "trash", "conflict"}
SPACE_MARGIN = 1.05


class PlanRejected(Exception):
    """The plan names something it must not. Nothing was changed."""


class NotEnoughSpace(Exception):
    pass


class Locked(Exception):
    pass


@dataclass
class Results:
    plan: str
    done: list = field(default_factory=list)
    skipped: list = field(default_factory=list)
    failed: list = field(default_factory=list)
    already: int = 0

    def counts(self):
        return {"done": len(self.done), "skipped": len(self.skipped), "failed": len(self.failed),
                "already_done": self.already}


class Lock:
    def __init__(self, repair_dir: str):
        self.path = os.path.join(repair_dir, "lock")
        self.held = False

    def acquire(self):
        os.makedirs(os.path.dirname(self.path), exist_ok=True)
        try:
            fd = os.open(self.path, os.O_CREAT | os.O_EXCL | os.O_WRONLY)
        except FileExistsError:
            try:
                with open(self.path, encoding="utf-8") as f:
                    info = json.load(f)
            except (OSError, ValueError):
                info = {}
            raise Locked(f"another run holds {self.path} (pid {info.get('pid', '?')} on {info.get('host', '?')}, "
                         f"since {info.get('since', '?')}). If that run is gone, remove the lock file yourself — "
                         f"it is never taken over automatically.") from None
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            json.dump({"pid": os.getpid(), "host": socket.gethostname(),
                       "since": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())}, f)
        self.held = True

    def release(self):
        if self.held:
            try:
                os.unlink(self.path)
            except OSError:
                pass
            self.held = False


# ---- validation -----------------------------------------------------------

def _no_symlink(library: str, rel: str) -> bool:
    cur = library
    for part in rel.split("/"):
        cur = os.path.join(cur, part)
        if os.path.islink(cur):
            return False
        if not os.path.lexists(cur):
            break
    return True


def validate(plan: dict, library: str):
    if plan.get("version") != 1 or not isinstance(plan.get("actions"), list) or not plan.get("id"):
        raise PlanRejected("not a plan this version understands")
    pid = plan["id"]
    for a in plan["actions"]:
        if a.get("kind") not in KINDS:
            raise PlanRejected(f"unknown action kind {a.get('kind')!r}")
        paths = [a.get(k) for k in ("src", "dst", "audio", "folder") if a.get(k) is not None]
        paths += [e.get("relpath") for e in a.get("entries", [])]
        for rel in paths:
            if not names.safe_rel(rel):
                raise PlanRejected(f"{a.get('id')}: path leaves the library: {rel!r}")
            if not _no_symlink(library, rel):
                raise PlanRejected(f"{a.get('id')}: path crosses a symbolic link: {rel!r}")
        if a["kind"] in ("trash", "orphan_nfo") and a.get("dst") != f".trash/{pid}/{a.get('src')}":
            raise PlanRejected(f"{a.get('id')}: a set-aside must go to .trash/{pid}/")


# ---- journal --------------------------------------------------------------

class Journal:
    def __init__(self, repair_dir: str, plan_id: str):
        self.path = os.path.join(repair_dir, f"journal-{plan_id}.jsonl")
        self.records: list[dict] = []
        if os.path.exists(self.path):
            with open(self.path, encoding="utf-8") as f:
                for line in f:
                    try:
                        self.records.append(json.loads(line))
                    except ValueError:
                        pass          # a torn final line from an interrupted run
        self._f = None
        self._n = 0

    def settled_ids(self):
        return {r["action_id"] for r in self.records if r.get("status") in ("done", "skipped")}

    def states(self):
        out = {}
        for r in self.records:
            if r.get("state"):
                out[r["src"]] = r["state"]
        return out

    def write(self, rec: dict):
        if self._f is None:
            os.makedirs(os.path.dirname(self.path), exist_ok=True)
            self._f = open(self.path, "a", encoding="utf-8")
        self._f.write(json.dumps(rec, ensure_ascii=False) + "\n")
        self._f.flush()
        self._n += 1
        if self._n % 100 == 0:
            os.fsync(self._f.fileno())
        self.records.append(rec)

    def close(self):
        if self._f is not None:
            self._f.flush()
            os.fsync(self._f.fileno())
            self._f.close()
            self._f = None


# ---- apply ----------------------------------------------------------------

def apply(plan: dict, library: str, repair_dir: str, *, fetch_cover=None, disk_usage=shutil.disk_usage) -> Results:
    validate(plan, library)
    need = int(plan.get("totals", {}).get("bytes_needed", 0) * SPACE_MARGIN)
    free = disk_usage(library).free
    if free < need:
        raise NotEnoughSpace(f"not enough free space: need about {need} bytes, {free} free — short by {need - free}")
    lock = Lock(repair_dir)
    lock.acquire()
    try:
        return _Run(plan, library, repair_dir, fetch_cover or sources.fetch_cover).run()
    finally:
        lock.release()


class _Run:
    def __init__(self, plan, library, repair_dir, fetch_cover):
        self.plan, self.library, self.repair = plan, library, repair_dir
        self.id = plan["id"]
        self.fetch_cover = fetch_cover
        self.journal = Journal(repair_dir, self.id)
        self.settled = self.journal.settled_ids()
        self.state = self.journal.states()
        self.res = Results(self.id)
        self.covers: dict[str, object] = {}
        self.dst_to_src = {a["dst"]: a["src"] for a in plan["actions"] if a["kind"] == "move"}
        self.moved_from: list[str] = []

    def abs(self, rel):
        return os.path.join(self.library, *rel.split("/"))

    def run(self):
        actions = self.plan["actions"]
        try:
            for i, a in enumerate(actions, 1):
                if a["id"] in self.settled:
                    self.res.already += 1
                    continue
                try:
                    status, note, extra = self.step(a)
                except (KeyboardInterrupt, SystemExit):
                    raise
                except Exception as exc:   # one bad step must not abandon the rest
                    status, note, extra = "failed", f"{type(exc).__name__}: {exc}", {}
                rec = {"action_id": a["id"], "kind": a["kind"], "status": status, "note": note,
                       "src": a.get("src"), "dst": a.get("dst"), **extra}
                self.journal.write(rec)
                bucket = {"done": self.res.done, "skipped": self.res.skipped, "failed": self.res.failed}[status]
                bucket.append({"action_id": a["id"], "kind": a["kind"], "src": a.get("src"), "note": note})
                if i % 200 == 0:
                    print(f"[apply] {i}/{len(actions)}", flush=True)
            self.cleanup_dirs()
        finally:
            self.journal.close()
        self.write_results()
        return self.res

    # -- fingerprint ------------------------------------------------------

    def current(self, a):
        """(relpath now, None) or (None, why it cannot be acted on)."""
        st = self.state.get(a["src"])
        rel = st["rel"] if st else a["src"]
        try:
            info = os.lstat(self.abs(rel))
        except FileNotFoundError:
            return None, "source no longer exists"
        want = (st["size"], st["mtime_ns"]) if st else (a.get("size"), a.get("mtime_ns"))
        if want[0] is not None and (info.st_size, info.st_mtime_ns) != tuple(want):
            # A retag legitimately changes the size. If the run was killed after
            # rewriting the tags but before journaling it, the file carries THIS
            # plan's marker: that is our own earlier write, not somebody's edit.
            if rel.endswith(".mp3") and self._ours(rel):
                return rel, None
            return None, "source changed since the plan was made"
        return rel, None

    def _ours(self, rel):
        try:
            return tagio.repair_marker(self.abs(rel)) == self.id
        except Exception:
            return False

    def started(self, action_id):
        """The journal record written BEFORE an in-place edit, if the run was killed after it."""
        return next((r for r in reversed(self.journal.records)
                     if r.get("action_id") == action_id and r.get("status") == "started"), None)

    def remember(self, a, rel):
        info = os.stat(self.abs(rel))
        self.state[a["src"]] = {"rel": rel, "size": info.st_size, "mtime_ns": info.st_mtime_ns}
        return {"rel": rel, "size": info.st_size, "mtime_ns": info.st_mtime_ns}

    # -- steps ------------------------------------------------------------

    def step(self, a):
        return getattr(self, "do_" + a["kind"])(a)

    def do_conflict(self, a):
        return "skipped", "reported conflict; nothing moved", {}

    def do_merge_dir(self, a):
        return "done", "folder is emptied by the moves and removed if nothing else is in it", {}

    def _rename(self, a, src_rel, dst_rel):
        if os.path.lexists(self.abs(dst_rel)):
            return "failed", f"destination exists: {dst_rel} (nothing overwritten)", {}
        os.makedirs(os.path.dirname(self.abs(dst_rel)), exist_ok=True)
        os.rename(self.abs(src_rel), self.abs(dst_rel))
        self.moved_from.append(src_rel)
        return "done", "", {"state": self.remember(a, dst_rel)}

    def do_move(self, a):
        rel, why = self.current(a)
        if rel is None:
            return "skipped", why, {}
        return self._rename(a, rel, a["dst"])

    do_rename_bin = do_move
    do_trash = do_move
    do_orphan_nfo = do_move

    def do_sidecar(self, a):
        if not os.path.exists(self.abs(a["audio"])):
            return "skipped", "its audio was not moved, so the lyrics stay with it", {}
        rel, why = self.current(a)
        if rel is None:
            return "skipped", why, {}
        return self._rename(a, rel, a["dst"])

    def do_retag(self, a):
        rel, why = self.current(a)
        if rel is None:
            return "skipped", why, {}
        path = self.abs(rel)
        begun = self.started(a["id"])
        if begun and self._ours(rel):
            # Killed after the write, before the journal: the tags are already
            # there, and what they REPLACED is in the record written before it.
            previous = begun["previous_tags"]
        else:
            # Journal what is about to be overwritten BEFORE overwriting it, so a
            # kill at any point leaves enough to finish or to undo.
            previous = tagio.read_tags(path, list(a["tags"]))
            self.journal.write({"action_id": a["id"], "kind": "retag", "status": "started", "src": a["src"],
                                "path": rel, "previous_tags": previous})
            tagio.write_tags(path, a["tags"])
        return "done", "", {"path": rel, "previous_tags": previous, "state": self.remember(a, rel)}

    def do_convert(self, a):
        rel, why = self.current(a)
        if rel is None:
            return "skipped", why, {}
        dst = a["dst"]
        if os.path.lexists(self.abs(dst)):
            return "failed", f"destination exists: {dst}", {}
        os.makedirs(os.path.dirname(self.abs(dst)), exist_ok=True)
        tmp = self.abs(dst) + f".{self.id}.tmp.mp3"
        out = subprocess.run(["ffmpeg", "-nostdin", "-loglevel", "error", "-y", "-i", self.abs(rel), "-vn",
                              "-codec:a", "libmp3lame", "-q:a", "2", tmp], capture_output=True, text=True)
        if out.returncode != 0:
            if os.path.exists(tmp):
                os.unlink(tmp)
            return "failed", "ffmpeg could not convert it", {}
        tagio.write_tags(tmp, a["tags"])
        os.rename(tmp, self.abs(dst))
        return "done", "", {"created": dst}

    def do_cover(self, a):
        audio = a["audio"]
        if not os.path.exists(self.abs(audio)):
            return "skipped", "cover: its audio was not moved", {}
        url = a["cover_url"]
        if url not in self.covers:
            try:
                self.covers[url] = self.fetch_cover(url)
            except sources.NotFound:
                self.covers[url] = "no cover art"
            except (sources.FetchError, sources.Blocked) as exc:
                self.covers[url] = f"cover source unreachable ({type(exc).__name__})"
        got = self.covers[url]
        if isinstance(got, str):
            return "skipped", f"cover: {got}", {}
        data, mime = got
        begun = self.started(a["id"])
        if begun:
            stash = begun["previous_cover"]      # killed after the embed: what it replaced was journaled first
        else:
            previous = tagio.read_cover(self.abs(audio))
            stash = None
            if previous:
                sha = hashlib.sha1(previous[0]).hexdigest()
                d = os.path.join(self.repair, f"prev-covers-{self.id}")
                os.makedirs(d, exist_ok=True)
                with open(os.path.join(d, sha), "wb") as f:
                    f.write(previous[0])
                stash = {"sha": sha, "mime": previous[1]}
            self.journal.write({"action_id": a["id"], "kind": "cover", "status": "started", "src": None,
                                "audio": audio, "previous_cover": stash})
        tagio.embed_cover(self.abs(audio), data, mime)
        created = None
        folder_jpg = posixpath.join(a["folder"], "folder.jpg")
        if not os.path.exists(self.abs(folder_jpg)):
            tmp = self.abs(folder_jpg) + f".{self.id}.tmp"
            with open(tmp, "wb") as f:
                f.write(data)
            os.replace(tmp, self.abs(folder_jpg))
            created = folder_jpg
        return "done", "", {"audio": audio, "previous_cover": stash, "created": created}

    def do_playlist(self, a):
        dst = self.abs(a["dst"])
        entries = []
        for e in a["entries"]:
            rel = e["relpath"]
            if not os.path.exists(self.abs(rel)):
                back = self.dst_to_src.get(rel)      # its move did not happen: point at where it still is
                rel = back if back and os.path.exists(self.abs(back)) else None
            if rel:
                entries.append(playlists.Entry(e["artist"], e["title"], rel, e.get("duration_s", 0)))
        if not entries:
            return "skipped", "playlist: none of its songs could be found", {}
        previous = None
        if os.path.exists(dst):
            with open(dst, encoding="utf-8", errors="replace") as f:
                previous = f.read()
        text = playlists.merge(previous or "", entries)
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        tmp = dst + f".{self.id}.tmp"
        with open(tmp, "w", encoding="utf-8", newline="\n") as f:
            f.write(text)
        os.replace(tmp, dst)
        return "done", "", {"previous_text": previous, "created": None if previous is not None else a["dst"]}

    # -- cleanup ----------------------------------------------------------

    def cleanup_dirs(self):
        """Remove folders the moves emptied. rmdir only ever succeeds on an empty one."""
        dirs = set()
        for rel in self.moved_from:
            d = posixpath.dirname(rel)
            while d:
                dirs.add(d)
                d = posixpath.dirname(d)
        for d in sorted(dirs, key=lambda d: (-d.count("/"), d)):
            top = d.split("/")[0]
            if top in (".trash", ".repair", "Playlists"):
                continue
            path = self.abs(d)
            try:
                if os.listdir(path) == [".DS_Store"]:
                    os.unlink(os.path.join(path, ".DS_Store"))
                os.rmdir(path)
            except OSError:
                pass

    def write_results(self):
        os.makedirs(self.repair, exist_ok=True)
        path = os.path.join(self.repair, f"results-{self.id}.json")
        tmp = path + ".tmp"
        with open(tmp, "w", encoding="utf-8") as f:
            json.dump({"plan": self.id, "finished": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                       "counts": self.res.counts(), "failed": self.res.failed, "skipped": self.res.skipped},
                      f, ensure_ascii=False, indent=1)
        os.replace(tmp, path)


# ---- restore --------------------------------------------------------------

def _has_cover_changed(path: str, rec: dict) -> bool:
    """For a cover step that only STARTED: is there anything to undo?"""
    return tagio.read_cover(path) is not None



def restore(plan_id: str, library: str, repair_dir: str) -> Results:
    journal = Journal(repair_dir, plan_id)
    if not journal.records:
        raise PlanRejected(f"no journal for plan {plan_id}: nothing to restore")
    lock = Lock(repair_dir)
    lock.acquire()
    res = Results(plan_id)

    def A(rel):
        return os.path.join(library, *rel.split("/"))

    def back(rec, src, dst):
        if not os.path.lexists(A(dst)):
            res.skipped.append({"action_id": rec["action_id"], "kind": rec["kind"], "src": src,
                                "note": f"{dst} is no longer there"})
        elif os.path.lexists(A(src)):
            res.skipped.append({"action_id": rec["action_id"], "kind": rec["kind"], "src": src,
                                "note": f"original location is occupied: {src}"})
        else:
            os.makedirs(os.path.dirname(A(src)), exist_ok=True)
            os.rename(A(dst), A(src))
            res.done.append({"action_id": rec["action_id"], "kind": rec["kind"], "src": src, "note": ""})

    try:
        finished = {r["action_id"] for r in journal.records if r.get("status") == "done"}
        # A step whose edit began but whose journal line never landed (the run was
        # killed) still changed the file, so it is undone too.
        replay = [r for r in journal.records if r.get("status") == "done"
                  or (r.get("status") == "started" and r["action_id"] not in finished)]
        for rec in reversed(replay):
            kind = rec["kind"]
            if kind in ("move", "rename_bin", "sidecar", "trash", "orphan_nfo"):
                back(rec, rec["src"], rec["dst"])
            elif kind == "retag":
                path = next((p for p in (rec.get("path"), rec["src"]) if p and os.path.exists(A(p))), None)
                if path and (rec["status"] == "done" or tagio.repair_marker(A(path)) == plan_id):
                    tagio.restore_tags(A(path), rec["previous_tags"])
                    res.done.append({"action_id": rec["action_id"], "kind": kind, "src": rec["src"], "note": ""})
            elif kind == "cover":
                if os.path.exists(A(rec["audio"])) and (rec["status"] == "done"
                                                        or _has_cover_changed(A(rec["audio"]), rec)):
                    prev = None
                    if rec.get("previous_cover"):
                        sha = rec["previous_cover"]["sha"]
                        with open(os.path.join(repair_dir, f"prev-covers-{plan_id}", sha), "rb") as f:
                            prev = (f.read(), rec["previous_cover"]["mime"])
                    tagio.restore_cover(A(rec["audio"]), prev)
                if rec.get("created") and os.path.exists(A(rec["created"])):
                    os.unlink(A(rec["created"]))
                res.done.append({"action_id": rec["action_id"], "kind": kind, "src": None, "note": ""})
            elif kind == "playlist":
                path = A(rec["dst"])
                if rec.get("previous_text") is not None:
                    with open(path, "w", encoding="utf-8", newline="\n") as f:
                        f.write(rec["previous_text"])
                elif os.path.exists(path):
                    os.unlink(path)
                res.done.append({"action_id": rec["action_id"], "kind": kind, "src": None, "note": ""})
            elif kind == "convert" and rec.get("created"):
                made = rec["created"]
                if os.path.exists(A(made)):
                    aside = f".trash/{plan_id}/undone/{made}"
                    os.makedirs(os.path.dirname(A(aside)), exist_ok=True)
                    os.rename(A(made), A(aside))
                res.done.append({"action_id": rec["action_id"], "kind": kind, "src": rec["src"], "note": ""})
        for d in ("Playlists",):
            try:
                os.rmdir(A(d))
            except OSError:
                pass
        with open(os.path.join(repair_dir, f"restored-{plan_id}.json"), "w", encoding="utf-8") as f:
            json.dump({"plan": plan_id, "counts": res.counts(), "skipped": res.skipped}, f, indent=1)
    finally:
        lock.release()
    return res
