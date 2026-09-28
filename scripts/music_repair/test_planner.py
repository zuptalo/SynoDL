import os
import tempfile
import unittest

from music_repair import fixtures, inventory, planner
from music_repair.matching import Match
from music_repair.sources import Result


class FakeFs:
    """The planner's IO, stubbed. With a `library` it stats real files, as RealFs does."""

    def __init__(self, audio=True, heads=None, library=None):
        self.audio, self.heads, self.library = audio, heads or {}, library

    def has_audio(self, rel):
        return self.audio

    def read_text(self, rel):
        if self.library:
            try:
                with open(os.path.join(self.library, rel), encoding="utf-8") as f:
                    return f.read()
            except OSError:
                return None
        return None

    def head(self, rel, n):
        return self.heads.get(rel, b"\x89PNG\r\n\x1a\n")[:n]

    def stat(self, rel):
        if self.library:
            st = os.stat(os.path.join(self.library, rel))
            return st.st_size, st.st_mtime_ns
        return (1, 1)


def matched(artist="50 Cent", title="In Da Club", album="Get Rich or Die Tryin'", no=5, year=2003, cover=True,
            source="musicbrainz"):
    return Result("matched", Match(source=source, artist=artist, title=title, length_s=223.0, delta_s=0.0,
                                   album=album, track_no=no, year=year, mbid_recording="rec",
                                   mbid_release="rel" if album else None,
                                   cover_url="https://coverartarchive.org/release/rel/front-500" if cover and album else None),
                  ["musicbrainz"])


