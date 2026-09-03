package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// uploadBody builds a multipart request with the fields BEFORE the file, which
// is the order the streaming reader requires — it lets an invalid title be
// refused without reading the body.
func uploadBody(t *testing.T, kind, title, season, filename, content string) (string, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)
	for _, kv := range [][2]string{{"kind", kind}, {"title", title}, {"season", season}} {
		if kv[1] != "" {
			if err := mw.WriteField(kv[0], kv[1]); err != nil {
				t.Fatal(err)
			}
		}
	}
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return mw.FormDataContentType(), buf
}

func upload(t *testing.T, h http.Handler, who map[string]string,
	kind, title, season, filename, content string) *httptest.ResponseRecorder {
	t.Helper()
	ct, body := uploadBody(t, kind, title, season, filename, content)
	r := httptest.NewRequest("POST", "/v1/fs/upload", body)
	r.Header.Set("Content-Type", ct)
	for k, v := range who {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func uploadDest(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != 200 {
		t.Fatalf("upload = %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Destination string `json:"destination"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out.Destination
}

// FR-006: an uploaded title is named exactly as a downloaded one would be.
func TestUploadNamesAMovieLikeADownload(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")

	got := uploadDest(t, upload(t, h, admin, "movie", "Dune 2021", "", "Dune.2021.mkv", "film"))
	if got != "movie/Dune (2021)" {
		t.Errorf("destination = %q, want movie/Dune (2021)", got)
	}
}

// FR-009: an episode goes into its season folder.
func TestUploadPlacesAnEpisodeInItsSeason(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")

	got := uploadDest(t, upload(t, h, admin, "tv", "Friends 1994 - 2004", "1", "Friends.S01E01.mkv", "ep"))
	if got != "tv-show/Friends (1994)/Season 01" {
		t.Errorf("destination = %q, want tv-show/Friends (1994)/Season 01", got)
	}
}

// FR-010: picking an existing show uses that folder exactly — the name is
// already in the right shape, and re-deriving it must not change it.
func TestUploadIntoAnExistingShowDoesNotCreateANearDuplicate(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")

	got := uploadDest(t, upload(t, h, admin, "tv", "Friends (1994)", "2", "Friends.S02E01.mkv", "ep"))
	if got != "tv-show/Friends (1994)/Season 02" {
		t.Errorf("destination = %q, want tv-show/Friends (1994)/Season 02", got)
	}
}

// FR-013a. The sharpest boundary here: the file name is client-supplied text
// that becomes part of a path on the NAS, and it is guarded in two layers.
//
// Go's multipart reader bases the name, so anything with forward slashes
// arrives already reduced to its last segment. It does NOT touch a backslash or
// a bare "..", which survive verbatim — those are what this handler's own check
// is for. The invariant that matters is the same either way: a hostile name
// never places a file outside the destination the server composed.
func TestUploadRefusesAFileNameThatIsAPath(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")

	// What Go leaves hostile must be refused outright.
	for _, name := range []string{`back\slash.mkv`, "..", "nul\x00.mkv"} {
		rec := upload(t, h, admin, "movie", "Dune 2021", "", name, "x")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("upload of %q = %d %s, want 400", name, rec.Code, rec.Body.String())
		}
	}

	// What Go bases is accepted, but only ever as its last segment, and the
	// destination is the one the SERVER composed — never the client's path.
	for _, c := range []struct{ sent, want string }{
		{"../../../etc/passwd.mkv", "passwd.mkv"},
		{"sub/dir.mkv", "dir.mkv"},
		{"/absolute.mkv", "absolute.mkv"},
	} {
		rec := upload(t, h, admin, "movie", "Dune 2021", "", c.sent, "x")
		if rec.Code != 200 {
			t.Fatalf("upload of %q = %d %s", c.sent, rec.Code, rec.Body.String())
		}
		var out struct{ Destination, File string }
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.File != c.want {
			t.Errorf("%q stored as %q, want %q", c.sent, out.File, c.want)
		}
		if out.Destination != "movie/Dune (2021)" {
			t.Errorf("%q escaped to %q", c.sent, out.Destination)
		}
	}
}

// FR-013b: the capability granted stays the size of the request.
func TestUploadRefusesNonMediaFiles(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")

	for _, name := range []string{"payload.sh", "tool.exe", "archive.zip", "noextension"} {
		rec := upload(t, h, admin, "movie", "Dune 2021", "", name, "x")
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Errorf("upload of %q = %d, want 415", name, rec.Code)
		}
	}
	// ...while the things a library is actually made of go through.
	for _, name := range []string{"Dune.srt", "poster.jpg", "movie.nfo", "Dune.mkv"} {
		if rec := upload(t, h, admin, "movie", "Dune 2021", "", name, "x"); rec.Code != 200 {
			t.Errorf("upload of %q = %d, want 200 (%s)", name, rec.Code, rec.Body.String())
		}
	}
}

// FR-005/FR-008: the name is what keeps the library tidy, so it is required.
func TestUploadRequiresAUsableTitle(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")

	for _, title := range []string{"", "   ", "...", "///"} {
		rec := upload(t, h, admin, "movie", title, "", "x.mkv", "x")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("upload titled %q = %d, want 400", title, rec.Code)
		}
	}
}

// FR-003: the same grant check a download passes.
func TestUploadHonoursFolderGrants(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")

	tvOnly := makeUser(t, h, admin, "frank", `"tv-show"`)
	if rec := upload(t, h, tvOnly, "tv", "Friends 1994", "1", "e.mkv", "x"); rec.Code != 200 {
		t.Errorf("granted upload = %d %s", rec.Code, rec.Body.String())
	}
	if rec := upload(t, h, tvOnly, "movie", "Dune 2021", "", "d.mkv", "x"); rec.Code != http.StatusForbidden {
		t.Errorf("ungranted upload = %d, want 403", rec.Code)
	}
}

// FR-012: a collision is reported, never resolved by overwriting.
func TestUploadNeverOverwrites(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")

	if rec := upload(t, h, admin, "movie", "Dune 2021", "", "Dune.mkv", "first"); rec.Code != 200 {
		t.Fatalf("first upload = %d %s", rec.Code, rec.Body.String())
	}
	rec := upload(t, h, admin, "movie", "Dune 2021", "", "Dune.mkv", "second")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "file_exists") {
		t.Errorf("second upload = %d %s, want 409 file_exists", rec.Code, rec.Body.String())
	}
}

// FR-022a: there is no anonymous path to this.
func TestUploadRequiresASession(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")

	if rec := upload(t, h, nil, "movie", "Dune 2021", "", "d.mkv", "x"); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous upload = %d, want 401", rec.Code)
	}
}

// FR-002: there is no request shape that names a path.
func TestUploadCannotTargetAnArbitraryParent(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")

	for _, kind := range []string{"home", "/home", "../home", "music", ""} {
		rec := upload(t, h, admin, kind, "Dune 2021", "", "d.mkv", "x")
		if rec.Code != http.StatusConflict {
			t.Errorf("upload to parent %q = %d, want 409", kind, rec.Code)
		}
	}
}
