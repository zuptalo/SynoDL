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
	"path/filepath"
	"syscall"
	"time"

	"synodl/server/internal/api"
	"synodl/server/internal/config"
	"synodl/server/internal/nas"
	"synodl/server/internal/push"
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
		mgr := nas.New(st, func(base string, insecure bool) syno.Client {
			return syno.NewHTTPClient(base, insecure)
		})
		deps.NAS = mgr

		// Web Push (spec 0003, Increment 4): generate the instance VAPID keys once,
		// then run the completion watcher that notifies opted-in devices when a
		// download finishes — server-side, so it works while every client is offline.
		if _, err := st.GetVAPID(); errors.Is(err, store.ErrNotFound) {
			pub, priv, gerr := push.GenerateVAPIDKeys()
			if gerr != nil {
				slog.Error("vapid keygen", "err", gerr)
				os.Exit(1)
			}
			subject := "mailto:synodl@localhost"
			if oc, e := st.GetOperatorConfig(); e == nil && oc.PublicURL != "" {
				subject = oc.PublicURL
			}
			if err := st.SaveVAPID(store.VAPID{Public: pub, Private: priv, Subject: subject}); err != nil {
				slog.Error("vapid save", "err", err)
				os.Exit(1)
			}
		}
		if v, err := st.GetVAPID(); err == nil {
			watcher := push.NewWatcher(st, func(ctx context.Context) ([]push.Task, error) {
				var out []push.Task
				derr := mgr.Do(ctx, func(c syno.Client, sid string) error {
					tasks, e := c.ListTasks(ctx, sid)
					if e != nil {
						return e
					}
					for _, t := range tasks {
						out = append(out, push.Task{
							ID: t.ID, Name: t.Name, Status: t.Status,
							Destination: t.Destination, URI: t.URI, Size: t.Size,
						})
					}
					return nil
				})
				return out, derr
			}, push.NewSender(*v), version, 30*time.Second)
			// Hold the "app updated" push back ~25s so a rolling deploy's new pod is
			// past its readiness probe and actually serving before clients are told
			// to reload — otherwise tapping the notice hit a not-yet-ready backend.
			watcher.SetUpdateNotifyDelay(25 * time.Second)
			go watcher.Run(context.Background())
		}
		// Keep the download-source session warm: a single gentle probe every 15
		// minutes keeps it from idle-expiring, self-heals a source stuck in
		// needs_refresh after a transient blip, and flags a genuine expiry early so
		// an admin is prompted to re-paste before a user hits it cold.
		go deps.RunSourceKeepAlive(context.Background(), 15*time.Minute)
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
