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

// A signed download link carries credentials and an account id in its query.
// Real DSM names the task after the file, not the whole URL, and the mock must
// match — otherwise those values end up on screen and inside test assertions.
func TestTaskNameFromURIDropsQuery(t *testing.T) {
	got := taskNameFromURI("https://dl6.example.info/Movies/The.Film.2026.1080p.mkv?md5=SECRET&u=1057599&expires=123")
	if want := "The.Film.2026.1080p.mkv"; got != want {
		t.Fatalf("taskNameFromURI = %q, want %q", got, want)
	}
	if strings.Contains(got, "u=") || strings.Contains(got, "md5=") {
		t.Fatalf("task name leaked link credentials: %q", got)
	}
	// Magnets keep their display-name handling.
	if got := taskNameFromURI("magnet:?xt=urn:btih:abc&dn=Some%20Name"); got != "Some Name" {
		t.Fatalf("magnet name = %q", got)
	}
}

// listFolderNames lists the directory names under an absolute path.
func listFolderNames(t *testing.T, srv *httptest.Server, sid, dir string) []string {
	t.Helper()
	out := post(t, srv, "/webapi/entry.cgi", url.Values{
		"api": {"SYNO.FileStation.List"}, "method": {"list"},
		"folder_path": {dir}, "_sid": {sid},
	})
	if out["success"] != true {
		t.Fatalf("list %s failed: %v", dir, out)
	}
	files := out["data"].(map[string]any)["files"].([]any)
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.(map[string]any)["name"].(string))
	}
	return names
}

func hasName(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// Spec 0008: the fixture tree is hardcoded in resetLocked with no way to add to
// it, which made Discover's ownership markers untestable end to end.
func TestMockLibrarySeeding(t *testing.T) {
	srv := httptest.NewServer(New().Handler())
	t.Cleanup(srv.Close)
	sid := loginSid(t, srv)

	seed := map[string]any{"folders": map[string][]string{
		"/movie":                         {"Dune 2021"},
		"/tv-show/Friends 1994/Season 1": {},
	}}
	postJSON(t, srv, "/__mock/library", seed)

	// The seeded title is listed under its parent...
	if got := listFolderNames(t, srv, sid, "/movie"); !hasName(got, "Dune 2021") {
		t.Errorf("/movie = %v, want it to contain Dune 2021", got)
	}
	// ...alongside the original fixtures, because seeding is additive.
	if got := listFolderNames(t, srv, sid, "/movie"); !hasName(got, "Kids") {
		t.Errorf("seeding dropped the fixture folders: %v", got)
	}
	// Intermediate levels are created implicitly, and can be descended into.
	if got := listFolderNames(t, srv, sid, "/tv-show"); !hasName(got, "Friends 1994") {
		t.Errorf("/tv-show = %v, want it to contain Friends 1994", got)
	}
	if got := listFolderNames(t, srv, sid, "/tv-show/Friends 1994"); !hasName(got, "Season 1") {
		t.Errorf("nested season folder missing: %v", got)
	}

	// Reseeding the same name must not duplicate it.
	postJSON(t, srv, "/__mock/library", seed)
	n := 0
	for _, name := range listFolderNames(t, srv, sid, "/movie") {
		if name == "Dune 2021" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("Dune 2021 appears %d times after reseeding, want 1", n)
	}

	// Reset restores the plain fixtures.
	postJSON(t, srv, "/__mock/reset", map[string]any{})
	sid = loginSid(t, srv)
	if got := listFolderNames(t, srv, sid, "/movie"); hasName(got, "Dune 2021") {
		t.Errorf("reset did not clear the seeded folders: %v", got)
	}
}

// The fake film site has to distinguish the series ARCHIVE from a series TITLE:
// both paths begin "/series". Getting that wrong is invisible from the outside —
// a title page full of cards parses as a series with no seasons — so it is
// asserted here rather than left to a driver test to notice.
func TestZarMockRoutesSeriesTitlesAndArchiveApart(t *testing.T) {
	srv := httptest.NewServer(New().Handler())
	defer srv.Close()

	get := func(path string) string {
		t.Helper()
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		var b bytes.Buffer
		if _, err := b.ReadFrom(resp.Body); err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		return b.String()
	}

	for _, archive := range []string{"/mocksrc/zar/series/", "/mocksrc/zar/series/page/2/"} {
		body := get(archive)
		if !strings.Contains(body, "inner_item_body_widget") {
			t.Fatalf("%s: expected a page of cards", archive)
		}
		if strings.Contains(body, "row_season_n_dl") {
			t.Fatalf("%s: archive served season markup", archive)
		}
	}

	title := get("/mocksrc/zar/series/zar-title-3/")
	if !strings.Contains(title, "row_season_n_dl") {
		t.Fatal("series title page served no seasons")
	}
	if strings.Contains(title, "inner_item_body_widget") {
		t.Fatal("series title page served a page of cards")
	}
	// Every title page describes the title; the listings deliberately do not.
	if !strings.Contains(title, `class="plot"`) {
		t.Fatal("series title page carried no synopsis")
	}
	if strings.Contains(get("/mocksrc/zar/series/"), `class="plot"`) {
		t.Fatal("archive carried a synopsis, which the real site's listings do not")
	}
}

// The fake JSON source must HONOUR the genre it is sent. A mock that accepts a
// filter and ignores it would let a broken cross-source translation pass — the
// results would look filtered because the other source filtered its half.
func TestTNMockHonoursTheGenreFilter(t *testing.T) {
	srv := httptest.NewServer(New().Handler())
	defer srv.Close()

	// The shape the real driver sends: form-encoded, with the filters as a JSON
	// object inside a single "parameters" field. Posting raw JSON here would test
	// a request the driver never makes.
	post := func(filters string) []map[string]any {
		t.Helper()
		resp, err := http.PostForm(srv.URL+"/mocksrc/tn/api/v1/action/advanced_search/page/1/orderby/favorite/order/desc",
			url.Values{"parameters": {filters}})
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer resp.Body.Close()
		var env struct {
			Result struct {
				Posts []map[string]any `json:"posts"`
			} `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return env.Result.Posts
	}

	all := post(`{}`)
	if len(all) == 0 {
		t.Fatal("no posts at all")
	}
	comedy := post(`{"genre":["101"]}`)
	if len(comedy) == 0 {
		t.Fatal("genre filter returned nothing")
	}
	if len(comedy) >= len(all) {
		t.Fatalf("filter changed nothing: %d of %d", len(comedy), len(all))
	}
	for _, p := range comedy {
		gs, _ := p["genre"].([]any)
		if len(gs) == 0 {
			t.Fatalf("post has no genre: %v", p["title"])
		}
		g, _ := gs[0].(map[string]any)
		if g["value"] != "101" {
			t.Fatalf("post %v has genre %v, want 101", p["title"], g["value"])
		}
	}
}
