import os
import tempfile
import unittest

from music_repair import fixtures, tagio


@fixtures.require_deps
class TestTagIO(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.path = fixtures.make_mp3(os.path.join(self.tmp.name, "a.mp3"), title="Old - Title (Official Video)",
                                      artist="OldArtist", album="Old Playlist", video_id="AAAAAAAAAAA", seconds=2)

    def tearDown(self):
        self.tmp.cleanup()

    def test_write_and_read_back(self):
        tagio.write_tags(self.path, {
            "title": "Title", "artist": "Artist", "albumartist": "Artist", "album": "Album", "track": "5",
            "date": "2003", "musicbrainz_recording": "rec-id", "musicbrainz_release": "rel-id",
            "featured": "Bob; Sue", "original_title": "Old - Title (Official Video)", "repair": "plan1",
            "repair_status": "matched"})
        from mutagen.id3 import ID3
        t = ID3(self.path)
        self.assertEqual(str(t["TIT2"]), "Title")
        self.assertEqual(str(t["TPE1"]), "Artist")
        self.assertEqual(str(t["TPE2"]), "Artist")
        self.assertEqual(str(t["TALB"]), "Album")
        self.assertEqual(str(t["TRCK"]), "5")
        self.assertEqual(str(t["TDRC"]), "2003")
        self.assertEqual(t["UFID:http://musicbrainz.org"].data, b"rec-id")
        self.assertEqual(str(t["TXXX:MusicBrainz Album Id"]), "rel-id")
        self.assertEqual(str(t["TXXX:SYNODL_REPAIR"]), "plan1")
        self.assertEqual(str(t["TXXX:SYNODL_STATUS"]), "matched")
        # what identifies the video survives, so the file can still be found at its source
        self.assertEqual(str(t["TXXX:purl"]), "https://www.youtube.com/watch?v=AAAAAAAAAAA")

    def test_previous_values_are_returned_so_a_restore_can_put_them_back(self):
        prev = tagio.write_tags(self.path, {"title": "T", "album": "A", "repair": "p"})
        self.assertEqual(prev["title"], "Old - Title (Official Video)")
        self.assertEqual(prev["album"], "Old Playlist")
        self.assertIsNone(prev["repair"])
        tagio.restore_tags(self.path, prev)
        from mutagen.id3 import ID3
        t = ID3(self.path)
        self.assertEqual(str(t["TIT2"]), "Old - Title (Official Video)")
        self.assertEqual(str(t["TALB"]), "Old Playlist")
        self.assertNotIn("TXXX:SYNODL_REPAIR", t)

    def test_mtime_is_preserved(self):
        os.utime(self.path, (1_000_000_000, 1_000_000_000))
        tagio.write_tags(self.path, {"title": "T"})
        self.assertEqual(os.stat(self.path).st_mtime_ns, 1_000_000_000 * 10**9)

    def test_only_the_asked_keys_change(self):
        tagio.write_tags(self.path, {"repair": "p", "repair_status": "no_match"})
        from mutagen.id3 import ID3
        t = ID3(self.path)
        self.assertEqual(str(t["TIT2"]), "Old - Title (Official Video)")
        self.assertEqual(str(t["TXXX:SYNODL_STATUS"]), "no_match")

    def test_cover_embed_replaces_the_thumbnail_and_returns_the_previous_one(self):
        jpeg1 = b"\xff\xd8\xff\xe0" + b"1" * 40
        jpeg2 = b"\xff\xd8\xff\xe0" + b"2" * 40
        self.assertIsNone(tagio.embed_cover(self.path, jpeg1, "image/jpeg"))
        prev = tagio.embed_cover(self.path, jpeg2, "image/jpeg")
        self.assertEqual(prev[0], jpeg1)
        from mutagen.id3 import ID3
        self.assertEqual(ID3(self.path).getall("APIC")[0].data, jpeg2)
        tagio.restore_cover(self.path, prev)
        self.assertEqual(ID3(self.path).getall("APIC")[0].data, jpeg1)


if __name__ == "__main__":
    unittest.main()
