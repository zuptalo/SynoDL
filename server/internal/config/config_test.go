package config

import (
	"strings"
	"testing"
)

// clearEnv resets every variable Load reads so tests are hermetic regardless
// of the invoking shell.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"ENV", "PORT", "ALLOWED_ORIGINS", "STATIC_DIR", "DEV_PROXY",
		"SYNO_URL", "SYNO_TLS_INSECURE", "MAX_TORRENT_MB", "LOGIN_PER_MINUTE",
		"YTDL_POT_PROVIDER_URL",
	} {
		t.Setenv(k, "")
	}
}

func TestLoad_YtdlPOTProviderURL(t *testing.T) {
	clearEnv(t)
	t.Setenv("YTDL_POT_PROVIDER_URL", "  http://bgutil.ytdlp.svc:4416  ")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.YtdlPOTProviderURL, "http://bgutil.ytdlp.svc:4416"; got != want {
		t.Errorf("YtdlPOTProviderURL = %q, want %q", got, want)
	}
}

func TestLoadDevDefaults(t *testing.T) {
	clearEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Env != "dev" {
		t.Errorf("Env = %q, want dev", cfg.Env)
	}
	if cfg.Port != "8280" {
		t.Errorf("Port = %q, want the dev-block default 8280", cfg.Port)
	}
	if cfg.SynoURL != "http://localhost:8291" {
		t.Errorf("SynoURL = %q, want mock default", cfg.SynoURL)
	}
	if cfg.SynoTLSInsecure {
		t.Error("SynoTLSInsecure must default to false")
	}
	if cfg.MaxTorrentMB != 16 {
		t.Errorf("MaxTorrentMB = %d, want 16", cfg.MaxTorrentMB)
	}
	if cfg.LoginPerMinute != 10 {
		t.Errorf("LoginPerMinute = %d, want 10", cfg.LoginPerMinute)
	}
	if cfg.StreamMax != 64 {
		t.Errorf("StreamMax = %d, want 64", cfg.StreamMax)
	}
	if len(cfg.AllowedOrigins) != 2 || cfg.AllowedOrigins[0] != "http://localhost:5273" {
		t.Errorf("AllowedOrigins = %v, want the two Vite dev origins", cfg.AllowedOrigins)
	}
}

func TestLoadProductionPortDefault(t *testing.T) {
	clearEnv(t)
	t.Setenv("ENV", "production")
	t.Setenv("SYNO_URL", "https://nas:5001")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want container-conventional 8080 in production", cfg.Port)
	}
}

func TestLoadProductionRequiresSynoURL(t *testing.T) {
	clearEnv(t)
	t.Setenv("ENV", "production")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing SYNO_URL in production")
	}
	if !strings.Contains(err.Error(), "SYNO_URL") {
		t.Errorf("error %q should name SYNO_URL", err)
	}
}

func TestLoadProductionComplete(t *testing.T) {
	clearEnv(t)
	t.Setenv("ENV", "production")
	t.Setenv("SYNO_URL", "https://nas.local:5001/")
	t.Setenv("SYNO_TLS_INSECURE", "true")
	t.Setenv("MAX_TORRENT_MB", "32")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SynoURL != "https://nas.local:5001" {
		t.Errorf("SynoURL = %q, want trailing slash trimmed", cfg.SynoURL)
	}
	if !cfg.SynoTLSInsecure {
		t.Error("SynoTLSInsecure = false, want true")
	}
	if cfg.MaxTorrentMB != 32 {
		t.Errorf("MaxTorrentMB = %d, want 32", cfg.MaxTorrentMB)
	}
	// DataDir defaults to /data when unset (spec 0003).
	if cfg.DataDir != "/data" {
		t.Errorf("DataDir = %q, want /data (default)", cfg.DataDir)
	}
}

func TestLoadStatefulEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("ENV", "production")
	t.Setenv("SYNO_URL", "https://nas:5001")
	t.Setenv("DATA_DIR", "/var/lib/synodl")
	t.Setenv("SECRETS_KEY", "kdf-input-for-tests")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DataDir != "/var/lib/synodl" {
		t.Errorf("DataDir = %q, want the DATA_DIR override", cfg.DataDir)
	}
	if cfg.SecretsKey != "kdf-input-for-tests" {
		t.Errorf("SecretsKey not loaded from SECRETS_KEY")
	}
}

