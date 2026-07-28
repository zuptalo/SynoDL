package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
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
