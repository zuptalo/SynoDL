"""Sources: the only code that talks to the network, behind its own allowlist.

Everything outbound goes through `guarded_get`, and nothing else in this package
opens a socket. That is what makes "the repair only ever contacts four public
services" (FR-015, SC-009) a property of one function rather than a habit.

The allowlist is this tool's OWN. It is deliberately not the catalog image
proxy's list: that one is assembled from the download-source drivers and the
operator's mirrors, and none of them is a music-metadata service (the same
separation internal/people keeps for IMDb, spec 0014).

Why the redirect hosts are on it: Cover Art Archive answers with a 307 to
archive.org and then a 302 to a numbered `dnNNNNNN.ca.archive.org` host. Every
hop is re-checked, so allowing the CDN does not allow a service to bounce us
anywhere else.
"""

from __future__ import annotations

import http.client
import logging
import ssl
import time
import urllib.parse
from dataclasses import dataclass, field

from . import VERSION

log = logging.getLogger("music_repair")

# Suffix match on a DOT BOUNDARY: "archive.org" and "x.archive.org" are allowed;
# "evilarchive.org" is not.
ALLOWED_HOSTS = (
    "musicbrainz.org",
    "coverartarchive.org",
    "archive.org",          # Cover Art Archive redirect target
    "itunes.apple.com",
    "mzstatic.com",         # Apple's artwork CDN
    "api.deezer.com",
    "dzcdn.net",            # Deezer's artwork CDN
)

MAX_HOPS = 3
DEFAULT_TIMEOUT_S = 15
DEFAULT_MAX_BYTES = 8 * 1024 * 1024
USER_AGENT = f"SynoDL-music-repair/{VERSION} (+https://github.com/zuptalo/synodl)"

# MusicBrainz asks for at most one request a second and blocks those that ignore
# it. The others are used at low volume and only as fallbacks.
MIN_INTERVAL_S = {"musicbrainz.org": 1.0}


class Blocked(Exception):
    """The request was refused by policy BEFORE anything was sent."""


class NotFound(Exception):
    """The source answered 404: there is nothing there (e.g. no cover art)."""


class FetchError(Exception):
    """The source could not be reached or answered badly. Retry on a later run."""


@dataclass
class Response:
    status: int
    headers: dict = field(default_factory=dict)
    body: bytes = b""


def host_allowed(host: str) -> bool:
    host = (host or "").lower().rstrip(".")
    if not host:
        return False
    return any(host == s or host.endswith("." + s) for s in ALLOWED_HOSTS)


def http_transport(url: str, headers: dict, timeout: float, max_bytes: int) -> Response:
    """One GET, no redirects followed, body capped. The real network."""
    parts = urllib.parse.urlsplit(url)
    conn = http.client.HTTPSConnection(parts.hostname, parts.port or 443, timeout=timeout,
                                       context=ssl.create_default_context())
    try:
        target = parts.path or "/"
        if parts.query:
            target += "?" + parts.query
        conn.request("GET", target, headers=headers)
        resp = conn.getresponse()
        body = resp.read(max_bytes + 1)
        return Response(resp.status, {k.lower(): v for k, v in resp.getheaders()}, body)
    finally:
        conn.close()


class RateLimiter:
    def __init__(self, clock=time.monotonic, sleep=time.sleep, intervals=None):
        self._clock, self._sleep = clock, sleep
        self._intervals = MIN_INTERVAL_S if intervals is None else intervals
        self._last: dict[str, float] = {}

    def wait(self, host: str):
        for suffix, interval in self._intervals.items():
            if host == suffix or host.endswith("." + suffix):
                last = self._last.get(suffix)
                if last is not None:
                    gap = self._clock() - last
                    if gap < interval:
                        self._sleep(interval - gap)
                self._last[suffix] = self._clock()
                return


