package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"synodl/server/internal/syno"
)

func TestListTasksBundlesStats(t *testing.T) {
	fake := &fakeSyno{
		tasks: []syno.Task{{ID: "dbid_1", Name: "a.iso", Status: "downloading"}},
		stats: syno.Stats{DownloadSpeed: 42, UploadSpeed: 7},
	}
	srv := newTestServer(t, fake)
	resp := doReq(t, srv, http.MethodGet, "/v1/tasks", "sid", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Tasks []syno.Task `json:"tasks"`
		Stats syno.Stats  `json:"stats"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Tasks) != 1 || body.Tasks[0].ID != "dbid_1" {
		t.Errorf("tasks = %+v", body.Tasks)
	}
	if body.Stats.DownloadSpeed != 42 {
		t.Errorf("stats = %+v", body.Stats)
	}
}

func TestListTasksStatsFailureDegrades(t *testing.T) {
	// A broken Statistic API must not blank the task list — stats just read 0.
	fake := &fakeSyno{
		tasks:    []syno.Task{{ID: "dbid_1"}},
		statsErr: &syno.Error{Kind: syno.KindNAS, API: "SYNO.DownloadStation.Statistic"},
	}
	srv := newTestServer(t, fake)
	resp := doReq(t, srv, http.MethodGet, "/v1/tasks", "sid", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 with degraded stats", resp.StatusCode)
	}
}

func TestListTasksRequiresSid(t *testing.T) {
	srv := newTestServer(t, &fakeSyno{})
	resp := doReq(t, srv, http.MethodGet, "/v1/tasks", "", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["error"] != "session" {
		t.Errorf("error = %q, want session", body["error"])
	}
}

func TestListTasksEmptyIsArrayNotNull(t *testing.T) {
	srv := newTestServer(t, &fakeSyno{})
	resp := doReq(t, srv, http.MethodGet, "/v1/tasks", "sid", "", nil)
	raw := new(bytes.Buffer)
	_, _ = raw.ReadFrom(resp.Body)
	if !strings.Contains(raw.String(), `"tasks":[]`) {
		t.Errorf("empty list must serialize as [], got %s", raw.String())
	}
}

func TestSessionExpiryMapsTo401(t *testing.T) {
	fake := &fakeSyno{err: &syno.Error{Kind: syno.KindSession, Code: 106, API: "SYNO.DownloadStation.Task"}}
	srv := newTestServer(t, fake)
	resp := doReq(t, srv, http.MethodGet, "/v1/tasks", "stale-sid", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["error"] != "session" {
		t.Errorf("error = %q, want session", body["error"])
	}
}

func TestCreateTaskURIs(t *testing.T) {
	fake := &fakeSyno{}
	srv := newTestServer(t, fake)
	resp := doReq(t, srv, http.MethodPost, "/v1/tasks", "sid", "application/json",
		strings.NewReader(`{"uris":[" http://a/x.iso ","","magnet:?xt=1"],"destination":"movie","username":"u","password":"p","unzipPassword":"z"}`))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if len(fake.gotURIs) != 2 || fake.gotURIs[0] != "http://a/x.iso" {
		t.Errorf("uris = %v, want trimmed + empties dropped", fake.gotURIs)
	}
	want := syno.CreateOpts{Destination: "movie", Username: "u", Password: "p", UnzipPassword: "z"}
	if fake.gotOpts != want {
		t.Errorf("opts = %+v, want %+v", fake.gotOpts, want)
	}
}

func TestCreateTaskNoURIs(t *testing.T) {
	srv := newTestServer(t, &fakeSyno{})
	resp := doReq(t, srv, http.MethodPost, "/v1/tasks", "sid", "application/json",
		strings.NewReader(`{"uris":["  "]}`))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func multipartBody(t *testing.T, field, filename string, content []byte, fields map[string]string) (string, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	fw, err := mw.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	_, _ = fw.Write(content)
	_ = mw.Close()
	return mw.FormDataContentType(), buf
}

func TestCreateTaskTorrentUpload(t *testing.T) {
	fake := &fakeSyno{}
	srv := newTestServer(t, fake)
	ct, buf := multipartBody(t, "torrent", "linux.torrent", []byte("d8:announce0:e"),
		map[string]string{"destination": "tv-show"})
	resp := doReq(t, srv, http.MethodPost, "/v1/tasks", "sid", ct, buf)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if fake.gotFilename != "linux.torrent" || string(fake.gotFileBytes) != "d8:announce0:e" {
		t.Errorf("forwarded file = %q (%d bytes)", fake.gotFilename, len(fake.gotFileBytes))
	}
	if fake.gotOpts.Destination != "tv-show" {
		t.Errorf("destination = %q", fake.gotOpts.Destination)
	}
}

func TestCreateTaskTorrentTooLarge(t *testing.T) {
	// Config caps at 1 MiB in newTestServer; a 2 MiB body must be refused.
	fake := &fakeSyno{}
	srv := newTestServer(t, fake)
	ct, buf := multipartBody(t, "torrent", "big.torrent", bytes.Repeat([]byte("x"), 2<<20), nil)
	resp := doReq(t, srv, http.MethodPost, "/v1/tasks", "sid", ct, buf)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", resp.StatusCode)
	}
	if fake.gotFilename != "" {
		t.Error("oversized upload must not reach the NAS client")
	}
}

func TestCreateTaskTorrentMissingFile(t *testing.T) {
	srv := newTestServer(t, &fakeSyno{})
	ct, buf := multipartBody(t, "wrongfield", "x.torrent", []byte("y"), nil)
	resp := doReq(t, srv, http.MethodPost, "/v1/tasks", "sid", ct, buf)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestTaskActions(t *testing.T) {
	for _, action := range []string{"pause", "resume", "delete"} {
		fake := &fakeSyno{}
		srv := newTestServer(t, fake)
		resp := doReq(t, srv, http.MethodPost, "/v1/tasks/"+action, "sid", "application/json",
			strings.NewReader(`{"ids":["dbid_1","dbid_2"]}`))
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("%s: status = %d", action, resp.StatusCode)
		}
		if fake.gotAction != action || len(fake.gotActionIDs) != 2 {
			t.Errorf("%s: forwarded %s %v", action, fake.gotAction, fake.gotActionIDs)
		}
	}
}

func TestTaskActionValidation(t *testing.T) {
	srv := newTestServer(t, &fakeSyno{})
	resp := doReq(t, srv, http.MethodPost, "/v1/tasks/pause", "sid", "application/json",
		strings.NewReader(`{"ids":[]}`))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
