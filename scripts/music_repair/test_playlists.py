import unittest

from music_repair import playlists
from music_repair.playlists import Entry


def e(artist, title, rel, dur=200.0):
    return Entry(artist=artist, title=title, relpath=rel, duration_s=dur)


class TestKey(unittest.TestCase):
    def test_case_and_whitespace_only(self):
        self.assertEqual(playlists.key("Old TikTok  Songs "), playlists.key(" old tiktok songs"))
        self.assertNotEqual(playlists.key("Old TikTok Songs"), playlists.key("Old TikTok Song"))
        self.assertNotEqual(playlists.key("A - B"), playlists.key("A: B"))


class TestRender(unittest.TestCase):
    def test_header_relative_paths_and_order_by_artist_then_title(self):
        text = playlists.render([e("Zed", "B", "Zed/Singles/B.mp3"), e("Abba", "Z", "Abba/Singles/Z.mp3"),
                                 e("Abba", "A", "Abba/Album/A.mp3")])
        lines = text.splitlines()
        self.assertEqual(lines[0], "#EXTM3U")
        paths = [l for l in lines if not l.startswith("#")]
        self.assertEqual(paths, ["../Abba/Album/A.mp3", "../Abba/Singles/Z.mp3", "../Zed/Singles/B.mp3"])

    def test_extinf_carries_seconds_and_artist_title(self):
        text = playlists.render([e("Abba", "A", "Abba/A.mp3", 200.4)])
        self.assertIn("#EXTINF:200,Abba - A", text)

    def test_paths_resolve_from_playlists_dir(self):
        import posixpath
        rel = "Abba/Album/A.mp3"
        line = [l for l in playlists.render([e("Abba", "A", rel)]).splitlines() if not l.startswith("#")][0]
        self.assertEqual(posixpath.normpath(posixpath.join("Playlists", line)), rel)

    def test_a_hostile_title_cannot_add_an_entry_or_a_directive(self):
        text = playlists.render([e("Evil\n#EXTINF:1,x", "T\r\n../../etc/passwd", "A/B/c.mp3")])
        self.assertEqual(len([l for l in text.splitlines() if l.startswith("#EXTINF")]), 1)
        self.assertEqual(len([l for l in text.splitlines() if not l.startswith("#")]), 1)
        self.assertNotIn("\r", text)

    def test_duplicates_collapse(self):
        text = playlists.render([e("A", "T", "A/T.mp3"), e("A", "T", "A/T.mp3")])
        self.assertEqual(len([l for l in text.splitlines() if not l.startswith("#")]), 1)


class TestMerge(unittest.TestCase):
    def test_union_with_an_existing_file_without_duplicates(self):
        old = playlists.render([e("A", "One", "A/One.mp3")])
        new = playlists.merge(old, [e("A", "One", "A/One.mp3"), e("B", "Two", "B/Two.mp3")])
        paths = [l for l in new.splitlines() if not l.startswith("#")]
        self.assertEqual(paths, ["../A/One.mp3", "../B/Two.mp3"])

    def test_existing_entries_keep_their_metadata_when_not_in_the_new_set(self):
        old = playlists.render([e("A", "One", "A/One.mp3", 100)])
        new = playlists.merge(old, [e("B", "Two", "B/Two.mp3")])
        self.assertIn("#EXTINF:100,A - One", new)

    def test_garbage_existing_file_is_treated_as_empty(self):
        new = playlists.merge("\x00\x01 not a playlist", [e("B", "Two", "B/Two.mp3")])
        self.assertIn("../B/Two.mp3", new)


class TestFilenames(unittest.TestCase):
    def test_names_are_sanitised_and_collisions_get_a_suffix(self):
        got = playlists.filenames(["Coldplay: Everyday Life", "coldplay： everyday life", "Hits"])
        self.assertEqual(len(set(got.values())), 3)
        for name in got.values():
            self.assertTrue(name.endswith(".m3u8"))
            self.assertNotIn(":", name)
            self.assertNotIn("/", name)


if __name__ == "__main__":
    unittest.main()
