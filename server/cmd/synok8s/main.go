// synok8s runs the fake Kubernetes Jobs API used by `make start` and the e2e
// harness, so neither ever needs a real cluster. See internal/k8smock.
//
// It accepts Job creates and lets a caller drive them through their lifecycle;
// it never downloads anything.
package main

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"synodl/server/internal/k8smock"
)

func main() {
	port := os.Getenv("MOCK_K8S_PORT")
	if port == "" {
		port = "8295"
	}

	// In dev it is useful for a submitted download to progress on its own, so a
	// developer can watch scheduled → started → completed without curling the
	// control endpoints. e2e leaves this off and drives the states explicitly.
	var advance time.Duration
	if v := os.Getenv("MOCK_K8S_AUTO_ADVANCE_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			advance = time.Duration(ms) * time.Millisecond
		}
	}

	slog.Info("synok8s (fake Jobs API) starting", "port", port, "autoAdvance", advance)
	if err := http.ListenAndServe(":"+port, k8smock.New(advance).Handler()); err != nil {
		slog.Error("listen", "err", err)
		os.Exit(1)
	}
}
