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
