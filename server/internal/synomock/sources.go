package synomock

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// Fake download sources (spec 0007).
//
// The mock DSM already means dev and e2e never need real hardware. Sources are
// the second outbound target, and without a fake for them the whole catalog
// feature is only reachable by pasting real credentials for a real site — which
// CI cannot do and a contributor should not have to.
//
// So this serves two fake sites from the same mock binary, one in each shape the
// real drivers meet:
//
//	/mocksrc/zar/…   HTML, like a WordPress film site
//	/mocksrc/tn/…    JSON, like the other provider's API
//
// The markup and envelopes are deliberately the same SHAPE as the real ones —
// the same class names and nesting the parsers key on — so the drivers exercise
// their real parsing paths rather than a simplified one.

// SourceState drives the conditions that are otherwise hard to reach: an expired
// session, an account with no entitlement, a source that stalls, and a catalog
// that runs out sooner than the other one.
type SourceState struct {
	mu         sync.Mutex
	loggedOut  bool
	paywalled  bool
	pages      int
	perPage    int
	titlePrefx string
}

func newSourceState(prefix string) *SourceState {
	return &SourceState{pages: 5, perPage: 6, titlePrefx: prefix}
}

func (s *SourceState) snapshot() (loggedOut, paywalled bool, pages, perPage int, prefix string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loggedOut, s.paywalled, s.pages, s.perPage, s.titlePrefx
}

func (s *SourceState) set(f func(*SourceState)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(s)
}

// registerSources wires the fake sites and their control endpoints onto the mux.
func (s *Server) registerSources(mux *http.ServeMux) {
	mux.HandleFunc("/mocksrc/zar/", s.handleZarMock)
	mux.HandleFunc("/mocksrc/tn/", s.handleTNMock)
	mux.HandleFunc("/__mock/source/", s.handleSourceControl)
}

