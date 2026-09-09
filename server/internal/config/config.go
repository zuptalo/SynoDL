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
	// AllowBrowserAccess lets the PWA run in an ordinary browser tab instead of
	// requiring installation first. Off by default: SynoDL is meant to be
	// installed, which is what makes Web Push and the app shell reliable. An
	// operator turns it on deliberately — to look at the app from a desktop
	// browser, or to debug something without installing.
	AllowBrowserAccess bool
	// UploadMaxMB caps a direct file upload into the library (spec 1022), in MB.
	// Every byte streams through this process on its way to the NAS and is never
	// buffered, so the cap bounds how long a transfer may run rather than how
	// much memory it can take — which is why it can sit at 10 GB on a container
	// limited to a couple of hundred MB of RAM.
	UploadMaxMB int
	// LoginPerMinute rate-limits POST /v1/session per client IP so the proxy
	// cannot be used to brute-force the NAS.
	LoginPerMinute int
	// StreamMax bounds concurrent SSE task streams (GET /v1/tasks/stream) so the
	// live-update feature cannot be used to open unbounded long-lived polls
	// against the NAS. Excess connections are shed with 503 + Retry-After.
	StreamMax int
	// DataDir is where the SQLite database lives (the single mounted volume of
	// constitution v2.0.0). Defaults to /data. Introduced dormant in spec 0003
	// Increment 1; the stateful path activates in a later increment.
	DataDir string
	// SecretsKey encrypts secret columns (NAS password, VAPID private key) at
	// rest. Required once the stateful store is active; optional while the
	// server still runs in the legacy SYNO_URL-only mode. Never logged.
	SecretsKey string

	// --- YouTube download workers (spec 0012) ------------------------------
	//
	// Every one of these is OPTIONAL. With no worker image and no library
	// claims configured the feature simply reports itself unavailable, so a
	// Compose or bare-container install is unaffected by its existence. There
	// is deliberately no default image: a missing one must mean "off", never a
	// surprise pull of something nobody chose.

	// YtdlImage is the worker image, which MUST be a pinned tag. An outdated
	// extractor stops working against the source, so this is bumped
	// deliberately alongside the supply-chain review rather than floated.
	YtdlImage string
	// YtdlNamespace is where workers are created. Empty means the pod's own
	// namespace, which is the only one the Role covers.
	YtdlNamespace string
	// YtdlOEmbedURL points the "what is this link?" lookup (spec 1034) at an
	// explicit endpoint instead of the source's real one. Dev and e2e set it to
	// the in-repo mock, which is what keeps the suite hermetic — without it the
	// tests would reach out to the public internet.
	YtdlOEmbedURL string
	// YtdlAPIURL points the worker client at an explicit Jobs API instead of
	// discovering the in-cluster one. Dev and e2e set it to the in-repo mock
	// orchestrator, so the SAME client code runs on a laptop as in the cluster.
	// Unset in production, where in-cluster discovery is the only path.
	YtdlAPIURL string
	// YtdlMusicClaim / YtdlMusicVideoClaim are the PVCs holding the operator's
	// two media libraries. A worker mounts exactly one of them; the server
	// container mounts neither (constitution v2.1.0).
	YtdlMusicClaim      string
	YtdlMusicVideoClaim string
	// YtdlUID / YtdlGID are the ownership the worker runs as, so saved files
	// are readable by the media server without permission repair.
	YtdlUID int64
	YtdlGID int64
	// YtdlDeadlineSeconds bounds a single download. Nothing may run forever.
	YtdlDeadlineSeconds int64
	// YtdlTTLSeconds is how long a finished job lingers before the cluster
	// sweeps it. It doubles as the visibility window for a SUCCESSFUL download;
	// failures are recorded durably and are unaffected by it.
	YtdlTTLSeconds int32
	// YtdlMinDurationSeconds separates a track from a clip in bulk runs. A
	// LOWER bound only — a long compilation is legitimate content.
	YtdlMinDurationSeconds int
	// YtdlMaxParallel bounds how many downloads run at once, instance-wide
	// rather than per user (spec 0013, FR-021/FR-022a) — so total load on the
	// cluster and on the source does not grow with the number of accounts.
	// Requests beyond it wait in a durable queue instead of piling onto the
	// cluster, which is what makes an uncapped channel expansion safe.
	YtdlMaxParallel int
}

