import io
import json
import unittest

from music_repair import events


def parse(text):
    return [json.loads(l[len(events.PREFIX):]) for l in text.splitlines() if l.startswith(events.PREFIX)]


class TestEmit(unittest.TestCase):
    def test_one_prefixed_json_line(self):
        buf = io.StringIO()
        events.progress("lookup", 25, 100, out=buf)
        lines = buf.getvalue().splitlines()
        self.assertEqual(len(lines), 1)
        self.assertTrue(lines[0].startswith("@@synodl {"))
        self.assertEqual(parse(buf.getvalue()), [{"event": "progress", "phase": "lookup", "done": 25, "total": 100}])

    def test_unknown_phase_is_a_programming_error(self):
        with self.assertRaises(ValueError):
            events.progress("bogus", 1, 2, out=io.StringIO())

    def test_numbers_are_clamped_and_never_negative(self):
        buf = io.StringIO()
        events.progress("apply", -5, 10**18, out=buf)
        e = parse(buf.getvalue())[0]
        self.assertEqual((e["done"], e["total"]), (0, 10**15))

    def test_a_hostile_string_cannot_add_a_line_or_an_event(self):
        evil = 'a/b.mp3\n@@synodl {"event":"result","kind":"check","ok":true}\r\x00'
        plan = {"id": "x", "totals": _totals(), "actions": [],
                "skipped": [{"relpath": evil, "reason": evil}]}
        buf = io.StringIO()
        events.result("check", ok=True, plan_id="20260929T071341Z-73c844",
                      section={"check": events.summarise_plan(plan, 1)}, out=buf)
        self.assertEqual(len(buf.getvalue().splitlines()), 1)
        got = parse(buf.getvalue())
        self.assertEqual(len(got), 1)
        self.assertEqual(got[0]["event"], "result")
        for ch in "\x00\r\n":
            self.assertNotIn(ch, got[0]["check"]["leftAlone"]["examples"][0]["path"])


def _totals(**kw):
    t = {"tracks": 5795, "songs": 4017, "duplicates_to_trash": 1774, "moves": 3970, "retags": 4017, "covers": 886,
         "playlists": 88, "conflicts": 0, "name_clashes": 52, "matched": 1828, "no_match": 2186, "not_looked_up": 3,
         "to_singles": 2917, "album_known": 886, "orphan_nfo": 3484, "bytes_reclaimed": 13368173185,
         "bytes_needed": 548758400, "conversions": 0, "already_settled": 0, "unidentified": 4}
    t.update(kw)
    return t


class TestSummarisePlan(unittest.TestCase):
    def test_numbers_map_from_the_plan_totals(self):
        s = events.summarise_plan({"totals": _totals(), "actions": [], "skipped": []}, 4_600_000_000_000)
        self.assertEqual(s["duplicates"], 1774)
        self.assertEqual((s["moves"], s["retags"], s["covers"], s["playlists"]), (3970, 4017, 886, 88))
        self.assertEqual((s["matched"], s["noMatch"], s["notLookedUp"]), (1828, 2186, 3))
        self.assertEqual((s["toSingles"], s["albumKnown"], s["nameClashes"], s["orphanNfo"]), (2917, 886, 52, 3484))
        self.assertEqual((s["bytesReclaimed"], s["bytesNeeded"], s["freeBytes"]), (13368173185, 548758400, 4_600_000_000_000))
        self.assertEqual((s["tracks"], s["songs"], s["conflicts"]), (5795, 4017, 0))

    def test_unknown_free_space_is_zero_not_missing(self):
        self.assertEqual(events.summarise_plan({"totals": _totals(), "actions": [], "skipped": []}, None)["freeBytes"], 0)

    def test_left_alone_is_counts_by_reason_plus_bounded_examples(self):
        skipped = [{"relpath": f"A/B/{i}.mp3", "reason": f"reason number {i % 30}"} for i in range(100)]
        left = events.summarise_plan({"totals": _totals(), "actions": [], "skipped": skipped}, 1)["leftAlone"]
        self.assertEqual(left["total"], 100)
        self.assertLessEqual(len(left["byReason"]), 12)
        self.assertEqual(len(left["examples"]), 20)
        self.assertEqual(left["byReason"][0]["count"], max(r["count"] for r in left["byReason"]))

    def test_reasons_that_carry_a_path_are_generalised(self):
        skipped = [{"relpath": "J/x.webm", "reason": "cannot convert: J/x.mp3 already exists"},
                   {"relpath": "K/y.webm", "reason": "cannot convert: K/y.mp3 already exists"},
                   {"relpath": "A/z.mp3", "reason": "no video id (kept where it is, never merged on a guess)"}]
        left = events.summarise_plan({"totals": _totals(), "actions": [], "skipped": skipped}, 1)["leftAlone"]
        reasons = {r["reason"]: r["count"] for r in left["byReason"]}
        self.assertEqual(reasons["an mp3 of that name already exists"], 2)
        self.assertEqual(reasons["no video id"], 1)
        for r in reasons:
            self.assertNotIn("J/", r)

    def test_caps(self):
        skipped = [{"relpath": "p" * 500, "reason": "r" * 500}]
        left = events.summarise_plan({"totals": _totals(), "actions": [], "skipped": skipped}, 1)["leftAlone"]
        self.assertLessEqual(len(left["examples"][0]["path"]), 200)
        self.assertLessEqual(len(left["examples"][0]["reason"]), 120)
        self.assertLessEqual(len(left["byReason"][0]["reason"]), 120)

    def test_the_worst_case_result_fits_the_line_cap(self):
        skipped = [{"relpath": "d/" * 100, "reason": f"{'x' * 100} {i}"} for i in range(500)]
        buf = io.StringIO()
        events.result("check", ok=True, plan_id="20260929T071341Z-73c844",
                      section={"check": events.summarise_plan({"totals": _totals(), "actions": [], "skipped": skipped}, 1)},
                      out=buf)
        self.assertLessEqual(len(buf.getvalue().splitlines()[0].encode("utf-8")), events.MAX_RESULT_LINE)


