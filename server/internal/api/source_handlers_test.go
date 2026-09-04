package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"synodl/server/internal/source"
)

// A controllable fake provider registered for handler tests, so tests exercise
// the HANDLER (auth, storage, error mapping, folder-grant, subfolder+task)
// without any real network. The concrete nama30 driver is tested in the
// source/providers package.
type fakeSrc struct{}

var (
	fakeVerifyErr   error
	fakeLastSession source.Session
	fakeSearch      source.SearchResult
	fakeSearchErr   error
	fakeLastQuery   source.SearchQuery
	fakeTitle       source.TitleDetail
	fakeTitleErr    error
	fakeLinks       []string
	fakeSize        string
	fakeResolveErr  error
	fakeParams      source.SearchParameters
	fakeParamsErr   error
)

func init() { source.Register(fakeSrc{}) }

func (fakeSrc) Kind() string        { return "faketest" }
func (fakeSrc) DisplayName() string { return "Fake Test Source" }
func (fakeSrc) SessionFields() []source.SessionField {
	return []source.SessionField{{Key: "c_token", Label: "token", Secret: true, Required: true}}
}
func (fakeSrc) Hosts() source.Config {
	return source.Config{
		APIHosts:      []string{"api.fake"},
		DownloadHosts: []string{"dl.fake"},
		ImageHosts:    []string{"127.0.0.1"}, // lets the image-proxy test hit httptest
	}
}
func (fakeSrc) VerifySession(_ context.Context, _ *source.Client, _ source.Config, s source.Session) error {
	fakeLastSession = s
	return fakeVerifyErr
}
func (fakeSrc) Search(_ context.Context, _ *source.Client, _ source.Config, _ source.Session, q source.SearchQuery) (source.SearchResult, error) {
	fakeLastQuery = q
	return fakeSearch, fakeSearchErr
}
func (fakeSrc) Title(context.Context, *source.Client, source.Config, source.Session, string) (source.TitleDetail, error) {
	return fakeTitle, fakeTitleErr
}
func (fakeSrc) ResolveDownload(context.Context, *source.Client, source.Config, source.Session, string, string) ([]string, string, error) {
	return fakeLinks, fakeSize, fakeResolveErr
}
func (fakeSrc) Parameters(context.Context, *source.Client, source.Config, source.Session) (source.SearchParameters, error) {
	return fakeParams, fakeParamsErr
}

func resetFake() {
	fakeVerifyErr, fakeSearchErr, fakeTitleErr, fakeResolveErr, fakeParamsErr = nil, nil, nil, nil, nil
	fakeSearch = source.SearchResult{}
	fakeParams = source.SearchParameters{}
	fakeTitle = source.TitleDetail{}
	fakeLinks = nil
	fakeSize = ""
	resetSourceFailures()
	source.ResetBreakers() // also package-global: isolate each test // the streak is a package global — isolate each test
}

// configureFake configures the fake provider via the admin API (verify passes).
func configureFake(t *testing.T, h http.Handler, admin map[string]string, moviesParent string) {
	t.Helper()
	body := `{"kind":"faketest","displayName":"Fake","moviesParent":"` + moviesParent +
		`","tvParent":"tv-show","session":{"cToken":"SECRET-TOKEN-VALUE","cfClearance":"CLR","userAgent":"UA"}}`
	rec := do(t, h, "PUT", "/v1/source/session", body, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("configure provider = %d %s", rec.Code, rec.Body.String())
	}
}

// Re-saving an already-configured provider with BLANK secret fields keeps the
// stored session (so the admin can re-verify or edit folders without re-pasting
// every cookie/token). The merged session is what gets verified.
func TestSourceSessionKeepsStoredSecretsWhenBlank(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")

	fakeLastSession = source.Session{}
	// Only change a destination folder; leave the session blank.
	body := `{"kind":"faketest","displayName":"Fake","moviesParent":"movies","tvParent":"series","session":{}}`
	rec := do(t, h, "PUT", "/v1/source/session", body, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("re-save with blank session = %d %s", rec.Code, rec.Body.String())
	}
	// Verification ran against the MERGED session (stored secrets preserved).
	if fakeLastSession.Get("c_token") != "SECRET-TOKEN-VALUE" || fakeLastSession.Get("cf_clearance") != "CLR" ||
		fakeLastSession.UserAgent != "UA" {
		t.Fatalf("blank re-save did not keep stored secrets: %+v", fakeLastSession)
	}
}

