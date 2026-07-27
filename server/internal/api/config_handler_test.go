package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"synodl/server/internal/config"
)

func TestConfigExposesHostOnly(t *testing.T) {
	// The NAS URL carries a scheme, port, and possibly credentials-adjacent
	// detail; only the bare hostname may reach the client (Principle III).
	cfg := config.Config{Env: "dev", SynoURL: "https://nas.example.com:5001", MaxTorrentMB: 1, LoginPerMinute: 10}
	srv := httptest.NewServer(NewRouter(Deps{
		Cfg: cfg, Syno: &fakeSyno{}, Version: "1.2.3",
		ReleaseNotes: []ReleaseNote{{SHA: "abc", Subject: "hi"}},
	}))
	t.Cleanup(srv.Close)

	resp := doReq(t, srv, http.MethodGet, "/v1/config", "", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Version      string        `json:"version"`
		NASHost      string        `json:"nasHost"`
		ReleaseNotes []ReleaseNote `json:"releaseNotes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Version != "1.2.3" {
		t.Errorf("version = %q", body.Version)
	}
	if body.NASHost != "nas.example.com" {
		t.Errorf("nasHost = %q, want bare hostname without scheme/port", body.NASHost)
	}
	if len(body.ReleaseNotes) != 1 || body.ReleaseNotes[0].Subject != "hi" {
		t.Errorf("releaseNotes = %+v", body.ReleaseNotes)
	}
}

func TestConfigHostFromStoredConfigInStatefulMode(t *testing.T) {
	// In stateful mode the Settings host must reflect the wizard-configured NAS
	// address from the store, not a stale SYNO_URL env value.
	h, _ := newStatefulRouter(t)
	adminAfterSetup(t, h) // setupBody configures nasAddress "nas"

	rec := do(t, h, "GET", "/v1/config", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("config = %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		NASHost string `json:"nasHost"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.NASHost != "nas" {
		t.Errorf("nasHost = %q, want the wizard's stored address 'nas'", body.NASHost)
	}
}
