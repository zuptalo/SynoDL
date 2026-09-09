package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func getMusicLibs(t *testing.T, h http.Handler, who map[string]string) musicLibraryView {
	t.Helper()
	rec := do(t, h, "GET", "/v1/library/music", "", who)
	if rec.Code != 200 {
		t.Fatalf("get = %d %s", rec.Code, rec.Body.String())
	}
	var v musicLibraryView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

func TestMusicLibraries_RoundTrip(t *testing.T) {
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	if got := getMusicLibs(t, h, admin); got.Music != "" || got.MusicVideo != "" {
		t.Fatalf("a fresh instance already had %+v", got)
	}

	rec := do(t, h, "PUT", "/v1/library/music",
		`{"music":"/music/","musicVideo":"music-video"}`, admin)
	if rec.Code != 200 {
		t.Fatalf("put = %d %s", rec.Code, rec.Body.String())
	}

	got := getMusicLibs(t, h, admin)
	// Normalised to the shape everything else uses, so the two ends cannot
	// disagree about whether a stored path is absolute.
	if got.Music != "music" || got.MusicVideo != "music-video" {
		t.Fatalf("stored %+v, want the slashes trimmed", got)
	}
}

func TestMusicLibraries_RefusesAPathThatIsNotOne(t *testing.T) {
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	for _, bad := range []string{`{"music":"../etc"}`, `{"music":"a/../b"}`, `{"music":"a//b"}`, `{"music":"a\\b"}`} {
		if rec := do(t, h, "PUT", "/v1/library/music", bad, admin); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", bad, rec.Code)
		}
	}
	// Empty is how a library is turned OFF, not an error.
	if rec := do(t, h, "PUT", "/v1/library/music", `{"music":"","musicVideo":""}`, admin); rec.Code != 200 {
		t.Errorf("clearing both = %d, want 200", rec.Code)
	}
}

func TestMusicLibraries_OnlyAnAdminMayChangeThem(t *testing.T) {
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	user := makeUser(t, h, admin, "bo", "")

	// Anyone signed in may READ them: the upload sheet has to know whether to
	// offer Music at all, and a library's folder name is not a secret.
	if v := getMusicLibs(t, h, user); v.CanManage {
		t.Error("a regular user was told they can manage the libraries")
	}
	if rec := do(t, h, "PUT", "/v1/library/music", `{"music":"music"}`, user); rec.Code != http.StatusForbidden {
		t.Fatalf("a regular user's write = %d, want 403", rec.Code)
	}
	if rec := do(t, h, "GET", "/v1/library/music", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("an anonymous read = %d, want 401", rec.Code)
	}
}