func makeUser(t *testing.T, h http.Handler, admin map[string]string, name string, folders string) map[string]string {
	t.Helper()
	do(t, h, "POST", "/v1/users", `{"username":"`+name+`","password":"example-user-pw","isAdmin":false}`, admin)
	if folders != "" {
		var uid int64
		rec := do(t, h, "GET", "/v1/users", "", admin)
		var wrap struct {
			Users []struct {
				ID       int64  `json:"id"`
				Username string `json:"username"`
			} `json:"users"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &wrap)
		for _, x := range wrap.Users {
			if x.Username == name {
				uid = x.ID
			}
		}
		do(t, h, "PUT", "/v1/users/"+itoa(uid)+"/folders", `{"folders":[`+folders+`]}`, admin)
	}
	rec := do(t, h, "POST", "/v1/session", `{"username":"`+name+`","password":"example-user-pw"}`, nil)
	var bl struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &bl)
	return map[string]string{"X-SynoDL-Session": bl.Token}
}

func TestSourceStatusAndAdminConfig(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	// Unconfigured status.
	rec := do(t, h, "GET", "/v1/source/status", "", admin)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"configured":false`) ||
		!strings.Contains(rec.Body.String(), `"canManage":true`) {
		t.Fatalf("status unconfigured = %d %s", rec.Code, rec.Body.String())
	}

	// Non-admin cannot manage and cannot configure.
	bob := makeUser(t, h, admin, "bob", "")
	rec = do(t, h, "GET", "/v1/source/status", "", bob)
	if !strings.Contains(rec.Body.String(), `"canManage":false`) {
		t.Fatalf("bob canManage should be false: %s", rec.Body.String())
	}
	if rec := do(t, h, "PUT", "/v1/source/session", `{"kind":"faketest","session":{}}`, bob); rec.Code != http.StatusForbidden {
		t.Fatalf("bob configure = %d, want 403", rec.Code)
	}

	// Verify-before-store: a failing verify stores nothing.
	fakeVerifyErr = &source.ErrProviderVerify{Reason: "invalid_token"}
	rec = do(t, h, "PUT", "/v1/source/session", `{"kind":"faketest","session":{"cToken":"x"}}`, admin)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "invalid_token") {
		t.Fatalf("failing verify = %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, "GET", "/v1/source/status", "", admin); !strings.Contains(rec.Body.String(), `"configured":false`) {
		t.Fatalf("nothing should be stored on verify failure: %s", rec.Body.String())
	}

	// Successful configure.
	fakeVerifyErr = nil
	configureFake(t, h, admin, "movie")
	rec = do(t, h, "GET", "/v1/source/status", "", admin)
	if !strings.Contains(rec.Body.String(), `"configured":true`) || !strings.Contains(rec.Body.String(), `"state":"active"`) {
		t.Fatalf("status active = %s", rec.Body.String())
	}
	// The secret token must never appear in any response.
	if strings.Contains(rec.Body.String(), "SECRET-TOKEN-VALUE") {
		t.Fatal("status response leaked the session token")
	}

	// Delete resets.
	do(t, h, "DELETE", "/v1/source/session", "", admin)
	if rec := do(t, h, "GET", "/v1/source/status", "", admin); !strings.Contains(rec.Body.String(), `"configured":false`) {
		t.Fatalf("after delete = %s", rec.Body.String())
	}
}

func TestSourceParameters(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	// Unavailable before configuration.
	if rec := do(t, h, "GET", "/v1/source/parameters", "", admin); rec.Code != http.StatusConflict {
		t.Fatalf("params unconfigured = %d", rec.Code)
	}

	configureFake(t, h, admin, "movie")
	fakeParams = source.SearchParameters{
		Genres:  []source.FacetOption{{Value: "3362", Slug: "drama", Name: "درام"}},
		Types:   []source.FacetOption{{Value: "15", Name: "Movie"}},
		MinYear: 1890, MaxYear: 2026,
	}
	rec := do(t, h, "GET", "/v1/source/parameters", "", admin)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"slug":"drama"`) ||
		!strings.Contains(rec.Body.String(), `"maxYear":2026`) {
		t.Fatalf("params = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSourceSearch(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	// Unavailable before configuration.
	if rec := do(t, h, "POST", "/v1/source/search", `{"query":"x"}`, admin); rec.Code != http.StatusConflict ||
		!strings.Contains(rec.Body.String(), "source_unavailable") {
		t.Fatalf("search unconfigured = %d %s", rec.Code, rec.Body.String())
	}

	configureFake(t, h, admin, "movie")
	fakeSearch = source.SearchResult{Page: 1, Pages: 3, Items: []source.CatalogTitle{
		{ID: "1", Type: "movie", Title: "Soul 2020", IMDbScore: 8},
	}}
	rec := do(t, h, "POST", "/v1/source/search", `{"query":"soul","page":1,"filters":{"quality":"4K"}}`, admin)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Soul 2020") || !strings.Contains(rec.Body.String(), `"pages":3`) {
		t.Fatalf("search = %d %s", rec.Code, rec.Body.String())
	}

	// A transient auth failure does NOT nuke the session — it returns a retryable
	// "busy" and the stored state stays active (a lone Cloudflare blip shouldn't
	// flip the whole source into needs_refresh).
	fakeSearchErr = &source.ErrNeedsRefresh{Layer: source.LayerToken}
	for i := 0; i < sourceFailThreshold-1; i++ {
		rec = do(t, h, "POST", "/v1/source/search", `{"query":"x"}`, admin)
		if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "source_busy") {
			t.Fatalf("transient search #%d = %d %s", i, rec.Code, rec.Body.String())
		}
	}
	if rec := do(t, h, "GET", "/v1/source/status", "", admin); !strings.Contains(rec.Body.String(), `"state":"active"`) {
		t.Fatalf("state should stay active through transient failures: %s", rec.Body.String())
	}

	// The threshold-th consecutive failure IS treated as a dead session → 409
	// needs-refresh(token) and the stored state flips.
	rec = do(t, h, "POST", "/v1/source/search", `{"query":"x"}`, admin)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "source_needs_refresh") ||
		!strings.Contains(rec.Body.String(), `"layer":"token"`) {
		t.Fatalf("expired search = %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, "GET", "/v1/source/status", "", admin); !strings.Contains(rec.Body.String(), `"state":"needs_refresh"`) {
		t.Fatalf("state should be needs_refresh: %s", rec.Body.String())
	}

	// A subsequent SUCCESS clears the streak and restores the session to active.
	fakeSearchErr = nil
	if rec := do(t, h, "POST", "/v1/source/search", `{"query":"x"}`, admin); rec.Code != 200 {
		t.Fatalf("recovery search = %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, "GET", "/v1/source/status", "", admin); !strings.Contains(rec.Body.String(), `"state":"active"`) {
		t.Fatalf("state should recover to active after a success: %s", rec.Body.String())
	}

	// Auth required.
	if rec := do(t, h, "POST", "/v1/source/search", `{"query":"x"}`, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-token search = %d, want 401", rec.Code)
	}
}

func TestSourceSearchContentRatingCap(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")

	kid := makeUser(t, h, admin, "kiddo", "")
	rec := do(t, h, "GET", "/v1/users", "", admin)
	var wrap struct {
		Users []struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"users"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &wrap)
	var kidID int64
	for _, x := range wrap.Users {
		if x.Username == "kiddo" {
			kidID = x.ID
		}
	}
	if pr := do(t, h, "PATCH", "/v1/users/"+itoa(kidID), `{"contentRating":"G"}`, admin); pr.Code != 200 {
		t.Fatalf("set rating = %d %s", pr.Code, pr.Body.String())
	}

	// The capped user's search is forced to age=G server-side even though they
	// never send it (and couldn't override it).
	do(t, h, "POST", "/v1/source/search", `{"query":"x","filters":{"score":"9"}}`, kid)
	if fakeLastQuery.Filters.Age != "G" {
		t.Fatalf("capped user age = %q, want G", fakeLastQuery.Filters.Age)
	}
	// An uncapped user (admin) carries no age cap.
	do(t, h, "POST", "/v1/source/search", `{"query":"x"}`, admin)
	if fakeLastQuery.Filters.Age != "" {
		t.Fatalf("uncapped age = %q, want empty", fakeLastQuery.Filters.Age)
	}
}

func TestSourceSendMovie(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie") // "movie" exists in the mock DSM

	fakeLinks = []string{"http://dl.fake/soul.mkv"}

	// Admin send → subfolder under movies parent + task, no leaked link.
	rec := do(t, h, "POST", "/v1/source/send", `{"titleId":"217561","qualityId":"q1","title":"Soul 2020"}`, admin)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"destination":"movie/Soul (2020)"`) {
		t.Fatalf("send = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "dl.fake") {
		t.Fatal("send response leaked the signed link")
	}

	// Repeat send reuses the existing subfolder (still 200).
	if rec := do(t, h, "POST", "/v1/source/send", `{"titleId":"217561","qualityId":"q1","title":"Soul 2020"}`, admin); rec.Code != 200 {
		t.Fatalf("repeat send = %d %s", rec.Code, rec.Body.String())
	}

	// Non-admin without a grant to movie → 403 destination_forbidden.
	bob := makeUser(t, h, admin, "carol", `"tv-show"`)
	rec = do(t, h, "POST", "/v1/source/send", `{"titleId":"1","qualityId":"q1","title":"Soul 2020"}`, bob)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "destination_forbidden") {
		t.Fatalf("non-admin send = %d %s", rec.Code, rec.Body.String())
	}

	// A lone auth failure at resolve is treated as transient (retryable "busy"),
	// not a dead session — same hysteresis as the browse paths.
	fakeResolveErr = &source.ErrNeedsRefresh{Layer: source.LayerToken}
	rec = do(t, h, "POST", "/v1/source/send", `{"titleId":"1","qualityId":"q1","title":"X"}`, admin)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "source_busy") {
		t.Fatalf("transient send = %d %s", rec.Code, rec.Body.String())
	}
}