@fixtures.require_deps
class PlannerBase(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tmp = tempfile.TemporaryDirectory()
        cls.root = fixtures.build_library(cls.tmp.name)

    @classmethod
    def tearDownClass(cls):
        cls.tmp.cleanup()

    def plan(self, results=None, fs=None, mutate=None):
        inv = inventory.scan(self.root)
        if mutate:
            mutate(inv)
        return planner.build_plan(inv, results or {}, plan_id="p1", fs=fs or FakeFs(), created="2026-09-28T00:00:00Z")

    @staticmethod
    def kind(plan, kind):
        return [a for a in plan.actions if a["kind"] == kind]

    def one(self, plan, kind, src_suffix=None, dst=None):
        found = [a for a in self.kind(plan, kind)
                 if (src_suffix is None or a.get("src", "").endswith(src_suffix)) and (dst is None or a.get("dst") == dst)]
        self.assertEqual(len(found), 1, f"{kind} {src_suffix} {dst}: {found}")
        return found[0]


class TestReadOnly(PlannerBase):
    def snapshot(self):
        out = {}
        for d, _, files in os.walk(self.root):
            for f in files:
                p = os.path.join(d, f)
                st = os.stat(p)
                out[os.path.relpath(p, self.root)] = (st.st_size, st.st_mtime_ns)
        return out

    def test_planning_changes_nothing(self):
        before = self.snapshot()
        self.plan()
        self.assertEqual(self.snapshot(), before)

    def test_plan_has_totals_reasons_and_a_fingerprint(self):
        p = self.plan()
        self.assertTrue(p.library_fingerprint)
        self.assertEqual(p.totals["tracks"], 9)
        self.assertEqual(p.totals["duplicates_to_trash"], 2)
        self.assertGreater(p.totals["bytes_reclaimed"], 0)
        for a in p.actions:
            self.assertTrue(a["reason"], a)
            self.assertTrue(a["id"].startswith("a"))

    def test_action_ids_are_unique_and_ordered(self):
        ids = [a["id"] for a in self.plan().actions]
        self.assertEqual(ids, sorted(ids))
        self.assertEqual(len(ids), len(set(ids)))

    def test_every_path_is_library_relative_and_safe(self):
        from music_repair import names
        for a in self.plan({"AAAAAAAAAAA": matched()}).actions:
            for k in ("src", "dst", "audio"):
                if a.get(k):
                    self.assertTrue(names.safe_rel(a[k]), a)


class TestDedupe(PlannerBase):
    def test_one_copy_kept_the_rest_trashed_by_video_id(self):
        p = self.plan()
        kept = self.one(p, "move", dst="50 Cent/Singles/In Da Club.mp3")
        self.assertEqual(kept["src"], "50 Cent/Best Songs/50 Cent - In Da Club (Official Music Video).mp3")
        trashed = sorted(a["src"] for a in self.kind(p, "trash") if a["src"].endswith(".mp3"))
        self.assertEqual(trashed, ["50 Cent/Dance Music/50 Cent - In Da Club (Official Music Video).mp3",
                                   "50 Cent/Party Hits/50 Cent - In Da Club (Official Music Video).mp3"])
        for a in self.kind(p, "trash"):
            self.assertTrue(a["dst"].startswith(".trash/p1/"))
            self.assertTrue(a["dst"].endswith(a["src"]))

    def test_lyrics_follow_the_kept_audio(self):
        p = self.plan()
        side = self.one(p, "sidecar")
        self.assertEqual(side["src"], "50 Cent/Party Hits/50 Cent - In Da Club (Official Music Video).lrc")
        self.assertEqual(side["dst"], "50 Cent/Singles/In Da Club.lrc")
        self.assertNotIn(side["src"], [a["src"] for a in self.kind(p, "trash")])

    def test_unidentified_is_kept_and_reported_never_merged(self):
        p = self.plan()
        self.assertEqual([s["relpath"] for s in p.skipped if s["reason"].startswith("no video id")],
                         ["ABBA/Party Hits/No Id Track.mp3"])
        self.assertFalse([a for a in p.actions if a.get("src") == "ABBA/Party Hits/No Id Track.mp3"
                          and a["kind"] in ("trash", "move")])

    def test_archive_is_never_touched(self):
        p = self.plan({"AAAAAAAAAAA": matched()})
        for a in p.actions:
            self.assertNotIn(".ytdlp-archive.txt", (a.get("src") or "") + (a.get("dst") or ""))


class TestPlaylists(PlannerBase):
    def test_a_playlist_per_distinct_title_across_artists(self):
        p = self.plan()
        pls = {a["dst"]: a for a in self.kind(p, "playlist")}
        self.assertIn("Playlists/Party Hits.m3u8", pls)
        party = [e["relpath"] for e in pls["Playlists/Party Hits.m3u8"]["entries"]]
        self.assertIn("50 Cent/Singles/In Da Club.mp3", party)
        self.assertIn("ABBA/Singles/Mamma Mia.mp3", party)
        self.assertIn("50 Cent/Singles/Hate Bein' Sober.mp3", party)
        self.assertIn("Playlists/Dance Music.m3u8", pls)
        self.assertIn("Playlists/Best Songs.m3u8", pls)

    def test_singles_folders_are_not_playlists(self):
        titles = [a["dst"] for a in self.kind(self.plan(), "playlist")]
        self.assertNotIn("Playlists/Singles.m3u8", titles)

    def test_album_folders_are_also_playlists(self):
        titles = [a["dst"] for a in self.kind(self.plan(), "playlist")]
        self.assertIn("Playlists/Coldplay - Parachutes.m3u8", titles)

    def test_the_colon_folder_and_its_twin_are_one_playlist(self):
        pls = {a["dst"]: a for a in self.kind(self.plan(), "playlist")}
        self.assertIn("Playlists/Coldplay - Everyday Life.m3u8", pls)
        self.assertNotIn("Playlists/Coldplay： Everyday Life.m3u8", pls)

    def test_an_artist_called_playlists_is_renamed(self):
        def mutate(inv):
            for t in inv.tracks:
                if t.folder_artist == "ABBA":
                    t.folder_artist = "Playlists"
        p = self.plan(mutate=mutate)
        dsts = [a["dst"] for a in self.kind(p, "move")]
        self.assertTrue(any(d.startswith("Playlists (artist)/") for d in dsts), dsts)
        self.assertFalse(any(d.startswith("Playlists/") and d.endswith(".mp3") for d in dsts))


class TestStructure(PlannerBase):
    def test_unmatched_keeps_the_uploader_folder_and_goes_to_singles(self):
        p = self.plan()
        self.one(p, "move", dst="ABBA/Singles/Mamma Mia.mp3")
        self.one(p, "move", dst="50 Cent/Singles/Hate Bein' Sober.mp3")   # NOT moved to Chief Keef on a guess

    def test_channel_markers_are_stripped_from_the_uploader_folder(self):
        p = self.plan()
        self.one(p, "move", dst="10cc/Singles/I'm Not in Love.mp3")

    def test_artist_files_follow_a_renamed_artist_folder(self):
        p = self.plan()
        self.one(p, "move", src_suffix="10ccVEVO/artist.nfo", dst="10cc/artist.nfo")
        self.one(p, "move", src_suffix="10ccVEVO/folder.jpg", dst="10cc/folder.jpg")

    def test_an_existing_album_folder_keeps_its_tracks_as_that_album(self):
        p = self.plan()
        self.one(p, "move", dst="Coldplay/Parachutes/Yellow.mp3")
        tags = self.one(p, "retag", src_suffix="Coldplay - Yellow (Official Video).mp3")["tags"]
        self.assertEqual(tags["album"], "Parachutes")
        self.assertNotIn("track", tags)

    def test_the_colon_folder_lands_in_its_twins_album(self):
        p = self.plan()
        self.one(p, "move", dst="Coldplay/Everyday Life/Orphans.mp3")
        md = self.one(p, "merge_dir")
        self.assertEqual(md["src"], "Coldplay/Coldplay: Everyday Life")
        self.assertEqual(md["dst"], "Coldplay/Coldplay - Everyday Life")

    def test_titles_are_cleaned_and_the_original_is_kept_in_the_tags(self):
        tags = self.one(self.plan(), "retag", src_suffix="Best Songs/50 Cent - In Da Club (Official Music Video).mp3")["tags"]
        self.assertEqual(tags["title"], "In Da Club")
        self.assertEqual(tags["original_title"], "50 Cent - In Da Club (Official Music Video)")
        self.assertEqual(tags["repair"], "p1")

    def test_featured_artists_go_to_the_tags(self):
        tags = self.one(self.plan(), "retag", src_suffix="Hate Bein' Sober.mp3")["tags"]
        self.assertEqual(tags["artist"], "Chief Keef")
        self.assertEqual(tags["featured"], "50 Cent; Wiz Khalifa")
        self.assertEqual(tags["albumartist"], "50 Cent")

    def test_no_produced_name_contains_a_mount_hostile_character(self):
        for a in self.plan({"AAAAAAAAAAA": matched(album='AC/DC: "Live"?')}).actions:
            for k in ("dst",):
                if a.get(k) and not a[k].startswith(".trash/"):
                    for part in a[k].split("/"):
                        self.assertFalse(set(':*?"<>|\\') & set(part), a[k])

    def test_two_songs_to_one_path_conflict_and_neither_moves(self):
        def mutate(inv):
            for t in inv.tracks:
                if t.video_id == "CCCCCCCCCCC":            # ABBA - Mamma Mia
                    t.tag_title = "Same Title"
                    t.relpath = t.relpath
                if t.video_id == "BBBBBBBBBBB":
                    t.tag_title = "Same Title"
                    t.tag_artist = "ABBA"
                    t.folder_artist = "ABBA"
        p = self.plan(mutate=mutate)
        conflicts = self.kind(p, "conflict")
        self.assertEqual(len(conflicts), 2)
        self.assertFalse([a for a in self.kind(p, "move") if a["dst"] == "ABBA/Singles/Same Title.mp3"])


class TestMatches(PlannerBase):
    def test_a_confident_match_files_under_the_lead_artist_and_album_with_a_track_number(self):
        p = self.plan({"BBBBBBBBBBB": matched(artist="Chief Keef", title="Hate Bein' Sober", album="Finally Rich", no=3,
                                              year=2012)})
        self.one(p, "move", dst="Chief Keef/Finally Rich/03 - Hate Bein' Sober.mp3")
        tags = self.one(p, "retag", src_suffix="Hate Bein' Sober.mp3")["tags"]
        self.assertEqual((tags["album"], tags["track"], tags["date"]), ("Finally Rich", "3", "2012"))
        self.assertEqual((tags["musicbrainz_recording"], tags["musicbrainz_release"]), ("rec", "rel"))
        self.one(p, "cover")

    def test_a_confident_match_wins_over_an_album_folder(self):
        p = self.plan({"DDDDDDDDDDD": matched(artist="Coldplay", title="Yellow", album="Parachutes (Deluxe)", no=5)})
        self.one(p, "move", dst="Coldplay/Parachutes (Deluxe)/05 - Yellow.mp3")

    def test_compilation_only_match_falls_back_to_the_album_folder_then_singles(self):
        p = self.plan({"DDDDDDDDDDD": matched(artist="Coldplay", title="Yellow", album=None, no=None),
                       "CCCCCCCCCCC": matched(artist="ABBA", title="Mamma Mia", album=None, no=None)})
        self.one(p, "move", dst="Coldplay/Parachutes/Yellow.mp3")
        self.one(p, "move", dst="ABBA/Singles/Mamma Mia.mp3")
        self.assertEqual(self.kind(p, "cover"), [])

    def test_no_match_and_not_looked_up_are_counted_separately(self):
        p = self.plan({"AAAAAAAAAAA": Result("no_match", None, ["musicbrainz", "itunes", "deezer"], 29.5),
                       "BBBBBBBBBBB": Result("not_looked_up", None, ["musicbrainz"], None)})
        self.assertEqual((p.totals["no_match"], p.totals["not_looked_up"], p.totals["matched"]), (1, 1, 0))

    def test_a_match_artist_spelling_reuses_the_existing_folder(self):
        p = self.plan({"AAAAAAAAAAA": matched(artist="50 cent")})
        self.one(p, "move", dst="50 Cent/Get Rich or Die Tryin'/05 - In Da Club.mp3")


class TestSettledCopies(PlannerBase):
    def test_a_settled_kept_copy_wins_and_new_copies_are_trashed_and_join_playlists(self):
        def mutate(inv):
            for x in inv.tracks:
                if x.video_id == "AAAAAAAAAAA" and "Best Songs" in x.relpath:
                    x.settled, x.repair_status = True, "no_match"
        p = self.plan(mutate=mutate)
        self.assertEqual(len([a for a in self.kind(p, "trash") if a["src"].endswith(".mp3")]), 2)
        self.assertFalse([a for a in p.actions if a["kind"] in ("move", "retag")
                          and "In Da Club" in (a.get("src") or "")])
        pls = {a["dst"]: a for a in self.kind(p, "playlist")}
        self.assertIn("Playlists/Party Hits.m3u8", pls)


class TestLeftovers(PlannerBase):
    def test_orphaned_album_nfo_is_set_aside_but_never_one_whose_audio_remains(self):
        p = self.plan()
        orphans = sorted(a["src"] for a in self.kind(p, "orphan_nfo"))
        self.assertIn("50 Cent/Best Songs/album.nfo", orphans)
        self.assertIn("50 Cent/Dance Music/album.nfo", orphans)
        self.assertIn("Coldplay/Coldplay - Everyday Life/album.nfo", orphans)
        self.assertFalse([o for o in orphans if o.endswith("artist.nfo")])

    def test_album_nfo_stays_when_a_track_in_the_folder_stays(self):
        def mutate(inv):
            # Chief Keef's track cannot be read, so it STAYS in 50 Cent/Party Hits.
            inv.tracks[:] = [x for x in inv.tracks if x.video_id != "BBBBBBBBBBB"]
            inv.unreadable.append(("50 Cent/Party Hits/Chief Keef Feat 50 Cent & Wiz Khalifa - Hate Bein' Sober.mp3",
                                   "HeaderNotFoundError"))
        orphans = [a["src"] for a in self.kind(self.plan(mutate=mutate), "orphan_nfo")]
        self.assertNotIn("50 Cent/Party Hits/album.nfo", orphans)
        self.assertIn("50 Cent/Dance Music/album.nfo", orphans)

    def test_webm_with_audio_converts_and_the_original_is_set_aside(self):
        p = self.plan(fs=FakeFs(audio=True))
        c = self.one(p, "convert")
        self.assertEqual((c["src"], c["dst"]), ("Jazz/Singles/Cozy Cabin.webm", "Jazz/Singles/Cozy Cabin.mp3"))
        self.assertIn("Jazz/Singles/Cozy Cabin.webm", [a["src"] for a in self.kind(p, "trash")])

    def test_webm_without_audio_is_reported_and_left(self):
        p = self.plan(fs=FakeFs(audio=False))
        self.assertEqual(self.kind(p, "convert"), [])
        self.assertIn(("Jazz/Singles/Cozy Cabin.webm", "no audio stream"),
                      [(s["relpath"], s["reason"]) for s in p.skipped])

    def test_bin_gets_its_real_type_or_is_set_aside(self):
        p = self.plan(fs=FakeFs(heads={"Alec Benjamin/logo.bin": b"\x89PNG\r\n\x1a\nxxxx"}))
        b = self.one(p, "rename_bin")
        self.assertEqual((b["src"], b["dst"]), ("Alec Benjamin/logo.bin", "Alec Benjamin/logo.png"))
        p = self.plan(fs=FakeFs(heads={"Alec Benjamin/logo.bin": b"not an image"}))
        self.assertEqual(self.kind(p, "rename_bin"), [])
        self.assertIn("Alec Benjamin/logo.bin", [a["src"] for a in self.kind(p, "trash")])

    def test_symlinks_and_unreadable_files_are_reported_not_acted_on(self):
        def mutate(inv):
            inv.symlinks.append("linked")
            inv.unreadable.append(("Bad/x.mp3", "HeaderNotFoundError"))
        p = self.plan(mutate=mutate)
        reasons = {s["relpath"]: s["reason"] for s in p.skipped}
        self.assertIn("linked", reasons)
        self.assertIn("Bad/x.mp3", reasons)


class TestNeedsLookup(PlannerBase):
    def test_unsettled_always_settled_only_when_the_lookup_never_completed(self):
        inv = inventory.scan(self.root)
        t = next(x for x in inv.tracks if x.video_id == "CCCCCCCCCCC")
        self.assertTrue(planner.needs_lookup(t))
        t.settled, t.repair_status = True, "matched"
        self.assertFalse(planner.needs_lookup(t))
        t.repair_status = "no_match"
        self.assertFalse(planner.needs_lookup(t))
        t.repair_status = "not_looked_up"
        self.assertTrue(planner.needs_lookup(t))


class TestRetryOfUnfinishedLookups(PlannerBase):
    def settle(self, status):
        def mutate(inv):
            for t in inv.tracks:
                if t.video_id == "CCCCCCCCCCC":
                    t.settled, t.repair_status = True, status
        return mutate

    def test_matched_and_no_match_settled_files_are_never_touched_again(self):
        for status in ("matched", "no_match"):
            p = self.plan({"CCCCCCCCCCC": matched(artist="ABBA", title="Mamma Mia", album="Arrival", no=1)},
                          mutate=self.settle(status))
            self.assertFalse([a for a in p.actions if "Mamma Mia" in (a.get("src") or "")], status)

    def test_not_looked_up_is_re_planned_when_the_answer_finally_comes(self):
        p = self.plan({"CCCCCCCCCCC": matched(artist="ABBA", title="Mamma Mia", album="Arrival", no=1)},
                      mutate=self.settle("not_looked_up"))
        self.one(p, "move", dst="ABBA/Arrival/01 - Mamma Mia.mp3")

    def test_not_looked_up_that_is_now_a_definite_no_only_records_the_status(self):
        p = self.plan({"CCCCCCCCCCC": Result("no_match", None, ["musicbrainz"], 30.0)},
                      mutate=self.settle("not_looked_up"))
        acts = [a for a in p.actions if "Mamma Mia" in (a.get("src") or "")]
        self.assertEqual([a["kind"] for a in acts], ["retag"])
        self.assertEqual(acts[0]["tags"]["repair_status"], "no_match")

    def test_still_not_looked_up_changes_nothing(self):
        p = self.plan({"CCCCCCCCCCC": Result("not_looked_up", None, ["musicbrainz"], None)},
                      mutate=self.settle("not_looked_up"))
        self.assertFalse([a for a in p.actions if "Mamma Mia" in (a.get("src") or "")])


if __name__ == "__main__":
    unittest.main()
