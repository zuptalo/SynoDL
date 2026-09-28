"""Opt-in smoke test against the REAL services. Never runs in CI.

    MUSIC_REPAIR_LIVE=1 scripts/music-repair-test.sh -k live

The other tests use responses captured from these services, which proves the
client parses what they used to say. This proves they still say it — worth
running before a multi-hour lookup pass, since an upstream change would
otherwise show up as thousands of songs quietly reported "not looked up".

It asks each service about one famous song, then feeds that service's OWN answer
back through the client's strict comparison, so it checks the request, the
parsing and the match logic without depending on a video's length.
"""

import os
import unittest
import urllib.parse

from music_repair import sources


@unittest.skipUnless(os.environ.get("MUSIC_REPAIR_LIVE") == "1", "set MUSIC_REPAIR_LIVE=1 to talk to the real services")
class TestLiveSources(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.lk = sources.Lookup()

    def test_musicbrainz_and_cover_art_archive(self):
        data = self.lk._json(f"{sources.MB}/recording?" + urllib.parse.urlencode(
            {"query": 'artist:"Coldplay" AND recording:"Yellow"', "limit": 25, "fmt": "json"}))
        rec = next(r for r in data["recordings"] if r.get("length") and r["title"] == "Yellow")
        credit = rec["artist-credit"][0]["name"]
        match, _ = self.lk._musicbrainz(credit, rec["title"], rec["length"] / 1000.0)
        self.assertIsNotNone(match, "MusicBrainz answered but the client found no confident match in its own answer")
        self.assertEqual(match.source, "musicbrainz")
        self.assertTrue(match.mbid_recording)
        if match.cover_url:
            data_, mime = sources.fetch_cover(match.cover_url)
            self.assertIn(mime, ("image/jpeg", "image/png"))

    def test_itunes(self):
        data = self.lk._json(sources.ITUNES + "?" + urllib.parse.urlencode(
            {"term": "Coldplay Yellow", "entity": "song", "limit": 5}))
        r = data["results"][0]
        match, _ = self.lk._itunes(r["artistName"], r["trackName"], r["trackTimeMillis"] / 1000.0)
        self.assertIsNotNone(match)
        self.assertTrue(match.album)
        self.assertIn("mzstatic.com", match.cover_url)

    def test_deezer(self):
        data = self.lk._json(sources.DEEZER + "/search?" + urllib.parse.urlencode({"q": "Coldplay Yellow", "limit": 5}))
        r = data["data"][0]
        match, _ = self.lk._deezer(r["artist"]["name"], r["title"], float(r["duration"]))
        self.assertIsNotNone(match)
        self.assertTrue(match.album)


if __name__ == "__main__":
    unittest.main()
