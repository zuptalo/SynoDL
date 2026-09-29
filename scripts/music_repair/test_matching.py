import unittest

from music_repair import matching


def rel(id_, title, date, primary="Album", secondary=(), status="Official"):
    return {"id": id_, "title": title, "date": date, "status": status,
            "release-group": {"primary-type": primary, "secondary-types": list(secondary)}}


class TestConfidence(unittest.TestCase):
    def cmp(self, delta=0.0, artist="50 Cent", title="In Da Club", cand_artist="50 Cent",
            cand_title="In Da Club"):
        return matching.compare("50 Cent", "In Da Club", 223.0, cand_artist, cand_title, 223.0 + delta)

    def test_exact_is_confident(self):
        self.assertTrue(self.cmp().confident)

    def test_three_seconds_passes_and_just_over_fails(self):
        self.assertTrue(self.cmp(3.0).confident)
        self.assertTrue(self.cmp(-3.0).confident)
        self.assertFalse(self.cmp(3.1).confident)
        self.assertFalse(self.cmp(-3.1).confident)

    def test_case_and_punctuation_are_normalised(self):
        c = matching.compare("SNAP!", "The Power", 200, "Snap!", "the power", 200)
        self.assertTrue(c.confident)

    def test_a_near_miss_title_is_not_confident(self):
        self.assertFalse(matching.compare("A", "In Da Club", 10, "A", "In Da Club Remix", 10).confident)
        self.assertFalse(matching.compare("A", "In Da Club", 10, "A", "In Da Clubs", 10).confident)

    def test_a_different_artist_is_not_confident(self):
        self.assertFalse(matching.compare("A", "Song", 10, "B", "Song", 10).confident)

    def test_feat_is_normalised_on_both_sides(self):
        c = matching.compare("Chief Keef", "Hate Bein' Sober", 200,
                             "Chief Keef", "Hate Bein' Sober (feat. 50 Cent & Wiz Khalifa)", 200)
        self.assertTrue(c.confident)

    def test_unknown_length_is_never_confident(self):
        self.assertFalse(matching.compare("A", "S", 0.0, "A", "S", 100).confident)
        self.assertFalse(matching.compare("A", "S", 100, "A", "S", None).confident)

    def test_comparison_records_why(self):
        c = self.cmp(2.0)
        self.assertEqual((c.artist_eq, c.title_eq), (True, True))
        self.assertAlmostEqual(c.delta_s, 2.0)


class TestChooseRelease(unittest.TestCase):
    def test_compilation_only_means_no_album(self):
        self.assertIsNone(matching.choose_release(
            [rel("a", "Bravo Hits", "2020-11-27", secondary=["Compilation"])]))

    def test_live_soundtrack_remix_are_not_albums(self):
        for sec in ("Live", "Soundtrack", "Remix", "DJ-mix", "Mixtape/Street", "Demo"):
            with self.subTest(sec=sec):
                self.assertIsNone(matching.choose_release([rel("a", "X", "2000", secondary=[sec])]))

    def test_album_beats_ep_beats_single_then_earliest(self):
        got = matching.choose_release([
            rel("s", "Single", "2003-01-01", primary="Single"),
            rel("e", "EP", "2003-01-02", primary="EP"),
            rel("b", "Album Later", "2005-05-05"),
            rel("a", "Album", "2003-02-06"),
        ])
        self.assertEqual(got["id"], "a")

    def test_single_is_used_when_it_is_all_there_is(self):
        self.assertEqual(matching.choose_release([rel("s", "S", "2003", primary="Single")])["id"], "s")

    def test_undated_sorts_last_and_ties_break_on_id(self):
        got = matching.choose_release([rel("z", "Z", None), rel("b", "B", "2001"), rel("a", "A", "2001")])
        self.assertEqual(got["id"], "a")

    def test_unofficial_is_ignored(self):
        self.assertIsNone(matching.choose_release([rel("a", "Boot", "2001", status="Bootleg")]))

    def test_nothing(self):
        self.assertIsNone(matching.choose_release([]))


class TestYear(unittest.TestCase):
    def test_year_prefix(self):
        self.assertEqual(matching.year_of("2003-02-06"), 2003)
        self.assertEqual(matching.year_of("2003"), 2003)
        self.assertIsNone(matching.year_of(""))
        self.assertIsNone(matching.year_of(None))
        self.assertIsNone(matching.year_of("garbage"))


if __name__ == "__main__":
    unittest.main()
