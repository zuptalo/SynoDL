import json
import os
import shutil
import subprocess
import tempfile
import unittest
from collections import namedtuple

from music_repair import applier, fixtures, inventory, planner, sources
from music_repair.matching import Match
from music_repair.sources import Result
from music_repair.test_planner import FakeFs, matched

JPEG = b"\xff\xd8\xff\xe0" + b"J" * 64


def read_bytes(path):
    with open(path, "rb") as f:
        return f.read()


def file_set(root):
    out = {}
    for d, dirs, files in os.walk(root):
        rel_d = os.path.relpath(d, root)
        if rel_d.split(os.sep)[0] in (".repair", ".trash"):
            dirs[:] = []
            continue
        for f in files:
            rel = os.path.normpath(os.path.join(rel_d, f))
            out[rel] = os.path.getsize(os.path.join(d, f))
    return out


def video_ids(root, include_trash=False):
    ids = []
    inv = inventory.scan(root)
    ids += [t.video_id for t in inv.tracks if t.video_id]
    if include_trash and os.path.isdir(os.path.join(root, ".trash")):
        for d, _, files in os.walk(os.path.join(root, ".trash")):
            for f in files:
                if f.endswith(".mp3"):
                    ids.append(inventory.read_track(root, os.path.relpath(os.path.join(d, f), root), "", None).video_id)
    return ids


