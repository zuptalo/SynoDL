package ytdl

import "testing"

func TestClassify_AcceptsAndScopes(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantURL string
		want    Scope
	}{
		{"short link", "https://youtu.be/zSGhyrF7YVo", "https://youtu.be/zSGhyrF7YVo", ScopeSingle},
		{"short link with share token", "https://youtu.be/zSGhyrF7YVo?si=abc123", "https://youtu.be/zSGhyrF7YVo", ScopeSingle},
		{"watch link", "https://www.youtube.com/watch?v=zSGhyrF7YVo", "https://www.youtube.com/watch?v=zSGhyrF7YVo", ScopeSingle},
		// A watch URL that also carries a list= is still ONE video: the caller
		// asked for a video, and the args carry --no-playlist to enforce it.
		{"watch inside a list", "https://www.youtube.com/watch?v=abc&list=PL123", "https://www.youtube.com/watch?v=abc", ScopeSingle},
		{"shorts link is a single video", "https://www.youtube.com/shorts/abc123", "https://www.youtube.com/shorts/abc123", ScopeSingle},
		{"playlist", "https://youtube.com/playlist?list=PLCgJuDokOARoOQCL", "https://www.youtube.com/playlist?list=PLCgJuDokOARoOQCL", ScopePlaylist},
		{"playlist with share token", "https://youtube.com/playlist?list=PL1&si=xyz", "https://www.youtube.com/playlist?list=PL1", ScopePlaylist},
		// Every channel form normalises to the videos tab: that is what excludes
		// Shorts without enumerating them (research.md §6).
		{"handle", "https://youtube.com/@bennyriversmusic", "https://www.youtube.com/@bennyriversmusic/videos", ScopeChannel},
		{"handle with share token", "https://youtube.com/@benny?si=q", "https://www.youtube.com/@benny/videos", ScopeChannel},
		{"handle already on videos tab", "https://www.youtube.com/@benny/videos", "https://www.youtube.com/@benny/videos", ScopeChannel},
		{"handle on another tab is redirected to videos", "https://www.youtube.com/@benny/playlists", "https://www.youtube.com/@benny/videos", ScopeChannel},
		{"channel id", "https://www.youtube.com/channel/UCKTGZKkiQhJqC1vIX0TbiCw", "https://www.youtube.com/channel/UCKTGZKkiQhJqC1vIX0TbiCw/videos", ScopeChannel},
		{"legacy /c/ channel", "https://www.youtube.com/c/SomeName", "https://www.youtube.com/c/SomeName/videos", ScopeChannel},
		{"legacy /user/ channel", "https://www.youtube.com/user/SomeName", "https://www.youtube.com/user/SomeName/videos", ScopeChannel},
		{"music subdomain", "https://music.youtube.com/watch?v=abc", "https://music.youtube.com/watch?v=abc", ScopeSingle},
		{"mobile host", "https://m.youtube.com/watch?v=abc", "https://www.youtube.com/watch?v=abc", ScopeSingle},
		{"uppercase host", "https://WWW.YouTube.COM/watch?v=abc", "https://www.youtube.com/watch?v=abc", ScopeSingle},
		{"whitespace is trimmed", "  https://youtu.be/abc  ", "https://youtu.be/abc", ScopeSingle},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Classify(tc.in)
			if err != nil {
				t.Fatalf("Classify(%q) unexpected error: %v", tc.in, err)
			}
			if got.Scope != tc.want {
				t.Errorf("scope = %q, want %q", got.Scope, tc.want)
			}
			if got.URL != tc.wantURL {
				t.Errorf("url = %q, want %q", got.URL, tc.wantURL)
			}
		})
	}
}

// The host allowlist is a constitution v2.1.0 rule, not a nicety: it is the
// no-open-proxy instinct applied to the second external surface. Each case here
// is a way a look-alike gets past a naive strings.Contains check.
func TestClassify_RejectsEverythingElse(t *testing.T) {
	cases := []struct{ name, in string }{
		{"empty", ""},
		{"not a url", "just some text"},
		{"suffix look-alike", "https://youtube.com.evil.tld/watch?v=abc"},
		{"prefix look-alike", "https://evil-youtube.com/watch?v=abc"},
		{"substring in path", "https://evil.tld/https://youtube.com/watch?v=abc"},
		{"substring in query", "https://evil.tld/?u=https://youtube.com/watch?v=abc"},
		{"userinfo trick", "https://www.youtube.com@evil.tld/watch?v=abc"},
		{"userinfo with password", "https://user:pass@evil.tld/youtube.com"},
		{"file scheme", "file:///etc/passwd"},
		{"javascript scheme", "javascript:alert(1)"},
		{"data scheme", "data:text/html,hi"},
		{"no scheme", "youtube.com/watch?v=abc"},
		{"scheme-relative", "//youtube.com/watch?v=abc"},
		{"unrelated host", "https://vimeo.com/12345"},
		{"subdomain not allowed", "https://evil.youtube.com.attacker.net/x"},
		{"trailing dot host", "https://youtube.com./watch?v=abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := Classify(tc.in); err == nil {
				t.Fatalf("Classify(%q) = %+v, want error", tc.in, got)
			}
		})
	}
}
