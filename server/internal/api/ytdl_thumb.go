package api

import (
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Artwork for YouTube downloads (spec 1034).
//
// This is a SECOND image proxy, and the duplication is deliberate. The catalog
// poster proxy's allowlist is assembled from the download-source drivers plus
// the operator's configured mirrors — and YouTube is neither a download source
// nor a mirror of one. Sharing one list would mean an operator editing a source
// could change what artwork this feature may fetch, and a change here could
// widen where source posters may come from. Two concerns, two lists.
//
// It exists at all so a viewer's browser never contacts Google directly, which
// is the same reason the poster proxy exists.

// ytdlArtworkHost matches the hosts YouTube serves thumbnails from: i.ytimg.com
// and its numbered siblings, plus s.ytimg.com. Anchored at both ends, so
// "i.ytimg.com.attacker.example" does not match.
var ytdlArtworkHost = regexp.MustCompile(`^(i[0-9]?|s)\.ytimg\.com$`)

// ytdlArtworkHostAllowed is this feature's own host rule, and knows nothing
// about the download sources.
func ytdlArtworkHostAllowed(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	return h == "img.youtube.com" || ytdlArtworkHost.MatchString(h)
}

var ytdlThumbClient = &http.Client{Timeout: 15 * time.Second}

// handleYtdlThumb proxies one artwork image.
//
// Deliberately NOT session-gated, and spec 0013 revisited that on purpose
// (FR-009d). Making downloads private raised the question of whether their
// artwork should be private too. It should not, for two reasons that matter
// more than the instinct:
//
//   - It reveals nothing. The caller supplies the URL, so this cannot be used
//     to discover which downloads exist or who made them. The content behind it
//     is public, content-addressed artwork that anyone can fetch from the source
//     directly — the proxy exists so the VIEWER's browser does not have to, not
//     to control access to it.
//   - It cannot carry a session anyway. These load through <img src>, which
//     sends no custom header. Gating it would mean either putting the session
//     token in a URL — where it lands in logs and history — or fetching every
//     image through script and losing HTTP caching, both worse trades than the
//     nothing being protected. The catalog poster proxy is unauthenticated for
//     the same reason.
//
// What actually protects this endpoint is the host allowlist below: it is the
// reason this is not an open relay.
func handleYtdlThumb(d Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.URL.Query().Get("u")
		u, err := url.Parse(raw)
		// https only: YouTube serves these over TLS, so accepting http would
		// only ever downgrade a request that did not need downgrading.
		if err != nil || u.Scheme != "https" || !ytdlArtworkHostAllowed(u.Hostname()) {
			http.Error(w, "bad image", http.StatusBadRequest)
			return
		}
		// Thumbnails are content-addressed by video id and do not change.
		w.Header().Set("Cache-Control", "public, max-age=604800, immutable")

		req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, raw, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 SynoDL")
		resp, err := ytdlThumbClient.Do(req)
		if err != nil {
			http.Error(w, "image unreachable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			http.Error(w, "image unavailable", http.StatusBadGateway)
			return
		}
		// A thumbnail is tens of kilobytes; the cap bounds a misbehaving host
		// rather than a legitimate image.
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			http.Error(w, "image read error", http.StatusBadGateway)
			return
		}
		ct := resp.Header.Get("Content-Type")
		if ct == "" || !strings.HasPrefix(ct, "image/") {
			ct = "image/jpeg"
		}
		w.Header().Set("Content-Type", ct)
		_, _ = w.Write(body)
	})
}
