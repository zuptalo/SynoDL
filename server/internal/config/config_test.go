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
	} {
		t.Setenv(k, "")
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
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.SynoURL != "http://localhost:8091" {
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
	if len(cfg.AllowedOrigins) != 2 || cfg.AllowedOrigins[0] != "http://localhost:5173" {
		t.Errorf("AllowedOrigins = %v, want the two Vite dev origins", cfg.AllowedOrigins)
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
}

func TestLoadClampsAndIgnoresGarbage(t *testing.T) {
	clearEnv(t)
	t.Setenv("SYNO_URL", "http://mock:8091")
	t.Setenv("MAX_TORRENT_MB", "0")
	t.Setenv("LOGIN_PER_MINUTE", "-5")
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
