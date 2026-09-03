//go:build !sourcemock

package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"synodl/server/internal/source"
)

// twoSites stands up a "main" and a "mirror", either of which can be made to
// fail, so failover can be exercised without depending on a real outage.
type twoSites struct {
	main, mirror      *httptest.Server
	mainDown          atomic.Bool
	mainHits, altHits atomic.Int32
}

func newTwoSites(t *testing.T) *twoSites {
	t.Helper()
	ts := &twoSites{}
	page := func(host *string, hits *atomic.Int32, down *atomic.Bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			if down != nil && down.Load() {
				// A server-side error is the address failing, which is the condition
				// that justifies trying the mirror.
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			base := *host
			_, _ = w.Write([]byte(`<script>var ajax_var = {"u":"7","logged":"1"};</script>` +
				`<div class="inner_item_body_widget">` +
				`<a class="bgbackitem" href="` + base + `/a-title-2026/" title="t 2026"><img src="` + base + `/p.jpg"></a>` +
				`<div class="item-foot-title"><h3 class="movie-title">A Title</h3>` +
				`<div class="score"><span class="year">2026</span><span class="rate">7.0<span class="ten">/10</span></span></div>` +
				`</div></div><a href="` + base + `/all-movie/page/3/">3</a>`))
		}
	}
	var mainURL, altURL string
	ts.main = httptest.NewServer(page(&mainURL, &ts.mainHits, &ts.mainDown))
	ts.mirror = httptest.NewServer(page(&altURL, &ts.altHits, nil))
	mainURL, altURL = ts.main.URL, ts.mirror.URL
	t.Cleanup(func() { ts.main.Close(); ts.mirror.Close() })

	old := zarBase
	zarBase = ts.main.URL
	t.Cleanup(func() { zarBase = old })
	ResetBasePrefs()
	return ts
}

func (ts *twoSites) cfg() source.Config {
	c := zarfilm{}.Hosts()
	c.APIHosts = []string{"127.0.0.1"}
	c.AltBase = ts.mirror.URL
	return c
}

func sess() source.Session {
	return source.Session{UserAgent: "t", Fields: map[string]string{zarFieldCookie: "wordpress_logged_in_x=v"}}
}

// FR-003: an availability failure on the main domain falls over to the mirror,
// and the user sees results rather than losing the source.
func TestFallsOverWhenMainDomainIsDown(t *testing.T) {
	ts := newTwoSites(t)
	ts.mainDown.Store(true)

	res, err := zarfilm{}.Search(context.Background(), source.NewClient(), ts.cfg(), sess(),
		source.SearchQuery{Page: 1})
	if err != nil {
		t.Fatalf("expected the mirror to serve the request: %v", err)
	}
	if len(res.Items) == 0 {
		t.Fatal("no results from the mirror")
	}
	if ts.altHits.Load() == 0 {
		t.Fatal("the mirror was never tried")
	}
}

// FR-006: the mirror's own links use the mirror's host, and titles browsed
// through it must still be addressable rather than discarded as off-site.
func TestMirrorLinksAreAccepted(t *testing.T) {
	ts := newTwoSites(t)
	ts.mainDown.Store(true)
	res, err := zarfilm{}.Search(context.Background(), source.NewClient(), ts.cfg(), sess(),
		source.SearchQuery{Page: 1})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Items) == 0 || res.Items[0].ID != "a-title-2026" {
		t.Fatalf("mirror-hosted link not parsed into a usable id: %+v", res.Items)
	}
}

// FR-004: being logged out is not an outage. Failing over would fail identically
// on the mirror and report the wrong cause.
func TestDoesNotFailOverOnAuthFailure(t *testing.T) {
	ts := newTwoSites(t)
	loggedOut := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.mainHits.Add(1)
		_, _ = w.Write([]byte(`<script>var ajax_var = {"u":"0","logged":""};</script>`))
	}))
	defer loggedOut.Close()
	zarBase = loggedOut.URL

	before := ts.altHits.Load()
	_, err := zarfilm{}.Search(context.Background(), source.NewClient(), ts.cfg(), sess(),
		source.SearchQuery{Page: 1})
	if _, ok := source.AsNeedsRefresh(err); !ok {
		t.Fatalf("expected a needs-refresh, got %v", err)
	}
	if ts.altHits.Load() != before {
		t.Fatal("an auth failure must not fail over to the mirror")
	}
}

// FR-007: once the mirror has answered, it is preferred briefly, so an outage
// doesn't make every later request pay for a failed attempt first.
func TestPrefersTheWorkingAddressAfterAFailover(t *testing.T) {
	ts := newTwoSites(t)
	ts.mainDown.Store(true)
	cfg := ts.cfg()

	drv := zarfilm{}
	if _, err := drv.Search(context.Background(), source.NewClient(), cfg, sess(),
		source.SearchQuery{Page: 1}); err != nil {
		t.Fatalf("first search: %v", err)
	}
	hitsAfterFirst := ts.mainHits.Load()

	for i := 0; i < 3; i += 1 {
		if _, err := drv.Search(context.Background(), source.NewClient(), cfg, sess(),
			source.SearchQuery{Page: 1}); err != nil {
			t.Fatalf("search %d: %v", i, err)
		}
	}
	if ts.mainHits.Load() != hitsAfterFirst {
		t.Fatalf("kept retrying the dead address: %d extra attempts",
			ts.mainHits.Load()-hitsAfterFirst)
	}

	// ...and recovery needs no operator action: once the memory is cleared the
	// main domain is used again.
	ResetBasePrefs()
	ts.mainDown.Store(false)
	if _, err := drv.Search(context.Background(), source.NewClient(), cfg, sess(),
		source.SearchQuery{Page: 1}); err != nil {
		t.Fatalf("recovery search: %v", err)
	}
	if ts.mainHits.Load() <= hitsAfterFirst {
		t.Fatal("main domain was never retried after recovery")
	}
}

// SC-004: a source with no alternate behaves exactly as before.
func TestNoAlternateConfiguredIsUnchanged(t *testing.T) {
	ts := newTwoSites(t)
	cfg := ts.cfg()
	cfg.AltBase = ""
	ts.mainDown.Store(true)

	_, err := zarfilm{}.Search(context.Background(), source.NewClient(), cfg, sess(),
		source.SearchQuery{Page: 1})
	if err == nil {
		t.Fatal("with no alternate, a dead main domain must surface as an error")
	}
	if ts.altHits.Load() != 0 {
		t.Fatal("nothing should have been sent to the mirror")
	}
}

// FR-005 / SC-001: both addresses down reports the source unavailable once.
func TestBothAddressesDown(t *testing.T) {
	ts := newTwoSites(t)
	ts.mainDown.Store(true)
	ts.mirror.Close()

	_, err := zarfilm{}.Search(context.Background(), source.NewClient(), ts.cfg(), sess(),
		source.SearchQuery{Page: 1})
	if err == nil {
		t.Fatal("expected an error when neither address answers")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unavailable") {
		t.Fatalf("expected an availability error, got %v", err)
	}
}