// YtdlConfigured reports whether the operator has set this feature up. When it
// is false the endpoints answer 503 with an explanation rather than failing.
func (c Config) YtdlConfigured() bool {
	return c.YtdlImage != "" && (c.YtdlMusicClaim != "" || c.YtdlMusicVideoClaim != "")
}

// Load reads configuration from the environment, applying dev defaults and
// failing fast (with every missing variable listed) outside dev.
func Load() (Config, error) {
	cfg := Config{
		Env:                env("ENV", "dev"),
		Port:               os.Getenv("PORT"),
		AllowedOrigins:     splitComma(env("ALLOWED_ORIGINS", "http://localhost:5273,http://localhost:5274")),
		StaticDir:          os.Getenv("STATIC_DIR"),
		DevProxy:           os.Getenv("DEV_PROXY"),
		SynoURL:            os.Getenv("SYNO_URL"),
		SynoTLSInsecure:    envBool("SYNO_TLS_INSECURE", false),
		MaxTorrentMB:       envInt("MAX_TORRENT_MB", 16),
		UploadMaxMB:        envInt("UPLOAD_MAX_MB", 10240),
		AllowBrowserAccess: envBool("ALLOW_BROWSER_ACCESS", false),
		LoginPerMinute:     envInt("LOGIN_PER_MINUTE", 10),
		StreamMax:          envInt("STREAM_MAX_CONCURRENT", 64),
		DataDir:            env("DATA_DIR", "/data"),
		SecretsKey:         os.Getenv("SECRETS_KEY"),

		YtdlImage:              os.Getenv("YTDL_IMAGE"),
		YtdlNamespace:          os.Getenv("YTDL_NAMESPACE"),
		YtdlAPIURL:             os.Getenv("YTDL_API_URL"),
		YtdlOEmbedURL:          os.Getenv("YTDL_OEMBED_URL"),
		YtdlMusicClaim:         os.Getenv("YTDL_MUSIC_CLAIM"),
		YtdlMusicVideoClaim:    os.Getenv("YTDL_MUSIC_VIDEO_CLAIM"),
		YtdlUID:                int64(envInt("YTDL_UID", 1000)),
		YtdlGID:                int64(envInt("YTDL_GID", 1000)),
		YtdlDeadlineSeconds:    int64(envInt("YTDL_DEADLINE_SECONDS", 7200)),
		YtdlTTLSeconds:         int32(envInt("YTDL_TTL_SECONDS", 86400)),
		YtdlMinDurationSeconds: envInt("YTDL_MIN_DURATION_SECONDS", 90),
		YtdlMaxParallel:        ytdlMaxParallel(envInt("YTDL_MAX_PARALLEL", defaultYtdlMaxParallel)),
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
	// SYNO_URL is required only in legacy (stateless) mode. When SECRETS_KEY is
	// set the server runs stateful (spec 0003): the NAS connection comes from the
	// setup wizard, not the environment.
	if cfg.SecretsKey == "" && cfg.SynoURL == "" {
		missing = append(missing, "SYNO_URL")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	cfg.SynoURL = strings.TrimRight(cfg.SynoURL, "/")
	if cfg.UploadMaxMB < 1 {
		cfg.UploadMaxMB = 1
	}
	if cfg.MaxTorrentMB < 1 {
		cfg.MaxTorrentMB = 1
	}
	if cfg.LoginPerMinute < 1 {
		cfg.LoginPerMinute = 1
	}
	if cfg.StreamMax < 1 {
		cfg.StreamMax = 1
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

// defaultYtdlMaxParallel is four because that is what the feature was specified
// with; it is a knob rather than a constant so an operator with a bigger cluster
// — or a slower link — can say otherwise.
const defaultYtdlMaxParallel = 4

// ytdlMaxParallel refuses a nonsensical limit rather than honouring it. Zero or
// negative would mean "admit nothing", which reads as the feature being broken
// rather than as a configuration choice, and there is no way to tell the two
// apart from inside the app.
func ytdlMaxParallel(n int) int {
	if n < 1 {
		return defaultYtdlMaxParallel
	}
	return n
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
