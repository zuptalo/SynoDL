// synodl is the SynoDL server: a stateless, credential-free proxy in front of
// a Synology NAS's Download Station, serving the built PWA and the /v1 API
// from one binary. It holds no database, no files, no sessions — everything it
// knows arrives with the request (constitution Principle III).
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"synodl/server/internal/api"
	"synodl/server/internal/config"
	"synodl/server/internal/syno"
)

// Stamped at build time via -ldflags (see Dockerfile); the same VERSION is
// compiled into the PWA so the update prompt and /v1/config always agree.
var (
	version         = "dev"
	releaseNotesB64 = ""
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	router := api.NewRouter(api.Deps{
		Cfg:          cfg,
		Syno:         syno.NewHTTPClient(cfg.SynoURL, cfg.SynoTLSInsecure),
		Version:      version,
		ReleaseNotes: decodeReleaseNotes(releaseNotesB64),
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("synodl starting", "version", version, "port", cfg.Port, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown", "err", err)
	}
	slog.Info("synodl stopped")
}

// decodeReleaseNotes unpacks the base64 JSON release-note list stamped by CI.
// Malformed input degrades to no notes — never a boot failure.
func decodeReleaseNotes(b64 string) []api.ReleaseNote {
	if b64 == "" {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		slog.Warn("release notes: bad base64, ignoring")
		return nil
	}
	var notes []api.ReleaseNote
	if err := json.Unmarshal(raw, &notes); err != nil {
		slog.Warn("release notes: bad JSON, ignoring")
		return nil
	}
	return notes
}
