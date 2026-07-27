// Package config loads all server configuration from environment variables.
// There is no config file: the container is configured entirely by env, and
// anything required is validated up front so a misconfigured deployment fails
// fast at boot with a clear message instead of limping along.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	// Env is "dev" or "production". Dev relaxes requirements (SYNO_URL defaults
	// to the local mock DSM) and is what `make start` runs.
	Env string
	// Port is the HTTP listen port.
	Port string
	// AllowedOrigins are the browser origins allowed to call the API via CORS.
	// In production the PWA is same-origin so this is usually empty/unused;
	// in dev it is the Vite dev servers.
	AllowedOrigins []string
	// StaticDir, when set, makes the server serve the built PWA from that
	// directory (production single-container mode).
	StaticDir string
	// DevProxy, when set (dev only), reverse-proxies non-API requests to a
	// running Vite dev server instead of serving StaticDir.
	DevProxy string
	// SynoURL is the base URL of the NAS's DSM, e.g. https://nas.local:5001.
	// The proxy only ever talks to this one target (no open-proxy behavior).
	SynoURL string
	// SynoTLSInsecure disables TLS certificate verification for the OUTBOUND
	// NAS connection only. Deliberate operator opt-in for self-signed certs.
	SynoTLSInsecure bool
	// MaxTorrentMB caps .torrent file uploads forwarded to the NAS.
	MaxTorrentMB int
	// LoginPerMinute rate-limits POST /v1/session per client IP so the proxy
	// cannot be used to brute-force the NAS.
	LoginPerMinute int
	// DataDir is where the SQLite database lives (the single mounted volume of
	// constitution v2.0.0). Defaults to /data. Introduced dormant in spec 0003
	// Increment 1; the stateful path activates in a later increment.
	DataDir string
	// SecretsKey encrypts secret columns (NAS password, VAPID private key) at
	// rest. Required once the stateful store is active; optional while the
	// server still runs in the legacy SYNO_URL-only mode. Never logged.
	SecretsKey string
}

// Load reads configuration from the environment, applying dev defaults and
// failing fast (with every missing variable listed) outside dev.
func Load() (Config, error) {
	cfg := Config{
		Env:             env("ENV", "dev"),
		Port:            os.Getenv("PORT"),
		AllowedOrigins:  splitComma(env("ALLOWED_ORIGINS", "http://localhost:5273,http://localhost:5274")),
		StaticDir:       os.Getenv("STATIC_DIR"),
		DevProxy:        os.Getenv("DEV_PROXY"),
		SynoURL:         os.Getenv("SYNO_URL"),
		SynoTLSInsecure: envBool("SYNO_TLS_INSECURE", false),
		MaxTorrentMB:    envInt("MAX_TORRENT_MB", 16),
		LoginPerMinute:  envInt("LOGIN_PER_MINUTE", 10),
		DataDir:         env("DATA_DIR", "/data"),
		SecretsKey:      os.Getenv("SECRETS_KEY"),
	}

	if cfg.Env == "dev" && cfg.SynoURL == "" {
		// Dev parity: `make start` runs the in-repo mock DSM on :8291, so a bare
		// dev boot always has a NAS to talk to without real hardware.
		cfg.SynoURL = "http://localhost:8291"
	}
	if cfg.Port == "" {
		// Dev defaults to SynoDL's own port block (see "Port allocation" in
		// CLAUDE.md) so this stack coexists with the user's other projects on
		// one machine; production keeps the conventional container-internal 8080.
		if cfg.Env == "dev" {
			cfg.Port = "8280"
		} else {
			cfg.Port = "8080"
		}
	}

	var missing []string
	if cfg.SynoURL == "" {
		missing = append(missing, "SYNO_URL")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	cfg.SynoURL = strings.TrimRight(cfg.SynoURL, "/")
	if cfg.MaxTorrentMB < 1 {
		cfg.MaxTorrentMB = 1
	}
	if cfg.LoginPerMinute < 1 {
		cfg.LoginPerMinute = 1
	}
	return cfg, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
