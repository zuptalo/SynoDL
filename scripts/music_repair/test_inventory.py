import os
import tempfile
import unittest

from music_repair import fixtures, inventory


@fixtures.require_deps
class TestInventory(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tmp = tempfile.TemporaryDirectory()
        cls.root = fixtures.build_library(cls.tmp.name)
        cls.inv = inventory.scan(cls.root)
        cls.by_rel = {t.relpath: t for t in cls.inv.tracks}

    @classmethod
    def tearDownClass(cls):
        cls.tmp.cleanup()

    def track(self, suffix):
        matches = [t for t in self.inv.tracks if t.relpath.endswith(suffix)]
        self.assertEqual(len(matches), 1, suffix)
        return matches[0]

    def test_video_id_from_purl(self):
        t = self.track("Party Hits/50 Cent - In Da Club (Official Music Video).mp3")
        self.assertEqual(t.video_id, "AAAAAAAAAAA")

    def test_missing_id_is_none_not_a_guess(self):
        self.assertIsNone(self.track("No Id Track.mp3").video_id)

    def test_fields(self):
        t = self.track("Dance Music/50 Cent - In Da Club (Official Music Video).mp3")
        self.assertEqual((t.folder_artist, t.folder_playlist), ("50 Cent", "Dance Music"))
        self.assertEqual(t.tag_artist, "50 Cent")
        self.assertEqual(t.tag_album, "Dance Music")
        self.assertGreater(t.size, 0)
        self.assertGreater(t.mtime_ns, 0)
        self.assertGreater(t.duration_s, 1.0)
        self.assertFalse(t.settled)

    def test_same_song_copies_differ_in_size(self):
        sizes = {t.size for t in self.inv.tracks if t.video_id == "AAAAAAAAAAA"}
        self.assertEqual(len(sizes), 3)

    def test_lyrics_pair_with_their_audio(self):
        t = self.track("Party Hits/50 Cent - In Da Club (Official Music Video).mp3")
        self.assertTrue(t.lyrics.endswith("In Da Club (Official Music Video).lrc"))
        self.assertIsNone(self.track("Dance Music/50 Cent - In Da Club (Official Music Video).mp3").lyrics)

    def test_other_files_are_classified(self):
        kinds = {k: sorted(v) for k, v in self.inv.other.items()}
        self.assertEqual(kinds["webm"], ["Jazz/Singles/Cozy Cabin.webm"])
        self.assertEqual(kinds["bin"], ["Alec Benjamin/logo.bin"])
        self.assertIn("50 Cent/artist.nfo", kinds["nfo"])
        self.assertIn("50 Cent/Party Hits/album.nfo", kinds["nfo"])
        self.assertIn("50 Cent/folder.jpg", kinds["image"])
        self.assertEqual(kinds["archive"], [".ytdlp-archive.txt"])

    def test_colon_folder_detected(self):
        self.assertEqual(self.inv.colon_dirs, ["Coldplay/Coldplay: Everyday Life"])

    def test_repair_trash_and_playlists_are_not_scanned(self):
        for d in (".repair", ".trash", "Playlists"):
            fixtures.make_mp3(os.path.join(self.root, d, "x", "y.mp3"), title="x", video_id="ZZZZZZZZZZZ")
        try:
            inv = inventory.scan(self.root)
            self.assertFalse(any(t.video_id == "ZZZZZZZZZZZ" for t in inv.tracks))
        finally:
            import shutil
            for d in (".repair", ".trash", "Playlists"):
                shutil.rmtree(os.path.join(self.root, d))

    def test_symlinks_are_not_followed(self):
        outside = tempfile.TemporaryDirectory()
        try:
            fixtures.make_mp3(os.path.join(outside.name, "out.mp3"), title="o", video_id="OOOOOOOOOOO")
            os.symlink(outside.name, os.path.join(self.root, "linked"))
            inv = inventory.scan(self.root)
            self.assertFalse(any(t.video_id == "OOOOOOOOOOO" for t in inv.tracks))
            self.assertEqual(inv.symlinks, ["linked"])
        finally:
            os.unlink(os.path.join(self.root, "linked"))
            outside.cleanup()

    def test_unreadable_mp3_is_reported_not_fatal(self):
        bad = fixtures.touch(os.path.join(self.root, "Bad", "Broken", "broken.mp3"), b"not an mp3")
        try:
            inv = inventory.scan(self.root)
            self.assertIn("Bad/Broken/broken.mp3", [r for r, _ in inv.unreadable] +
                          [t.relpath for t in inv.tracks if t.video_id is None])
        finally:
            import shutil
            shutil.rmtree(os.path.join(self.root, "Bad"))

    def test_settled_marker(self):
        from mutagen.id3 import ID3, TXXX
        t = self.track("Party Hits/ABBA - Mamma Mia (Official Music Video).mp3")
        path = os.path.join(self.root, t.relpath)
        tags = ID3(path)
        tags.add(TXXX(encoding=3, desc="SYNODL_REPAIR", text="plan1"))
        tags.save(path, v2_version=3)
        try:
            again = {x.relpath: x for x in inventory.scan(self.root).tracks}[t.relpath]
            self.assertTrue(again.settled)
        finally:
            tags = ID3(path)
            tags.delall("TXXX:SYNODL_REPAIR")
            tags.save(path, v2_version=3)


if __name__ == "__main__":
    unittest.main()
