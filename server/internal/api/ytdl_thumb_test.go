package api

import (
	"net/http"
	"net/url"
	"testing"
)

// The point of this proxy having its own rule is that it CANNOT be widened by
// anything happening to the download sources. This table is that rule.
func TestYtdlArtworkHostAllowed(t *testing.T) {
	allowed := []string{
		"i.ytimg.com",
		"i1.ytimg.com",
		"i9.ytimg.com",
		"s.ytimg.com",
		"img.youtube.com",
		"I.YTIMG.COM", // case is not a security boundary
	}
	for _, h := range allowed {
		if !ytdlArtworkHostAllowed(h) {
			t.Errorf("%q should be allowed", h)
		}
	}

	refused := []string{
		"",
		"ytimg.com",
		"youtube.com",
		"www.youtube.com",
		// Look-alikes: each one gets past a naive suffix or substring check.
		"i.ytimg.com.attacker.example",
		"evil-i.ytimg.com",
		"i.ytimg.com.evil",
		"notimg.youtube.com",
		"img.youtube.com.evil.example",
		// A host the catalog poster proxy might well accept. The two lists are
		// separate on purpose, so this must be refused HERE regardless.
		"zarfilm.example",
		"cdn.some-download-source.example",
		// And a plain internal address, which is what an open proxy would let
		// a caller reach.
		"127.0.0.1",
		"localhost",
		"169.254.169.254",
	}
	for _, h := range refused {
		if ytdlArtworkHostAllowed(h) {
			t.Errorf("%q should be REFUSED", h)
		}
	}
}

// The endpoint refuses before it fetches anything, so a bad host never becomes
// an outbound request.
func TestYtdlThumb_RefusesBadTargets(t *testing.T) {
	h, _ := newYtdlRouter(t, &fakeJobs{}, ytdlCfg())
	for _, raw := range []string{
		"",
		"https://evil.example/x.jpg",
		"http://i.ytimg.com/x.jpg", // https only
		"https://i.ytimg.com.evil.example/x.jpg",
		"https://127.0.0.1/x.jpg",
		"not-a-url",
	} {
		rec := do(t, h, "GET", "/v1/ytdl/thumb?u="+url.QueryEscape(raw), "", nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("u=%q gave %d, want 400", raw, rec.Code)
		}
	}
}
