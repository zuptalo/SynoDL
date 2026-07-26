package api

import "net/http"

// handleHealth reports the proxy's own liveness. It deliberately does NOT
// probe the NAS: the proxy is healthy (and should keep serving the PWA + login
// screen) even when the NAS is off — the UI surfaces NAS reachability itself.
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
