import json
import os
import tempfile
import unittest

from music_repair import sources
from music_repair.sources import Response


class Recorder:
    """A transport that never touches the network: it answers from a route table."""

    def __init__(self, routes=None):
        self.routes = routes or []          # [(substring, Response | callable)]
        self.requests = []

    def __call__(self, url, headers, timeout, max_bytes):
        self.requests.append((url, dict(headers)))
        for needle, resp in self.routes:
            if needle in url:
                return resp(url) if callable(resp) else resp
        return Response(404, {}, b"")


def ok(obj):
    return Response(200, {}, json.dumps(obj).encode())


def redirect(to, status=302):
    return Response(status, {"location": to}, b"")


class FakeClock:
    def __init__(self):
        self.now = 0.0
        self.slept = []

    def time(self):
        return self.now

    def sleep(self, s):
        self.slept.append(s)
        self.now += s


class TestHostAllowlist(unittest.TestCase):
    def test_exact_and_subdomain_are_allowed(self):
        for h in ("musicbrainz.org", "coverartarchive.org", "archive.org", "dn710704.ca.archive.org",
                  "itunes.apple.com", "is1-ssl.mzstatic.com", "api.deezer.com", "cdn-images.dzcdn.net"):
            self.assertTrue(sources.host_allowed(h), h)

    def test_look_alikes_and_others_are_refused(self):
        for h in ("evilarchive.org", "archive.org.evil.com", "musicbrainz.org.evil.net", "apple.com",
                  "www.deezer.com", "example.com", "localhost", "", "127.0.0.1", "xdzcdn.net"):
            self.assertFalse(sources.host_allowed(h), h)


class TestGuardedGet(unittest.TestCase):
    def get(self, url, transport, **kw):
        clock = FakeClock()
        limiter = sources.RateLimiter(clock=clock.time, sleep=clock.sleep)
        return sources.guarded_get(url, transport=transport, limiter=limiter, **kw), clock

    def test_https_only_and_refused_before_any_request(self):
        rec = Recorder()
        for url in ("http://musicbrainz.org/x", "ftp://musicbrainz.org/x", "https://example.com/x",
                    "https://evilarchive.org/x", "file:///etc/passwd"):
            with self.subTest(url=url):
                with self.assertRaises(sources.Blocked):
                    self.get(url, rec)
        self.assertEqual(rec.requests, [], "a refused URL must never be sent")

    def test_redirect_hops_are_followed_and_each_is_checked(self):
        rec = Recorder([
            ("coverartarchive.org/release/x/front-500", redirect("https://archive.org/download/a.jpg", 307)),
            ("archive.org/download/a.jpg", redirect("https://dn1.ca.archive.org/0/a.jpg")),
            ("dn1.ca.archive.org/0/a.jpg", Response(200, {}, b"IMG")),
        ])
        resp, _ = self.get("https://coverartarchive.org/release/x/front-500", rec)
        self.assertEqual(resp.body, b"IMG")
        self.assertEqual(len(rec.requests), 3)

    def test_a_redirect_off_the_allowlist_is_refused_without_following(self):
        rec = Recorder([("musicbrainz.org", redirect("https://evil.example.com/steal"))])
        with self.assertRaises(sources.Blocked):
            self.get("https://musicbrainz.org/ws/2/x", rec)
        self.assertEqual(len(rec.requests), 1)

    def test_a_redirect_to_http_is_refused(self):
        rec = Recorder([("musicbrainz.org", redirect("http://archive.org/x"))])
        with self.assertRaises(sources.Blocked):
            self.get("https://musicbrainz.org/x", rec)

    def test_hops_are_bounded(self):
        rec = Recorder([("archive.org", lambda u: redirect("https://archive.org/again"))])
        with self.assertRaises(sources.Blocked):
            self.get("https://archive.org/start", rec)
        self.assertLessEqual(len(rec.requests), sources.MAX_HOPS + 1)

    def test_relative_redirect_is_resolved_and_checked(self):
        rec = Recorder([("archive.org/a", redirect("/b")), ("archive.org/b", Response(200, {}, b"ok"))])
        resp, _ = self.get("https://archive.org/a", rec)
        self.assertEqual(resp.body, b"ok")

    def test_status_mapping(self):
        with self.assertRaises(sources.NotFound):
            self.get("https://musicbrainz.org/x", Recorder([("musicbrainz", Response(404, {}, b""))]))
        for code in (429, 500, 503):
            with self.assertRaises(sources.FetchError):
                self.get("https://musicbrainz.org/x", Recorder([("musicbrainz", Response(code, {}, b""))]))

    def test_transport_errors_are_fetch_errors(self):
        def boom(*a, **k):
            raise TimeoutError("slow")
        with self.assertRaises(sources.FetchError):
            self.get("https://musicbrainz.org/x", boom)

    def test_body_over_the_cap_is_rejected(self):
        rec = Recorder([("musicbrainz", Response(200, {}, b"x" * 100))])
        with self.assertRaises(sources.FetchError):
            self.get("https://musicbrainz.org/x", rec, max_bytes=10)

    def test_user_agent_names_synodl(self):
        rec = Recorder([("musicbrainz", Response(200, {}, b"{}"))])
        self.get("https://musicbrainz.org/x", rec)
        ua = rec.requests[0][1]["User-Agent"]
        self.assertIn("SynoDL", ua)
        self.assertNotIn("Authorization", rec.requests[0][1])
        self.assertNotIn("Cookie", rec.requests[0][1])

    def test_musicbrainz_is_spaced_one_second_apart_others_are_not(self):
        rec = Recorder([("", Response(200, {}, b"{}"))])
        clock = FakeClock()
        limiter = sources.RateLimiter(clock=clock.time, sleep=clock.sleep)
        for _ in range(3):
            sources.guarded_get("https://musicbrainz.org/x", transport=rec, limiter=limiter)
        self.assertEqual(len(clock.slept), 2)
        self.assertTrue(all(s >= 0.99 for s in clock.slept))
        clock.slept.clear()
        for _ in range(3):
            sources.guarded_get("https://itunes.apple.com/x", transport=rec, limiter=limiter)
        self.assertEqual(clock.slept, [])

    def test_a_log_line_never_carries_the_query_or_environment(self):
        import io
        import logging
        stream = io.StringIO()
        h = logging.StreamHandler(stream)
        log = logging.getLogger("music_repair")
        log.addHandler(h)
        log.setLevel(logging.DEBUG)
        os.environ["SYNODL_TEST_SECRET"] = "hunter2"
        try:
            rec = Recorder([("musicbrainz", Response(200, {}, b"{}"))])
            self.get("https://musicbrainz.org/ws/2/recording?query=artist:%22Secret%20Artist%22", rec)
        finally:
            log.removeHandler(h)
            del os.environ["SYNODL_TEST_SECRET"]
        text = stream.getvalue()
        self.assertNotIn("Secret", text)
        self.assertNotIn("hunter2", text)
        self.assertNotIn("query=", text)


if __name__ == "__main__":
    unittest.main()
