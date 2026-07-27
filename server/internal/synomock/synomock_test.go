package synomock

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func post(t *testing.T, srv *httptest.Server, path string, form url.Values) map[string]any {
	t.Helper()
	resp, err := http.PostForm(srv.URL+path, form)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return out
}

func postJSON(t *testing.T, srv *httptest.Server, path string, body any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+path, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	resp.Body.Close()
}

func loginSid(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	out := post(t, srv, "/webapi/auth.cgi", url.Values{
		"method": {"login"}, "account": {"admin"}, "passwd": {"secret"},
	})
	if out["success"] != true {
		t.Fatalf("login failed: %v", out)
	}
	return out["data"].(map[string]any)["sid"].(string)
}

func listTasks(t *testing.T, srv *httptest.Server, sid string) []any {
	t.Helper()
	out := post(t, srv, "/webapi/DownloadStation/task.cgi", url.Values{
		"method": {"list"}, "_sid": {sid},
	})
	if out["success"] != true {
		t.Fatalf("list failed: %v", out)
	}
	return out["data"].(map[string]any)["tasks"].([]any)
}

func TestSeedTickAdvancesProgressDeterministically(t *testing.T) {
	srv := httptest.NewServer(New().Handler())
	t.Cleanup(srv.Close)
	sid := loginSid(t, srv)

	postJSON(t, srv, "/__mock/seed", map[string]any{"tasks": []Task{{
		Name: "fixture.iso", Type: "http", Status: "downloading",
		Size: 1000, Downloaded: 0, Rate: 100,
	}}})
	// The virtual clock only moves via /__mock/tick for rate math… real time
	// also passes, so allow >= rather than == on the lower bound.
	postJSON(t, srv, "/__mock/tick", map[string]int{"seconds": 5})
	tasks := listTasks(t, srv, sid)
	got := tasks[0].(map[string]any)["additional"].(map[string]any)["transfer"].(map[string]any)["size_downloaded"].(float64)
	if got < 500 {
		t.Fatalf("downloaded = %v after 5 virtual seconds at rate 100, want >= 500", got)
	}

	// Enough ticks to finish: status flips to finished and clamps at size.
	postJSON(t, srv, "/__mock/tick", map[string]int{"seconds": 60})
	tasks = listTasks(t, srv, sid)
	task := tasks[0].(map[string]any)
	if task["status"] != "finished" {
		t.Errorf("status = %v, want finished", task["status"])
	}
}

func TestResetRestoresFixtures(t *testing.T) {
	srv := httptest.NewServer(New().Handler())
	t.Cleanup(srv.Close)
	sid := loginSid(t, srv)

	postJSON(t, srv, "/__mock/seed", map[string]any{"tasks": []Task{}})
	if got := len(listTasks(t, srv, sid)); got != 0 {
		t.Fatalf("after empty seed: %d tasks", got)
	}
	postJSON(t, srv, "/__mock/reset", nil)
	// Reset drops sessions too — old sid must now fail with 106.
	out := post(t, srv, "/webapi/DownloadStation/task.cgi", url.Values{
		"method": {"list"}, "_sid": {sid},
	})
	if out["success"] != false {
		t.Fatal("stale sid survived reset")
	}
	sid = loginSid(t, srv)
	if got := len(listTasks(t, srv, sid)); got != 3 {
		t.Fatalf("after reset: %d tasks, want 3 fixtures", got)
	}
}

func TestTaskNameFromURI(t *testing.T) {
	cases := map[string]string{
		"http://example.com/a/file.iso":         "file.iso",
		"http://example.com/dir/":               "dir",
		"magnet:?xt=urn:btih:abc&dn=My%20Movie": "My Movie",
		"magnet:?xt=urn:btih:abc":               "magnet download",
	}
	for uri, want := range cases {
		if got := taskNameFromURI(uri); got != want {
			t.Errorf("taskNameFromURI(%q) = %q, want %q", uri, got, want)
		}
	}
}

func TestGuideAccountStates(t *testing.T) {
	// The DSM-side account states from the Login Web API Guide, reproducible
	// with the right password (spec 1001 FR-004).
	srv := httptest.NewServer(New().Handler())
	t.Cleanup(srv.Close)
	cases := map[string]float64{
		"disabled": 401,
		"blocked":  407,
		"expired":  409,
	}
	for acct, wantCode := range cases {
		out := post(t, srv, "/webapi/auth.cgi", url.Values{
			"method": {"login"}, "account": {acct}, "passwd": {"secret"},
		})
		if out["success"] != false {
			t.Errorf("%s: login unexpectedly succeeded", acct)
			continue
		}
		if code := out["error"].(map[string]any)["code"].(float64); code != wantCode {
			t.Errorf("%s: code %v, want %v", acct, code, wantCode)
		}
	}
}

func TestAuthRequiredEverywhere(t *testing.T) {
	srv := httptest.NewServer(New().Handler())
	t.Cleanup(srv.Close)
	for _, ep := range []string{
		"/webapi/DownloadStation/task.cgi",
		"/webapi/DownloadStation/statistic.cgi",
		"/webapi/entry.cgi",
	} {
		form := url.Values{"method": {"list"}, "api": {"SYNO.FileStation.List"}}
		out := post(t, srv, ep, form)
		if out["success"] != false {
			t.Errorf("%s without sid: success=true", ep)
		}
		code := out["error"].(map[string]any)["code"].(float64)
		if code != 106 {
			t.Errorf("%s without sid: code %v, want 106", ep, code)
		}
	}
}

func TestStatisticSumsDownloadingRates(t *testing.T) {
	srv := httptest.NewServer(New().Handler())
	t.Cleanup(srv.Close)
	sid := loginSid(t, srv)
	postJSON(t, srv, "/__mock/seed", map[string]any{"tasks": []Task{
		{Name: "a", Status: "downloading", Size: 1 << 30, Rate: 100, UpRate: 10},
		{Name: "b", Status: "downloading", Size: 1 << 30, Rate: 50},
		{Name: "c", Status: "paused", Size: 1 << 30, Rate: 999},
	}})
	out := post(t, srv, "/webapi/DownloadStation/statistic.cgi", url.Values{
		"method": {"getinfo"}, "_sid": {sid},
	})
	data := out["data"].(map[string]any)
	if data["speed_download"].(float64) != 150 {
		t.Errorf("speed_download = %v, want 150", data["speed_download"])
	}
	if data["speed_upload"].(float64) != 10 {
		t.Errorf("speed_upload = %v, want 10", data["speed_upload"])
	}
}

func TestFileStationListShareAndUnknownPath(t *testing.T) {
	srv := httptest.NewServer(New().Handler())
	t.Cleanup(srv.Close)
	sid := loginSid(t, srv)
	out := post(t, srv, "/webapi/entry.cgi", url.Values{
		"api": {"SYNO.FileStation.List"}, "method": {"list_share"}, "_sid": {sid},
	})
	shares := out["data"].(map[string]any)["shares"].([]any)
	names := make([]string, 0)
	for _, sh := range shares {
		names = append(names, sh.(map[string]any)["name"].(string))
	}
	if !strings.Contains(strings.Join(names, ","), "tv-show") {
		t.Errorf("shares = %v, want tv-show among them", names)
	}
	out = post(t, srv, "/webapi/entry.cgi", url.Values{
		"api": {"SYNO.FileStation.List"}, "method": {"list"},
		"folder_path": {"/does-not-exist"}, "_sid": {sid},
	})
	if out["success"] != false {
		t.Error("unknown folder_path should fail")
	}
}
