import unittest

from music_repair import identity
from music_repair.inventory import Track


def t(rel, vid="AAAAAAAAAAA", size=100, mtime=10, playlist="P", artist="A", settled=False):
    return Track(relpath=rel, size=size, mtime_ns=mtime, video_id=vid, folder_artist=artist,
                 folder_playlist=playlist, settled=settled)


class TestChooseKept(unittest.TestCase):
    def test_largest_size_wins(self):
        a, b = t("A/P1/x.mp3", size=100), t("A/P2/x.mp3", size=200)
        self.assertIs(identity.choose_kept([a, b]), b)

    def test_tie_goes_to_the_earliest_mtime(self):
        a, b = t("A/P1/x.mp3", mtime=20), t("A/P2/x.mp3", mtime=10)
        self.assertIs(identity.choose_kept([a, b]), b)

    def test_tie_goes_to_the_path(self):
        a, b = t("A/P2/x.mp3"), t("A/P1/x.mp3")
        self.assertIs(identity.choose_kept([a, b]), b)

    def test_deterministic_whatever_the_input_order(self):
        tracks = [t(f"A/P{i}/x.mp3", size=100 + (i % 2)) for i in range(6)]
        want = identity.choose_kept(tracks)
        self.assertIs(identity.choose_kept(list(reversed(tracks))), want)

    def test_an_already_settled_copy_is_kept_over_a_bigger_new_one(self):
        settled, new = t("A/Album/x.mp3", size=100, settled=True), t("A/P/x.mp3", size=999)
        self.assertIs(identity.choose_kept([new, settled]), settled)


class TestGroup(unittest.TestCase):
    def test_grouped_by_video_id_not_by_name(self):
        tracks = [t("A/P1/one.mp3", vid="AAAAAAAAAAA"), t("B/P2/different name.mp3", vid="AAAAAAAAAAA"),
                  t("A/P1/other.mp3", vid="BBBBBBBBBBB")]
        songs, unidentified = identity.group(tracks)
        self.assertEqual(sorted(s.key for s in songs), ["AAAAAAAAAAA", "BBBBBBBBBBB"])
        self.assertEqual(unidentified, [])
        a = next(s for s in songs if s.key == "AAAAAAAAAAA")
        self.assertEqual(len(a.copies), 1)

    def test_same_name_different_ids_are_two_songs(self):
        songs, _ = identity.group([t("A/P/x.mp3", vid="AAAAAAAAAAA"), t("A/Q/x.mp3", vid="BBBBBBBBBBB")])
        self.assertEqual(len(songs), 2)

    def test_no_id_is_kept_and_listed_never_merged(self):
        songs, unidentified = identity.group([t("A/P/x.mp3", vid=None), t("A/Q/x.mp3", vid=None)])
        self.assertEqual(songs, [])
        self.assertEqual(sorted(x.relpath for x in unidentified), ["A/P/x.mp3", "A/Q/x.mp3"])

    def test_playlists_come_from_unsettled_copies_only(self):
        songs, _ = identity.group([
            t("A/Old Mix/x.mp3", playlist="Old Mix"),
            t("A/Album/x.mp3", playlist="Album", settled=True),
            t("B/New Mix/x.mp3", playlist="New Mix"),
            t("A/x.mp3", playlist=None)])
        self.assertEqual(sorted(songs[0].playlists), ["New Mix", "Old Mix"])

    def test_copies_exclude_the_kept_one(self):
        songs, _ = identity.group([t("A/P1/x.mp3", size=1), t("A/P2/x.mp3", size=2), t("A/P3/x.mp3", size=3)])
        s = songs[0]
        self.assertEqual(s.kept.relpath, "A/P3/x.mp3")
        self.assertEqual(sorted(c.relpath for c in s.copies), ["A/P1/x.mp3", "A/P2/x.mp3"])


if __name__ == "__main__":
    unittest.main()
