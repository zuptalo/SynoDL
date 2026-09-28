import json
import os
import tempfile
import unittest

from music_repair import names, sources
from music_repair.sources import FetchError, Lookup, Response, Cache
from music_repair.test_sources import FakeClock, Recorder, ok

REC_ID = "fc00ffd5-fb0a-48cc-b1f9-045d77317cf3"
REL_ALBUM = "11111111-aaaa-bbbb-cccc-000000000001"
REL_COMP = "22222222-aaaa-bbbb-cccc-000000000002"


def mb_search(*recs):
    return ok({"recordings": list(recs)})


def rec(id_, length_ms, title="In Da Club", artist="50 Cent", first="2003-01-07"):
    return {"id": id_, "title": title, "length": length_ms, "score": 100, "first-release-date": first,
            "artist-credit": [{"name": artist, "artist": {"name": artist}}]}


def release(id_, title, date, primary="Album", secondary=()):
    return {"id": id_, "title": title, "date": date, "status": "Official",
            "release-group": {"primary-type": primary, "secondary-types": list(secondary)}}


def mb_routes(recs, releases, tracks=None):
    tracks = tracks if tracks is not None else {REL_ALBUM: 5}
    routes = [("/ws/2/recording?", mb_search(*recs)),
              ("/ws/2/release?recording=", ok({"release-count": len(releases), "releases": releases}))]
    for rid, no in tracks.items():
        routes.append((f"/ws/2/release/{rid}?", ok({"media": [{"position": 1, "tracks": [
            {"number": str(no), "position": no, "recording": {"id": REC_ID}}]}]})))
    return routes


def make_lookup(routes, cache=None):
    clock = FakeClock()
    return Lookup(transport=Recorder(routes), limiter=sources.RateLimiter(clock=clock.time, sleep=clock.sleep),
                  cache=cache), clock


ITUNES_OK = ok({"results": [{"trackName": "In da Club", "artistName": "50 Cent",
                              "collectionName": "Get Rich or Die Tryin'", "trackNumber": 5,
                              "releaseDate": "2003-01-07T12:00:00Z", "trackTimeMillis": 223500,
                              "artworkUrl100": "https://is1-ssl.mzstatic.com/x/100x100bb.jpg"}]})
ITUNES_LONG = ok({"results": [{"trackName": "In da Club", "artistName": "50 Cent",
                                "collectionName": "GRODT", "trackNumber": 5, "releaseDate": "2003-01-07",
                                "trackTimeMillis": 193467, "artworkUrl100": "https://is1-ssl.mzstatic.com/x/100x100bb.jpg"}]})
DEEZER_SEARCH = ok({"data": [{"id": 42, "title": "In Da Club", "artist": {"name": "50 Cent"}, "duration": 223,
                               "album": {"title": "Get Rich", "cover_xl": "https://cdn-images.dzcdn.net/x.jpg"}}]})
DEEZER_TRACK = ok({"id": 42, "track_position": 5, "release_date": "2003-02-06",
                   "album": {"title": "Get Rich or Die Tryin'", "cover_xl": "https://cdn-images.dzcdn.net/x.jpg"}})


class TestMusicBrainz(unittest.TestCase):
    def resolve(self, lk, **kw):
        args = dict(key="AAAAAAAAAAA", lead="50 Cent", title="In Da Club", duration_s=223.0, featured=[])
        args.update(kw)
        return lk.resolve(**args)

    def test_filters_recordings_by_length_then_picks_the_album(self):
        routes = mb_routes(
            [rec("live-1", 250_000), rec(REC_ID, 223_000)],
            [release(REL_COMP, "Bravo Hits", "2020-11-27", secondary=["Compilation"]),
             release(REL_ALBUM, "Get Rich or Die Tryin'", "2003-02-06")])
        lk, _ = make_lookup(routes)
        r = self.resolve(lk)
        self.assertEqual(r.status, "matched")
        m = r.match
        self.assertEqual((m.source, m.album, m.track_no, m.year), ("musicbrainz", "Get Rich or Die Tryin'", 5, 2003))
        self.assertEqual((m.mbid_recording, m.mbid_release), (REC_ID, REL_ALBUM))
        self.assertEqual(m.cover_url, f"https://coverartarchive.org/release/{REL_ALBUM}/front-500")
        self.assertAlmostEqual(m.delta_s, 0.0)

    def test_compilation_only_is_confident_with_no_album_and_the_first_release_year(self):
        routes = mb_routes([rec(REC_ID, 223_000)],
                           [release(REL_COMP, "Bravo Hits", "2020-11-27", secondary=["Compilation"])], tracks={})
        lk, _ = make_lookup(routes)
        m = self.resolve(lk).match
        self.assertIsNone(m.album)
        self.assertIsNone(m.track_no)
        self.assertIsNone(m.cover_url)
        self.assertEqual(m.year, 2003)
        self.assertEqual(m.mbid_recording, REC_ID)

    def test_one_request_a_second_across_the_three_calls(self):
        routes = mb_routes([rec(REC_ID, 223_000)], [release(REL_ALBUM, "A", "2003")])
        lk, clock = make_lookup(routes)
        self.resolve(lk)
        self.assertEqual(len(clock.slept), 2)

    def test_the_query_is_escaped(self):
        routes = mb_routes([], [])
        lk, _ = make_lookup(routes)
        self.resolve(lk, lead='A "B" \\ C', title='Say "hi"')
        url = lk.transport.requests[0][0]
        self.assertNotIn('"B"', url)

    def test_featured_are_carried_from_the_file(self):
        routes = mb_routes([rec(REC_ID, 223_000)], [release(REL_ALBUM, "A", "2003")])
        lk, _ = make_lookup(routes)
        m = self.resolve(lk, featured=["Bob"]).match
        self.assertEqual(m.featured, ["Bob"])


