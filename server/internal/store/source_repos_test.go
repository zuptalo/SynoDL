package store

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSourceProviderConfigRoundTrip(t *testing.T) {
	s := openTestStore(t)

	// No provider yet.
	if p, err := s.GetProvider(); err != nil || p != nil {
		t.Fatalf("GetProvider empty = %v, %v; want nil, nil", p, err)
	}

	id, err := s.SaveProviderConfig(SourceProvider{
		Kind:          "30nama",
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
	if got.Kind != "30nama" || got.DisplayName != "30nama" ||
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
		Kind: "30nama", DisplayName: "30nama HD", MoviesParent: "/media/movies",
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
	id, _ := s.SaveProviderConfig(SourceProvider{Kind: "30nama"}, 1)

	sess := SourceSession{
		UserAgent: "Mozilla/5.0 test",
		Fields: map[string]string{
			"cf_clearance":  "CFCLEAR-secret-value",
			"c_api_key":     "APIKEY-secret",
			"c_token":       "TOKEN-secret",
			"c_platform":    "PWA",
			"c_app_version": "1.2.3",
		},
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
	if got.Get("cf_clearance") != sess.Get("cf_clearance") || got.Get("c_token") != sess.Get("c_token") ||
		got.Get("c_api_key") != sess.Get("c_api_key") || got.UserAgent != sess.UserAgent {
		t.Fatalf("session round-trip mismatch: %+v", got)
	}
}

func TestSourceProviderStateTransitionsAndDelete(t *testing.T) {
	s := openTestStore(t)
	id, _ := s.SaveProviderConfig(SourceProvider{Kind: "30nama", State: SourceNotConfigured}, 1)

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
	_ = s.SaveProviderSession(id, SourceSession{Fields: map[string]string{"c_token": "x"}}, 1)
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

// The 30nama driver's registry key changed from "thirtynama" (migration 0019).
// `kind` is how a stored source finds its driver, so a row left on the old key
// would find none: the operator's configured source goes dark, its sealed
// session intact but unusable. This exercises the shipped migration by rolling
// the schema version back one and reopening, the way an upgrade actually runs.
func TestMigrationRewritesLegacy30namaKind(t *testing.T) {
	c, err := NewCipher("kdf-input-for-tests")
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	path := filepath.Join(t.TempDir(), "synodl.db")
	s, err := Open(path, c)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := s.db.Exec(
		`INSERT INTO source_providers (kind, display_name, enabled, state, created_at, updated_at)
		 VALUES ('thirtynama', 'Legacy Source', 1, 'active', 0, 0)`); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	// Rewind to just before the rename itself, then reopen so it runs.
	//
	// Found by searching for the statement rather than by counting from the end:
	// this used to rewind "the last migration", which quietly stopped exercising
	// the rename the moment anything was appended after it.
	rename := -1
	for i, m := range migrations {
		if strings.Contains(m, "thirtynama") {
			rename = i
			break
		}
	}
	if rename < 0 {
		t.Fatal("the rename migration is gone; this test no longer tests anything")
	}
	if _, err := s.db.Exec(`DELETE FROM schema_migrations WHERE version > ?`, rename); err != nil {
		t.Fatalf("rewind schema version: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := Open(path, c)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })

	providers, err := s2.ListProviders()
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(providers) != 1 {
		t.Fatalf("got %d providers, want 1", len(providers))
	}
	if providers[0].Kind != "30nama" {
		t.Errorf("kind = %q, want 30nama — the stored source would find no driver", providers[0].Kind)
	}
	if providers[0].DisplayName != "Legacy Source" {
		t.Errorf("the migration disturbed the row: %+v", providers[0])
	}
}

// Spec 0009: the alternate address's material rides inside the same sealed
// payload. An older payload has none, which must read back as "one set for both"
// so nothing an operator pasted before needs pasting again (FR-011).
func TestProviderSessionCarriesAlternateMaterial(t *testing.T) {
	st := openTestStore(t)

	id, err := st.CreateProvider(SourceProvider{Kind: "k", DisplayName: "S", Enabled: true}, 1)
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	alt := SourceSession{Fields: map[string]string{"cf_clearance": "alt-value"}, UserAgent: "alt-ua"}
	if err := st.SaveProviderSession(id, SourceSession{
		Fields: map[string]string{"cf_clearance": "main-value"}, UserAgent: "main-ua", Alt: &alt,
	}, 1); err != nil {
		t.Fatalf("SaveProviderSession: %v", err)
	}

	got, err := st.LoadProviderSession(id)
	if err != nil || got == nil {
		t.Fatalf("LoadProviderSession: %v", err)
	}
	if got.Get("cf_clearance") != "main-value" {
		t.Errorf("main material = %q", got.Get("cf_clearance"))
	}
	if got.Alt == nil || got.Alt.Get("cf_clearance") != "alt-value" {
		t.Fatalf("alternate material did not round-trip: %+v", got.Alt)
	}
	if got.ForAlt().Get("cf_clearance") != "alt-value" {
		t.Error("ForAlt must return the alternate's own material")
	}
}

// A source stored before this change has one set, used for both addresses.
func TestProviderSessionWithoutAlternateFallsBackToTheOneSet(t *testing.T) {
	st := openTestStore(t)

	id, err := st.CreateProvider(SourceProvider{Kind: "k", DisplayName: "S", Enabled: true}, 1)
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	if err := st.SaveProviderSession(id, SourceSession{
		Fields: map[string]string{"cf_clearance": "the-only-one"},
	}, 1); err != nil {
		t.Fatalf("SaveProviderSession: %v", err)
	}
	got, _ := st.LoadProviderSession(id)
	if got.Alt != nil {
		t.Fatal("no alternate material was stored")
	}
	if got.ForAlt().Get("cf_clearance") != "the-only-one" {
		t.Error("the single set must serve the alternate address too")
	}
}

// Both addresses are plain configuration and must round-trip.
func TestProviderStoresBothAddresses(t *testing.T) {
	st := openTestStore(t)

	id, err := st.CreateProvider(SourceProvider{
		Kind: "k", DisplayName: "S", Enabled: true,
		MainBase: "https://main.example", AltBase: "https://mirror.example",
	}, 1)
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	p, err := st.GetProviderByID(id)
	if err != nil || p == nil {
		t.Fatalf("GetProviderByID: %v", err)
	}
	if p.MainBase != "https://main.example" || p.AltBase != "https://mirror.example" {
		t.Fatalf("addresses did not round-trip: %+v", p)
	}
}

// --- spec 1029: forgetting content that has left the NAS -------------------

func TestSourceDownloadMissingMarkRoundTrips(t *testing.T) {
	s := openTestStore(t)
	if err := s.SaveSourceDownload(SourceDownload{
		Destination: "movie/Gone", Title: "Gone", QualityResolution: "1080p", QualityEncoder: "Pahe",
	}, 1); err != nil {
		t.Fatalf("SaveSourceDownload: %v", err)
	}

	all, err := s.SourceDownloads()
	if err != nil {
		t.Fatalf("SourceDownloads: %v", err)
	}
	if got := all["movie/Gone"].MissingSince; !got.IsZero() {
		t.Fatalf("a fresh record is not missing, got %v", got)
	}

	at := time.Unix(1_700_000_000, 0)
	if err := s.MarkSourceDownloadMissing("movie/Gone", at); err != nil {
		t.Fatalf("MarkSourceDownloadMissing: %v", err)
	}
	all, _ = s.SourceDownloads()
	if got := all["movie/Gone"].MissingSince; !got.Equal(at) {
		t.Errorf("missingSince = %v, want %v", got, at)
	}

	// Marking again must NOT move the timestamp: the grace period runs from when
	// the folder was FIRST seen gone, and re-stamping it every cycle would mean
	// the record is never old enough to delete.
	if err := s.MarkSourceDownloadMissing("movie/Gone", at.Add(time.Hour)); err != nil {
		t.Fatalf("re-mark: %v", err)
	}
	all, _ = s.SourceDownloads()
	if got := all["movie/Gone"].MissingSince; !got.Equal(at) {
		t.Errorf("re-marking moved the clock to %v; the grace period would never elapse", got)
	}

	if err := s.ClearSourceDownloadMissing("movie/Gone"); err != nil {
		t.Fatalf("ClearSourceDownloadMissing: %v", err)
	}
	all, _ = s.SourceDownloads()
	if got := all["movie/Gone"].MissingSince; !got.IsZero() {
		t.Errorf("a restored folder should clear the mark, got %v", got)
	}
}

// A re-send of a title that had been marked missing must clear the mark: the
// content is on its way back, and leaving the mark would delete the record we
// just wrote.
func TestSavingASourceDownloadClearsTheMissingMark(t *testing.T) {
	s := openTestStore(t)
	rec := SourceDownload{Destination: "movie/Back", Title: "Back"}
	if err := s.SaveSourceDownload(rec, 1); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := s.MarkSourceDownloadMissing("movie/Back", time.Unix(1_700_000_000, 0)); err != nil {
		t.Fatalf("mark: %v", err)
	}
	if err := s.SaveSourceDownload(rec, 2); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	all, _ := s.SourceDownloads()
	if got := all["movie/Back"].MissingSince; !got.IsZero() {
		t.Errorf("re-sending a title left it marked missing (%v); its record would then be deleted", got)
	}
}

func TestDeleteSourceDownload(t *testing.T) {
	s := openTestStore(t)
	for _, d := range []string{"movie/Keep", "movie/Drop"} {
		if err := s.SaveSourceDownload(SourceDownload{Destination: d, Title: d}, 1); err != nil {
			t.Fatalf("save %s: %v", d, err)
		}
	}
	if err := s.DeleteSourceDownload("movie/Drop"); err != nil {
		t.Fatalf("DeleteSourceDownload: %v", err)
	}
	all, _ := s.SourceDownloads()
	if _, still := all["movie/Drop"]; still {
		t.Error("the deleted record is still there")
	}
	if _, gone := all["movie/Keep"]; !gone {
		t.Error("deleting one record removed another")
	}
}

// The per-user history is an append-only statistics and quota log. Removing
// content from the NAS must not rewrite what a user downloaded, or how much of
// their allowance they used.
func TestForgettingADownloadLeavesTheHistoryAlone(t *testing.T) {
	s := openTestStore(t)
	uid, err := s.CreateUser("u", "h", true)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := s.SaveSourceDownload(SourceDownload{Destination: "movie/Drop", Title: "Drop"}, 1); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := s.AddDownloadHistory(DownloadHistory{
		UserID: uid, Source: SourceCatalog, Category: CategoryMovie,
		Destination: "movie/Drop", TaskName: "Drop.2024.1080p.mkv", CreatedAt: 1,
	}); err != nil {
		t.Fatalf("AddDownloadHistory: %v", err)
	}

	if err := s.DeleteSourceDownload("movie/Drop"); err != nil {
		t.Fatalf("DeleteSourceDownload: %v", err)
	}
	names, err := s.RecordedNamesFor("movie/Drop")
	if err != nil {
		t.Fatalf("RecordedNamesFor: %v", err)
	}
	if len(names) != 1 {
		t.Errorf("history rows = %d, want 1: forgetting content must not rewrite the statistics log", len(names))
	}
}