// A send persists the catalog metadata for the Tasks list, including the poster
// URL and the catalog title id (spec 1016), keyed by the destination folder.
func TestSourceSendPersistsCatalogMetadata(t *testing.T) {
	resetFake()
	h, st := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")
	fakeLinks = []string{"http://dl.fake/soul.mkv"}

	body := `{"titleId":"217561","qualityId":"q1","title":"Soul 2020","type":"movie","year":"2020","imdbScore":8,"posterUrl":"https://cdn.example.info/poster/soul-l.webp"}`
	if rec := do(t, h, "POST", "/v1/source/send", body, admin); rec.Code != 200 {
		t.Fatalf("send = %d %s", rec.Code, rec.Body.String())
	}

	media, err := st.SourceDownloads()
	if err != nil {
		t.Fatalf("SourceDownloads: %v", err)
	}
	md, ok := media["movie/Soul (2020)"]
	if !ok {
		t.Fatalf("no stored row for movie/Soul (2020); got %v", media)
	}
	if md.PosterURL != "https://cdn.example.info/poster/soul-l.webp" {
		t.Fatalf("poster not persisted: %q", md.PosterURL)
	}
	if md.CatalogID != "217561" {
		t.Fatalf("catalog id not persisted: %q", md.CatalogID)
	}
}

