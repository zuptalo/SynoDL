package store

import (
	"bytes"
	"testing"
)

func TestSourceProviderConfigRoundTrip(t *testing.T) {
	s := openTestStore(t)

	// No provider yet.
	if p, err := s.GetProvider(); err != nil || p != nil {
		t.Fatalf("GetProvider empty = %v, %v; want nil, nil", p, err)
	}

	id, err := s.SaveProviderConfig(SourceProvider{
		Kind:          "thirtynama",
		DisplayName:   "30nama",
		APIHosts:      []string{"interface.30nama.com", "30nama.com"},
		DownloadHosts: []string{"divyacamilla.info"},
		MoviesParent:  "/movies",
		TVParent:      "/tv",
		Enabled:       true,
		State:         SourceActive,
	}, 100)
	if err != nil {
		t.Fatalf("SaveProviderConfig: %v", err)
	}
	if id == 0 {
		t.Fatal("want non-zero provider id")
	}

	got, err := s.GetProvider()
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if got.Kind != "thirtynama" || got.DisplayName != "30nama" ||
		got.MoviesParent != "/movies" || got.TVParent != "/tv" || !got.Enabled {
		t.Fatalf("provider mismatch: %+v", got)
	}
	if len(got.APIHosts) != 2 || got.APIHosts[0] != "interface.30nama.com" {
		t.Fatalf("api hosts = %v", got.APIHosts)
	}
	if len(got.DownloadHosts) != 1 || got.DownloadHosts[0] != "divyacamilla.info" {
		t.Fatalf("download hosts = %v", got.DownloadHosts)
	}

	// Update is an upsert on the singleton (same id, new values).
	id2, err := s.SaveProviderConfig(SourceProvider{
		Kind: "thirtynama", DisplayName: "30nama HD", MoviesParent: "/media/movies",
	}, 200)
	if err != nil {
		t.Fatalf("SaveProviderConfig update: %v", err)
	}
	if id2 != id {
		t.Fatalf("singleton id changed: %d -> %d", id, id2)
	}
	got2, _ := s.GetProvider()
	if got2.DisplayName != "30nama HD" || got2.MoviesParent != "/media/movies" {
		t.Fatalf("update not applied: %+v", got2)
	}
}

func TestSourceSessionSealedAndWriteOnly(t *testing.T) {
	s := openTestStore(t)
	id, _ := s.SaveProviderConfig(SourceProvider{Kind: "thirtynama"}, 1)

	sess := SourceSession{
		CFClearance: "CFCLEAR-secret-value",
		CAPIKey:     "APIKEY-secret",
		CToken:      "TOKEN-secret",
		UserAgent:   "Mozilla/5.0 test",
		CPlatform:   "PWA",
		CAppVersion: "1.2.3",
	}
	if err := s.SaveProviderSession(id, sess, 10); err != nil {
		t.Fatalf("SaveProviderSession: %v", err)
	}

	// The stored blob must be ciphertext — none of the secret values appear.
	var sealed []byte
	if err := s.DB().QueryRow(
		`SELECT session_enc FROM source_provider_secrets WHERE provider_id=?`, id).
		Scan(&sealed); err != nil {
		t.Fatalf("read blob: %v", err)
	}
	for _, secret := range [][]byte{
		[]byte("CFCLEAR-secret-value"), []byte("APIKEY-secret"), []byte("TOKEN-secret"),
	} {
		if bytes.Contains(sealed, secret) {
			t.Fatalf("plaintext secret %q found in stored blob", secret)
		}
	}

	// Open recovers it in-process.
	got, err := s.LoadProviderSession(id)
	if err != nil {
		t.Fatalf("LoadProviderSession: %v", err)
	}
	if got.CFClearance != sess.CFClearance || got.CToken != sess.CToken ||
		got.CAPIKey != sess.CAPIKey || got.UserAgent != sess.UserAgent {
		t.Fatalf("session round-trip mismatch: %+v", got)
	}
}