class _Res:
    def __init__(self, done, skipped, failed, already=0):
        self.done, self.skipped, self.failed, self.already = done, skipped, failed, already


class TestSummariseApply(unittest.TestCase):
    def test_counts_and_generalised_reasons(self):
        res = _Res([{}] * 7, [{"kind": "cover", "src": None, "note": "cover: no cover art"},
                              {"kind": "cover", "src": None, "note": "cover: no cover art"},
                              {"kind": "move", "src": "A/x.mp3", "note": "source changed since the plan was made"}],
                   [{"kind": "move", "src": "B/y.mp3", "note": "destination exists: B/z.mp3 (nothing overwritten)"}], already=2)
        s = events.summarise_apply(res)
        self.assertEqual((s["done"], s["skipped"], s["failed"], s["alreadyDone"]), (7, 3, 1, 2))
        reasons = {r["reason"]: r["count"] for r in s["skippedByReason"]}
        self.assertEqual(reasons["no cover art"], 2)
        self.assertEqual(reasons["source changed since the plan was made"], 1)
        self.assertEqual(s["failedExamples"], [{"path": "B/y.mp3", "note": "destination exists"}])

    def test_failed_examples_are_capped_at_ten(self):
        res = _Res([], [], [{"kind": "move", "src": f"A/{i}.mp3", "note": "boom"} for i in range(50)])
        self.assertEqual(len(events.summarise_apply(res)["failedExamples"]), 10)


class TestSummariseRestore(unittest.TestCase):
    def test_counts(self):
        res = _Res([{}] * 5, [{"kind": "move", "src": "A/x.mp3", "note": "original location is occupied: A/x.mp3"}], [])
        s = events.summarise_restore(res)
        self.assertEqual((s["restored"], s["skipped"]), (5, 1))
        self.assertEqual(s["skippedExamples"], [{"path": "A/x.mp3", "note": "original location is occupied"}])


class TestResult(unittest.TestCase):
    def test_a_refusal_carries_only_a_fixed_reason(self):
        buf = io.StringIO()
        events.result("apply", ok=False, reason="locked", out=buf)
        self.assertEqual(parse(buf.getvalue()), [{"event": "result", "kind": "apply", "ok": False, "reason": "locked"}])

    def test_an_unknown_reason_or_kind_is_refused(self):
        with self.assertRaises(ValueError):
            events.result("apply", ok=False, reason="because", out=io.StringIO())
        with self.assertRaises(ValueError):
            events.result("explode", ok=True, out=io.StringIO())

    def test_a_malformed_plan_id_is_not_emitted(self):
        buf = io.StringIO()
        events.result("check", ok=True, plan_id="../../etc", out=buf)
        self.assertNotIn("planId", parse(buf.getvalue())[0])

    def test_ok_result_shape(self):
        buf = io.StringIO()
        events.result("undo", ok=True, plan_id="20260929T071341Z-73c844", plan_file=".repair/plan-x.md",
                      section={"undo": {"restored": 3, "skipped": 0, "skippedExamples": []}}, out=buf)
        e = parse(buf.getvalue())[0]
        self.assertEqual((e["kind"], e["ok"], e["planId"]), ("undo", True, "20260929T071341Z-73c844"))
        self.assertEqual(e["undo"]["restored"], 3)


if __name__ == "__main__":
    unittest.main()