// handleSourceControl drives the fake sources' states, e.g.
//
//	POST /__mock/source/zar/logged-out
//	POST /__mock/source/zar/paywalled
//	POST /__mock/source/tn/pages?n=2
//	POST /__mock/source/reset
func (s *Server) handleSourceControl(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/__mock/source/")
	if rest == "reset" {
		s.zarSrc.set(func(st *SourceState) { st.loggedOut, st.paywalled, st.pages, st.perPage = false, false, 5, 6 })
		s.tnSrc.set(func(st *SourceState) { st.loggedOut, st.paywalled, st.pages, st.perPage = false, false, 5, 6 })
		w.WriteHeader(http.StatusNoContent)
		return
	}
	which, action, _ := strings.Cut(rest, "/")
	target := s.zarSrc
	if which == "tn" {
		target = s.tnSrc
	}
	switch action {
	case "logged-out":
		target.set(func(st *SourceState) { st.loggedOut = true })
	case "logged-in":
		target.set(func(st *SourceState) { st.loggedOut = false })
	case "paywalled":
		target.set(func(st *SourceState) { st.paywalled = true })
	case "entitled":
		target.set(func(st *SourceState) { st.paywalled = false })
	case "pages":
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		if n < 0 {
			n = 0
		}
		target.set(func(st *SourceState) { st.pages = n })
	default:
		http.Error(w, "unknown source control", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- the HTML-shaped fake site ----------

func (s *Server) handleZarMock(w http.ResponseWriter, r *http.Request) {
	loggedOut, paywalled, pages, perPage, prefix := s.zarSrc.snapshot()
	path := strings.TrimPrefix(r.URL.Path, "/mocksrc/zar")
	path = "/" + strings.Trim(path, "/")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Every page inlines the login flag the driver reads. A logged-out fake
	// reports u=0, exactly as the real site does.
	head := zarAjaxVar(!loggedOut)

	// "/series" is BOTH the series archive and the prefix every series title id
	// carries, so the archive has to be matched exactly rather than by prefix.
	// Matching it loosely made "/series/<slug>" serve a page of cards, which the
	// series parser reads as a title with no seasons — a fake site that could
	// never answer the one question a series page exists to answer.
	seriesArchive := path == "/series" || strings.HasPrefix(path, "/series/page/")

	switch {
	case strings.HasPrefix(path, "/all-movie"), path == "/", seriesArchive:
		page := pageFromPath(path)
		if page > pages {
			// Past the end: a real archive returns a page with no cards, which is how
			// the client learns a source is exhausted.
			fmt.Fprint(w, head+`<div class="posts_hoder_archive"></div>`)
			return
		}
		fmt.Fprint(w, head+zarListingHTML(zarMockBaseFor(r), prefix, page, perPage, pages, seriesArchive))
	default:
		// A title page.
		slug := strings.Trim(path, "/")
		fmt.Fprint(w, head+zarTitleHTML(zarMockBaseFor(r), slug, paywalled))
	}
}

func zarAjaxVar(loggedIn bool) string {
	u, logged := "0", ""
	if loggedIn {
		u, logged = "424242", "1"
	}
	return `<script>var ajax_var = {"ajaxurl":"/x","u":"` + u + `","logged":"` + logged + `"};</script>` +
		`<a href="https://www.imdb.com/title/tt1234567/">IMDb</a>`
}

func pageFromPath(path string) int {
	if i := strings.Index(path, "/page/"); i >= 0 {
		n, _ := strconv.Atoi(strings.Trim(strings.TrimPrefix(path[i:], "/page/"), "/"))
		if n > 0 {
			return n
		}
	}
	return 1
}

func zarListingHTML(base, prefix string, page, perPage, pages int, series bool) string {
	var b strings.Builder
	b.WriteString(`<div class="posts_hoder_archive">`)
	for i := 1; i <= perPage; i++ {
		n := (page-1)*perPage + i
		slug := fmt.Sprintf("%s-title-%d", prefix, n)
		// /series is all series; the DEFAULT browse mixes them in, every third
		// item. A catalog of nothing but movies cannot exercise anything
		// series-shaped — season packs, the episode picker, or which seasons are
		// already on the NAS — and a test that cannot find a series does not fail,
		// it skips. That is the worst outcome: coverage that reports success while
		// asserting nothing.
		if series || n%3 == 0 {
			slug = "series/" + slug
		}
		fmt.Fprintf(&b, `
<div class="inner_item_body_widget">
  <a class="bgbackitem" href="%s/%s/" title="Mock %s %d 20%02d">
    <div class="genres_links"><h3><span>Drama</span></h3><h3><span>Action</span></h3></div>
    <img src="%s/poster-%d.jpg">
  </a>
  <div class="item-foot-title">
    <h3 class="movie-title">%s Title %d</h3>
    <div class="score"><span class="year">20%02d</span><span class="rate">%d.%d<span class="ten">/10</span></span></div>
  </div>
</div>`, base, slug, prefix, n, 10+n%15, base, n,
			strings.ToUpper(prefix[:1])+prefix[1:], n, 10+n%15, 5+n%5, n%10)
	}
	b.WriteString(`</div>`)
	// Pagination, which is where the driver reads how deep the catalog goes.
	for p := 1; p <= pages; p++ {
		fmt.Fprintf(&b, `<a href="%s/all-movie/page/%d/">%d</a>`, base, p, p)
	}
	return b.String()
}

// zarMockBaseFor builds the ABSOLUTE prefix the fake site's own links use, from
// the request that asked for the page. The real site emits absolute URLs and the
// driver resolves title ids against them, rejecting anything off-site — so a
// mock emitting site-relative links would be parsed as zero results and would
// hide exactly the behaviour it exists to exercise.
func zarMockBaseFor(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/mocksrc/zar"
}

// zarMetaHTML is the block a real title page carries beside its cover: the
// synopsis, printed twice because the site repeats it for its narrow layout. The
// listing pages carry neither, which is the whole reason the driver reads them
// from here (spec 1023) — so a mock that only ever served the download table
// could not tell a working parser from one that returns nothing.
func zarMetaHTML(slug string) string {
	plot := "Mock synopsis for " + slug + ", written right here in the fake site."
	return `<div class="main_inner_single">` +
		`<div class="right_side"><div class="meta_side_cover"><div class="plot">` + plot + `</div></div></div>` +
		`<div class="left_side"><div class="mobile_plot">` + plot + `</div></div>` +
		`</div>`
}

func zarTitleHTML(base, slug string, paywalled bool) string {
	if strings.HasPrefix(slug, "series/") {
		return zarMetaHTML(slug) + zarSeriesHTML(base, paywalled)
	}
	var b strings.Builder
	b.WriteString(zarMetaHTML(slug))
	b.WriteString(`<div class="single_dlbox">`)
	for _, variant := range []struct {
		badge  string
		dubbed bool
	}{{"subtitle_row", false}, {"double_row", true}} {
		fmt.Fprintf(&b, `<div class="inner_dl_box_n_single">
  <div class="title_rows_dls"><h3>Mock variant</h3><span class="label_dl_row %s">x</span></div>`, variant.badge)
		for _, res := range []string{"1080p", "720p", "480p"} {
			link := fmt.Sprintf(`https://dl9.mockdl.invalid/Movies/%s.%s.mkv?md5=MOCKSIG&u=424242&expires=99999999999`, slug, res)
			cls := "dllink subtitle_link"
			if paywalled {
				// No entitlement: the row becomes an upsell, which is exactly how the
				// real site expresses it and how the driver detects it.
				link = base + "/pricing/"
				cls = "dllink vip_link"
			}
			fmt.Fprintf(&b, `
  <div class="item_row_dl">
    <div class="dl_btn_side"><a class="%s" href="%s">download</a></div>
    <div class="meta_row">
      <div class="item_meta_n_dl"><span class="label">انکود</span><span class="value">MockEnc</span></div>
      <div class="item_meta_n_dl"><span class="label">حجم</span><span class="value">1.5 GB</span></div>
      <div class="item_meta_n_dl"><span class="label">نوع زیرنویس</span><span class="value">SoftSub</span></div>
    </div>
  </div>`, cls, link)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func zarSeriesHTML(base string, paywalled bool) string {
	var b strings.Builder
	b.WriteString(`<div class="single_dlbox"><div class="row_season_n_dl">
  <div class="season_name"><span>فصل 1</span></div>
  <div class="body_row_season_n_dl"><div class="item_quality_n_row">
    <div class="item_meta_qu_r_n"><div class="label_meta_qu">کیفیت : </div><div class="value_meta_qu">WEB-DL 1080p</div></div>
    <div class="item_meta_qu_r_n"><div class="label_meta_qu">حجم : </div><div class="value_meta_qu">900 MB</div></div>
    <div class="item_meta_qu_r_n sub_type_item_meta"><div class="value_meta_qu">Soft</div></div>
    <div class="inner_parts_holder">`)
	for ep := 1; ep <= 4; ep++ {
		link := fmt.Sprintf(`https://dl9.mockdl.invalid/Series/Mock.S01E%02d.1080p.mkv?md5=MOCKSIG&u=424242&expires=99999999999`, ep)
		cls := "dllinkhref"
		if paywalled {
			link, cls = base+"/pricing/", "dllinkhref vip_link"
		}
		fmt.Fprintf(&b, `<div class="item_part"><a href="%s" class="%s">ep %d</a></div>`, link, cls, ep)
	}
	b.WriteString(`</div></div></div></div></div>`)
	return b.String()
}

// ---------- the JSON-shaped fake site ----------

func (s *Server) handleTNMock(w http.ResponseWriter, r *http.Request) {
	loggedOut, _, pages, perPage, prefix := s.tnSrc.snapshot()
	w.Header().Set("Content-Type", "application/json")
	if loggedOut {
		// The real API answers an unauthenticated call with success:false rather
		// than a status code, which is what the driver keys on.
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "unauthenticated"})
		return
	}
	page := 1
	if i := strings.Index(r.URL.Path, "/page/"); i >= 0 {
		if n, err := strconv.Atoi(strings.Trim(r.URL.Path[i+len("/page/"):], "/")); err == nil && n > 0 {
			page = n
		}
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	base := scheme + "://" + r.Host + "/mocksrc/tn"
	posts := make([]map[string]any, 0, perPage)
	if page <= pages {
		for i := 1; i <= perPage; i++ {
			n := (page-1)*perPage + i
			// The post shape mirrors the real API's, including the nested image
			// object — a flattened stand-in would unmarshal to an empty poster and
			// quietly test nothing.
			posts = append(posts, map[string]any{
				"id":            n,
				"title":         fmt.Sprintf("%s Title %d", prefix, n),
				"title_type":    "movie",
				"imdb_id":       "tt7654321",
				"imdb_score":    6.5,
				"30nama_score":  7.1,
				"english_plot":  "A mock title served by the in-repo fake source.",
				"coming_soon":   false,
				"free_download": true,
				"image": map[string]any{
					"cover": fmt.Sprintf("%s/cover-%d.jpg", base, n),
					"poster": map[string]any{
						"big":    fmt.Sprintf("%s/poster-%d.jpg", base, n),
						"large":  fmt.Sprintf("%s/poster-%d.jpg", base, n),
						"medium": fmt.Sprintf("%s/poster-%d.jpg", base, n),
						"small":  fmt.Sprintf("%s/poster-%d.jpg", base, n),
					},
				},
			})
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"result":  map[string]any{"page": page, "pages": pages, "posts": posts},
	})
}
