import contextlib
import io
import json
import logging
import os
import tempfile
import unittest

from music_repair import __main__ as cli
from music_repair import fixtures, sources
from music_repair.test_applier import file_set, read_bytes
from music_repair.test_sources import Recorder


def run(argv, **kw):
    out, err = io.StringIO(), io.StringIO()
    with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
        code = cli.main(argv, **kw)
    return code, out.getvalue(), err.getvalue()


@fixtures.require_deps
class CliBase(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = fixtures.build_library(os.path.join(self.tmp.name, "lib"))
        self.repair = os.path.join(self.tmp.name, "repair")

    def tearDown(self):
        self.tmp.cleanup()

    def plan(self, *extra, **kw):
        code, out, err = run(["plan", "--library", self.root, "--repair-dir", self.repair, "--no-lookup", *extra], **kw)
        self.assertEqual(code, 0, err)
        pid = [l.split()[-1] for l in out.splitlines() if "plan id" in l][0]
        return pid, out


class TestPlanCommand(CliBase):
    def test_writes_json_and_markdown_and_changes_nothing_in_the_library(self):
        before = file_set(self.root)
        pid, out = self.plan()
        self.assertEqual(file_set(self.root), before)
        self.assertFalse(os.path.exists(os.path.join(self.root, ".repair")))
        self.assertTrue(os.path.isfile(os.path.join(self.repair, f"plan-{pid}.json")))
        md = read_bytes(os.path.join(self.repair, f"plan-{pid}.md")).decode()
        self.assertIn("Nothing has been changed", md)
        self.assertIn("Totals", md)
        self.assertIn("No video id", md)

    def test_default_repair_dir_is_inside_the_library(self):
        code, out, _ = run(["plan", "--library", self.root, "--no-lookup"])
        self.assertEqual(code, 0)
        self.assertTrue(os.path.isdir(os.path.join(self.root, ".repair")))

    def test_two_plans_have_different_ids_and_the_same_actions(self):
        a, _ = self.plan()
        b, _ = self.plan()
        self.assertNotEqual(a, b)
        kinds = lambda pid: [(x["kind"], x.get("src"), x.get("dst", "").replace(pid, "ID"))
                             for x in json.loads(read_bytes(os.path.join(self.repair, f"plan-{pid}.json")))["actions"]]
        self.assertEqual(kinds(a), kinds(b))

    def test_limit_restricts_the_scan(self):
        _, out = self.plan("--limit", "1")
        self.assertIn("plan id", out)
        pid = [l.split()[-1] for l in out.splitlines() if "plan id" in l][0]
        plan = json.loads(read_bytes(os.path.join(self.repair, f"plan-{pid}.json")))
        self.assertLess(plan["totals"]["tracks"], 9)

    def test_a_missing_library_is_refused(self):
        code, _, err = run(["plan", "--library", os.path.join(self.tmp.name, "nope"), "--no-lookup"])
        self.assertEqual(code, cli.REFUSED)

    def test_a_plan_made_offline_says_songs_were_not_looked_up(self):
        pid, _ = self.plan()
        plan = json.loads(read_bytes(os.path.join(self.repair, f"plan-{pid}.json")))
        self.assertEqual(plan["totals"]["not_looked_up"], plan["totals"]["songs"])
        self.assertEqual(plan["totals"]["no_match"], 0)


class TestApplyCommand(CliBase):
    def test_apply_needs_a_plan_id(self):
        with contextlib.redirect_stderr(io.StringIO()):
            with self.assertRaises(SystemExit) as cm:
                cli.main(["apply", "--library", self.root])
        self.assertEqual(cm.exception.code, 2)

    def test_an_unknown_plan_is_refused_and_nothing_changes(self):
        before = file_set(self.root)
        code, _, err = run(["apply", "--library", self.root, "--repair-dir", self.repair, "--plan", "nope"])
        self.assertEqual(code, cli.REFUSED)
        self.assertIn("no plan", err)
        self.assertEqual(file_set(self.root), before)

    def test_plan_apply_plan_finds_nothing_more_and_restore_undoes_it(self):
        before = file_set(self.root)
        pid, _ = self.plan()
        code, out, err = run(["apply", "--library", self.root, "--repair-dir", self.repair, "--plan", pid],
                             fetch_cover=lambda u: (_ for _ in ()).throw(sources.NotFound("x")))
        self.assertEqual(code, 0, out + err)
        _, out2 = self.plan()
        self.assertIn("nothing to do", out2)
        code, out3, err3 = run(["restore", "--library", self.root, "--repair-dir", self.repair, "--plan", pid])
        self.assertEqual(code, 0, err3)
        after = file_set(self.root)
        for rel in before:
            self.assertIn(rel, after, rel)

    def test_applying_when_the_library_grew_a_lock_is_refused(self):
        pid, _ = self.plan()
        os.makedirs(self.repair, exist_ok=True)
        with open(os.path.join(self.repair, "lock"), "w") as f:
            json.dump({"pid": 1, "host": "h", "since": "then"}, f)
        code, _, err = run(["apply", "--library", self.root, "--repair-dir", self.repair, "--plan", pid])
        self.assertEqual(code, cli.REFUSED)
        self.assertIn("lock", err)


def events_of(text):
    from music_repair import events
    return [json.loads(l[len(events.PREFIX):]) for l in text.splitlines() if l.startswith(events.PREFIX)]


class TestEvents(CliBase):
    """What the server reads (spec 1053). The human lines stay; these are added."""

    def plan_events(self, *extra):
        code, out, err = run(["plan", "--library", self.root, "--repair-dir", self.repair, "--no-lookup", *extra])
        self.assertEqual(code, 0, err)
        return out, events_of(out)

    def test_plan_reports_progress_and_ends_with_one_result_that_equals_the_plan(self):
        out, evs = self.plan_events()
        pid = [l.split()[-1] for l in out.splitlines() if "plan id" in l][0]
        self.assertIn("[plan] plan id", out, "the human output is unchanged")
        phases = [e["phase"] for e in evs if e["event"] == "progress"]
        self.assertEqual(phases[0], "scan")
        self.assertIn("plan", phases)
        self.assertEqual(evs[-1]["event"], "result")
        self.assertEqual(len([e for e in evs if e["event"] == "result"]), 1)
        res = evs[-1]
        plan = json.loads(read_bytes(os.path.join(self.repair, f"plan-{pid}.json")))
        self.assertEqual((res["kind"], res["ok"], res["planId"]), ("check", True, pid))
        c, t = res["check"], plan["totals"]
        self.assertEqual((c["duplicates"], c["moves"], c["retags"], c["playlists"], c["songs"], c["tracks"]),
                         (t["duplicates_to_trash"], t["moves"], t["retags"], t["playlists"], t["songs"], t["tracks"]))
        self.assertEqual(c["notLookedUp"], t["not_looked_up"])
        self.assertGreater(c["freeBytes"], 0)
        self.assertEqual(c["leftAlone"]["total"], len(plan["skipped"]))

    def test_lookup_progress_is_reported_every_25(self):
        calls = {"n": 0}

        def transport(url, headers, timeout, max_bytes):
            calls["n"] += 1
            return sources.Response(404, {}, b"")
        code, out, err = run(["plan", "--library", self.root, "--repair-dir", self.repair], transport=transport)
        self.assertEqual(code, 0, err)
        looked = [e for e in events_of(out) if e["event"] == "progress" and e["phase"] == "lookup"]
        self.assertTrue(looked)
        self.assertEqual(looked[-1]["done"], looked[-1]["total"])

    def apply_events(self):
        out, _ = self.plan_events()
        pid = [l.split()[-1] for l in out.splitlines() if "plan id" in l][0]
        code, out2, err2 = run(["apply", "--library", self.root, "--repair-dir", self.repair, "--plan", pid],
                               fetch_cover=lambda u: (_ for _ in ()).throw(sources.NotFound("x")))
        self.assertEqual(code, 0, out2 + err2)
        return pid, out2, events_of(out2)

    def test_apply_reports_progress_and_a_result_equal_to_its_results_file(self):
        pid, out, evs = self.apply_events()
        self.assertIn("[apply] done", out)
        self.assertTrue([e for e in evs if e["event"] == "progress" and e["phase"] == "apply"])
        res = evs[-1]
        self.assertEqual((res["event"], res["kind"], res["ok"], res["planId"]), ("result", "apply", True, pid))
        counts = json.loads(read_bytes(os.path.join(self.repair, f"results-{pid}.json")))["counts"]
        self.assertEqual((res["apply"]["done"], res["apply"]["skipped"], res["apply"]["failed"]),
                         (counts["done"], counts["skipped"], counts["failed"]))

    def test_restore_reports_a_result(self):
        pid, _, _ = self.apply_events()
        code, out, err = run(["restore", "--library", self.root, "--repair-dir", self.repair, "--plan", pid])
        self.assertEqual(code, 0, err)
        evs = events_of(out)
        self.assertEqual((evs[-1]["kind"], evs[-1]["ok"]), ("undo", True))
        self.assertGreater(evs[-1]["undo"]["restored"], 0)

    def refusal(self, argv, **kw):
        code, out, err = run(argv, **kw)
        self.assertEqual(code, cli.REFUSED, out + err)
        evs = events_of(out)
        self.assertEqual(evs[-1]["event"], "result")
        self.assertFalse(evs[-1]["ok"])
        return evs[-1]

    def test_refusals_end_with_a_result_carrying_a_fixed_reason(self):
        r = self.refusal(["apply", "--library", self.root, "--repair-dir", self.repair, "--plan", "20260929T071341Z-aaaaaa"])
        self.assertEqual((r["kind"], r["reason"]), ("apply", "no_plan"))

        os.makedirs(self.repair, exist_ok=True)
        with open(os.path.join(self.repair, "lock"), "w") as f:
            json.dump({"pid": 1, "host": "h", "since": "then"}, f)
        r = self.refusal(["plan", "--library", self.root, "--repair-dir", self.repair, "--no-lookup"])
        self.assertEqual((r["kind"], r["reason"]), ("check", "locked"))
        os.unlink(os.path.join(self.repair, "lock"))

        r = self.refusal(["plan", "--library", os.path.join(self.tmp.name, "nope"), "--no-lookup"])
        self.assertEqual(r["reason"], "rejected")

    def test_a_plan_that_leaves_the_library_is_refused_as_rejected(self):
        out, _ = self.plan_events()
        pid = [l.split()[-1] for l in out.splitlines() if "plan id" in l][0]
        path = os.path.join(self.repair, f"plan-{pid}.json")
        plan = json.loads(read_bytes(path))
        plan["actions"].append({"id": "a999999", "kind": "move", "reason": "x", "src": "../x", "dst": "y"})
        with open(path, "w") as f:
            json.dump(plan, f)
        r = self.refusal(["apply", "--library", self.root, "--repair-dir", self.repair, "--plan", pid])
        self.assertEqual(r["reason"], "rejected")

    def test_not_enough_space_is_reported(self):
        from collections import namedtuple
        from music_repair import applier
        out, _ = self.plan_events()
        pid = [l.split()[-1] for l in out.splitlines() if "plan id" in l][0]
        Usage = namedtuple("Usage", "total used free")
        real = applier.shutil.disk_usage
        applier.shutil.disk_usage = lambda p: Usage(10**12, 10**12 - 10, 10)
        try:
            r = self.refusal(["apply", "--library", self.root, "--repair-dir", self.repair, "--plan", pid])
        finally:
            applier.shutil.disk_usage = real
        self.assertEqual(r["reason"], "no_space")

    def test_no_description_url_video_id_or_environment_reaches_an_event(self):
        os.environ["SYNODL_TEST_SECRET"] = "hunter2"
        try:
            _, out, _ = self.apply_events()
        finally:
            del os.environ["SYNODL_TEST_SECRET"]
        mine = "\n".join(l for l in out.splitlines() if l.startswith("@@synodl "))
        for forbidden in ("SECRET-DESCRIPTION", "example.com", "token=abc123", "hunter2", "AAAAAAAAAAA", "youtube.com"):
            self.assertNotIn(forbidden, mine)


class TestTermination(CliBase):
    """Kubernetes ends a Job with SIGTERM (deadline, delete, node drain). Python's default
    skips `finally`, which would leave the lock behind and lose unflushed lookups."""

    def test_sigterm_releases_the_lock_and_keeps_the_lookups_already_made(self):
        import signal
        calls = {"n": 0}

        def transport(url, headers, timeout, max_bytes):
            calls["n"] += 1
            if calls["n"] == 5:
                os.kill(os.getpid(), signal.SIGTERM)
            return sources.Response(404, {}, b"")

        before = signal.getsignal(signal.SIGTERM)
        with self.assertRaises(SystemExit) as cm:
            run(["plan", "--library", self.root, "--repair-dir", self.repair], transport=transport)
        self.assertEqual(cm.exception.code, 143)
        self.assertFalse(os.path.exists(os.path.join(self.repair, "lock")), "the lock must not outlive the run")
        cache = json.loads(read_bytes(os.path.join(self.repair, "cache.json")))
        self.assertGreater(len(cache["entries"]), 0, "lookups made before the signal were kept")
        self.assertEqual(signal.getsignal(signal.SIGTERM), before, "the handler is put back")


class TestOutbound(CliBase):
    """SC-009 and FR-020, checked over a whole run rather than one function."""

    def test_every_request_is_allowlisted_https_and_carries_only_artist_and_title(self):
        rec = Recorder()       # answers 404 to everything: the run still asks every source
        code, out, err = run(["plan", "--library", self.root, "--repair-dir", self.repair], transport=rec)
        self.assertEqual(code, 0, err)
        self.assertGreater(len(rec.requests), 0)
        import urllib.parse
        for url, headers in rec.requests:
            parts = urllib.parse.urlsplit(url)
            self.assertEqual(parts.scheme, "https", url)
            self.assertTrue(sources.host_allowed(parts.hostname), url)
            text = urllib.parse.unquote(parts.query)
            for forbidden in (self.root, self.repair, "AAAAAAAAAAA", "SECRET-DESCRIPTION", "hunter2", "token=abc123"):
                self.assertNotIn(forbidden, text)
            self.assertEqual(set(headers), {"User-Agent", "Accept"})

    def test_a_full_run_logs_no_descriptions_urls_video_ids_or_environment(self):
        os.environ["SYNODL_TEST_SECRET"] = "hunter2"
        stream = io.StringIO()
        h = logging.StreamHandler(stream)
        log = logging.getLogger("music_repair")
        log.addHandler(h)
        log.setLevel(logging.DEBUG)
        try:
            rec = Recorder()
            code, out, err = run(["plan", "--library", self.root, "--repair-dir", self.repair, "--verbose"], transport=rec)
            pid = [l.split()[-1] for l in out.splitlines() if "plan id" in l][0]
            code2, out2, err2 = run(["apply", "--library", self.root, "--repair-dir", self.repair, "--plan", pid,
                                     "--verbose"], fetch_cover=lambda u: (_ for _ in ()).throw(sources.NotFound("x")))
        finally:
            log.removeHandler(h)
            del os.environ["SYNODL_TEST_SECRET"]
        text = out + err + out2 + err2 + stream.getvalue()
        for forbidden in ("SECRET-DESCRIPTION", "example.com", "token=abc123", "hunter2", "AAAAAAAAAAA",
                          "youtube.com/watch", "query=", "?q="):
            self.assertNotIn(forbidden, text)


if __name__ == "__main__":
    unittest.main()