func TestSourceSendMaxSize(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")
	fakeLinks = []string{"http://dl.fake/x.mkv"}
	do(t, h, "PUT", "/v1/source/policy", `{"maxDownloadMB":1024}`, admin) // 1 GB cap

	fakeSize = "2 GB"
	rec := do(t, h, "POST", "/v1/source/send", `{"titleId":"1","qualityId":"q","title":"Big"}`, admin)
	if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "download_too_large") {
		t.Fatalf("oversize send = %d %s", rec.Code, rec.Body.String())
	}
	fakeSize = "500 MB"
	if rec := do(t, h, "POST", "/v1/source/send", `{"titleId":"1","qualityId":"q","title":"Small"}`, admin); rec.Code != 200 {
		t.Fatalf("under-limit send = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSourceSendDailyLimit(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")
	fakeLinks, fakeSize = []string{"http://dl.fake/x.mkv"}, "500 MB"

	dl := makeUser(t, h, admin, "dl", `"movie"`)
	rec := do(t, h, "GET", "/v1/users", "", admin)
	var wrap struct {
		Users []struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"users"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &wrap)
	var id int64
	for _, x := range wrap.Users {
		if x.Username == "dl" {
			id = x.ID
		}
	}
	do(t, h, "PATCH", "/v1/users/"+itoa(id), `{"dailyDownloadLimit":1}`, admin)

	if rec := do(t, h, "POST", "/v1/source/send", `{"titleId":"1","qualityId":"q","title":"One"}`, dl); rec.Code != 200 {
		t.Fatalf("first send = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "POST", "/v1/source/send", `{"titleId":"1","qualityId":"q","title":"Two"}`, dl)
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "daily_limit_exceeded") {
		t.Fatalf("second send = %d %s", rec.Code, rec.Body.String())
	}
	// The rejection carries the remaining allowance so the client can offer to
	// send just what fits.
	if !strings.Contains(rec.Body.String(), `"remaining":0`) {
		t.Fatalf("over-limit response missing remaining: %s", rec.Body.String())
	}
}

func TestSourcePrefs(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	if rec := do(t, h, "GET", "/v1/source/prefs", "", admin); !strings.Contains(rec.Body.String(), `"preferredQuality":""`) {
		t.Fatalf("empty prefs = %s", rec.Body.String())
	}
	do(t, h, "PUT", "/v1/source/prefs", `{"preferredQuality":"1080p"}`, admin)
	if rec := do(t, h, "GET", "/v1/source/prefs", "", admin); !strings.Contains(rec.Body.String(), `"preferredQuality":"1080p"`) {
		t.Fatalf("prefs after set = %s", rec.Body.String())
	}
}

// ---- spec 0008: ownership markers on search results ----------------------

// seedMockFolders adds folders to the mock DSM's tree so a test can set up "the
// NAS already holds these titles".
// seedMockTree seeds FILES per directory, which is what ownership now reads.
// Seeding a folder NAME alone no longer makes a title owned — that was the bug.
func seedMockTree(t *testing.T, mockURL string, tree map[string][]string) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"tree": tree})
	resp, err := http.Post(mockURL+"/__mock/library", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("seed mock tree: %v", err)
	}
	resp.Body.Close()
}