class TestFallbackOrder(unittest.TestCase):
    def run_it(self, routes, cache=None):
        lk, _ = make_lookup(routes, cache)
        return lk, lk.resolve(key="AAAAAAAAAAA", lead="50 Cent", title="In Da Club",
                              duration_s=223.0, featured=[])

    def test_musicbrainz_first_and_stops_at_the_first_confident_answer(self):
        routes = mb_routes([rec(REC_ID, 223_000)], [release(REL_ALBUM, "A", "2003")]) + [("itunes", ITUNES_OK)]
        lk, r = self.run_it(routes)
        self.assertEqual(r.match.source, "musicbrainz")
        self.assertFalse(any("itunes" in u for u, _ in lk.transport.requests))

    def test_itunes_when_musicbrainz_is_not_confident(self):
        routes = mb_routes([rec("x", 300_000)], []) + [("itunes.apple.com", ITUNES_OK)]
        _, r = self.run_it(routes)
        self.assertEqual(r.match.source, "itunes")
        self.assertEqual((r.match.album, r.match.track_no, r.match.year), ("Get Rich or Die Tryin'", 5, 2003))
        self.assertIn("600x600bb", r.match.cover_url)

    def test_deezer_last(self):
        routes = mb_routes([], []) + [("itunes.apple.com", ITUNES_LONG),
                                      ("api.deezer.com/search", DEEZER_SEARCH),
                                      ("api.deezer.com/track/42", DEEZER_TRACK)]
        _, r = self.run_it(routes)
        self.assertEqual(r.match.source, "deezer")
        self.assertEqual((r.match.album, r.match.track_no, r.match.year), ("Get Rich or Die Tryin'", 5, 2003))

    def test_no_confident_answer_anywhere_is_no_match_with_the_nearest_length(self):
        routes = mb_routes([rec("x", 300_000)], []) + [("itunes.apple.com", ITUNES_LONG),
                                                       ("api.deezer.com/search", ok({"data": []}))]
        _, r = self.run_it(routes)
        self.assertEqual(r.status, "no_match")
        self.assertEqual(r.tried, ["musicbrainz", "itunes", "deezer"])
        self.assertAlmostEqual(abs(r.nearest_delta_s), 29.533, places=2)

    def test_a_source_that_fails_is_not_looked_up_not_no_match(self):
        routes = [("musicbrainz.org", Response(503, {}, b"")),
                  ("itunes.apple.com", ok({"results": []})), ("api.deezer.com", ok({"data": []}))]
        _, r = self.run_it(routes)
        self.assertEqual(r.status, "not_looked_up")

    def test_a_confident_answer_from_a_later_source_still_wins_when_an_earlier_one_failed(self):
        routes = [("musicbrainz.org", Response(503, {}, b"")), ("itunes.apple.com", ITUNES_OK)]
        _, r = self.run_it(routes)
        self.assertEqual((r.status, r.match.source), ("matched", "itunes"))

    def test_blocked_redirect_is_a_failed_source_not_a_crash(self):
        routes = [("musicbrainz.org", Response(302, {"location": "https://evil.example.com/"}, b"")),
                  ("itunes.apple.com", ok({"results": []})), ("api.deezer.com", ok({"data": []}))]
        _, r = self.run_it(routes)
        self.assertEqual(r.status, "not_looked_up")


