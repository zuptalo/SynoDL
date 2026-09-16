// Package people resolves and remembers what somebody looks like (spec 0014).
//
// Both download sources name the people behind a title, and both are uneven
// about photographs: one serves a grey stand-in for every crew member and about
// half of a typical cast, the other publishes none on the page that names them.
// So a face has to be assembled — use what the source has, and fall back to
// IMDb, which is where the sources' own re-hosted stills came from anyway.
//
// Two things are true of everything in here:
//
//   - It is best-effort. Reading a public page for an image location is not an
//     API and carries no promise. Every failure is the same answer — "no
//     photograph" — and the tile shows initials. Nothing in Discover breaks.
//   - It is bounded. A fixed two-host allowlist belonging to THIS feature, a cap
//     on bytes read, a cap on concurrent lookups, and a cache so the same person
//     is never resolved twice.
package people

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// The outbound surface this feature adds, and all of it.
//
// It is deliberately NOT merged into the catalog poster proxy's allowlist, for
// the reason spec 1034 gives for the YouTube artwork proxy and which applies
// here word for word: that list is assembled from the download-source drivers
// plus the operator's configured mirrors, and IMDb is neither. Sharing one list
// would mean an operator editing a source could change what this feature may
// fetch, and a change here could widen where source posters may come from. Two
// concerns, two lists.
const (
	imdbHost  = "www.imdb.com"
	imageHost = "m.media-amazon.com"
)

// HostAllowed is this feature's own host rule. It knows nothing about the
// download sources, and nothing outside it is reachable.
//
// Anchored equality rather than a suffix match: "m.media-amazon.com.evil.example"
// is a different host and must not pass.
func HostAllowed(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == imdbHost || h == imageHost {
		return true
	}
	// Set only by this package's own tests (see export_test.go), which is a file
	// no build of the binary contains. It is here rather than as a parameter so
	// the production call sites cannot pass one.
	if testHostOverride != "" && h == testHostOverride {
		return true
	}
	// Dev/e2e only, and only in a build carrying the `sourcemock` tag. See
	// mockbase_prod.go: a release binary contains no such branch, so there is no
	// environment variable that could point this anywhere but IMDb.
	if b := mockIMDbBase(); b != "" {
		if u, err := url.Parse(b); err == nil && u.Hostname() != "" {
			return h == strings.ToLower(u.Hostname())
		}
	}
	return false
}

// imdbBase is where a person's page is read from.
func imdbBase() string {
	if testBaseOverride != "" {
		return testBaseOverride
	}
	if b := mockIMDbBase(); b != "" {
		return b
	}
	return "https://" + imdbHost
}

// Set only from export_test.go. Never written by anything a build ships.
var (
	testHostOverride string
	testBaseOverride string
)

// How much of a person's page is read before giving up on finding the answer.
//
// The page is well over a megabyte, and og:image is in the <head> — in practice
// within the first few kilobytes. Reading the whole thing to find something that
// is always at the top would be a megabyte of a third party's HTML through the
// server for every face (FR-020).
const maxHeadBytes = 256 << 10

// How large a photograph may be. A tile-sized rendition is tens of kilobytes;
// this bounds a misbehaving host, not a legitimate image.
const maxImageBytes = 8 << 20

var (
	lookupClient = &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: outboundTLS()},
		// A redirect must not carry the fetch off the allowlist (FR-018b).
		// Refusing is right rather than following-and-checking: by the time a
		// response came back from an unlisted host, the request would already
		// have been made to it.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !HostAllowed(req.URL.Hostname()) {
				return errOffAllowlist
			}
			if len(via) >= 5 {
				return errors.New("people: too many redirects")
			}
			return nil
		},
	}
	imageClient = &http.Client{
		Timeout:       15 * time.Second,
		Transport:     &http.Transport{TLSClientConfig: outboundTLS()},
		CheckRedirect: lookupClient.CheckRedirect,
	}
)