def guarded_get(url: str, *, transport=http_transport, limiter: RateLimiter | None = None,
                max_hops: int = MAX_HOPS, max_bytes: int = DEFAULT_MAX_BYTES,
                timeout: float = DEFAULT_TIMEOUT_S, accept: str = "application/json") -> Response:
    """GET `url` under the allowlist. Returns the final 200 response.

    Refuses BEFORE sending: a non-HTTPS URL, a host not on the list, and every
    redirect that leaves it. Raises NotFound on 404 and FetchError on anything
    that means "try again later" — a source that is down is reported as not
    looked up, never as "no match" (FR-013).

    Logs the HOST and the status only. Never the path, the query (it holds the
    artist and title being looked up) or any header (FR-020).
    """
    limiter = limiter or RateLimiter()
    headers = {"User-Agent": USER_AGENT, "Accept": accept}
    for hop in range(max_hops + 1):
        parts = urllib.parse.urlsplit(url)
        host = (parts.hostname or "").lower()
        if parts.scheme != "https" or not host_allowed(host):
            log.debug("refused: host=%s", host or "?")
            raise Blocked(f"not allowed: {host or '?'}")
        limiter.wait(host)
        try:
            resp = transport(url, headers, timeout, max_bytes)
        except Exception as exc:  # timeouts, resets, DNS — all "later"
            log.debug("fetch failed: host=%s error=%s", host, type(exc).__name__)
            raise FetchError(f"{host}: {type(exc).__name__}") from exc
        log.debug("get: host=%s status=%s", host, resp.status)
        if resp.status in (301, 302, 303, 307, 308):
            location = resp.headers.get("location")
            if not location:
                raise FetchError(f"{host}: redirect without a location")
            url = urllib.parse.urljoin(url, location)
            continue
        if resp.status == 404:
            raise NotFound(host)
        if resp.status != 200:
            raise FetchError(f"{host}: HTTP {resp.status}")
        if len(resp.body) > max_bytes:
            raise FetchError(f"{host}: response too large")
        return resp
    raise Blocked("too many redirects")


# ---------------------------------------------------------------------------
# Clients. Each answers ONE question — "is there a confident match for this
# artist, title and length?" — and returns a Match or None. A source that cannot
# be reached raises FetchError; the caller turns that into "not looked up".
# ---------------------------------------------------------------------------

import json
import os
import tempfile
from dataclasses import dataclass, field

from . import matching, names
from .matching import Match

MB = "https://musicbrainz.org/ws/2"
CAA = "https://coverartarchive.org"
ITUNES = "https://itunes.apple.com/search"
DEEZER = "https://api.deezer.com"

CACHE_FLUSH_EVERY = 25


def _lucene(s: str) -> str:
    return (s or "").replace("\\", "\\\\").replace('"', '\\"')


@dataclass
class Result:
    status: str                       # matched | no_match | not_looked_up
    match: Match | None = None
    tried: list[str] = field(default_factory=list)
    nearest_delta_s: float | None = None


class Cache:
    """Answers already found, keyed by video id. Public facts only (FR-014).

    Written atomically and every CACHE_FLUSH_EVERY puts, because the first pass
    is hours long and a replaced pod must resume, not restart. An unreadable
    file is ignored, not fatal: losing the cache costs lookups and nothing else.
    """

    def __init__(self, path: str):
        self.path = path
        self._entries: dict = {}
        self._dirty = 0
        try:
            with open(path, encoding="utf-8") as f:
                data = json.load(f)
            if isinstance(data, dict) and isinstance(data.get("entries"), dict):
                self._entries = data["entries"]
        except (OSError, ValueError):
            pass

    def get(self, key: str):
        return self._entries.get(key)

    def put(self, key: str, entry: dict):
        self._entries[key] = entry
        self._dirty += 1
        if self._dirty >= CACHE_FLUSH_EVERY:
            self.flush()

    def flush(self):
        if not self._dirty and os.path.exists(self.path):
            return
        os.makedirs(os.path.dirname(self.path) or ".", exist_ok=True)
        fd, tmp = tempfile.mkstemp(dir=os.path.dirname(self.path) or ".", prefix=".cache-", suffix=".tmp")
        try:
            with os.fdopen(fd, "w", encoding="utf-8") as f:
                json.dump({"version": 1, "entries": self._entries}, f, ensure_ascii=False)
                f.flush()
                os.fsync(f.fileno())
            os.replace(tmp, self.path)
        except BaseException:
            try:
                os.unlink(tmp)
            except OSError:
                pass
            raise
        self._dirty = 0


