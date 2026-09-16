package people

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The outbound surface this feature adds, and the reason it is not an open
// relay. Independent of the download sources' image allowlist on purpose: an
// operator editing a source must not be able to change what this may fetch.
func TestHostAllowlist(t *testing.T) {
	for _, ok := range []string{"www.imdb.com", "m.media-amazon.com", "WWW.IMDB.COM"} {
		if !HostAllowed(ok) {
			t.Fatalf("%q should be allowed", ok)
		}
	}
	for _, bad := range []string{
		"imdb.com",                        // the allowlist is exact, not a family
		"m.media-amazon.com.evil.example", // a lookalike suffix
		"evil.example",
		"cdn.30nama.com", // a SOURCE's image host — a different list entirely
		"zarfilm.com",
		"",
	} {
		if HostAllowed(bad) {
			t.Fatalf("%q must not be allowed", bad)
		}
	}
}

// Being NAMED by an allowlisted page does not make a URL allowlisted (FR-018a).
func TestSanitizeImageURLRejectsForeignHosts(t *testing.T) {
	good := sanitizeImageURL("https://m.media-amazon.com/images/M/MV5B123._V1_FMjpg_UX1000_.jpg")
	if good == "" {
		t.Fatal("a legitimate photograph was rejected")
	}
	// And it asks for a tile-sized rendition rather than the full-size image.
	if !strings.Contains(good, "._V1_UX300_.jpg") {
		t.Fatalf("rendition not reduced: %q", good)
	}
	for _, bad := range []string{
		"https://evil.example/x.jpg",
		"http://m.media-amazon.com/x.jpg", // https only
		"javascript:alert(1)",
		"//m.media-amazon.com/x.jpg",
		"",
	} {
		if got := sanitizeImageURL(bad); got != "" {
			t.Fatalf("%q survived as %q", bad, got)
		}
	}
}

// A URL that carries no rendition marker is left alone rather than mangled.
func TestSanitizeImageURLLeavesUnmarkedURLs(t *testing.T) {
	in := "https://m.media-amazon.com/images/M/plain.jpg"
	if got := sanitizeImageURL(in); got != in {
		t.Fatalf("got %q, want it unchanged", got)
	}
}

// The page is well over a megabyte and the answer is in the first few kilobytes.
// Reading it all would move a third party's whole page through the server for
// every face (FR-020).
func TestReadHeadIsBounded(t *testing.T) {
	var served int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		head := `<html><head><meta property="og:image" content="https://m.media-amazon.com/images/M/a._V1_.jpg"></head><body>`
		w.Write([]byte(head))
		// A body far larger than the cap. If the reader does not stop, this is
		// what it pulls through.
		chunk := strings.Repeat("x", 64<<10)
		for i := 0; i < 40; i++ {
			if _, err := w.Write([]byte(chunk)); err != nil {
				return
			}
			served += len(chunk)
		}
	}))
	defer srv.Close()

	body, err := readHeadAllowingHost(t, srv.URL+"/name/nm0000621/")
	if err != nil {
		t.Fatalf("readHead: %v", err)
	}
	if len(body) > maxHeadBytes {
		t.Fatalf("read %d bytes, cap is %d", len(body), maxHeadBytes)
	}
	// It also stops AT the head, so the regexp never runs over the page body.
	if strings.Contains(strings.ToLower(string(body)), "<body") {
		t.Fatal("read past </head>")
	}
}

// A person IMDb has no photograph of is a real answer — not an error, and not a
// reason to keep asking.
func TestLookupPhotoNoImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><title>Someone - IMDb</title></head><body></body></html>`))
	}))
	defer srv.Close()

	got, err := lookupAllowingHost(t, srv.URL, "nm0000621")
	if err != nil {
		t.Fatalf("a page with no photograph is not an error: %v", err)
	}
	if got != "" {
		t.Fatalf("invented a photograph: %q", got)
	}
}

// A redirect must not carry the fetch off the allowlist (FR-018b). Following it
// and checking afterwards would be too late: the request would already have been
// made to the host we do not trust.
func TestRedirectOffAllowlistIsRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://evil.example/gotcha", http.StatusFound)
	}))
	defer srv.Close()

	_, err := lookupAllowingHost(t, srv.URL, "nm0000621")
	if err == nil {
		t.Fatal("want an error, got a successful lookup")
	}
	// Refused BEFORE the request is made, not judged after a response came back
	// from a host we do not trust.
	if !errors.Is(err, errOffAllowlist) {
		t.Fatalf("want the allowlist to be the reason, got %v", err)
	}
}

// The lookup only ever asks about a person id, and only in the shape IMDb uses.
func TestLookupPhotoRejectsBadIDs(t *testing.T) {
	for _, bad := range []string{"tt2948372", "nm1", "../etc", "nm0000621/extra"} {
		if _, err := LookupPhoto(context.Background(), bad); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
}