func seedMockFolders(t *testing.T, mockURL string, folders map[string][]string) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"folders": folders})
	resp, err := http.Post(mockURL+"/__mock/library", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("seed mock library: %v", err)
	}
	resp.Body.Close()
}

// FR-001/FR-011: a title whose folder already exists under the configured parent
// comes back flagged, and one that does not comes back unflagged.
func TestSearchMarksTitlesAlreadyOnTheNAS(t *testing.T) {
	resetFake()
	// Needs the mock rather than the fake: ownership now reads FILES, and the
	// mock is the only thing that models a folder's contents faithfully.
	h, _, mockURL := newStatefulRouterWithMock(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")

	// The fixture tree holds /tv-show/Friends and /movie/Kids, but a folder NAME
	// is no longer evidence — the content has to actually be there (FR-001a).
	seedMockTree(t, mockURL, map[string][]string{
		"/tv-show/Friends": {"Friends.S01E01.mkv"},
		"/movie/Kids":      {"Kids.1995.mkv"},
	})
	fakeSearch = source.SearchResult{Page: 1, Pages: 1, Items: []source.CatalogTitle{
		{ID: "1", Type: "series", Title: "Friends 1994 - 2004"},      // on the NAS
		{ID: "2", Type: "movie", Title: "Kids 1995"},                 // on the NAS
		{ID: "3", Type: "movie", Title: "Some Film Nobody Has 2020"}, // not
	}}
	rec := do(t, h, "POST", "/v1/source/search", `{"page":1}`, admin)
	if rec.Code != 200 {
		t.Fatalf("search = %d %s", rec.Code, rec.Body.String())
	}
	got := decodeOwnership(t, rec.Body.Bytes())
	if len(got) != 3 {
		t.Fatalf("got %d items, want 3", len(got))
	}
	// Keyed by TITLE, not id: the response re-qualifies ids as "<sourceId>:<id>",
	// so an id-keyed assertion silently matches nothing and passes vacuously.
	want := map[string]string{
		"Friends 1994 - 2004":       source.OwnershipOwned,
		"Kids 1995":                 source.OwnershipOwned,
		"Some Film Nobody Has 2020": source.OwnershipAbsent,
	}
	for title, wantIn := range want {
		if got[title] != wantIn {
			t.Errorf("%q ownership = %q, want %q", title, got[title], wantIn)
		}
	}
	// An absent title must omit the field entirely (omitempty), so an older
	// client sees exactly the payload it saw before.
	if strings.Contains(rec.Body.String(), `"Some Film Nobody Has 2020","posterUrl":"","inLibrary"`) {
		t.Error("an absent title should omit inLibrary entirely")
	}
}

// decodeOwnership maps each returned title to its inLibrary flag.
func decodeOwnership(t *testing.T, body []byte) map[string]string {
	t.Helper()
	var res struct {
		Items []struct {
			Title     string `json:"title"`
			Ownership string `json:"ownership"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	out := make(map[string]string, len(res.Items))
	for _, it := range res.Items {
		out[it.Title] = it.Ownership
	}
	return out
}

// FR-005: the one failure this feature cannot afford. A folder for the 2017 It
// must never mark the 1990 one as owned.
func TestSearchDoesNotMarkASameNameDifferentYearTitle(t *testing.T) {
	resetFake()
	h, _, mockURL := newStatefulRouterWithMock(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")
	seedMockFolders(t, mockURL, map[string][]string{"/movie": {"It 2017"}})
	seedMockTree(t, mockURL, map[string][]string{"/movie/It 2017": {"It.2017.mkv"}})

	fakeSearch = source.SearchResult{Page: 1, Pages: 1, Items: []source.CatalogTitle{
		{ID: "1", Type: "movie", Title: "It 2017"},
		{ID: "2", Type: "movie", Title: "It 1990"},
	}}
	rec := do(t, h, "POST", "/v1/source/search", `{"page":1}`, admin)
	got := decodeOwnership(t, rec.Body.Bytes())
	if got["It 2017"] != source.OwnershipOwned {
		t.Errorf("It 2017 = %q, want owned — its folder holds the film", got["It 2017"])
	}
	if got["It 1990"] == source.OwnershipOwned {
		t.Error("It 1990 was wrongly marked; a false positive makes a user skip a title they wanted")
	}
}

// FR-009: when the parent folders cannot be read, the search still succeeds and
// simply marks nothing. A failed scan is never an error the user sees.
func TestSearchStillSucceedsWhenTheLibraryCannotBeRead(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	// Parents that do not exist on the NAS: every listing fails.
	configureFake(t, h, admin, "no-such-parent")

	fakeSearch = source.SearchResult{Page: 1, Pages: 1, Items: []source.CatalogTitle{
		{ID: "1", Type: "movie", Title: "Kids 1995"},
	}}
	rec := do(t, h, "POST", "/v1/source/search", `{"page":1}`, admin)
	if rec.Code != 200 {
		t.Fatalf("search = %d %s — an unreadable parent must not fail the search", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"inLibrary":true`) {
		t.Error("nothing should be marked when the library could not be read")
	}
}

// ---- spec 1021: destinations in the shape a media server scrapes -----------

func sendDest(t *testing.T, h http.Handler, who map[string]string, body string) string {
	t.Helper()
	rec := do(t, h, "POST", "/v1/source/send", body, who)
	if rec.Code != 200 {
		t.Fatalf("send = %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Destination string `json:"destination"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out.Destination
}

// FR-001: a movie's year moves into parentheses.
func TestSendNamesAMovieForPlex(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")
	fakeLinks = []string{"http://dl.fake/dm4.mkv"}

	got := sendDest(t, h, admin, `{"titleId":"1","qualityId":"q1","title":"Despicable Me 4 2024"}`)
	if got != "movie/Despicable Me 4 (2024)" {
		t.Errorf("destination = %q, want movie/Despicable Me 4 (2024)", got)
	}
}

// FR-002/FR-003: a series lands in "Show (Year)/Season NN", and a year RANGE
// collapses to the first year — a scraper keys a show on when it started.
func TestSendNamesASeasonForPlex(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")
	fakeLinks = []string{
		"http://dl.fake/Friends.S01E01.1080p.mkv",
		"http://dl.fake/Friends.S01E02.1080p.mkv",
	}

	got := sendDest(t, h, admin,
		`{"titleId":"1","qualityId":"q1","type":"series","title":"Friends 1994 - 2004"}`)
	if got != "tv-show/Friends (1994)/Season 01" {
		t.Errorf("destination = %q, want tv-show/Friends (1994)/Season 01", got)
	}
}

// FR-006: an undeterminable season goes in the show's folder rather than a
// guessed one — a scraper still reads episodes there.
func TestSendWithoutADetectableSeasonUsesTheShowFolder(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")
	fakeLinks = []string{"http://dl.fake/chernobyl-part-one.mkv"}

	got := sendDest(t, h, admin,
		`{"titleId":"1","qualityId":"q1","type":"series","title":"Chernobyl 2019"}`)
	if got != "tv-show/Chernobyl (2019)" {
		t.Errorf("destination = %q, want tv-show/Chernobyl (2019)", got)
	}
}

// A pack spanning two seasons is ambiguous: filing half of it under the wrong
// season is worse than filing all of it in the show's folder.
func TestSendSpanningSeasonsUsesTheShowFolder(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")
	fakeLinks = []string{
		"http://dl.fake/Show.S01E10.mkv",
		"http://dl.fake/Show.S02E01.mkv",
	}

	got := sendDest(t, h, admin, `{"titleId":"1","qualityId":"q1","type":"series","title":"Show 2020"}`)
	if got != "tv-show/Show (2020)" {
		t.Errorf("destination = %q, want tv-show/Show (2020)", got)
	}
}

// FR-008: the catalog metadata is keyed by destination, so it must be stored
// against the NEW path or the Tasks list loses the poster and the owner.
func TestSendRemembersMetadataAgainstTheNewDestination(t *testing.T) {
	resetFake()
	h, st := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")
	fakeLinks = []string{"http://dl.fake/Friends.S01E01.mkv"}

	dest := sendDest(t, h, admin,
		`{"titleId":"1","qualityId":"q1","type":"series","title":"Friends 1994 - 2004","posterUrl":"http://p/x.jpg"}`)
	downloads, err := st.SourceDownloads()
	if err != nil {
		t.Fatalf("SourceDownloads: %v", err)
	}
	md, ok := downloads[dest]
	if !ok {
		t.Fatalf("no metadata stored for %q; have %v", dest, downloads)
	}
	if md.PosterURL != "http://p/x.jpg" || md.OwnerName == "" {
		t.Errorf("metadata lost its poster or owner: %+v", md)
	}
}

// FR-013: the deeper season path is still governed by folder grants.
func TestSeasonPathStillHonoursFolderGrants(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")
	fakeLinks = []string{"http://dl.fake/Friends.S01E01.mkv"}

	// Granted the TV parent: the season subfolder beneath it is allowed.
	allowed := makeUser(t, h, admin, "dave", `"tv-show"`)
	if got := sendDest(t, h, allowed,
		`{"titleId":"1","qualityId":"q1","type":"series","title":"Friends 1994"}`); got != "tv-show/Friends (1994)/Season 01" {
		t.Errorf("granted user got %q", got)
	}
	// Granted only the movies parent: refused, season folder or not.
	denied := makeUser(t, h, admin, "erin", `"movie"`)
	rec := do(t, h, "POST", "/v1/source/send",
		`{"titleId":"1","qualityId":"q1","type":"series","title":"Friends 1994"}`, denied)
	if rec.Code != http.StatusForbidden {
		t.Errorf("ungranted user = %d, want 403", rec.Code)
	}
}