class TestCache(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.tmp.name, "cache.json")

    def tearDown(self):
        self.tmp.cleanup()

    def test_a_hit_avoids_the_network(self):
        routes = mb_routes([rec(REC_ID, 223_000)], [release(REL_ALBUM, "A", "2003")])
        cache = Cache(self.path)
        lk, _ = make_lookup(routes, cache)
        lk.resolve(key="K1", lead="50 Cent", title="In Da Club", duration_s=223.0, featured=[])
        n = len(lk.transport.requests)
        again = lk.resolve(key="K1", lead="50 Cent", title="In Da Club", duration_s=223.0, featured=[])
        self.assertEqual(len(lk.transport.requests), n)
        self.assertEqual(again.match.album, "A")

    def test_no_match_is_cached_but_not_looked_up_is_not(self):
        cache = Cache(self.path)
        lk, _ = make_lookup([("musicbrainz.org", Response(503, {}, b"")),
                             ("itunes.apple.com", ok({"results": []})), ("api.deezer.com", ok({"data": []}))], cache)
        lk.resolve(key="F", lead="A", title="B", duration_s=100.0, featured=[])
        self.assertIsNone(cache.get("F"))
        lk2, _ = make_lookup(mb_routes([], []) + [("itunes.apple.com", ok({"results": []})),
                                                 ("api.deezer.com", ok({"data": []}))], cache)
        lk2.resolve(key="N", lead="A", title="B", duration_s=100.0, featured=[])
        self.assertEqual(cache.get("N")["status"], "no_match")

    def test_persists_and_reloads(self):
        cache = Cache(self.path)
        cache.put("K", {"status": "no_match", "match": None, "sources_tried": ["musicbrainz"]})
        cache.flush()
        self.assertEqual(Cache(self.path).get("K")["status"], "no_match")

    def test_flushes_every_25_puts_so_a_lost_pod_resumes(self):
        cache = Cache(self.path)
        for i in range(25):
            cache.put(f"K{i}", {"status": "no_match", "match": None})
        self.assertTrue(os.path.exists(self.path))
        with open(self.path) as f:
            self.assertEqual(len(json.load(f)["entries"]), 25)

    def test_write_is_atomic_no_temp_left_behind(self):
        cache = Cache(self.path)
        cache.put("K", {"status": "no_match", "match": None})
        cache.flush()
        self.assertEqual(sorted(os.listdir(self.tmp.name)), ["cache.json"])

    def test_corrupt_file_is_ignored_not_fatal(self):
        with open(self.path, "w") as f:
            f.write("{not json")
        cache = Cache(self.path)
        self.assertIsNone(cache.get("K"))
        cache.put("K", {"status": "no_match", "match": None})
        cache.flush()
        self.assertEqual(Cache(self.path).get("K")["status"], "no_match")


class TestCover(unittest.TestCase):
    JPEG = b"\xff\xd8\xff\xe0" + b"0" * 64
    PNG = b"\x89PNG\r\n\x1a\n" + b"0" * 64

    def fetch(self, routes, **kw):
        clock = FakeClock()
        return sources.fetch_cover(f"https://coverartarchive.org/release/{REL_ALBUM}/front-500",
                                   transport=Recorder(routes),
                                   limiter=sources.RateLimiter(clock=clock.time, sleep=clock.sleep), **kw)

    def test_follows_the_archive_org_hops_and_sniffs_the_type(self):
        routes = [("coverartarchive.org", Response(307, {"location": "https://archive.org/download/a.jpg"}, b"")),
                  ("archive.org/download", Response(302, {"location": "https://dn1.ca.archive.org/a.jpg"}, b"")),
                  ("dn1.ca.archive.org", Response(200, {"content-type": "text/plain"}, self.JPEG))]
        data, mime = self.fetch(routes)
        self.assertEqual((data, mime), (self.JPEG, "image/jpeg"))

    def test_png_by_magic_bytes_not_by_header(self):
        data, mime = self.fetch([("coverartarchive.org", Response(200, {"content-type": "image/jpeg"}, self.PNG))])
        self.assertEqual(mime, "image/png")

    def test_not_an_image_is_discarded(self):
        with self.assertRaises(sources.FetchError):
            self.fetch([("coverartarchive.org", Response(200, {"content-type": "image/jpeg"}, b"<html>nope</html>"))])

    def test_missing_art_is_not_found(self):
        with self.assertRaises(sources.NotFound):
            self.fetch([("coverartarchive.org", Response(404, {}, b""))])

    def test_oversize_is_discarded(self):
        with self.assertRaises(sources.FetchError):
            self.fetch([("coverartarchive.org", Response(200, {}, self.JPEG + b"0" * 5000))], max_bytes=1000)


if __name__ == "__main__":
    unittest.main()