func TestSourceProviderStateTransitionsAndDelete(t *testing.T) {
	s := openTestStore(t)
	id, _ := s.SaveProviderConfig(SourceProvider{Kind: "thirtynama", State: SourceNotConfigured}, 1)

	if err := s.SetProviderState(id, SourceActive, 500, 500); err != nil {
		t.Fatalf("SetProviderState active: %v", err)
	}
	p, _ := s.GetProvider()
	if p.State != SourceActive || p.LastVerifiedAt != 500 {
		t.Fatalf("state after activate: %+v", p)
	}

	// needs_refresh without a verify time must not clobber last_verified_at.
	if err := s.SetProviderState(id, SourceNeedsRefresh, 0, 600); err != nil {
		t.Fatalf("SetProviderState needs_refresh: %v", err)
	}
	p, _ = s.GetProvider()
	if p.State != SourceNeedsRefresh || p.LastVerifiedAt != 500 {
		t.Fatalf("state after needs_refresh: %+v", p)
	}

	// Delete cascades secrets.
	_ = s.SaveProviderSession(id, SourceSession{CToken: "x"}, 1)
	if err := s.DeleteProvider(id); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
	if p, _ := s.GetProvider(); p != nil {
		t.Fatalf("provider still present after delete: %+v", p)
	}
	var n int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM source_provider_secrets WHERE provider_id=?`, id).Scan(&n)
	if n != 0 {
		t.Fatalf("secrets not cascaded on delete: %d rows", n)
	}
}

func TestSourcePrefRoundTrip(t *testing.T) {
	s := openTestStore(t)
	uid, err := s.CreateUser("mover", "hash", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if q, err := s.GetSourcePref(uid); err != nil || q != "" {
		t.Fatalf("empty pref = %q, %v; want \"\", nil", q, err)
	}
	if err := s.SaveSourcePref(uid, "1080p", 10); err != nil {
		t.Fatalf("SaveSourcePref: %v", err)
	}
	if q, _ := s.GetSourcePref(uid); q != "1080p" {
		t.Fatalf("pref = %q, want 1080p", q)
	}
	// Upsert.
	_ = s.SaveSourcePref(uid, "4K", 20)
	if q, _ := s.GetSourcePref(uid); q != "4K" {
		t.Fatalf("pref after update = %q, want 4K", q)
	}
}

func TestSourceViewRoundTrip(t *testing.T) {
	s := openTestStore(t)
	uid, err := s.CreateUser("viewer", "hash", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	// Seed a quality pref first so we can prove the view save doesn't clobber it.
	_ = s.SaveSourcePref(uid, "1080p", 5)

	if f, so, ord, err := s.GetSourceView(uid); err != nil || f != "" || so != "" || ord != "" {
		t.Fatalf("empty view = %q/%q/%q, %v", f, so, ord, err)
	}
	if err := s.SaveSourceView(uid, `{"type":"movie"}`, "favorite", "asc", 10); err != nil {
		t.Fatalf("SaveSourceView: %v", err)
	}
	f, so, ord, _ := s.GetSourceView(uid)
	if f != `{"type":"movie"}` || so != "favorite" || ord != "asc" {
		t.Fatalf("view = %q/%q/%q, want {\"type\":\"movie\"}/favorite/asc", f, so, ord)
	}
	// The quality pref on the same row survives a view update, and vice versa.
	if q, _ := s.GetSourcePref(uid); q != "1080p" {
		t.Fatalf("quality clobbered by view save: %q", q)
	}
	_ = s.SaveSourcePref(uid, "4K", 15)
	if f2, so2, ord2, _ := s.GetSourceView(uid); f2 != `{"type":"movie"}` || so2 != "favorite" || ord2 != "asc" {
		t.Fatalf("view clobbered by quality save: %q/%q/%q", f2, so2, ord2)
	}
}

// SaveSourceDownload/SourceDownloads must round-trip the catalog metadata,
// including the poster URL and catalog id added in spec 1016.
func TestSourceDownloadRoundTrip(t *testing.T) {
	s := openTestStore(t)

	in := SourceDownload{
		Destination: "/movies/Dexter Resurrection 2025/",
		MediaType:   "series",
		Title:       "Dexter Resurrection",
		Year:        "2025",
		IMDbScore:   9.0,
		PosterURL:   "https://cdn.example.info/poster/dexter-l.webp",
		CatalogID:   "716665",
	}
	if err := s.SaveSourceDownload(in, 1000); err != nil {
		t.Fatalf("SaveSourceDownload: %v", err)
	}

	all, err := s.SourceDownloads()
	if err != nil {
		t.Fatalf("SourceDownloads: %v", err)
	}
	// Keyed by the normalized destination (slashes trimmed).
	got, ok := all["movies/Dexter Resurrection 2025"]
	if !ok {
		t.Fatalf("row not found; keys=%v", keysOf(all))
	}
	if got.PosterURL != in.PosterURL {
		t.Fatalf("PosterURL = %q, want %q", got.PosterURL, in.PosterURL)
	}
	if got.CatalogID != in.CatalogID {
		t.Fatalf("CatalogID = %q, want %q", got.CatalogID, in.CatalogID)
	}
	if got.MediaType != "series" || got.Year != "2025" || got.IMDbScore != 9.0 {
		t.Fatalf("metadata mismatch: %+v", got)
	}

	// A re-send upserts the poster/catalog id too.
	in.PosterURL = "https://cdn.example.info/poster/dexter-new.webp"
	in.CatalogID = "999"
	if err := s.SaveSourceDownload(in, 2000); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	all, _ = s.SourceDownloads()
	if g := all["movies/Dexter Resurrection 2025"]; g.PosterURL != in.PosterURL || g.CatalogID != "999" {
		t.Fatalf("upsert not applied: %+v", g)
	}
}

func keysOf(m map[string]SourceDownload) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
