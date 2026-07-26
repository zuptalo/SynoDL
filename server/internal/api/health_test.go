package api

import (
	"net/http"
	"testing"
)

func TestHealthzNeedsNoSession(t *testing.T) {
	srv := newTestServer(t, &fakeSyno{})
	resp := doReq(t, srv, http.MethodGet, "/healthz", "", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