func TestLoadClampsAndIgnoresGarbage(t *testing.T) {
	clearEnv(t)
	t.Setenv("SYNO_URL", "http://mock:8091")
	t.Setenv("MAX_TORRENT_MB", "0")
	t.Setenv("LOGIN_PER_MINUTE", "-5")
	t.Setenv("STREAM_MAX_CONCURRENT", "0")
	t.Setenv("SYNO_TLS_INSECURE", "banana")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MaxTorrentMB != 1 {
		t.Errorf("MaxTorrentMB = %d, want floor of 1", cfg.MaxTorrentMB)
	}
	if cfg.LoginPerMinute != 1 {
		t.Errorf("LoginPerMinute = %d, want floor of 1", cfg.LoginPerMinute)
	}
	if cfg.StreamMax != 1 {
		t.Errorf("StreamMax = %d, want floor of 1", cfg.StreamMax)
	}
	if cfg.SynoTLSInsecure {
		t.Error("unparseable SYNO_TLS_INSECURE must fall back to false")
	}
}

func TestSplitComma(t *testing.T) {
	got := splitComma(" a, b ,,c ")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("splitComma = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("splitComma[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// The install gate is lifted only by an operator turning it on deliberately, so
// the default has to be off and the parsing has to be unambiguous.
func TestAllowBrowserAccess(t *testing.T) {
	clearEnv(t)
	t.Setenv("SYNO_URL", "https://nas:5001")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AllowBrowserAccess {
		t.Error("AllowBrowserAccess defaults to true; the gate must be on unless asked for")
	}

	t.Setenv("ALLOW_BROWSER_ACCESS", "true")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.AllowBrowserAccess {
		t.Error(`ALLOW_BROWSER_ACCESS="true" did not lift the gate`)
	}

	// Anything that is not a true value leaves the gate up, rather than being
	// read as "set, therefore on".
	for _, v := range []string{"false", "0", "no", "", "maybe"} {
		t.Setenv("ALLOW_BROWSER_ACCESS", v)
		cfg, err = Load()
		if err != nil {
			t.Fatalf("Load(%q): %v", v, err)
		}
		if cfg.AllowBrowserAccess {
			t.Errorf("ALLOW_BROWSER_ACCESS=%q lifted the gate", v)
		}
	}
}

// The parallel limit is the one ytdl knob an operator is likely to touch, so it
// gets the same treatment as the other bounds: a default that works, an override
// that is honoured, and nonsense that is refused rather than obeyed (spec 0013).
func TestLoad_YtdlMaxParallel(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  string
		want int
	}{
		{name: "unset falls back to four", set: "", want: 4},
		{name: "operator override is honoured", set: "8", want: 8},
		{name: "one is a legitimate choice", set: "1", want: 1},
		{name: "zero would admit nothing, so it is refused", set: "0", want: 4},
		{name: "negative is refused", set: "-3", want: 4},
		{name: "non-numeric falls back", set: "lots", want: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set == "" {
				t.Setenv("YTDL_MAX_PARALLEL", "")
			} else {
				t.Setenv("YTDL_MAX_PARALLEL", tc.set)
			}
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.YtdlMaxParallel != tc.want {
				t.Fatalf("YtdlMaxParallel = %d, want %d", cfg.YtdlMaxParallel, tc.want)
			}
		})
	}
}

// Automatic retry of a refused YouTube download (spec 1043) is on by default,
// with a cool-down worth waiting for, and an operator can switch it off.
func TestLoad_YtdlAutoRetry(t *testing.T) {
	t.Setenv("YTDL_AUTO_RETRY_AFTER_SECONDS", "")
	t.Setenv("YTDL_AUTO_RETRY_MAX_ATTEMPTS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.YtdlAutoRetryAfterSeconds != 30 || cfg.YtdlAutoRetryMaxAttempts != 3 {
		t.Fatalf("defaults = %ds / %d attempts, want 30s / 3", cfg.YtdlAutoRetryAfterSeconds, cfg.YtdlAutoRetryMaxAttempts)
	}

	t.Setenv("YTDL_AUTO_RETRY_AFTER_SECONDS", "600")
	t.Setenv("YTDL_AUTO_RETRY_MAX_ATTEMPTS", "1")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.YtdlAutoRetryAfterSeconds != 600 || cfg.YtdlAutoRetryMaxAttempts != 1 {
		t.Fatalf("override = %ds / %d attempts, want 600s / 1 (off)", cfg.YtdlAutoRetryAfterSeconds, cfg.YtdlAutoRetryMaxAttempts)
	}
}
