// Package ytdl turns a user-supplied link and a mode into the exact worker
// command that fetches it, and maps the resulting Job's lifecycle back to the
// four states the app shows.
//
// The recipe here is not invented: every flag was verified end-to-end against
// real content before the spec was written (see the spec's research.md). Treat
// changes to it as behaviour changes, not tidying.
package ytdl

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Mode is which library a request writes to, and therefore how it is fetched.
type Mode string

const (
	ModeMusic      Mode = "music"
	ModeMusicVideo Mode = "music-video"
)

// Valid reports whether m is a mode the server accepts.
func (m Mode) Valid() bool { return m == ModeMusic || m == ModeMusicVideo }

// Scope is derived from the URL, never supplied by the caller. Letting a client
// choose it would allow aiming a channel run at a playlist URL and vice versa.
type Scope string

const (
	ScopeSingle   Scope = "single"
	ScopePlaylist Scope = "playlist"
	ScopeChannel  Scope = "channel"
)

// Target is a link that has passed the host allowlist, normalised to the form
// the worker should actually be given.
type Target struct {
	URL   string
	Scope Scope
}

// ErrBlocked is returned for anything outside the allowlist. Callers map it to
// a 400 with a plain-language message.
var ErrBlocked = errors.New("link is not a supported YouTube address")

// canonicalHost maps every accepted host to the one we actually request.
// Anything absent from this map is refused — the check is an exact match on the
// parsed hostname, never a substring test, because "youtube.com.evil.tld" and
// "https://youtube.com@evil.tld/" both contain the string "youtube.com".
var canonicalHost = map[string]string{
	"youtube.com":       "www.youtube.com",
	"www.youtube.com":   "www.youtube.com",
	"m.youtube.com":     "www.youtube.com",
	"music.youtube.com": "music.youtube.com", // kept: its art tracks carry real album metadata
	"youtu.be":          "youtu.be",
}

// Classify validates a link against the host allowlist and works out what kind
// of thing it addresses, normalising it on the way through.
func Classify(raw string) (Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Target{}, ErrBlocked
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Target{}, ErrBlocked
	}
	// Scheme and userinfo are checked before the host: a URL carrying userinfo
	// is a classic way to make a hostile host look like a friendly one in a
	// glance-read, and we have no use for credentials in a public link.
	if u.Scheme != "http" && u.Scheme != "https" {
		return Target{}, ErrBlocked
	}
	if u.User != nil {
		return Target{}, ErrBlocked
	}
	host, ok := canonicalHost[strings.ToLower(u.Hostname())]
	if !ok {
		return Target{}, ErrBlocked
	}

	path := strings.TrimRight(u.Path, "/")
	q := u.Query()

	out := &url.URL{Scheme: "https", Host: host}

	switch {
	// youtu.be/<id> — the whole path is the video id.
	case host == "youtu.be":
		if path == "" {
			return Target{}, ErrBlocked
		}
		out.Path = path
		return Target{URL: out.String(), Scope: ScopeSingle}, nil

	// A watch link is one video even when it also names a playlist: the caller
	// asked for a video, and the args carry --no-playlist to hold us to it.
	case path == "/watch":
		v := q.Get("v")
		if v == "" {
			return Target{}, ErrBlocked
		}
		out.Path = "/watch"
		out.RawQuery = url.Values{"v": {v}}.Encode()
		return Target{URL: out.String(), Scope: ScopeSingle}, nil

	// An explicitly submitted Shorts link is an explicit choice, so it is
	// accepted as a single video. The duration filter that excludes Shorts
	// applies only to bulk scopes, where nobody chose them individually.
	case strings.HasPrefix(path, "/shorts/"):
		out.Path = path
		return Target{URL: out.String(), Scope: ScopeSingle}, nil

	case path == "/playlist":
		list := q.Get("list")
		if list == "" {
			return Target{}, ErrBlocked
		}
		out.Path = "/playlist"
		out.RawQuery = url.Values{"list": {list}}.Encode()
		return Target{URL: out.String(), Scope: ScopePlaylist}, nil

	// Every channel form is normalised to its videos tab. That is what excludes
	// Shorts WITHOUT enumerating them: a bare channel URL expands to the Videos
	// and Shorts tabs, and the Shorts tab can be far larger than the Videos one.
	case strings.HasPrefix(path, "/@"),
		strings.HasPrefix(path, "/channel/"),
		strings.HasPrefix(path, "/c/"),
		strings.HasPrefix(path, "/user/"):
		out.Path = channelVideosPath(path)
		return Target{URL: out.String(), Scope: ScopeChannel}, nil
	}
	return Target{}, ErrBlocked
}

// channelVideosPath trims any tab already on the path and appends /videos.
func channelVideosPath(path string) string {
	segs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	var base []string
	switch {
	case strings.HasPrefix(segs[0], "@"):
		base = segs[:1] // /@handle
	default:
		if len(segs) < 2 {
			return "/" + segs[0] + "/videos"
		}
		base = segs[:2] // /channel/UC…, /c/Name, /user/Name
	}
	return "/" + strings.Join(base, "/") + "/videos"
}

// Describe renders a target for a log line or an error message. It deliberately
// never includes anything else about the request.
func (t Target) Describe() string { return fmt.Sprintf("%s (%s)", t.URL, t.Scope) }
