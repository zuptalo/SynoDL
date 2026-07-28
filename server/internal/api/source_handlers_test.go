package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"synodl/server/internal/source"
)

// A controllable fake provider registered for handler tests, so tests exercise
// the HANDLER (auth, storage, error mapping, folder-grant, subfolder+task)
// without any real network. The concrete thirtynama driver is tested in the
// source/providers package.
type fakeSrc struct{}

var (
	fakeVerifyErr  error
	fakeSearch     source.SearchResult
	fakeSearchErr  error
	fakeLastQuery  source.SearchQuery
	fakeTitle      source.TitleDetail
	fakeTitleErr   error
	fakeLinks      []string
	fakeSize       string
	fakeResolveErr error
)

func init() { source.Register(fakeSrc{}) }

func (fakeSrc) Kind() string { return "faketest" }
func (fakeSrc) Hosts() source.Config {
	return source.Config{APIHosts: []string{"api.fake"}, DownloadHosts: []string{"dl.fake"}}
}
func (fakeSrc) VerifySession(context.Context, *source.Client, source.Config, source.Session) error {
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

func resetFake() {
	fakeVerifyErr, fakeSearchErr, fakeTitleErr, fakeResolveErr = nil, nil, nil, nil
	fakeSearch = source.SearchResult{}
	fakeTitle = source.TitleDetail{}
	fakeLinks = nil
	fakeSize = ""
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

	// Expired session → 409 needs-refresh(token) and the stored state flips.
	fakeSearchErr = &source.ErrNeedsRefresh{Layer: source.LayerToken}
	rec = do(t, h, "POST", "/v1/source/search", `{"query":"x"}`, admin)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "source_needs_refresh") ||
		!strings.Contains(rec.Body.String(), `"layer":"token"`) {
		t.Fatalf("expired search = %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, "GET", "/v1/source/status", "", admin); !strings.Contains(rec.Body.String(), `"state":"needs_refresh"`) {
		t.Fatalf("state should be needs_refresh: %s", rec.Body.String())
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
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"destination":"movie/Soul 2020"`) {
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

	// Expired session at resolve → 409 needs-refresh.
	fakeResolveErr = &source.ErrNeedsRefresh{Layer: source.LayerToken}
	rec = do(t, h, "POST", "/v1/source/send", `{"titleId":"1","qualityId":"q1","title":"X"}`, admin)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "source_needs_refresh") {
		t.Fatalf("expired send = %d %s", rec.Code, rec.Body.String())
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
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "daily_limit_reached") {
		t.Fatalf("second send = %d %s", rec.Code, rec.Body.String())
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
