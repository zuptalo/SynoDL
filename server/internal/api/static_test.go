package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func newStaticServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>shell</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app-abc123.js"), []byte("js"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(spaHandler(dir))
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, srv *httptest.Server, path string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(body)
}

func TestSpaServesRealFilesWithCacheHeaders(t *testing.T) {
	srv := newStaticServer(t)
	resp, body := get(t, srv, "/assets/app-abc123.js")
	if resp.StatusCode != http.StatusOK || body != "js" {
		t.Fatalf("asset: %d %q", resp.StatusCode, body)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("asset Cache-Control = %q", cc)
	}
}

func TestSpaFallsBackToIndexForRoutes(t *testing.T) {
	srv := newStaticServer(t)
	resp, body := get(t, srv, "/tabs/tasks")
	if resp.StatusCode != http.StatusOK || body != "<html>shell</html>" {
		t.Fatalf("route: %d %q", resp.StatusCode, body)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("shell Cache-Control = %q", cc)
	}
}

func TestSpaMissingAssetIs404NotShell(t *testing.T) {
	// A stale PWA importing a superseded chunk must get an honest 404 —
	// serving the HTML shell as JavaScript kills features silently.
	srv := newStaticServer(t)
	resp, _ := get(t, srv, "/assets/app-oldhash.js")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing asset: %d, want 404", resp.StatusCode)
	}
}

func TestSpaNeverShadowsAPI(t *testing.T) {
	srv := newStaticServer(t)
	for _, path := range []string{"/healthz", "/v1/anything"} {
		resp, _ := get(t, srv, path)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d, want 404 from the static handler", path, resp.StatusCode)
		}
	}
}

func TestSpaBlocksTraversal(t *testing.T) {
	// net/http rejects raw ".." request lines before any handler runs, so to
	// exercise the handler's own path.Clean guard we must call it directly
	// with a hand-built traversal path.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>shell</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	req.URL.Path = "/../../etc/passwd"
	rec := httptest.NewRecorder()
	spaHandler(dir).ServeHTTP(rec, req)
	// Two layers defend here: the handler's path.Clean join, and ServeFile's
	// own ".." rejection (400). Either way nothing outside dir may be served.
	if rec.Code == http.StatusOK && rec.Body.String() != "<html>shell</html>" {
		t.Fatalf("traversal leaked file content: %q", rec.Body.String())
	}
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusOK {
		t.Fatalf("traversal: unexpected status %d", rec.Code)
	}
}
