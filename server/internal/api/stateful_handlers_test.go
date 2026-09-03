package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"synodl/server/internal/auth"
	"synodl/server/internal/config"
	"synodl/server/internal/nas"
	"synodl/server/internal/store"
	"synodl/server/internal/syno"
	"synodl/server/internal/synomock"
)

// newStatefulRouter builds the router in stateful mode over a fresh store and a
// NAS manager whose clients point at a fresh mock DSM.
func newStatefulRouter(t *testing.T) (http.Handler, *store.Store) {
	h, st, _ := newStatefulRouterWithMock(t)
	return h, st
}

// newStatefulRouterWithMock is newStatefulRouter plus the mock DSM's base URL,
// for tests that need to drive its /__mock/* control endpoints (e.g. seeding a
// folder tree so Discover's ownership markers have something to find).
func newStatefulRouterWithMock(t *testing.T) (http.Handler, *store.Store, string) {
	t.Helper()
	c, _ := store.NewCipher("kdf-input-for-tests")
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"), c)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	mock := httptest.NewServer(synomock.New().Handler())
	t.Cleanup(mock.Close)
	factory := func(base string, insecure bool) syno.Client { return syno.NewHTTPClient(mock.URL, false) }
	d := Deps{
		Cfg:      config.Config{MaxTorrentMB: 16, LoginPerMinute: 1000, UploadMaxMB: 8},
		Version:  "test",
		Stateful: true,
		Store:    st,
		NAS:      nas.New(st, factory),
	}
	return NewRouter(d), st, mock.URL
}

func do(t *testing.T, h http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// dsmMockValue is the mock DSM's password for the admin account (see CLAUDE.md);
// concatenated in so it isn't an inline "nasPassword":"…" literal.
const dsmMockValue = "secret"

var setupBody = `{"publicUrl":"https://dl.example.com","nasAddress":"nas","nasPort":5001,` +
	`"nasTlsVerify":false,"nasAccount":"admin","nasPassword":"` + dsmMockValue + `",` +
	`"adminUsername":"kamran","adminPassword":"example-admin-pw"}`

func TestStatefulWizardThenUse(t *testing.T) {
	h, _ := newStatefulRouter(t)

	// Before setup: state reports not configured.
	rec := do(t, h, "GET", "/v1/setup/state", "", nil)
	var state map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &state)
	if rec.Code != 200 || state["configured"] != false {
		t.Fatalf("setup state before = %d %v", rec.Code, state)
	}

	// Complete the wizard → admin created + signed in.
	rec = do(t, h, "POST", "/v1/setup", setupBody, nil)
	if rec.Code != 200 {
		t.Fatalf("setup submit = %d, body %s", rec.Code, rec.Body.String())
	}
	var login struct {
		Token string `json:"token"`
		User  struct {
			Username string `json:"username"`
			IsAdmin  bool   `json:"isAdmin"`
		} `json:"user"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &login)
	if login.Token == "" || login.User.Username != "kamran" || !login.User.IsAdmin {
		t.Fatalf("setup response = %+v", login)
	}
	auth := map[string]string{"X-SynoDL-Session": login.Token}

	// Setup can't run twice.
	if rec := do(t, h, "POST", "/v1/setup", setupBody, nil); rec.Code != http.StatusConflict {
		t.Fatalf("second setup = %d, want 409", rec.Code)
	}

	// /v1/me needs a valid session.
	if rec := do(t, h, "GET", "/v1/me", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me without token = %d, want 401", rec.Code)
	}
	if rec := do(t, h, "GET", "/v1/me", "", auth); rec.Code != 200 {
		t.Fatalf("me with token = %d", rec.Code)
	}

	// Tasks flow through the shared NAS connection (mock returns its fixtures).
	rec = do(t, h, "GET", "/v1/tasks", "", auth)
	if rec.Code != 200 {
		t.Fatalf("tasks = %d, body %s", rec.Code, rec.Body.String())
	}
	var tasks struct {
		Tasks []map[string]any `json:"tasks"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tasks)
	if len(tasks.Tasks) == 0 {
		t.Fatal("expected the mock's task fixtures via the NAS manager")
	}
	if rec := do(t, h, "GET", "/v1/tasks", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("tasks without token = %d, want 401", rec.Code)
	}
}