@fixtures.require_deps
class ApplyBase(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = fixtures.build_library(os.path.join(self.tmp.name, "lib"))
        self.repair = os.path.join(self.root, ".repair")
        self.cover_calls = []

    def tearDown(self):
        self.tmp.cleanup()

    def make_plan(self, results=None, fs=None, plan_id="p1"):
        inv = inventory.scan(self.root)
        plan = planner.build_plan(inv, results or {}, plan_id=plan_id, fs=fs or FakeFs(library=self.root),
                                  created="2026-09-28T00:00:00Z")
        return plan.to_dict()

    def fetch(self, url):
        self.cover_calls.append(url)
        return JPEG, "image/jpeg"

    def run_apply(self, plan, **kw):
        kw.setdefault("fetch_cover", self.fetch)
        return applier.apply(plan, self.root, self.repair, **kw)

    def exists(self, rel):
        return os.path.exists(os.path.join(self.root, rel))


class TestApplyEndToEnd(ApplyBase):
    def test_the_whole_repair(self):
        before_ids = set(video_ids(self.root))
        archive = read_bytes(os.path.join(self.root, ".ytdlp-archive.txt"))
        plan = self.make_plan()
        res = self.run_apply(plan)
        self.assertEqual(res.failed, [], res.failed)

        # one copy, at the right place, with the lyrics beside it
        self.assertTrue(self.exists("50 Cent/Singles/In Da Club.mp3"))
        self.assertTrue(self.exists("50 Cent/Singles/In Da Club.lrc"))
        self.assertFalse(self.exists("50 Cent/Party Hits/50 Cent - In Da Club (Official Music Video).mp3"))
        # nothing deleted: the duplicates are in .trash, where they came from
        self.assertTrue(self.exists(".trash/p1/50 Cent/Party Hits/50 Cent - In Da Club (Official Music Video).mp3"))
        # SC-001: no song lost, none twice
        live = video_ids(self.root)
        self.assertEqual(len(live), len(set(live)))
        self.assertTrue(before_ids <= set(video_ids(self.root, include_trash=True)))
        # SC-008
        self.assertEqual(read_bytes(os.path.join(self.root, ".ytdlp-archive.txt")), archive)
        # structure
        for rel in ("ABBA/Singles/Mamma Mia.mp3", "Coldplay/Parachutes/Yellow.mp3",
                    "Coldplay/Everyday Life/Orphans.mp3", "10cc/Singles/I'm Not in Love.mp3",
                    "50 Cent/Singles/Hate Bein' Sober.mp3", "10cc/artist.nfo", "10cc/folder.jpg",
                    "Jazz/Singles/Cozy Cabin.mp3", "Alec Benjamin/logo.png"):
            self.assertTrue(self.exists(rel), rel)
        self.assertFalse(self.exists("Coldplay/Coldplay: Everyday Life"))
        self.assertFalse(self.exists("10ccVEVO"))
        self.assertFalse(self.exists("50 Cent/Party Hits"))
        self.assertTrue(self.exists(".trash/p1/50 Cent/Best Songs/album.nfo"))
        self.assertTrue(self.exists("ABBA/Party Hits/No Id Track.mp3"))     # unidentified: untouched

    def test_tags_are_rewritten_and_the_video_id_survives(self):
        self.run_apply(self.make_plan())
        from mutagen.id3 import ID3
        t = ID3(os.path.join(self.root, "50 Cent/Singles/In Da Club.mp3"))
        self.assertEqual(str(t["TIT2"]), "In Da Club")
        self.assertEqual(str(t["TALB"]), "Singles")
        self.assertEqual(str(t["TXXX:SYNODL_REPAIR"]), "p1")
        self.assertEqual(str(t["TXXX:purl"]), "https://www.youtube.com/watch?v=AAAAAAAAAAA")

    def test_every_playlist_entry_resolves(self):
        self.run_apply(self.make_plan())
        pdir = os.path.join(self.root, "Playlists")
        names_ = os.listdir(pdir)
        self.assertIn("Party Hits.m3u8", names_)
        for n in names_:
            for line in read_bytes(os.path.join(pdir, n)).decode("utf-8").splitlines():
                if line and not line.startswith("#"):
                    self.assertTrue(os.path.exists(os.path.normpath(os.path.join(pdir, line))), (n, line))

    def test_a_second_plan_after_apply_finds_nothing_to_do(self):
        self.run_apply(self.make_plan())
        again = self.make_plan(plan_id="p2")
        self.assertEqual(again["actions"], [], [(a["kind"], a.get("src")) for a in again["actions"]][:10])

    def test_applying_the_same_plan_twice_changes_nothing(self):
        plan = self.make_plan()
        self.run_apply(plan)
        snap = file_set(self.root)
        res = self.run_apply(plan)
        self.assertEqual(res.failed, [])
        self.assertEqual(file_set(self.root), snap)

    def test_results_and_journal_are_written(self):
        self.run_apply(self.make_plan())
        results = json.loads(read_bytes(os.path.join(self.repair, "results-p1.json")))
        self.assertEqual(results["plan"], "p1")
        self.assertIn("done", results["counts"])
        self.assertTrue(os.path.exists(os.path.join(self.repair, "journal-p1.jsonl")))


class TestRefusals(ApplyBase):
    def test_a_plan_that_leaves_the_library_is_rejected_before_anything_moves(self):
        for bad in ("../outside.mp3", "/etc/passwd", "a/../../x"):
            plan = self.make_plan()
            plan["actions"].append({"id": "a999999", "kind": "move", "reason": "x", "src": bad, "dst": "ok/x.mp3"})
            snap = file_set(self.root)
            with self.assertRaises(applier.PlanRejected):
                self.run_apply(plan)
            self.assertEqual(file_set(self.root), snap)

    def test_a_path_through_a_symlink_is_rejected(self):
        outside = os.path.join(self.tmp.name, "outside")
        os.makedirs(outside)
        fixtures.touch(os.path.join(outside, "x.mp3"), b"x")
        os.symlink(outside, os.path.join(self.root, "linked"))
        plan = self.make_plan()
        plan["actions"].append({"id": "a999999", "kind": "trash", "reason": "x", "src": "linked/x.mp3",
                                "dst": ".trash/p1/linked/x.mp3", "size": 1, "mtime_ns": 1})
        with self.assertRaises(applier.PlanRejected):
            self.run_apply(plan)
        self.assertTrue(os.path.exists(os.path.join(outside, "x.mp3")))

    def test_not_enough_free_space_refuses_and_states_the_shortfall(self):
        Usage = namedtuple("Usage", "total used free")
        plan = self.make_plan()
        snap = file_set(self.root)
        with self.assertRaises(applier.NotEnoughSpace) as cm:
            self.run_apply(plan, disk_usage=lambda p: Usage(10**12, 10**12 - 10, 10))
        self.assertIn("short by", str(cm.exception))
        self.assertEqual(file_set(self.root), snap)

    def test_a_second_concurrent_run_refuses_and_a_stale_lock_is_reported_not_taken_over(self):
        lock = applier.Lock(self.repair)
        lock.acquire()
        try:
            with self.assertRaises(applier.Locked) as cm:
                self.run_apply(self.make_plan())
            self.assertIn("pid", str(cm.exception))
            self.assertIn("lock", str(cm.exception))
        finally:
            lock.release()
        self.run_apply(self.make_plan())     # released: runs

    def test_the_lock_is_released_when_the_run_fails(self):
        plan = self.make_plan()
        plan["actions"].append({"id": "a999999", "kind": "move", "reason": "x", "src": "../x", "dst": "y"})
        with self.assertRaises(applier.PlanRejected):
            self.run_apply(plan)
        self.assertFalse(os.path.exists(os.path.join(self.repair, "lock")))


class TestStaleness(ApplyBase):
    def test_a_source_that_changed_since_the_plan_is_skipped_and_reported(self):
        plan = self.make_plan()
        victim = os.path.join(self.root, "ABBA/Party Hits/ABBA - Mamma Mia (Official Music Video).mp3")
        with open(victim, "ab") as f:
            f.write(b"a download landed here")
        res = self.run_apply(plan)
        self.assertTrue(self.exists("ABBA/Party Hits/ABBA - Mamma Mia (Official Music Video).mp3"))
        self.assertFalse(self.exists("ABBA/Singles/Mamma Mia.mp3"))
        self.assertTrue(any("changed" in s["note"] for s in res.skipped), res.skipped)
        self.assertTrue(self.exists("Coldplay/Parachutes/Yellow.mp3"))      # the rest proceeded

    def test_one_file_retagged_and_moved_in_one_apply_is_not_stale(self):
        # retagging changes the size; the following move must not read that as somebody else's edit
        res = self.run_apply(self.make_plan())
        self.assertFalse([s for s in res.skipped if "changed" in s["note"]], res.skipped)
        self.assertTrue(self.exists("Coldplay/Parachutes/Yellow.mp3"))

    def test_a_destination_that_appeared_is_a_failure_never_an_overwrite(self):
        plan = self.make_plan()
        fixtures.touch(os.path.join(self.root, "ABBA/Singles/Mamma Mia.mp3"), b"precious")
        res = self.run_apply(plan)
        self.assertEqual(read_bytes(os.path.join(self.root, "ABBA/Singles/Mamma Mia.mp3")), b"precious")
        self.assertTrue(self.exists("ABBA/Party Hits/ABBA - Mamma Mia (Official Music Video).mp3"))
        self.assertTrue(res.failed or res.skipped)


class TestResume(ApplyBase):
    def test_an_interrupted_apply_resumes_to_the_same_end_state(self):
        reference = tempfile.TemporaryDirectory()
        try:
            ref_root = fixtures.build_library(os.path.join(reference.name, "lib"))
            inv = inventory.scan(ref_root)
            ref_plan = planner.build_plan(inv, {}, plan_id="p1", fs=FakeFs(library=ref_root), created="x").to_dict()
            applier.apply(ref_plan, ref_root, os.path.join(ref_root, ".repair"), fetch_cover=self.fetch)
            want = file_set(ref_root)
        finally:
            reference.cleanup()

        plan = self.make_plan()
        real = os.rename
        calls = {"n": 0}

        def flaky(src, dst, *a, **k):
            calls["n"] += 1
            if calls["n"] == 7:
                raise KeyboardInterrupt("pod evicted")
            return real(src, dst, *a, **k)

        applier.os.rename = flaky
        try:
            with self.assertRaises(KeyboardInterrupt):
                self.run_apply(plan)
        finally:
            applier.os.rename = real
        self.assertFalse(os.path.exists(os.path.join(self.repair, "lock")), "an interrupted run releases its lock")
        res = self.run_apply(plan)
        self.assertEqual(res.failed, [], res.failed)
        self.assertEqual(file_set(self.root), want)


class TestInterruptedRetag(ApplyBase):
    """A kill between rewriting a file's tags and journaling the step must not strand the file.

    Retagging changes the size, so a resumed run cannot tell "somebody edited this"
    from "I already did this" by size alone. The file's own repair marker tells it.
    """

    def growing(self, plan):
        """Make every retag add ~5 KB, so the file's SIZE changes — the case that strands a file."""
        for a in plan["actions"]:
            if a["kind"] == "retag" and "album" in a.get("tags", {}):
                a["tags"]["featured"] = "x" * 5000
        return plan

    def interrupt_after_nth_tag_write(self, n):
        real = applier.tagio.write_tags
        calls = {"n": 0}

        def flaky(path, tags):
            out = real(path, tags)
            calls["n"] += 1
            if calls["n"] == n:
                raise KeyboardInterrupt("killed after the write, before the journal")
            return out
        applier.tagio.write_tags = flaky
        return real

    def reference_files(self):
        reference = tempfile.TemporaryDirectory()
        try:
            ref_root = fixtures.build_library(os.path.join(reference.name, "lib"))
            plan = self.growing(planner.build_plan(inventory.scan(ref_root), {}, plan_id="p1",
                                                   fs=FakeFs(library=ref_root), created="x").to_dict())
            applier.apply(plan, ref_root, os.path.join(ref_root, ".repair"), fetch_cover=self.fetch)
            return file_set(ref_root)
        finally:
            reference.cleanup()

    def test_resume_after_a_kill_between_the_tag_write_and_the_journal_reaches_the_same_end_state(self):
        want = self.reference_files()
        plan = self.growing(self.make_plan())
        real = self.interrupt_after_nth_tag_write(3)
        try:
            with self.assertRaises(KeyboardInterrupt):
                self.run_apply(plan)
        finally:
            applier.tagio.write_tags = real
        res = self.run_apply(plan)
        self.assertEqual(res.failed, [], res.failed)
        self.assertFalse([s for s in res.skipped if "changed" in s["note"]], res.skipped)
        self.assertEqual(file_set(self.root), want)
        live = [t for t in inventory.scan(self.root).tracks if t.video_id]
        self.assertTrue(all(t.settled for t in live), "every song was actually retagged")
        self.assertFalse([t.relpath for t in live if "/Singles/" not in t.relpath and "Parachutes" not in t.relpath
                          and "Everyday Life" not in t.relpath], "no file was left stranded in an old playlist folder")

    def test_restore_after_that_recovery_still_brings_the_old_tags_back(self):
        from mutagen.id3 import ID3
        before = {}
        for t in inventory.scan(self.root).tracks:
            if t.video_id:
                before[t.video_id] = t.tag_title
        plan = self.growing(self.make_plan())
        real = self.interrupt_after_nth_tag_write(3)
        try:
            with self.assertRaises(KeyboardInterrupt):
                self.run_apply(plan)
        finally:
            applier.tagio.write_tags = real
        self.run_apply(plan)
        applier.restore("p1", self.root, self.repair)
        after = {t.video_id: t.tag_title for t in inventory.scan(self.root).tracks if t.video_id}
        # the copy set aside as a duplicate comes back too, so every id is present with its original title
        for vid, title in before.items():
            self.assertEqual(after.get(vid), title, vid)

    def test_a_file_someone_else_edited_is_still_reported_changed(self):
        plan = self.make_plan()
        victim = os.path.join(self.root, "ABBA/Party Hits/ABBA - Mamma Mia (Official Music Video).mp3")
        with open(victim, "ab") as f:
            f.write(b"an unrelated edit")           # no repair marker: this is not our work
        res = self.run_apply(plan)
        self.assertTrue(any("changed" in s["note"] for s in res.skipped), res.skipped)


class TestRestore(ApplyBase):
    def test_restore_returns_files_and_previous_tags(self):
        from mutagen.id3 import ID3
        before_files = file_set(self.root)
        before_titles = {}
        for rel in before_files:
            if rel.endswith(".mp3") and "Cozy" not in rel:
                before_titles[rel] = (str(ID3(os.path.join(self.root, rel)).get("TIT2")),
                                      str(ID3(os.path.join(self.root, rel)).get("TALB")))
        self.run_apply(self.make_plan())
        applier.restore("p1", self.root, self.repair)
        after = file_set(self.root)
        for rel in before_files:
            if rel.endswith(".webm") or rel.endswith(".bin"):
                continue
            self.assertIn(rel, after, rel)
        for rel, (title, album) in before_titles.items():
            t = ID3(os.path.join(self.root, rel))
            self.assertEqual((str(t.get("TIT2")), str(t.get("TALB"))), (title, album), rel)
            self.assertNotIn("TXXX:SYNODL_REPAIR", t)
        self.assertFalse(self.exists("Playlists/Party Hits.m3u8"))
        self.assertTrue(self.exists(".trash"), "restore never empties .trash")

    def test_restore_skips_an_entry_whose_original_location_is_occupied(self):
        self.run_apply(self.make_plan())
        occupied = "50 Cent/Party Hits/50 Cent - In Da Club (Official Music Video).mp3"
        fixtures.touch(os.path.join(self.root, occupied), b"someone else's file")
        res = applier.restore("p1", self.root, self.repair)
        self.assertEqual(read_bytes(os.path.join(self.root, occupied)), b"someone else's file")
        self.assertTrue(any(occupied in s["note"] or occupied == s.get("src") for s in res.skipped), res.skipped)


class TestPlaylistsAndLeftovers(ApplyBase):
    def test_a_playlist_that_already_exists_is_merged_not_replaced(self):
        fixtures.touch(os.path.join(self.root, "Playlists/Party Hits.m3u8"),
                       b"#EXTM3U\n#EXTINF:100,Old Artist - Old Song\n../Old Artist/Singles/Old Song.mp3\n")
        self.run_apply(self.make_plan())
        text = read_bytes(os.path.join(self.root, "Playlists/Party Hits.m3u8")).decode("utf-8")
        self.assertIn("../Old Artist/Singles/Old Song.mp3", text)
        self.assertIn("../ABBA/Singles/Mamma Mia.mp3", text)

    def test_a_bin_that_is_not_an_image_is_set_aside_not_deleted(self):
        fixtures.touch(os.path.join(self.root, "Alec Benjamin/logo.bin"), b"junk")
        plan = self.make_plan(fs=FakeFs(heads={"Alec Benjamin/logo.bin": b"junk"}, library=self.root))
        self.run_apply(plan)
        self.assertTrue(self.exists(".trash/p1/Alec Benjamin/logo.bin"))
        self.assertFalse(self.exists("Alec Benjamin/logo.bin"))


class TestConvert(ApplyBase):
    def test_a_webm_with_audio_becomes_a_tagged_mp3_and_the_original_is_set_aside(self):
        plan = self.make_plan(fs=planner.RealFs(self.root))
        res = self.run_apply(plan)
        self.assertEqual(res.failed, [], res.failed)
        from mutagen.id3 import ID3
        t = ID3(os.path.join(self.root, "Jazz/Singles/Cozy Cabin.mp3"))
        self.assertEqual(str(t["TIT2"]), "Cozy Cabin")
        self.assertEqual(str(t["TALB"]), "Singles")
        self.assertTrue(self.exists(".trash/p1/Jazz/Singles/Cozy Cabin.webm"))

    def test_restore_undoes_the_conversion(self):
        plan = self.make_plan(fs=planner.RealFs(self.root))
        self.run_apply(plan)
        applier.restore("p1", self.root, self.repair)
        self.assertTrue(self.exists("Jazz/Singles/Cozy Cabin.webm"))
        self.assertFalse(self.exists("Jazz/Singles/Cozy Cabin.mp3"))


class TestCover(ApplyBase):
    def matched_plan(self):
        return self.make_plan({"BBBBBBBBBBB": matched(artist="Chief Keef", title="Hate Bein' Sober", album="Finally Rich",
                                                      no=3)})

    def test_cover_is_embedded_and_saved_beside_the_album(self):
        self.run_apply(self.matched_plan())
        from mutagen.id3 import ID3
        path = "Chief Keef/Finally Rich/03 - Hate Bein' Sober.mp3"
        self.assertEqual(ID3(os.path.join(self.root, path)).getall("APIC")[0].data, JPEG)
        self.assertEqual(read_bytes(os.path.join(self.root, "Chief Keef/Finally Rich/folder.jpg")), JPEG)
        t = ID3(os.path.join(self.root, path))
        self.assertEqual(str(t["TXXX:SYNODL_STATUS"]), "matched")
        self.assertEqual(str(t["TRCK"]), "3")

    def test_one_download_serves_every_track_of_an_album(self):
        plan = self.make_plan({"AAAAAAAAAAA": matched(album="Same Album", no=1),
                               "CCCCCCCCCCC": matched(artist="50 Cent", title="Mamma Mia", album="Same Album", no=2)})
        self.run_apply(plan)
        self.assertEqual(len(self.cover_calls), 1)

    def test_missing_cover_art_is_reported_not_a_failure(self):
        def none(url):
            raise sources.NotFound("coverartarchive.org")
        res = self.run_apply(self.matched_plan(), fetch_cover=none)
        self.assertEqual(res.failed, [])
        self.assertTrue(any("cover" in s["note"] for s in res.skipped))
        self.assertTrue(self.exists("Chief Keef/Finally Rich/03 - Hate Bein' Sober.mp3"))

    def test_an_unreachable_source_for_a_cover_does_not_fail_the_repair(self):
        def down(url):
            raise sources.FetchError("archive.org: TimeoutError")
        res = self.run_apply(self.matched_plan(), fetch_cover=down)
        self.assertEqual(res.failed, [])

    def test_restore_puts_back_the_previous_cover(self):
        from mutagen.id3 import ID3
        path = os.path.join(self.root, "50 Cent/Best Songs/50 Cent - In Da Club (Official Music Video).mp3")
        from music_repair import tagio
        tagio.embed_cover(path, b"\xff\xd8\xff\xe0" + b"T" * 30, "image/jpeg")     # the YouTube thumbnail
        thumb = ID3(path).getall("APIC")[0].data
        self.run_apply(self.make_plan({"AAAAAAAAAAA": matched()}))
        self.assertEqual(ID3(os.path.join(self.root, "50 Cent/Get Rich or Die Tryin'/05 - In Da Club.mp3")).getall("APIC")[0].data, JPEG)
        applier.restore("p1", self.root, self.repair)
        self.assertEqual(ID3(path).getall("APIC")[0].data, thumb)
        self.assertFalse(self.exists("50 Cent/Get Rich or Die Tryin'/folder.jpg"))


if __name__ == "__main__":
    unittest.main()
