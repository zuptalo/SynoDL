package api

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"synodl/server/internal/config"
	"synodl/server/internal/nas"
	"synodl/server/internal/source"
	"synodl/server/internal/store"
	"synodl/server/internal/syno"
	"synodl/server/internal/synomock"
)

// nasRecorder wraps the fake DSM and keeps every request it received, so a test
// can assert on what actually crossed to the NAS — the only way to check a
// negative like "no session material went with it".
type nasRecorder struct {
	mu   sync.Mutex
	seen []string
}

func (n *nasRecorder) log(r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	var b strings.Builder
	b.WriteString(r.Method + " " + r.URL.String() + "\n")
	for k, v := range r.Header {
		b.WriteString(k + ": " + strings.Join(v, ",") + "\n")
	}
	b.Write(body)
	n.mu.Lock()
	defer n.mu.Unlock()
	n.seen = append(n.seen, b.String())
}

func (n *nasRecorder) all() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return strings.Join(n.seen, "\n---\n")
}

// newRecordingRouter is newStatefulRouter with a NAS that remembers its traffic.
func newRecordingRouter(t *testing.T) (http.Handler, *store.Store, *nasRecorder) {
	t.Helper()
	c, _ := store.NewCipher("kdf-input-for-tests")
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"), c)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	rec := &nasRecorder{}
	inner := synomock.New().Handler()
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.log(r) // log BEFORE anything reads the body away

		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(mock.Close)
	factory := func(base string, insecure bool) syno.Client { return syno.NewHTTPClient(mock.URL, false) }
	d := Deps{
		Cfg:      config.Config{MaxTorrentMB: 16, LoginPerMinute: 1000},
		Version:  "test",
		Stateful: true,
		Store:    st,
		NAS:      nas.New(st, factory),
	}
	return NewRouter(d), st, rec
}

// addProvider creates a source through the multi-source route.
func addProvider(t *testing.T, h http.Handler, admin map[string]string, name string, order int) map[string]string {
	t.Helper()
	body := `{"kind":"faketest","displayName":"` + name + `","moviesParent":"movie","tvParent":"tv-show",` +
		`"sortOrder":` + strconv.Itoa(order) + `,"session":{"c_token":"TOK-` + name + `","user_agent":"UA"}}`
	rec := do(t, h, "POST", "/v1/source/providers", body, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s = %d %s", name, rec.Code, rec.Body.String())
	}
	return admin
}

// FR-001: two sources of the SAME kind must be configurable — that is what makes
// the multi-source plumbing testable without a second driver.
func TestProvidersCRUDAndTwoOfTheSameKind(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	addProvider(t, h, admin, "Alpha", 0)
	addProvider(t, h, admin, "Beta", 1)

	rec := do(t, h, "GET", "/v1/source/providers", "", admin)
	if rec.Code != 200 {
		t.Fatalf("list = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Alpha") || !strings.Contains(body, "Beta") {
		t.Fatalf("both sources should be listed: %s", body)
	}
	// The kinds catalogue drives the admin form.
	if !strings.Contains(body, `"sessionFields"`) {
		t.Fatalf("kinds should advertise their session fields: %s", body)
	}
	// Session material is WRITE-ONLY: nothing in any response may echo it.
	for _, secret := range []string{"TOK-Alpha", "TOK-Beta", "SECRET"} {
		if strings.Contains(body, secret) {
			t.Fatalf("listing leaked session material %q: %s", secret, body)
		}
	}

	// Update one without touching the other.
	rec = do(t, h, "PUT", "/v1/source/providers/1", `{"displayName":"Alpha Renamed","session":{}}`, admin)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Alpha Renamed") {
		t.Fatalf("update = %d %s", rec.Code, rec.Body.String())
	}
	// A blank session field keeps the stored value rather than clearing it.
	if fakeLastSession.Get("c_token") != "TOK-Alpha" {
		t.Fatalf("blank update lost the stored secret: %+v", fakeLastSession)
	}

	// Delete one; the other survives.
	rec = do(t, h, "DELETE", "/v1/source/providers/1", "", admin)
	if rec.Code != 200 {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "GET", "/v1/source/providers", "", admin)
	if strings.Contains(rec.Body.String(), "Alpha") || !strings.Contains(rec.Body.String(), "Beta") {
		t.Fatalf("delete removed the wrong source: %s", rec.Body.String())
	}
}

