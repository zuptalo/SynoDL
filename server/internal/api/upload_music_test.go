package api

import (
	"bytes"
	"errors"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"synodl/server/internal/store"
)

// Uploading a track (spec 1040).
//
// The interesting assertions here are all about ONE thing: the server composes
// the destination, and three client strings reach a path segment. What a
// determined value can and cannot do to that path is the point.

func uploadMusic(
	t *testing.T, h http.Handler, who map[string]string,
	kind, track, artist, album, filename, content string,
) *httptest.ResponseRecorder {
	t.Helper()
	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)
	for _, kv := range [][2]string{
		{"kind", kind}, {"track", track}, {"artist", artist}, {"album", album},
		{"size", strconv.Itoa(len(content))},
	} {
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
	r := httptest.NewRequest("POST", "/v1/fs/upload", buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	for k, v := range who {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// setMusicParents configures the libraries straight through the store, which is
// what the admin endpoint does.
func setMusicParents(t *testing.T, st *store.Store, music, video string) {
	t.Helper()
	if err := st.SetMusicParents(store.MusicParents{Music: music, MusicVideo: video}); err != nil {
		t.Fatalf("set music parents: %v", err)
	}
}

func destOf(t *testing.T, rec *httptest.ResponseRecorder) (string, string) {
	t.Helper()
	if rec.Code != 200 {
		t.Fatalf("upload = %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Destination string `json:"destination"`
		File        string `json:"file"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out.Destination, out.File
}

// FR-005: the same layout a downloaded track gets, so the two sit together.
func TestUploadMusic_FilesItLikeADownloadedTrack(t *testing.T) {
	resetFake()
	h, st := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	setMusicParents(t, st, "music", "music-video")

	rec := uploadMusic(t, h, admin, "music", "Lucente", "Anyma", "The End Of Genesys",
		"whatever-the-phone-called-it.mp3", "audio")
	dest, file := destOf(t, rec)

	if dest != "music/Anyma/The End Of Genesys" {
		t.Errorf("destination = %q", dest)
	}
	if file != "Lucente.mp3" {
		t.Errorf("file = %q, want it named after the track", file)
	}
}

// FR-005: no album is the Singles case, and it must use the same word the
// download recipe falls back to or the two end up in different folders.
func TestUploadMusic_NoAlbumIsSingles(t *testing.T) {
	resetFake()
	h, st := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	setMusicParents(t, st, "music", "")

	rec := uploadMusic(t, h, admin, "music", "Lucente", "Anyma", "", "t.mp3", "audio")
	dest, _ := destOf(t, rec)
	if dest != "music/Anyma/Singles" {
		t.Errorf("destination = %q, want the Singles fallback", dest)
	}
}

// FR-008. A media server pairs a .lrc to its audio by IDENTICAL base name, so
// uploading them under two different names is the same as not uploading the
// lyrics at all.
func TestUploadMusic_LyricsTakeTheTracksName(t *testing.T) {
	resetFake()
	h, st := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	setMusicParents(t, st, "music", "")

	_, audio := destOf(t, uploadMusic(t, h, admin, "music", "Lucente", "Anyma", "Genesys", "a.mp3", "x"))
	_, lyrics := destOf(t, uploadMusic(t, h, admin, "music", "Lucente", "Anyma", "Genesys", "downloaded.lrc", "y"))

	if audio != "Lucente.mp3" || lyrics != "Lucente.lrc" {
		t.Fatalf("got %q and %q; they must differ only in extension", audio, lyrics)
	}
}

// FR-009. Artwork belongs to the ALBUM, so it is the one file not named after
// the track — under the name a media server looks for.
func TestUploadMusic_ArtworkBecomesTheAlbumCover(t *testing.T) {
	resetFake()
	h, st := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	setMusicParents(t, st, "music", "")

	dest, file := destOf(t, uploadMusic(t, h, admin, "music", "Lucente", "Anyma", "Genesys", "IMG_2231.JPG", "img"))
	if file != "cover.jpg" {
		t.Errorf("artwork stored as %q, want cover.jpg", file)
	}
	if dest != "music/Anyma/Genesys" {
		t.Errorf("artwork went to %q, want the album folder", dest)
	}
}

// FR-011 / SC-003. The one that matters: nothing typed into the three fields may
// write outside the library.
func TestUploadMusic_CannotEscapeTheLibrary(t *testing.T) {
	resetFake()
	h, st := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	setMusicParents(t, st, "music", "")

	// A bare ".." is the dangerous one: it holds no separator to replace, so most
	// cleaning leaves it exactly as it was. Spec 0013 found this in the download
	// recipe; typing it is a shorter route to the same place.
	for _, artist := range []string{"..", ".", "   ", "...", "/"} {
		rec := uploadMusic(t, h, admin, "music", "Lucente", artist, "Genesys", "t.mp3", "x")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("artist %q = %d, want it refused", artist, rec.Code)
		}
	}
	for _, track := range []string{"..", ".", "  "} {
		rec := uploadMusic(t, h, admin, "music", track, "Anyma", "Genesys", "t.mp3", "x")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("track %q = %d, want it refused", track, rec.Code)
		}
	}
	// A separator in a name is neutered rather than refused: it is a legitimate
	// artist name, not an attack, and it must stay inside one folder.
	dest, _ := destOf(t, uploadMusic(t, h, admin, "music", "Back In Black", "AC/DC", "", "t.mp3", "x"))
	if dest != "music/AC DC/Singles" {
		t.Errorf("destination = %q, want one folder inside the library", dest)
	}
}

// FR-010. Each library keeps holding what it is for.
func TestUploadMusic_RefusesTheWrongKindOfFile(t *testing.T) {
	resetFake()
	h, st := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	setMusicParents(t, st, "music", "music-video")

	if rec := uploadMusic(t, h, admin, "music", "L", "Anyma", "", "clip.mkv", "x"); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("a video in a music upload = %d, want 415", rec.Code)
	}
	if rec := uploadMusic(t, h, admin, "music-video", "L", "Anyma", "", "track.mp3", "x"); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("audio in a music-video upload = %d, want 415", rec.Code)
	}
	if rec := uploadMusic(t, h, admin, "music", "L", "Anyma", "", "run.sh", "x"); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("a script = %d, want 415", rec.Code)
	}
}

// A music video goes to its OWN library, never the audio one.
func TestUploadMusic_VideoUsesItsOwnLibrary(t *testing.T) {
	resetFake()
	h, st := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	setMusicParents(t, st, "music", "music-video")

	dest, file := destOf(t, uploadMusic(t, h, admin, "music-video", "Lucente", "Anyma", "Genesys", "clip.mp4", "x"))
	if dest != "music-video/Anyma/Genesys" {
		t.Errorf("destination = %q, want the music-video library", dest)
	}
	if file != "Lucente.mp4" {
		t.Errorf("file = %q", file)
	}
}

// FR-002. Not configured means the upload cannot happen — reported as the same
// "no parent" answer a film upload gives, so the client can hide the option.
func TestUploadMusic_RefusesWhenNoLibraryIsConfigured(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	rec := uploadMusic(t, h, admin, "music", "Lucente", "Anyma", "", "t.mp3", "x")
	if rec.Code != http.StatusConflict {
		t.Fatalf("unconfigured music upload = %d, want 409", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("parent_unset")) {
		t.Errorf("body = %s, want parent_unset", rec.Body.String())
	}
}

// FR-012. The worker is started with the values that were typed, addressing the
// file by where it was actually written.
func TestUploadMusic_StartsATaggingWorker(t *testing.T) {
	resetFake()
	jobs := &fakeJobs{}
	h, st := newStatefulRouterWithJobs(t, jobs)
	admin := adminAfterSetup(t, h)
	setMusicParents(t, st, "music", "")

	destOf(t, uploadMusic(t, h, admin, "music", "Lucente", "Anyma", "Genesys", "t.mp3", "x"))

	if len(jobs.created) != 1 {
		t.Fatalf("created %d workers, want one tagging worker", len(jobs.created))
	}
	job := jobs.created[0]
	if job.Metadata.Labels["synodl.io/job"] != "tag" {
		t.Fatalf("worker is not marked as a tagging one: %+v", job.Metadata.Labels)
	}
	args := job.Spec.Template.Spec.Containers[0].Args
	joined := strings.Join(args, "\x00")
	for _, want := range []string{"/out/Anyma/Genesys/Lucente.mp3", "Lucente", "Anyma", "Genesys"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args are missing %q: %v", want, args)
		}
	}
}

// FR-013. A cosmetic step must never change the outcome of an upload that
// landed — including when there is no orchestrator at all.
func TestUploadMusic_TaggingFailureDoesNotFailTheUpload(t *testing.T) {
	resetFake()
	jobs := &fakeJobs{createErr: errors.New("apiserver unreachable")}
	h, st := newStatefulRouterWithJobs(t, jobs)
	admin := adminAfterSetup(t, h)
	setMusicParents(t, st, "music", "")

	rec := uploadMusic(t, h, admin, "music", "Lucente", "Anyma", "Genesys", "t.mp3", "x")
	if rec.Code != 200 {
		t.Fatalf("upload = %d, want it to have succeeded despite tagging failing", rec.Code)
	}
}

// A cover is the thing being embedded and lyrics are read from beside the file,
// so neither is worth its own worker.
func TestUploadMusic_TagsOnlyTheContentFile(t *testing.T) {
	resetFake()
	jobs := &fakeJobs{}
	h, st := newStatefulRouterWithJobs(t, jobs)
	admin := adminAfterSetup(t, h)
	setMusicParents(t, st, "music", "")

	destOf(t, uploadMusic(t, h, admin, "music", "Lucente", "Anyma", "G", "art.jpg", "img"))
	destOf(t, uploadMusic(t, h, admin, "music", "Lucente", "Anyma", "G", "words.lrc", "txt"))
	if len(jobs.created) != 0 {
		t.Fatalf("started %d workers for a cover and lyrics, want none", len(jobs.created))
	}
}