var errOffAllowlist = errors.New("people: redirect off the allowlist")

// reOGImage finds the photograph's location in a person page's head. Matched on
// the tag rather than parsed as a document, because the input is a deliberately
// truncated prefix of an HTML page and a real parser would object to that.
var reOGImage = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:image["'][^>]+content=["']([^"']+)["']`)

// reOGImageRev is the same tag with its attributes the other way round, which is
// how several generators emit it.
var reOGImageRev = regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+property=["']og:image["']`)

// reAmazonRendition matches the image-transform marker in an Amazon media URL.
// The segment after _V1_ selects a rendition, so replacing it asks for a smaller
// one (FR-022).
var reAmazonRendition = regexp.MustCompile(`(?i)(\._V1_)[^/]*(\.(?:jpg|jpeg|png|webp))$`)

// tileRendition is the size a tile actually needs. Asking for the full-size
// image would move roughly ten times the bytes to draw a 64-pixel circle.
const tileRendition = "._V1_UX300_"

// LookupPhoto finds a person's photograph on IMDb, or reports that there is
// none. The error is reserved for "we could not ask" — a caller treats that the
// same as "none", but the distinction lets the answer be cached for a shorter
// time.
func LookupPhoto(ctx context.Context, imdbID string) (string, error) {
	if !rePersonID.MatchString(imdbID) {
		return "", errors.New("people: not a person id")
	}
	raw, err := readHead(ctx, imdbBase()+"/name/"+imdbID+"/")
	if err != nil {
		return "", err
	}
	m := reOGImage.FindSubmatch(raw)
	if m == nil {
		m = reOGImageRev.FindSubmatch(raw)
	}
	if m == nil {
		return "", nil // IMDb has no photograph of them. A real answer.
	}
	return sanitizeImageURL(string(m[1])), nil
}

// rePersonID mirrors source.PersonIDRe. Duplicated rather than imported so this
// package does not depend on the source package for one regexp — and so the
// bound on what may reach an outbound URL is visible right here.
var rePersonID = regexp.MustCompile(`^nm\d{6,9}$`)

// sanitizeImageURL accepts a photograph location only if it is on the allowlist,
// and asks for a tile-sized rendition of it.
//
// This is the check the security checklist opened as a gap: being NAMED by an
// allowlisted page does not make a URL allowlisted (FR-018a). Without it, a page
// could point the server's next request at any host it liked.
func sanitizeImageURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || !HostAllowed(u.Hostname()) {
		return ""
	}
	u.Path = reAmazonRendition.ReplaceAllString(u.Path, tileRendition+"$2")
	return u.String()
}

// readHead reads the beginning of a page — enough for its <head>, and no more.
func readHead(ctx context.Context, raw string) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil || !HostAllowed(u.Hostname()) {
		return nil, errors.New("people: host not allowed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	// A plain browser-ish UA and nothing else. No cookie, no referrer, no
	// session: the only thing this request says about anybody is which public
	// figure is being looked up.
	req.Header.Set("User-Agent", "Mozilla/5.0 SynoDL")
	req.Header.Set("Accept-Language", "en")
	resp, err := lookupClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("people: lookup refused")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHeadBytes))
	if err != nil {
		return nil, err
	}
	// Stop at the head: everything after it is a megabyte of page we have no use
	// for, and cutting there keeps the regexp off it.
	if i := strings.Index(strings.ToLower(string(body)), "</head>"); i >= 0 {
		body = body[:i]
	}
	return body, nil
}

// FetchImage retrieves one photograph's bytes.
func FetchImage(ctx context.Context, raw string) ([]byte, string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !HostAllowed(u.Hostname()) {
		return nil, "", errors.New("people: host not allowed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 SynoDL")
	resp, err := imageClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", errors.New("people: image unavailable")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes))
	if err != nil {
		return nil, "", err
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" || !strings.HasPrefix(ct, "image/") {
		ct = "image/jpeg"
	}
	return body, ct, nil
}
