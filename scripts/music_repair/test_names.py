import json
import os
import unittest

from music_repair import names

CASES = os.path.join(os.path.dirname(__file__), "names_cases.json")


class TestSanitize(unittest.TestCase):
    def test_shared_table(self):
        with open(CASES, encoding="utf-8") as f:
            table = json.load(f)["sanitize"]
        for raw, want in table:
            with self.subTest(raw=raw):
                self.assertEqual(names.sanitize_name(raw), want)

    def test_length_bound_is_on_a_rune_boundary(self):
        got = names.sanitize_name("é" * 500)
        self.assertLessEqual(len(got.encode("utf-8")), names.MAX_NAME_LENGTH)
        got.encode("utf-8").decode("utf-8")  # not cut mid-character

    def test_nfc(self):
        self.assertEqual(names.sanitize_name("é"), "é")

    def test_no_hostile_character_survives(self):
        for ch in ':*?"<>|/\\':
            self.assertNotIn(ch, names.sanitize_name(f"a{ch}b"))


class TestFold(unittest.TestCase):
    def test_fullwidth_and_case_and_punctuation(self):
        self.assertEqual(names.fold("＂Hate Bein' Sober＂"), names.fold('"hate bein sober"'))
        self.assertEqual(names.fold("SNAP!"), names.fold("snap"))
        self.assertEqual(names.fold("Beyoncé"), names.fold("beyonce"))


class TestCleanTitle(unittest.TestCase):
    def check(self, raw, want_title, want_feat=()):
        title, feat = names.clean_title(raw)
        self.assertEqual(title, want_title)
        self.assertEqual(list(feat), list(want_feat))

    def test_noise_groups_removed(self):
        self.check("In Da Club (Official Music Video)", "In Da Club")
        self.check("Please Don't Go [Official Video]", "Please Don't Go")
        self.check("The Power (Official 4K Music Video)", "The Power")
        self.check("Song (Lyric Video) [HD]", "Song")
        self.check("Song (Official Audio)", "Song")

    def test_versions_are_different_recordings_and_kept(self):
        self.check("Song (Live)", "Song (Live)")
        self.check("Song (Remix)", "Song (Remix)")
        self.check("Song (Acoustic Version)", "Song (Acoustic Version)")
        self.check("Song (Remastered 2011)", "Song (Remastered 2011)")

    def test_feat_lifted_out(self):
        self.check("Song (feat. Bob & Sue)", "Song", ["Bob", "Sue"])
        self.check("Song [ft. Bob]", "Song", ["Bob"])
        self.check("Song (Official Video) (featuring Bob)", "Song", ["Bob"])

    def test_untouched_when_nothing_to_clean(self):
        self.check("Yellow", "Yellow")
        self.check("(What's the Story) Morning Glory?", "(What's the Story) Morning Glory?")

    def test_idempotent(self):
        for raw in ("In Da Club (Official Music Video)", "Song (feat. Bob)", "Song (Live)"):
            once, _ = names.clean_title(raw)
            twice, _ = names.clean_title(once)
            self.assertEqual(once, twice)

    def test_never_empty(self):
        self.check("(Official Video)", "(Official Video)")


class TestSplitArtistTitle(unittest.TestCase):
    def test_prefix_equal_to_tag_artist(self):
        got = names.split_artist_title("50 Cent - In Da Club (Official Music Video)", "50 Cent")
        self.assertEqual(got.lead, "50 Cent")
        self.assertEqual(got.title, "In Da Club")

    def test_prefix_with_feat_and_a_different_lead(self):
        got = names.split_artist_title(
            "Chief Keef Feat 50 Cent & Wiz Khalifa - ＂Hate Bein' Sober＂", "50 Cent")
        self.assertEqual(got.lead, "Chief Keef")
        self.assertEqual(got.featured, ["50 Cent", "Wiz Khalifa"])
        self.assertEqual(got.title, "Hate Bein' Sober")

    def test_no_prefix_uses_the_tag_artist(self):
        got = names.split_artist_title("Yellow", "Coldplay")
        self.assertEqual((got.lead, got.title), ("Coldplay", "Yellow"))

    def test_prefix_that_is_not_an_artist_is_not_trusted_when_tag_disagrees(self):
        got = names.split_artist_title("Live - Tonight (Official Video)", "Someone Else")
        self.assertEqual(got.lead, "Someone Else")
        self.assertEqual(got.title, "Live - Tonight")

    def test_already_clean_is_idempotent(self):
        got = names.split_artist_title("In Da Club", "50 Cent")
        self.assertEqual((got.lead, got.title), ("50 Cent", "In Da Club"))


class TestArtistFolder(unittest.TestCase):
    def test_channel_markers_stripped_as_whole_suffix_tokens_only(self):
        self.assertEqual(names.clean_artist_folder("10ccVEVO"), "10cc")
        self.assertEqual(names.clean_artist_folder("ABKCO VEVO"), "ABKCO")
        self.assertEqual(names.clean_artist_folder("Some Band - Topic"), "Some Band")
        self.assertEqual(names.clean_artist_folder("Some Band Official"), "Some Band")
        self.assertEqual(names.clean_artist_folder("Some Band Official YouTube Channel"), "Some Band")

    def test_real_names_containing_the_words_are_untouched(self):
        for n in ("42 dugg Music", "Music Brokers", "Official Nonsense Band", "Vevoland", "Topical"):
            self.assertEqual(names.clean_artist_folder(n), n)

    def test_never_empty(self):
        self.assertEqual(names.clean_artist_folder("VEVO"), "VEVO")


class TestAlbumFolder(unittest.TestCase):
    def test_artist_dash_album(self):
        self.assertEqual(names.album_of_folder("Coldplay - Parachutes", ["Coldplay"]), "Parachutes")
        self.assertEqual(names.album_of_folder("Coldplay X BTS - My Universe", ["Coldplay"]), None)

    def test_playlists_are_not_albums(self):
        self.assertIsNone(names.album_of_folder("Old TikTok Songs That We Forgot About", ["50 Cent"]))
        self.assertIsNone(names.album_of_folder("Singles", ["50 Cent"]))

    def test_all_tracks_must_be_by_that_artist(self):
        self.assertIsNone(names.album_of_folder("Coldplay - Parachutes", ["Coldplay", "Someone"]))

    def test_colon_twin(self):
        self.assertEqual(names.album_of_folder("Coldplay: Everyday Life", ["Coldplay"]), "Everyday Life")


class TestSafeRel(unittest.TestCase):
    def test_accepts_plain_relative(self):
        self.assertTrue(names.safe_rel("A/B/c.mp3"))

    def test_rejects_escapes(self):
        for bad in ("/etc/passwd", "../x", "a/../../x", "a/./b/../../../x", "", "a\x00b"):
            with self.subTest(bad=bad):
                self.assertFalse(names.safe_rel(bad))


if __name__ == "__main__":
    unittest.main()