class Lookup:
    """Resolve a song against the sources in order; the first confident answer wins."""

    ORDER = ("musicbrainz", "itunes", "deezer")

    def __init__(self, transport=http_transport, limiter: RateLimiter | None = None,
                 cache: Cache | None = None):
        self.transport = transport
        self.limiter = limiter or RateLimiter()
        self.cache = cache

    def _json(self, url: str) -> dict:
        try:
            resp = guarded_get(url, transport=self.transport, limiter=self.limiter)
        except NotFound:
            return {}  # the source answered: there is nothing here. Not a failure.
        try:
            return json.loads(resp.body.decode("utf-8"))
        except ValueError as exc:
            raise FetchError("unreadable response") from exc

    def resolve(self, *, key: str, lead: str, title: str, duration_s: float,
                featured: list[str]) -> Result:
        cached = self.cache.get(key) if self.cache else None
        if cached and cached.get("status") in ("matched", "no_match"):
            m = Match.from_dict(cached["match"]) if cached.get("match") else None
            return Result(cached["status"], m, list(cached.get("sources_tried", [])),
                          cached.get("nearest_delta_s"))

        tried, failed, nearest = [], False, None
        for name in self.ORDER:
            tried.append(name)
            try:
                match, near = getattr(self, "_" + name)(lead, title, duration_s)
            except (FetchError, Blocked) as exc:
                log.debug("source failed: %s (%s)", name, type(exc).__name__)
                failed = True
                continue
            if near is not None and (nearest is None or abs(near) < abs(nearest)):
                nearest = near
            if match is not None:
                match.featured = list(featured)
                result = Result("matched", match, tried, nearest)
                self._remember(key, result)
                return result
        if failed:
            # A source that was down says nothing about the song. Not cached, so
            # the next run asks again (FR-013).
            return Result("not_looked_up", None, tried, nearest)
        result = Result("no_match", None, tried, nearest)
        self._remember(key, result)
        return result

    def _remember(self, key: str, r: Result):
        if self.cache:
            self.cache.put(key, {"status": r.status, "match": r.match.to_dict() if r.match else None,
                                 "sources_tried": r.tried, "nearest_delta_s": r.nearest_delta_s,
                                 "looked_up_at": int(time.time())})

    # -- MusicBrainz + Cover Art Archive ------------------------------------

    def _musicbrainz(self, lead, title, duration_s):
        q = f'artist:"{_lucene(lead)}" AND recording:"{_lucene(title)}"'
        data = self._json(f"{MB}/recording?" + urllib.parse.urlencode({"query": q, "limit": 25, "fmt": "json"}))
        near, chosen = None, None
        for r in data.get("recordings", []):
            credit = (r.get("artist-credit") or [{}])[0]
            length = r.get("length")
            length_s = length / 1000.0 if isinstance(length, (int, float)) else None
            cmp = None
            for cand in (credit.get("name"), (credit.get("artist") or {}).get("name")):
                if not cand:
                    continue
                c = matching.compare(lead, title, duration_s, cand, r.get("title", ""), length_s)
                cmp = c if cmp is None or c.confident else cmp
                if c.confident:
                    break
            if cmp is None:
                continue
            if cmp.artist_eq and cmp.title_eq and cmp.delta_s is not None:
                near = cmp.delta_s if near is None or abs(cmp.delta_s) < abs(near) else near
            if cmp.confident and chosen is None:
                chosen = (r, cmp)
        if chosen is None:
            return None, near
        r, cmp = chosen
        rid = r["id"]
        rel_data = self._json(f"{MB}/release?" + urllib.parse.urlencode(
            {"recording": rid, "inc": "release-groups", "status": "official", "limit": 100, "fmt": "json"}))
        release = matching.choose_release(rel_data.get("releases", []))
        album = year = track_no = None
        cover = None
        if release:
            album, year = release.get("title"), matching.year_of(release.get("date"))
            cover = f"{CAA}/release/{release['id']}/front-500"
            detail = self._json(f"{MB}/release/{release['id']}?" + urllib.parse.urlencode(
                {"inc": "recordings", "fmt": "json"}))
            track_no = _track_number(detail, rid)
        if year is None:
            year = matching.year_of(r.get("first-release-date"))
        return Match(source="musicbrainz", artist=lead, title=title, length_s=r["length"] / 1000.0,
                     delta_s=cmp.delta_s, album=album, track_no=track_no, year=year,
                     mbid_recording=rid, mbid_release=release["id"] if release else None,
                     cover_url=cover), near

    # -- iTunes Search --------------------------------------------------------

    def _itunes(self, lead, title, duration_s):
        data = self._json(ITUNES + "?" + urllib.parse.urlencode(
            {"term": f"{lead} {title}", "entity": "song", "limit": 10}))
        near = None
        for r in data.get("results", []):
            ms = r.get("trackTimeMillis")
            length = ms / 1000.0 if isinstance(ms, (int, float)) else None
            c = matching.compare(lead, title, duration_s, r.get("artistName", ""), r.get("trackName", ""), length)
            if c.artist_eq and c.title_eq and c.delta_s is not None:
                near = c.delta_s if near is None or abs(c.delta_s) < abs(near) else near
            if c.confident:
                art = (r.get("artworkUrl100") or "").replace("100x100bb", "600x600bb") or None
                return Match(source="itunes", artist=lead, title=title, length_s=length, delta_s=c.delta_s,
                             album=r.get("collectionName"), track_no=r.get("trackNumber"),
                             year=matching.year_of(r.get("releaseDate")), cover_url=art), near
        return None, near

    # -- Deezer ---------------------------------------------------------------

    def _deezer(self, lead, title, duration_s):
        # The advanced `artist:"x" track:"y"` syntax returned nothing when probed;
        # the plain query works, and the strict comparison below is ours anyway.
        data = self._json(DEEZER + "/search?" + urllib.parse.urlencode({"q": f"{lead} {title}", "limit": 10}))
        near = None
        for r in data.get("data", []):
            dur = r.get("duration")
            c = matching.compare(lead, title, duration_s, (r.get("artist") or {}).get("name", ""),
                                 r.get("title", ""), float(dur) if isinstance(dur, (int, float)) else None)
            if c.artist_eq and c.title_eq and c.delta_s is not None:
                near = c.delta_s if near is None or abs(c.delta_s) < abs(near) else near
            if not c.confident:
                continue
            album = (r.get("album") or {})
            title_album, cover, year, track_no = album.get("title"), album.get("cover_xl"), None, None
            try:  # the search hit has no track number or date; one more call gives both
                t = self._json(f"{DEEZER}/track/{r['id']}")
                alb = t.get("album") or {}
                title_album, cover = alb.get("title", title_album), alb.get("cover_xl", cover)
                year, track_no = matching.year_of(t.get("release_date")), t.get("track_position")
                if alb.get("record_type") == "compile":
                    title_album, cover = None, None
            except (FetchError, Blocked):
                pass
            return Match(source="deezer", artist=lead, title=title, length_s=float(dur), delta_s=c.delta_s,
                         album=title_album, track_no=track_no, year=year, cover_url=cover), near
        return None, near


def _track_number(release_detail: dict, recording_id: str) -> int | None:
    for medium in release_detail.get("media", []):
        for t in medium.get("tracks", []):
            if (t.get("recording") or {}).get("id") == recording_id:
                num = t.get("number")
                if isinstance(num, str) and num.isdigit():
                    return int(num)
                pos = t.get("position")
                return pos if isinstance(pos, int) else None
    return None


_IMAGE_MAGIC = ((b"\xff\xd8\xff", "image/jpeg"), (b"\x89PNG\r\n\x1a\n", "image/png"))


def fetch_cover(url: str, *, transport=http_transport, limiter: RateLimiter | None = None,
                max_bytes: int = DEFAULT_MAX_BYTES) -> tuple[bytes, str]:
    """Download artwork and verify it IS an image, by its bytes (FR-015).

    The server's Content-Type and the URL's extension are both just claims from
    a host we do not control; a response that is not a JPEG or PNG is discarded.
    """
    resp = guarded_get(url, transport=transport, limiter=limiter, max_bytes=max_bytes, accept="image/*")
    for magic, mime in _IMAGE_MAGIC:
        if resp.body.startswith(magic):
            return resp.body, mime
    raise FetchError("not an image")
