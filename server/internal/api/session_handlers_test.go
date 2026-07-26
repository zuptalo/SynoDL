package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"synodl/server/internal/syno"
)

func TestLoginReturnsSid(t *testing.T) {
	fake := &fakeSyno{loginSid: "sid-123"}
	srv := newTestServer(t, fake)
	resp := doReq(t, srv, http.MethodPost, "/v1/session", "", "application/json",
		strings.NewReader(`{"account":"admin","password":"secret"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["sid"] != "sid-123" || body["account"] != "admin" {
		t.Errorf("body = %v", body)
	}
	if fake.gotLogin != [3]string{"admin", "secret", ""} {
		t.Errorf("forwarded login = %v", fake.gotLogin)
	}
}

func TestLoginValidation(t *testing.T) {
	srv := newTestServer(t, &fakeSyno{loginSid: "x"})
	cases := []string{
		`{"account":"","password":"p"}`,
		`{"account":"a","password":""}`,
		`not json`,
	}
	for _, body := range cases {
		resp := doReq(t, srv, http.MethodPost, "/v1/session", "", "application/json", strings.NewReader(body))
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("body %q: status %d, want 400", body, resp.StatusCode)
		}
	}
}

func TestLoginErrorContract(t *testing.T) {
	// Every auth failure kind surfaces as 401 with the kind string, so the
	// client can switch on one field.
	cases := []struct {
		kind syno.Kind
		want int
	}{
		{syno.KindCredentials, http.StatusUnauthorized},
		{syno.KindOTPRequired, http.StatusUnauthorized},
		{syno.KindOTPInvalid, http.StatusUnauthorized},
		{syno.KindPermission, http.StatusForbidden},
		{syno.KindUnreachable, http.StatusBadGateway},
	}
	for _, tc := range cases {
		fake := &fakeSyno{err: &syno.Error{Kind: tc.kind, API: "SYNO.API.Auth"}}
		srv := newTestServer(t, fake)
		resp := doReq(t, srv, http.MethodPost, "/v1/session", "", "application/json",
			strings.NewReader(`{"account":"a","password":"b"}`))
		if resp.StatusCode != tc.want {
			t.Errorf("%s: status %d, want %d", tc.kind, resp.StatusCode, tc.want)
		}
		var body map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if body["error"] != string(tc.kind) {
			t.Errorf("%s: error = %q", tc.kind, body["error"])
		}
	}
}

func TestLogout(t *testing.T) {
	fake := &fakeSyno{}
	srv := newTestServer(t, fake)
	resp := doReq(t, srv, http.MethodDelete, "/v1/session", "sid-9", "", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if fake.gotLogoutSid != "sid-9" {
		t.Errorf("forwarded sid = %q", fake.gotLogoutSid)
	}
	// Without a sid header there is nothing to log out.
	resp = doReq(t, srv, http.MethodDelete, "/v1/session", "", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-sid status = %d, want 401", resp.StatusCode)
	}
}

func TestLogoutIsBestEffort(t *testing.T) {
	// A NAS failure during logout still 204s: the client drops its sid either
	// way and there is no server-side state to worry about.
	fake := &fakeSyno{err: &syno.Error{Kind: syno.KindUnreachable, API: "SYNO.API.Auth"}}
	srv := newTestServer(t, fake)
	resp := doReq(t, srv, http.MethodDelete, "/v1/session", "sid-9", "", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 despite NAS error", resp.StatusCode)
	}
}