// Only admins may see or change the source list.
func TestProvidersRoutesAreAdminOnly(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	addProvider(t, h, admin, "Alpha", 0)
	user := makeUser(t, h, admin, "plainuser", "")

	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/v1/source/providers", ""},
		{"POST", "/v1/source/providers", `{"kind":"faketest"}`},
		{"PUT", "/v1/source/providers/1", `{"displayName":"x"}`},
		{"DELETE", "/v1/source/providers/1", ""},
	} {
		rec := do(t, h, tc.method, tc.path, tc.body, user)
		if rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s as non-admin = %d, want denied", tc.method, tc.path, rec.Code)
		}
	}
}

// FR-006/FR-009: combined by default, every source represented, each item
// attributed and source-qualified.
func TestSearchCombinesSourcesAndQualifiesIDs(t *testing.T) {
	resetFake()
	source.ResetBreakers()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	addProvider(t, h, admin, "Alpha", 0)
	addProvider(t, h, admin, "Beta", 1)

	fakeSearch = source.SearchResult{Page: 1, Pages: 2, Items: []source.CatalogTitle{
		{ID: "the-matrix-1999", Type: "movie", Title: "The Matrix"},
	}}
	rec := do(t, h, "POST", "/v1/source/search", `{"page":1}`, admin)
	if rec.Code != 200 {
		t.Fatalf("search = %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// One entry per source, never merged (FR-005a), each addressable.
	if !strings.Contains(body, `"1:the-matrix-1999"`) || !strings.Contains(body, `"2:the-matrix-1999"`) {
		t.Fatalf("expected one qualified id per source: %s", body)
	}
	if !strings.Contains(body, `"sourceName":"Alpha"`) || !strings.Contains(body, `"sourceName":"Beta"`) {
		t.Fatalf("results must name their source: %s", body)
	}

	// Narrowing to one source returns only that source.
	rec = do(t, h, "POST", "/v1/source/search", `{"page":1,"source":"2"}`, admin)
	body = rec.Body.String()
	if strings.Contains(body, `"1:the-matrix-1999"`) || !strings.Contains(body, `"2:the-matrix-1999"`) {
		t.Fatalf("single-source search leaked the other source: %s", body)
	}
}

// FR-033/FR-034: a crafted or malformed title id is a client error, and can
// never address a source the caller does not have.
func TestTitleIDSafety(t *testing.T) {
	resetFake()
	source.ResetBreakers()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	addProvider(t, h, admin, "Alpha", 0)
	fakeTitle = source.TitleDetail{ID: "x", Type: "movie", Sendable: true}

	for _, bad := range []string{
		"99:some-title",                // a source this caller does not have
		"1:https%3A%2F%2Fevil.example", // absolute URL
		"1:..%2F..%2Fetc%2Fpasswd",     // traversal
		"0:x",                          // non-positive provider
		"abc:x",                        // non-numeric provider
	} {
		rec := do(t, h, "GET", "/v1/source/title/"+bad, "", admin)
		if rec.Code == http.StatusOK {
			t.Fatalf("title id %q was accepted", bad)
		}
	}
	// A legitimate qualified id still works.
	if rec := do(t, h, "GET", "/v1/source/title/1:the-matrix-1999", "", admin); rec.Code != 200 {
		t.Fatalf("legitimate id rejected: %d %s", rec.Code, rec.Body.String())
	}
}

// FR-023 / SC-006: nothing from a source's session may cross to the NAS. The
// fake NAS records what it was actually sent.
func TestSendForwardsNoSessionMaterialToNAS(t *testing.T) {
	resetFake()
	source.ResetBreakers()
	h, st, nasLog := newRecordingRouter(t)
	admin := adminAfterSetup(t, h)
	addProvider(t, h, admin, "Alpha", 0)

	fakeTitle = source.TitleDetail{ID: "the-matrix-1999", Type: "movie", Sendable: true,
		Qualities: []source.QualityOption{{ID: "q1", Label: "1080p"}}}
	fakeLinks = []string{"https://dl.fake/The.Matrix.1999.1080p.mkv?md5=SIG&u=4242&expires=1"}
	fakeSize = "2.0 GB"

	rec := do(t, h, "POST", "/v1/source/send",
		`{"titleId":"1:the-matrix-1999","qualityId":"q1","title":"The Matrix","type":"movie"}`, admin)
	if rec.Code != 200 {
		t.Fatalf("send = %d %s", rec.Code, rec.Body.String())
	}
	// Whatever reached the NAS must contain the link and nothing session-derived.
	sent := nasLog.all()
	if !strings.Contains(sent, "The.Matrix.1999.1080p.mkv") {
		t.Fatalf("the resolved link never reached the NAS: %s", sent)
	}
	for _, secret := range []string{"TOK-Alpha", "c_token", "cf_clearance", "wordpress_logged_in", "X-SynoDL-Session"} {
		if strings.Contains(sent, secret) {
			t.Fatalf("session material %q crossed to the NAS: %s", secret, sent)
		}
	}
	_ = st
}

// SC-010: no session value, cookie or signed link may appear in a client-facing
// error payload.
func TestErrorsNeverEchoSecrets(t *testing.T) {
	resetFake()
	source.ResetBreakers()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	addProvider(t, h, admin, "Alpha", 0)

	fakeSearchErr = &source.ErrNeedsRefresh{Layer: source.LayerToken}
	for i := 0; i < sourceFailThreshold+1; i++ {
		rec := do(t, h, "POST", "/v1/source/search", `{"page":1}`, admin)
		body := rec.Body.String()
		for _, secret := range []string{"TOK-Alpha", "SECRET", "md5=", "u=4242"} {
			if strings.Contains(body, secret) {
				t.Fatalf("error payload leaked %q: %s", secret, body)
			}
		}
	}
}

// FR-035: unreadable sealed material is reported, never silently treated as
// "never configured" — which would strand the operator.
func TestUnreadableSessionIsReportedNotDiscarded(t *testing.T) {
	resetFake()
	source.ResetBreakers()
	h, st := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	addProvider(t, h, admin, "Alpha", 0)

	if _, err := st.DB().Exec(
		`UPDATE source_provider_secrets SET session_enc = ? WHERE provider_id = 1`,
		[]byte("corrupted-not-decryptable")); err != nil {
		t.Fatalf("corrupt session: %v", err)
	}
	rec := do(t, h, "POST", "/v1/source/search", `{"page":1}`, admin)
	if !strings.Contains(rec.Body.String(), string(store.SourceNeedsRefresh)) &&
		!strings.Contains(rec.Body.String(), "needs_refresh") {
		t.Fatalf("unreadable session should surface as needs-refresh: %d %s", rec.Code, rec.Body.String())
	}
	// The material itself must still be there — not quietly deleted.
	var n int
	_ = st.DB().QueryRow(`SELECT COUNT(*) FROM source_provider_secrets WHERE provider_id=1`).Scan(&n)
	if n != 1 {
		t.Fatal("unreadable session material was discarded rather than retained")
	}
}

// FR-012, at the level that actually blanks the screen: /v1/source/status drives
// a whole-view gate in the client, so with several sources it must be an
// aggregate. Reporting the first source's health let one unhealthy source hide a
// Discover view the others could fill — found by the browser test, fixed here
// because the client gate must behave the same for non-admins, who cannot list
// providers at all.
func TestStatusIsHealthyWhileAnySourceIsUsable(t *testing.T) {
	resetFake()
	source.ResetBreakers()
	h, st := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	addProvider(t, h, admin, "Broken", 0)
	addProvider(t, h, admin, "Healthy", 1)

	// Source 1 is dead, source 2 is fine.
	now := time.Now().Unix()
	if err := st.SetProviderStateErr(1, store.SourceNeedsRefresh, source.ReasonNeedsRefresh, 0, now); err != nil {
		t.Fatalf("set state: %v", err)
	}
	if err := st.SetProviderStateErr(2, store.SourceActive, "", now, now); err != nil {
		t.Fatalf("set state: %v", err)
	}

	rec := do(t, h, "GET", "/v1/source/status", "", admin)
	body := rec.Body.String()
	if !strings.Contains(body, `"state":"active"`) {
		t.Fatalf("one dead source must not make the whole catalog unavailable: %s", body)
	}

	// With EVERY source dead, the gate is correct to close.
	if err := st.SetProviderStateErr(2, store.SourceNeedsRefresh, source.ReasonNeedsRefresh, 0, now); err != nil {
		t.Fatalf("set state: %v", err)
	}
	rec = do(t, h, "GET", "/v1/source/status", "", admin)
	if !strings.Contains(rec.Body.String(), `"state":"needs_refresh"`) {
		t.Fatalf("all sources dead should report needs_refresh: %s", rec.Body.String())
	}
}
