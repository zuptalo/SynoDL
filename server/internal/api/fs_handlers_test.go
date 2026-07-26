package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"synodl/server/internal/syno"
)

func TestListShares(t *testing.T) {
	fake := &fakeSyno{shares: []syno.Folder{{Name: "movie", Path: "/movie"}}}
	srv := newTestServer(t, fake)
	resp := doReq(t, srv, http.MethodGet, "/v1/fs/shares", "sid", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Folders []syno.Folder `json:"folders"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Folders) != 1 || body.Folders[0].Path != "/movie" {
		t.Errorf("folders = %+v", body.Folders)
	}
}

func TestListFolder(t *testing.T) {
	fake := &fakeSyno{subfolders: map[string][]syno.Folder{
		"/tv-show": {{Name: "Friends", Path: "/tv-show/Friends"}},
	}}
	srv := newTestServer(t, fake)
	resp := doReq(t, srv, http.MethodGet, "/v1/fs/list?path=/tv-show", "sid", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if fake.gotFolderPath != "/tv-show" {
		t.Errorf("forwarded path = %q", fake.gotFolderPath)
	}
}

func TestListFolderRejectsRelativePath(t *testing.T) {
	fake := &fakeSyno{}
	srv := newTestServer(t, fake)
	for _, p := range []string{"", "tv-show", "..%2Fetc"} {
		resp := doReq(t, srv, http.MethodGet, "/v1/fs/list?path="+p, "sid", "", nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("path %q: status = %d, want 400", p, resp.StatusCode)
		}
	}
	if fake.gotFolderPath != "" {
		t.Error("invalid paths must not reach the NAS client")
	}
}

func TestFsRequiresSid(t *testing.T) {
	srv := newTestServer(t, &fakeSyno{})
	for _, path := range []string{"/v1/fs/shares", "/v1/fs/list?path=/x"} {
		resp := doReq(t, srv, http.MethodGet, path, "", "", nil)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", path, resp.StatusCode)
		}
	}
}
