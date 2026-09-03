package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHostAllowed(t *testing.T) {
	allow := []string{"interface.30nama.com", "divyacamilla.info"}
	cases := []struct {
		host string
		want bool
	}{
		{"interface.30nama.com", true},
		{"eu-download-storage-11.divyacamilla.info", true}, // subdomain suffix
		{"divyacamilla.info", true},
		{"evil.com", false},
		{"notdivyacamilla.info", false}, // must be a dot-boundary suffix, not substring
		{"30nama.com", false},           // not in this list
	}
	for _, c := range cases {
		if got := HostAllowed(c.host, allow); got != c.want {
			t.Errorf("HostAllowed(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestClientRefusesDisallowedHost(t *testing.T) {
	c := NewClient()
	_, err := c.Do(context.Background(), Session{}, []string{"interface.30nama.com"},
		Req{Method: "GET", URL: "https://evil.example.com/x"})
	if err != ErrHostNotAllowed {
		t.Fatalf("err = %v, want ErrHostNotAllowed", err)
	}
}

func TestClientSendsSessionHeadersAndBody(t *testing.T) {
	var gotAuth, gotCookie, gotUA, gotBody, gotCType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("c-token")
		gotCookie = r.Header.Get("Cookie")
		gotUA = r.Header.Get("User-Agent")
		gotCType = r.Header.Get("Content-Type")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient()
	// Auth is now supplied by the caller (the driver), not assembled by the client
	// from field names it knows — so one source's material can never ride along to
	// another source's host.
	sess := Session{UserAgent: "UA/1.0"}
	// httptest host is 127.0.0.1 — allow it.
	resp, err := c.Do(context.Background(), sess, []string{"127.0.0.1"},
		Req{Method: "POST", URL: srv.URL, Body: "query=matrix", XHR: true,
			Origin: "https://30nama.com", Referer: "https://30nama.com/",
			Headers: map[string]string{"c-token": "TOK", "c-api-key": "KEY"},
			Cookies: map[string]string{"cf_clearance": "CLR"}})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("status %d", resp.Status)
	}
	if gotAuth != "TOK" {
		t.Errorf("c-token = %q", gotAuth)
	}
	if gotCookie != "cf_clearance=CLR" {
		t.Errorf("cookie = %q", gotCookie)
	}
	if gotUA != "UA/1.0" {
		t.Errorf("user-agent = %q", gotUA)
	}
	if gotBody != "query=matrix" {
		t.Errorf("body = %q", gotBody)
	}
	if gotCType != "application/x-www-form-urlencoded" {
		t.Errorf("content-type = %q", gotCType)
	}
}

func TestClientDetectsChallenge(t *testing.T) {
	// A Cloudflare challenge body → needs-refresh(clearance), and the body is not
	// returned to the caller.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`<!DOCTYPE html><title>Just a moment...</title><div class="challenge-platform"></div>`))
	}))
	defer srv.Close()

	c := NewClient()
	_, err := c.Do(context.Background(), Session{UserAgent: "UA"}, []string{"127.0.0.1"},
		Req{Method: "GET", URL: srv.URL})
	nr, ok := AsNeedsRefresh(err)
	if !ok {
		t.Fatalf("err = %v, want ErrNeedsRefresh", err)
	}
	if nr.Layer != LayerClearance {
		t.Fatalf("layer = %q, want clearance", nr.Layer)
	}
}

func TestClientDetectsChallengeViaHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("CF-Mitigated", "challenge")
		w.Write([]byte(`whatever`))
	}))
	defer srv.Close()
	c := NewClient()
	_, err := c.Do(context.Background(), Session{}, []string{"127.0.0.1"}, Req{URL: srv.URL})
	if _, ok := AsNeedsRefresh(err); !ok {
		t.Fatalf("err = %v, want ErrNeedsRefresh via header", err)
	}
}
