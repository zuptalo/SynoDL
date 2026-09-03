package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"synodl/server/internal/config"
	"synodl/server/internal/syno"
)

// fakeSyno satisfies syno.Client for handler tests: canned results, recorded
// calls, injectable errors — no network anywhere.
type fakeSyno struct {
	loginSid   string
	loginOTP   string // OTP the fake requires; "" = none
	err        error  // when set, every method fails with it
	tasks      []syno.Task
	stats      syno.Stats
	statsErr   error
	shares     []syno.Folder
	subfolders map[string][]syno.Folder

	// Streaming test hooks: fail ListTasks after failListAfter successful calls,
	// returning listErr (used to exercise a mid-stream session expiry).
	listCalls     int
	failListAfter int
	listErr       error

	gotLogin      [3]string // account, password, otp
	gotLogoutSid  string
	gotURIs       []string
	gotFilename   string
	gotFileBytes  []byte
	gotOpts       syno.CreateOpts
	gotAction     string
	gotActionIDs  []string
	gotFolderPath string
	gotFolderName string

	// folderListCalls counts ListFolder calls, so the library snapshot tests can
	// assert that a shared parent is listed once rather than once per source.
	folderListCalls int
}

func (f *fakeSyno) Login(_ context.Context, account, password, otp string) (string, error) {
	f.gotLogin = [3]string{account, password, otp}
	if f.err != nil {
		return "", f.err
	}
	if f.loginOTP != "" && otp != f.loginOTP {
		kind := syno.KindOTPRequired
		if otp != "" {
			kind = syno.KindOTPInvalid
		}
		return "", &syno.Error{Kind: kind, API: "SYNO.API.Auth"}
	}
	return f.loginSid, nil
}

func (f *fakeSyno) Logout(_ context.Context, sid string) error {
	f.gotLogoutSid = sid
	return f.err
}

func (f *fakeSyno) ListTasks(_ context.Context, _ string) ([]syno.Task, error) {
	// failListAfter lets a streaming test succeed for the first N calls and then
	// fail (e.g. a mid-stream session expiry). 0 keeps the simple canned behavior.
	f.listCalls++
	if f.failListAfter > 0 && f.listCalls > f.failListAfter {
		return nil, f.listErr
	}
	return f.tasks, f.err
}

func (f *fakeSyno) CreateTaskURIs(_ context.Context, _ string, uris []string, opts syno.CreateOpts) error {
	f.gotURIs, f.gotOpts = uris, opts
	return f.err
}

func (f *fakeSyno) CreateTaskFile(_ context.Context, _ string, filename string, file io.Reader, opts syno.CreateOpts) error {
	f.gotFilename, f.gotOpts = filename, opts
	f.gotFileBytes, _ = io.ReadAll(file)
	return f.err
}

func (f *fakeSyno) PauseTasks(_ context.Context, _ string, ids []string) error {
	f.gotAction, f.gotActionIDs = "pause", ids
	return f.err
}

func (f *fakeSyno) ResumeTasks(_ context.Context, _ string, ids []string) error {
	f.gotAction, f.gotActionIDs = "resume", ids
	return f.err
}

func (f *fakeSyno) DeleteTasks(_ context.Context, _ string, ids []string) error {
	f.gotAction, f.gotActionIDs = "delete", ids
	return f.err
}

func (f *fakeSyno) Stats(_ context.Context, _ string) (syno.Stats, error) {
	if f.statsErr != nil {
		return syno.Stats{}, f.statsErr
	}
	return f.stats, f.err
}

func (f *fakeSyno) ListShares(_ context.Context, _ string) ([]syno.Folder, error) {
	return f.shares, f.err
}

func (f *fakeSyno) ListFolder(_ context.Context, _ string, path string) ([]syno.Folder, error) {
	f.gotFolderPath = path
	f.folderListCalls++
	if f.err != nil {
		return nil, f.err
	}
	return f.subfolders[path], nil
}

func (f *fakeSyno) CreateFolder(_ context.Context, _ string, path, name string) (syno.Folder, error) {
	f.gotFolderPath, f.gotFolderName = path, name
	if f.err != nil {
		return syno.Folder{}, f.err
	}
	return syno.Folder{Name: name, Path: strings.TrimRight(path, "/") + "/" + name}, nil
}

// newTestServer builds the real router (full middleware chain) over the fake.
func newTestServer(t *testing.T, fake *fakeSyno) *httptest.Server {
	t.Helper()
	cfg := config.Config{
		Env: "dev", Port: "0", SynoURL: "http://fake",
		MaxTorrentMB: 1, LoginPerMinute: 100, StreamMax: 64,
	}
	srv := httptest.NewServer(NewRouter(Deps{
		Cfg: cfg, Syno: fake, Version: "test",
		ReleaseNotes: []ReleaseNote{{SHA: "abc", Subject: "hello"}},
	}))
	t.Cleanup(srv.Close)
	return srv
}

// doReq is the shared request helper: optional sid header, optional body.
func doReq(t *testing.T, srv *httptest.Server, method, path, sid, contentType string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, body)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if sid != "" {
		req.Header.Set("X-Syno-Sid", sid)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}
