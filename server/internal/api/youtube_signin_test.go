package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"synodl/server/internal/k8s"
	"synodl/server/internal/store"
)

const sm = "SECRETVALUE-c0ffee" // marker planted in every cookie value

func header() string {
	return "SAPISID=" + sm + "1; __Secure-3PSID=" + sm + "2; LOGIN_INFO=" + sm + "3; YSC=" + sm + "4"
}

func putSignIn(t *testing.T, h http.Handler, who map[string]string, text string) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"text": text})
	return do(t, h, "PUT", "/v1/youtube/signin", string(b), who)
}

func TestSignIn_OnlyAnAdminMayUseAnyOfIt(t *testing.T) {
	h, _ := newRepairRouter(t, &fakeJobs{}, repairCfg())
	admin := adminAfterSetup(t, h)
	user := makeUser(t, h, admin, "bo", "")
	for _, c := range []struct{ m, body string }{{"GET", ""}, {"PUT", `{"text":"a=1; b=2; c=3"}`}, {"DELETE", ""}} {
		if rec := do(t, h, c.m, "/v1/youtube/signin", c.body, user); rec.Code != http.StatusForbidden {
			t.Errorf("%s as a regular user = %d, want 403", c.m, rec.Code)
		}
		if rec := do(t, h, c.m, "/v1/youtube/signin", c.body, nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s anonymously = %d, want 401", c.m, rec.Code)
		}
	}
}

func TestSignIn_SaveShowsMetadataAndNeverAValue(t *testing.T) {
	h, st := newRepairRouter(t, &fakeJobs{}, repairCfg())
	admin := adminAfterSetup(t, h)
	if rec := do(t, h, "GET", "/v1/youtube/signin", "", admin); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"saved":false`) {
		t.Fatalf("empty GET = %d %s", rec.Code, rec.Body.String())
	}
	rec := putSignIn(t, h, admin, "Cookie: "+header())
	if rec.Code != 200 {
		t.Fatalf("PUT = %d %s", rec.Code, rec.Body.String())
	}
	var v signinView
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	if !v.Saved || v.CookieCount != 4 || len(v.LoginCookies) != 3 || len(v.MissingLogin) != 1 || v.MissingLogin[0] != "SID" || v.Warning != "" {
		t.Fatalf("view = %+v", v)
	}
	// Nothing that answers a browser may carry a value, on any route, any verb.
	for _, path := range []string{"/v1/youtube/signin"} {
		if body := do(t, h, "GET", path, "", admin).Body.String(); strings.Contains(body, sm) {
			t.Fatalf("GET %s leaks a cookie value: %s", path, body)
		}
	}
	if strings.Contains(rec.Body.String(), sm) {
		t.Fatalf("PUT response leaks a cookie value: %s", rec.Body.String())
	}
	// The plaintext exists exactly once, behind the worker path.
	plain, ok, _ := st.OpenYoutubeSignInCookies()
	if !ok || !strings.Contains(string(plain), sm+"1") {
		t.Fatal("cookies were not stored")
	}
}

func TestSignIn_WarnsWithoutLoginCookiesAndRejectsJunk(t *testing.T) {
	h, _ := newRepairRouter(t, &fakeJobs{}, repairCfg())
	admin := adminAfterSetup(t, h)
	rec := putSignIn(t, h, admin, "YSC="+sm+"; PREF="+sm+"; VISITOR_INFO1_LIVE="+sm)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"warning":"no_login_cookies"`) {
		t.Fatalf("no-login paste = %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range []struct{ in, code string }{
		{"SAPISID=" + sm, "too_few_cookies"},
		{"just words " + sm, "unrecognised"},
		{"a=1; b=2; c=x\x01" + sm, "invalid"},
		{strings.Repeat("a=1;", 30000), "too_large"},
	} {
		rec := putSignIn(t, h, admin, c.in)
		if rec.Code != http.StatusBadRequest || errCode(t, rec) != c.code {
			t.Errorf("%.20q => %d %s, want 400 %s", c.in, rec.Code, rec.Body.String(), c.code)
		}
		if strings.Contains(rec.Body.String(), sm) {
			t.Errorf("an error repeats the pasted text: %s", rec.Body.String())
		}
	}
}

func TestSignIn_DeleteLeavesNothing(t *testing.T) {
	h, st := newRepairRouter(t, &fakeJobs{}, repairCfg())
	admin := adminAfterSetup(t, h)
	putSignIn(t, h, admin, header())
	if rec := do(t, h, "DELETE", "/v1/youtube/signin", "", admin); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d", rec.Code)
	}
	if _, ok, _ := st.OpenYoutubeSignInCookies(); ok {
		t.Fatal("cookies remain after removal")
	}
}

// ---- the worker fetch --------------------------------------------------------

func fetchDeps(t *testing.T, podIP string) (Deps, *signinGrants, string, *fakeJobs) {
	t.Helper()
	c, _ := store.NewCipher("kdf-input-for-tests")
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_ = st.SaveYoutubeSignIn([]byte("# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t1\tSAPISID\t"+sm+"\n"), 3, nil, "Anna", 1)
	fj := &fakeJobs{pods: []k8s.Pod{{
		Metadata: k8s.ObjectMeta{Name: "ytdl-x-abc", Labels: map[string]string{"job-name": "ytdl-x"}},
		Status:   k8s.PodStatus{PodIP: podIP},
	}}}
	g := newSigninGrants()
	tok, err := g.mint("ytdl-x")
	if err != nil {
		t.Fatal(err)
	}
	return Deps{Store: st, Jobs: fj, signin: g}, g, tok, fj
}

