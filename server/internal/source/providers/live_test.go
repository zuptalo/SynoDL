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
		CFClearance: cf,
		CToken:      os.Getenv("LIVE_TOKEN"),
		CAPIKey:     os.Getenv("LIVE_APIKEY"),
		UserAgent:   os.Getenv("LIVE_UA"),
		CPlatform:   os.Getenv("LIVE_PLATFORM"),
		CAppVersion: os.Getenv("LIVE_APPVER"),
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
