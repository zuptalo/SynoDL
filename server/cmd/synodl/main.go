// synodl is the SynoDL server in front of a Synology NAS's Download Station,
// serving the built PWA and the /v1 API from one binary. With SECRETS_KEY set it
// runs STATEFUL (constitution v2.0.0): its own accounts + a setup wizard, backed
// by a single encrypted SQLite volume, reaching the NAS through one stored
// connection. Without SECRETS_KEY it runs the legacy stateless path (SYNO_URL +
// client-carried sid) for dev/e2e continuity.
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

	"path/filepath"

	"synodl/server/internal/api"
	"synodl/server/internal/config"
	"synodl/server/internal/nas"
	"synodl/server/internal/store"
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

	deps := api.Deps{
		Cfg:          cfg,
		Version:      version,
		ReleaseNotes: decodeReleaseNotes(releaseNotesB64),
	}

	// Stateful mode (spec 0003) activates when SECRETS_KEY is configured: open the
	// SQLite store on the mounted volume and build the shared NAS connection
	// manager. Without it, the server runs the legacy stateless path.
	if cfg.SecretsKey != "" {
		cipher, err := store.NewCipher(cfg.SecretsKey)
		if err != nil {
			slog.Error("secrets key", "err", err)
			os.Exit(1)
		}
		if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
			slog.Error("data dir", "err", err)
			os.Exit(1)
		}
		st, err := store.Open(filepath.Join(cfg.DataDir, "synodl.db"), cipher)
		if err != nil {
			slog.Error("open store", "err", err)
			os.Exit(1)
		}
		// Boot canary: a stored config we can't decrypt means the wrong/missing
		// SECRETS_KEY — fail fast rather than silently resetting stored secrets.
		if _, err := st.GetOperatorConfig(); err != nil && !errors.Is(err, store.ErrNotFound) {
			slog.Error("cannot decrypt stored config — wrong or missing SECRETS_KEY?", "err", err)
			os.Exit(1)
		}
		deps.Stateful = true
		deps.Store = st
		deps.NAS = nas.New(st, func(base string, insecure bool) syno.Client {
			return syno.NewHTTPClient(base, insecure)
		})
		slog.Info("synodl stateful mode", "dataDir", cfg.DataDir)
	} else {
		deps.Syno = syno.NewHTTPClient(cfg.SynoURL, cfg.SynoTLSInsecure)
	}

	router := api.NewRouter(deps)

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
