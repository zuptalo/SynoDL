package providers

import (
	"context"
	"os"
	"testing"

	"synodl/server/internal/source"
)

// TestLiveThirtynama exercises the real driver against the real provider using a
// session supplied via LIVE_* env vars. It SKIPS unless LIVE_CF is set, so it
// never runs in CI — it's a hands-on integration check for when you have a live
// session (and the same public IP the clearance cookie was minted on).
//
//	LIVE_CF=… LIVE_TOKEN=… LIVE_APIKEY=… LIVE_UA=… LIVE_PLATFORM=… LIVE_APPVER=… \
//	  go test ./internal/source/providers/ -run TestLiveThirtynama -v
func TestLiveThirtynama(t *testing.T) {
	cf := os.Getenv("LIVE_CF")
	if cf == "" {
		t.Skip("no LIVE_CF — skipping live provider test")
	}
	s := source.Session{
		UserAgent: os.Getenv("LIVE_UA"),
		Fields: map[string]string{
			"cf_clearance":  cf,
			"c_token":       os.Getenv("LIVE_TOKEN"),
			"c_api_key":     os.Getenv("LIVE_APIKEY"),
			"c_platform":    os.Getenv("LIVE_PLATFORM"),
			"c_app_version": os.Getenv("LIVE_APPVER"),
		},
	}
	cfg := thirtynama{}.Hosts()
	c := source.NewClient()
	p := thirtynama{}
	ctx := context.Background()

	if err := p.VerifySession(ctx, c, cfg, s); err != nil {
		t.Fatalf("verify: %v", err)
	}

	// Browsing must work across pages (regression: a coverless post on a later
	// page used to fail the parse and trip a false "needs refresh").
	for _, page := range []int{1, 2, 3} {
		res, err := p.Search(ctx, c, cfg, s, source.SearchQuery{Page: page})
		if err != nil {
			t.Fatalf("browse page %d: %v", page, err)
		}
		if len(res.Items) == 0 {
			t.Fatalf("browse page %d returned no items", page)
		}
		t.Logf("browse page %d: %d items (of %d pages)", page, len(res.Items), res.Pages)
	}

	// Text search must return results (regression: full_search nests its posts).
	rq, err := p.Search(ctx, c, cfg, s, source.SearchQuery{Query: "matrix"})
	if err != nil {
		t.Fatalf("text search: %v", err)
	}
	if len(rq.Items) == 0 {
		t.Fatal("text search 'matrix' returned no items")
	}
	t.Logf("text search 'matrix': %d items", len(rq.Items))
}

// TestLiveZarfilm exercises the zarfilm driver against the REAL site using a
// session supplied via env vars. It SKIPS unless LIVE_ZAR_COOKIE is set, so it
// never runs in CI — no credentials there, and no stable public address.
//
//	LIVE_ZAR_COOKIE='wordpress_logged_in_xxx=yyy' LIVE_ZAR_VARY='...' LIVE_ZAR_UA='...' \
//	  go test ./internal/source/providers/ -run TestLiveZarfilm -v
//
// This is the check that catches the site changing its markup, which unit tests
// against captured fixtures cannot.
func TestLiveZarfilm(t *testing.T) {
	cookie := os.Getenv("LIVE_ZAR_COOKIE")
	if cookie == "" {
		t.Skip("no LIVE_ZAR_COOKIE — skipping live zarfilm test")
	}
	s := source.Session{
		UserAgent: os.Getenv("LIVE_ZAR_UA"),
		Fields: map[string]string{
			zarFieldCookie: cookie,
			zarFieldVary:   os.Getenv("LIVE_ZAR_VARY"),
		},
	}
	p := zarfilm{}
	cfg := p.Hosts()
	c := source.NewClient()
	ctx := context.Background()

	if err := p.VerifySession(ctx, c, cfg, s); err != nil {
		t.Fatalf("verify: %v", err)
	}

	// Browse several pages: a later page with an unusual card used to be where a
	// listing parser first fell over.
	var sample string
	for _, page := range []int{1, 2, 3} {
		res, err := p.Search(ctx, c, cfg, s, source.SearchQuery{Page: page})
		if err != nil {
			t.Fatalf("browse page %d: %v", page, err)
		}
		if len(res.Items) == 0 {
			t.Fatalf("browse page %d returned no items", page)
		}
		if sample == "" {
			sample = res.Items[0].ID
		}
		t.Logf("browse page %d: %d items (of %d pages)", page, len(res.Items), res.Pages)
	}

	if res, err := p.Search(ctx, c, cfg, s, source.SearchQuery{Query: "the"}); err != nil {
		t.Fatalf("text search: %v", err)
	} else {
		t.Logf("text search: %d items", len(res.Items))
	}

	td, err := p.Title(ctx, c, cfg, s, sample)
	if err != nil {
		t.Fatalf("title %s: %v", sample, err)
	}
	if len(td.Qualities) == 0 {
		t.Fatalf("title %s has no qualities — entitlement or markup drift", sample)
	}
	t.Logf("title %s: %d qualities", sample, len(td.Qualities))

	links, size, err := p.ResolveDownload(ctx, c, cfg, s, sample, td.Qualities[0].ID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	// Never log the link itself: it embeds the account id and grants
	// unauthenticated access until it expires.
	t.Logf("resolved %d link(s), size %s", len(links), size)
}