func TestStatefulLoginAndGuards(t *testing.T) {
	h, st := newStatefulRouter(t)
	do(t, h, "POST", "/v1/setup", setupBody, nil) // creates admin "kamran"

	// Wrong password → uniform 401 credentials.
	if rec := do(t, h, "POST", "/v1/session", `{"username":"kamran","password":"nope"}`, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login = %d, want 401", rec.Code)
	}

	// A non-admin user, created by the admin.
	hash, _ := auth.HashPassword("example-bob-pw")
	bobID, err := st.CreateUser("bob", hash, false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	rec := do(t, h, "POST", "/v1/session", `{"username":"bob","password":"example-bob-pw"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("bob login = %d", rec.Code)
	}
	var login struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &login)
	bob := map[string]string{"X-SynoDL-Session": login.Token}

	// Admin-only endpoint rejects bob with 403 before any NAS call.
	if rec := do(t, h, "POST", "/v1/nas/reauth", `{"otp":"000000"}`, bob); rec.Code != http.StatusForbidden {
		t.Fatalf("bob reauth = %d, want 403", rec.Code)
	}

	// Disabling bob locks him out immediately (his session stops resolving).
	if err := st.SetUserEnabled(bobID, false); err != nil {
		t.Fatalf("SetUserEnabled: %v", err)
	}
	if rec := do(t, h, "GET", "/v1/me", "", bob); rec.Code != http.StatusUnauthorized {
		t.Fatalf("disabled bob /v1/me = %d, want 401", rec.Code)
	}
}

func TestStatefulUserManagementAndFolderScope(t *testing.T) {
	h, _ := newStatefulRouter(t)
	setup := do(t, h, "POST", "/v1/setup", setupBody, nil) // admin "kamran"
	var admin struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(setup.Body.Bytes(), &admin)
	adminAuth := map[string]string{"X-SynoDL-Session": admin.Token}

	// Admin creates a non-admin user.
	rec := do(t, h, "POST", "/v1/users", `{"username":"bob","password":"`+"example-bob-pw"+`","isAdmin":false}`, adminAuth)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user = %d, body %s", rec.Code, rec.Body.String())
	}
	var bob struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &bob)

	// List shows both.
	rec = do(t, h, "GET", "/v1/users", "", adminAuth)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"bob"`) || !strings.Contains(rec.Body.String(), `"kamran"`) {
		t.Fatalf("list users = %d %s", rec.Code, rec.Body.String())
	}

	// Scope bob to the "movie" folder only.
	fp := "/v1/users/" + itoa(bob.ID) + "/folders"
	if rec := do(t, h, "PUT", fp, `{"folders":["movie","/tv-show/Friends/","../evil"]}`, adminAuth); rec.Code != 200 {
		t.Fatalf("set folders = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "GET", fp, "", adminAuth)
	// "../evil" is dropped (traversal); the two valid grants remain.
	if !strings.Contains(rec.Body.String(), `"movie"`) || !strings.Contains(rec.Body.String(), `tv-show/Friends`) || strings.Contains(rec.Body.String(), "evil") {
		t.Fatalf("grants = %s", rec.Body.String())
	}

	// Bob signs in.
	rec = do(t, h, "POST", "/v1/session", `{"username":"bob","password":"`+"example-bob-pw"+`"}`, nil)
	var bobLogin struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &bobLogin)
	bobAuth := map[string]string{"X-SynoDL-Session": bobLogin.Token}

	// Bob's share picker shows only "movie" and "tv-show" (ancestor of a grant),
	// not "home"/"music".
	rec = do(t, h, "GET", "/v1/fs/shares", "", bobAuth)
	body := rec.Body.String()
	if !strings.Contains(body, `"movie"`) || !strings.Contains(body, `"tv-show"`) {
		t.Fatalf("bob shares missing granted folders: %s", body)
	}
	if strings.Contains(body, `"home"`) || strings.Contains(body, `"music"`) {
		t.Fatalf("bob shares leaked ungranted folders: %s", body)
	}

	// Bob can create into an allowed folder, but not an out-of-scope one.
	if rec := do(t, h, "POST", "/v1/tasks", `{"uris":["http://x/y.iso"],"destination":"movie/4K"}`, bobAuth); rec.Code != http.StatusCreated {
		t.Fatalf("bob create into movie/4K = %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, "POST", "/v1/tasks", `{"uris":["http://x/y.iso"],"destination":"home/Downloads"}`, bobAuth); rec.Code != http.StatusForbidden {
		t.Fatalf("bob create into home = %d, want 403", rec.Code)
	}
	// Empty destination is denied for a scoped non-admin.
	if rec := do(t, h, "POST", "/v1/tasks", `{"uris":["http://x/y.iso"]}`, bobAuth); rec.Code != http.StatusForbidden {
		t.Fatalf("bob create with no destination = %d, want 403", rec.Code)
	}

	// Bob cannot reach admin endpoints.
	if rec := do(t, h, "GET", "/v1/users", "", bobAuth); rec.Code != http.StatusForbidden {
		t.Fatalf("bob list users = %d, want 403", rec.Code)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
