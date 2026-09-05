package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"synodl/server/internal/store"
	"testing"
	"time"
)

func TestSourceImageProxyAndCache(t *testing.T) {
	h, _ := newStatefulRouter(t)

	// Disallowed host is refused (no open proxy).
	rec := do(t, h, "GET", "/v1/source/image?u="+url.QueryEscape("https://evil.example.com/x.jpg"), "", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("disallowed host = %d, want 400", rec.Code)
	}

	// An allowed host (127.0.0.1, via fakeSrc.Hosts) is fetched, streamed, cached.
	var hits int
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("JPEGBYTES"))
	}))
	defer origin.Close()

	src := origin.URL + "/cover/1.jpg"
	rec = do(t, h, "GET", "/v1/source/image?u="+url.QueryEscape(src), "", nil)
	if rec.Code != 200 || rec.Body.String() != "JPEGBYTES" || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("proxy = %d ct=%q body=%q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	if rec.Header().Get("X-Cache") != "MISS" {
		t.Fatalf("first fetch X-Cache = %q, want MISS", rec.Header().Get("X-Cache"))
	}

	// Second request is served from cache — origin isn't hit again.
	rec = do(t, h, "GET", "/v1/source/image?u="+url.QueryEscape(src), "", nil)
	if rec.Header().Get("X-Cache") != "HIT" || rec.Body.String() != "JPEGBYTES" {
		t.Fatalf("second fetch X-Cache = %q body=%q", rec.Header().Get("X-Cache"), rec.Body.String())
	}
	if hits != 1 {
		t.Fatalf("origin hit %d times, want 1 (cache should serve the repeat)", hits)
	}
}

// Spec 1028: when a source's main domain is blocked the operator sets a mirror,
// and the driver then fetches pages from it — but those pages name their images
// on the mirror's host. Nothing taught the image proxy about that, so every
// poster from that source became a placeholder the moment the main domain went
// down: each URL was rejected outright, in microseconds, before any fetch.
func TestImageProxyAllowsAConfiguredMirror(t *testing.T) {
	c, _ := store.NewCipher("kdf-input-for-tests")
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"), c)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateProvider(store.SourceProvider{
		Kind: "faketest", DisplayName: "Mirrored", Enabled: true,
		MoviesParent: "movie", TVParent: "tv-show", State: store.SourceActive,
		AltBase: "https://mirror.example",
	}, time.Now().Unix()); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	d := Deps{Store: st}
	resetMirrorHostCache()

	if !d.imageHostAllowed("mirror.example") {
		t.Error("the operator's own mirror must be allowed to serve posters")
	}
	if !d.imageHostAllowed("cdn.mirror.example") {
		t.Error("a subdomain of the mirror must be allowed, as it is for the main host")
	}
	// Still bounded: this is an unauthenticated endpoint and must never become an
	// open proxy just because some source has a mirror configured.
	if d.imageHostAllowed("example.com") {
		t.Error("an unrelated host must stay rejected")
	}
	if d.imageHostAllowed("notmirror.example.evil") {
		t.Error("a lookalike host must stay rejected")
	}
}

// With no mirror configured the proxy is exactly as narrow as it was.
func TestImageProxyWithoutAMirrorIsUnchanged(t *testing.T) {
	c, _ := store.NewCipher("kdf-input-for-tests")
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"), c)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	d := Deps{Store: st}
	resetMirrorHostCache()

	for _, h := range []string{"mirror.example", "example.com", ""} {
		if d.imageHostAllowed(h) {
			t.Errorf("%q must not be allowed", h)
		}
	}
}