func fetch(d Deps, token, remote string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/v1/internal/ytdl-signin", nil)
	req.RemoteAddr = remote
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	handleYtdlSignInFetch(d).ServeHTTP(rec, req)
	return rec
}

func TestFetch_TheRightPodGetsTheCookiesExactlyOnce(t *testing.T) {
	d, _, tok, _ := fetchDeps(t, "10.42.0.97")
	rec := fetch(d, tok, "10.42.0.97:41234", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), sm) || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("first fetch = %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
	if again := fetch(d, tok, "10.42.0.97:41234", nil); again.Code != 404 || again.Body.Len() != 0 {
		t.Fatalf("a grant worked twice: %d %q", again.Code, again.Body.String())
	}
}

func TestFetch_PendingPodAddressCanRetryTheSameGrant(t *testing.T) {
	d, g, tok, fj := fetchDeps(t, "")
	first := fetch(d, tok, "10.42.0.97:41234", nil)
	if first.Code != 404 || first.Body.Len() != 0 {
		t.Fatalf("fetch before pod IP is visible = %d %q", first.Code, first.Body.String())
	}
	if _, still := g.peek(tok); !still {
		t.Fatal("an address-pending fetch consumed the grant")
	}

	// Kubernetes has now published the address. This is the second request made
	// by the init container's bounded retry loop, not a new grant or Job.
	fj.pods[0].Status.PodIP = "10.42.0.97"
	second := fetch(d, tok, "10.42.0.97:41234", nil)
	if second.Code != 200 || !strings.Contains(second.Body.String(), sm) {
		t.Fatalf("fetch after pod IP became visible = %d %q", second.Code, second.Body.String())
	}
}

func TestFetch_EveryRefusalIsTheSameEmpty404(t *testing.T) {
	cases := map[string]func(d Deps, tok string) *httptest.ResponseRecorder{
		"wrong address": func(d Deps, tok string) *httptest.ResponseRecorder { return fetch(d, tok, "10.42.0.55:1", nil) },
		"no grant":      func(d Deps, _ string) *httptest.ResponseRecorder { return fetch(d, "", "10.42.0.97:1", nil) },
		"unknown grant": func(d Deps, _ string) *httptest.ResponseRecorder { return fetch(d, "deadbeef", "10.42.0.97:1", nil) },
		"forwarded": func(d Deps, tok string) *httptest.ResponseRecorder {
			return fetch(d, tok, "10.42.0.97:1", map[string]string{"X-Forwarded-For": "10.42.0.97"})
		},
		"forwarded (rfc)": func(d Deps, tok string) *httptest.ResponseRecorder {
			return fetch(d, tok, "10.42.0.97:1", map[string]string{"Forwarded": "for=10.42.0.97"})
		},
		"real-ip": func(d Deps, tok string) *httptest.ResponseRecorder {
			return fetch(d, tok, "10.42.0.97:1", map[string]string{"X-Real-Ip": "10.42.0.97"})
		},
		"no remote": func(d Deps, tok string) *httptest.ResponseRecorder { return fetch(d, tok, "", nil) },
	}
	for name, run := range cases {
		d, g, tok, _ := fetchDeps(t, "10.42.0.97")
		rec := run(d, tok)
		if rec.Code != 404 || rec.Body.Len() != 0 {
			t.Errorf("%s = %d %q, want an empty 404", name, rec.Code, rec.Body.String())
		}
		// A refused attempt must not spend the real worker's grant.
		if _, still := g.peek(tok); name != "unknown grant" && name != "no grant" && !still {
			t.Errorf("%s consumed the grant", name)
		}
	}
}

func TestFetch_NoPodNoSignInExpiredAndRevoked(t *testing.T) {
	d, _, tok, fj := fetchDeps(t, "10.42.0.97")
	fj.pods = nil
	if rec := fetch(d, tok, "10.42.0.97:1", nil); rec.Code != 404 {
		t.Errorf("a Job with no pod = %d", rec.Code)
	}
	d, _, tok, _ = fetchDeps(t, "10.42.0.97")
	_ = d.Store.DeleteYoutubeSignIn()
	if rec := fetch(d, tok, "10.42.0.97:1", nil); rec.Code != 404 {
		t.Errorf("nothing saved = %d", rec.Code)
	}
	d, g, tok, _ := fetchDeps(t, "10.42.0.97")
	g.now = func() time.Time { return time.Now().Add(signinGrantTTL + time.Second) }
	if rec := fetch(d, tok, "10.42.0.97:1", nil); rec.Code != 404 {
		t.Errorf("an expired grant = %d", rec.Code)
	}
	d, g, tok, _ = fetchDeps(t, "10.42.0.97")
	g.revoke("ytdl-x")
	if rec := fetch(d, tok, "10.42.0.97:1", nil); rec.Code != 404 {
		t.Errorf("a revoked grant = %d", rec.Code)
	}
	if rec := fetch(Deps{}, tok, "10.42.0.97:1", nil); rec.Code != 404 {
		t.Errorf("no orchestrator = %d", rec.Code)
	}
}

func TestFetch_AnotherJobsPodDoesNotMatch(t *testing.T) {
	d, _, tok, fj := fetchDeps(t, "10.42.0.97")
	fj.pods[0].Metadata.Labels["job-name"] = "some-other-job"
	if rec := fetch(d, tok, "10.42.0.97:1", nil); rec.Code != 404 {
		t.Fatalf("a pod of a different Job redeemed the grant: %d", rec.Code)
	}
}
