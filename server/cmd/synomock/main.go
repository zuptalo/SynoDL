// synomock runs the fake Synology DSM used by `make start` and the e2e
// harness, so neither ever needs a real NAS. See internal/synomock.
package main

import (
	"log/slog"
	"net/http"
	"os"

	"synodl/server/internal/synomock"
)

func main() {
	port := os.Getenv("MOCK_PORT")
	if port == "" {
		port = "8291"
	}
	slog.Info("synomock (fake DSM) starting", "port", port,
		"accounts", "admin/secret, otpuser/secret+OTP 000000, disabled/blocked/expired (guide states)")
	if err := http.ListenAndServe(":"+port, synomock.New().Handler()); err != nil {
		slog.Error("listen", "err", err)
		os.Exit(1)
	}
}
