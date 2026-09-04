package synomock

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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

// zarMockGenres is the fake site's genre taxonomy. The VALUE is deliberately not
// the slug: the real site's filter parameter takes its own (Persian) wording
// while its archive routes carry an English slug, and that gap is the whole
// reason cross-source translation exists. A mock whose value equalled its slug
// would let a broken translator pass.
var zarMockGenres = []struct{ Value, Slug, Label string }{
	{"g-comedy", "comedy", "Comedy"},
	{"g-drama", "drama", "Drama"},
	{"g-action", "action", "Action"},
}

// zarMockPanelHTML is the capability panel a real archive page carries. Groups
// are told apart by the shape of their values, so the headings are only labels.
func zarMockPanelHTML(base string) string {
	var b strings.Builder
	b.WriteString(`<div class="filter_orderby"><div class="label_orderby">sort</div><div class="filter_orderby_selecr">`)
	for _, v := range []string{"newest", "modified", "popular", "imdb_rate", "release"} {
		fmt.Fprintf(&b, `<div class="item_filter_orderby" data-filter="%s">%s</div>`, v, v)
	}
	b.WriteString(`</div></div><div class="filter_orderby"><div class="label_orderby">score</div><div class="filter_orderby_selecr">`)
	b.WriteString(`<div class="item_filter_orderby" data-filter="">all</div>`)
	for _, v := range []string{"9", "8", "7", "6", "5"} {
		fmt.Fprintf(&b, `<div class="item_filter_orderby" data-filter="%s">over %s</div>`, v, v)
	}
	b.WriteString(`</div></div><div class="filter_orderby"><div class="label_orderby">genre</div><div class="filter_orderby_selecr">`)
	b.WriteString(`<div class="item_filter_orderby" data-filter="">all</div>`)
	for _, g := range zarMockGenres {
		fmt.Fprintf(&b, `<div class="item_filter_orderby" data-filter="%s">%s</div>`, g.Value, g.Label)
	}
	b.WriteString(`</div></div>`)
	// The archive's own genre routes, which is where the English slug for each
	// label comes from.
	for _, g := range zarMockGenres {
		fmt.Fprintf(&b, `<a href="%s/genre/%s/">%s(42)</a>`, base, g.Slug, g.Label)
	}
	return b.String()
}

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
		q := r.URL.Query()
		fmt.Fprint(w, head+zarMockPanelHTML(zarMockBaseFor(r))+
			zarListingHTML(zarMockBaseFor(r), prefix, page, perPage, pages, seriesArchive,
				q.Get("filter_genre"), q.Get("imdb_rate"), q.Get("sortby")))
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

func zarListingHTML(base, prefix string, page, perPage, pages int, series bool, genre, minScore, sortBy string) string {
	var b strings.Builder
	b.WriteString(`<div class="posts_hoder_archive">`)
	// The cards this page would carry, before the user's narrowing.
	type card struct {
		n     int
		genre int
		score float64
	}
	cards := make([]card, 0, perPage)
	for i := 1; i <= perPage; i++ {
		n := (page-1)*perPage + i
		cards = append(cards, card{n: n, genre: n % len(zarMockGenres), score: float64(5+n%5) + float64(n%10)/10})
	}
	// Honour what was asked for. A mock that accepted a filter and ignored it
	// would let a driver sending the wrong parameter name pass — which is exactly
	// the bug this spec fixes.
	if genre != "" {
		want := -1
		for i, g := range zarMockGenres {
			if g.Value == genre {
				want = i
			}
		}
		kept := cards[:0]
		for _, c := range cards {
			if c.genre == want {
				kept = append(kept, c)
			}
		}
		cards = kept
	}
	if minScore != "" {
		if min, err := strconv.ParseFloat(minScore, 64); err == nil {
			kept := cards[:0]
			for _, c := range cards {
				if c.score >= min {
					kept = append(kept, c)
				}
			}
			cards = kept
		}
	}
	if sortBy == "imdb_rate" {
		sort.SliceStable(cards, func(i, j int) bool { return cards[i].score > cards[j].score })
	}
	for _, c := range cards {
		n := c.n
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
    <div class="genres_links"><h3><span>%s</span></h3></div>
    <img src="%s/poster-%d.jpg">
  </a>
  <div class="item-foot-title">
    <h3 class="movie-title">%s Title %d</h3>
    <div class="score"><span class="year">20%02d</span><span class="rate">%d.%d<span class="ten">/10</span></span></div>
  </div>
</div>`, base, slug, prefix, n, 10+n%15, zarMockGenres[c.genre].Label, base, n,
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
	// The two variants deliberately share a resolution AND an encoder, differing
	// only in the file each produces — the real site is shaped exactly this way
	// (a dubbed encode and a subtitled one, both labelled with the site's own
	// name), and it is the pair a token comparison cannot separate.
	for _, variant := range []struct {
		badge  string
		dubbed bool
		tag    string
	}{{"subtitle_row", false, "x264"}, {"double_row", true, "Dubbed"}} {
		fmt.Fprintf(&b, `<div class="inner_dl_box_n_single">
  <div class="title_rows_dls"><h3>Mock variant</h3><span class="label_dl_row %s">x</span></div>`, variant.badge)
		for _, res := range []string{"1080p", "720p", "480p"} {
			link := fmt.Sprintf(
				`https://dl9.mockdl.invalid/Movies/%s.%s.WEBRip.%s.MockSite.mkv?md5=MOCKSIG&u=424242&expires=99999999999`,
				slug, res, variant.tag)
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

// zarMockSeasons is how many seasons the fake series carries. More than one on
// purpose: a single-season series cannot exercise "open the first season you do
// not have, and opening another closes it", which is the whole of spec 1025's
// second story.
const zarMockSeasons = 3

func zarSeriesHTML(base string, paywalled bool) string {
	var b strings.Builder
	b.WriteString(`<div class="single_dlbox">`)
	// Two qualities per season at the SAME resolution, named for who encoded them
	// the way the real site names them. They are indistinguishable by resolution
	// and, once the site's own suffix is applied, by release group too — only the
	// files tell them apart, which is what spec 1026 matches on.
	qualities := []struct{ encoder, tag, size string }{
		{"Alpha", "x264.Alpha", "900 MB"},
		{"MockSite", "Dubbed", "700 MB"},
	}
	for season := 1; season <= zarMockSeasons; season++ {
		fmt.Fprintf(&b, `<div class="row_season_n_dl">
  <div class="season_name"><span>فصل %d</span></div>
  <div class="body_row_season_n_dl">`, season)
		for _, q := range qualities {
			fmt.Fprintf(&b, `<div class="item_quality_n_row">
    <div class="item_meta_qu_r_n"><div class="label_meta_qu">کیفیت : </div><div class="value_meta_qu">WEB-DL 1080p - %s</div></div>
    <div class="item_meta_qu_r_n"><div class="label_meta_qu">حجم : </div><div class="value_meta_qu">%s</div></div>
    <div class="item_meta_qu_r_n sub_type_item_meta"><div class="value_meta_qu">Soft</div></div>
    <div class="inner_parts_holder">`, q.encoder, q.size)
			for ep := 1; ep <= 4; ep++ {
				link := fmt.Sprintf(
					`https://dl9.mockdl.invalid/Series/Mock.S%02dE%02d.1080p.WEB-DL.%s.MockSite.mkv?md5=MOCKSIG&u=424242&expires=99999999999`,
					season, ep, q.tag)
				cls := "dllinkhref"
				if paywalled {
					link, cls = base+"/pricing/", "dllinkhref vip_link"
				}
				fmt.Fprintf(&b, `<div class="item_part"><a href="%s" class="%s">ep %d</a></div>`, link, cls, ep)
			}
			b.WriteString(`</div></div>`)
		}
		b.WriteString(`</div></div>`)
	}
	b.WriteString(`</div>`)
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
	// The facet endpoint: what this source says it can be narrowed by. Its genre
	// VALUES are opaque codes and its slugs are English — the opposite shape to
	// the HTML-shaped source beside it, which is what makes the two a real test
	// of cross-source translation rather than a pair of look-alikes.
	if strings.Contains(r.URL.Path, "advanced_search_parametres") {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": map[string]any{
				"genre": tnMockGenreFacets(),
				"type": []map[string]any{
					{"name": "Movie", "value": "movie", "slug": "movie"},
					{"name": "Series", "value": "series", "slug": "series"},
				},
				"score":    []map[string]any{{"name": "8+", "value": "8"}, {"name": "7+", "value": "7"}},
				"quality":  []map[string]any{{"name": "BluRay", "value": "bluray", "slug": "bluray"}},
				"country":  []map[string]any{},
				"language": []map[string]any{},
				"channel":  []map[string]any{},
				"encoder":  []string{"MockEnc"},
				"age":      []string{"G"},
				"min_year": 1990,
				"max_year": 2026,
			},
		})
		return
	}
	page := 1
	if i := strings.Index(r.URL.Path, "/page/"); i >= 0 {
		if n, err := strconv.Atoi(strings.Trim(r.URL.Path[i+len("/page/"):], "/")); err == nil && n > 0 {
			page = n
		}
	}
	// The genre the caller narrowed by, in THIS source's vocabulary.
	//
	// The real API takes its filters FORM-encoded, as a single "parameters" field
	// holding a JSON object — not as a JSON request body. Reading it the easy way
	// (decoding the body as JSON) parses nothing, and a mock that quietly filters
	// by nothing reports every filter as working.
	wantGenre := ""
	if err := r.ParseForm(); err == nil {
		var params struct {
			Genre []string `json:"genre"`
		}
		if raw := r.PostForm.Get("parameters"); raw != "" {
			if err := json.Unmarshal([]byte(raw), &params); err == nil && len(params.Genre) > 0 {
				wantGenre = params.Genre[0]
			}
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
				"genre":         []map[string]any{tnMockGenreFor(n)},
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
	if wantGenre != "" {
		kept := posts[:0]
		for _, p := range posts {
			for _, g := range p["genre"].([]map[string]any) {
				if g["value"] == wantGenre {
					kept = append(kept, p)
					break
				}
			}
		}
		posts = kept
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"result":  map[string]any{"page": page, "pages": pages, "posts": posts},
	})
}

// tnMockGenres pairs this source's own opaque code with the English slug that
// lets it join with another source's genre. "period-drama" is deliberately
// unshared: it proves a facet only one source offers drops out of a combined
// view instead of being offered and then half-honoured.
var tnMockGenres = []struct{ Value, Slug, Name string }{
	{"101", "comedy", "Comedy"},
	{"102", "drama", "Drama"},
	{"103", "period-drama", "Period drama"},
}

func tnMockGenreFacets() []map[string]any {
	out := make([]map[string]any, 0, len(tnMockGenres))
	for _, g := range tnMockGenres {
		out = append(out, map[string]any{"name": g.Name, "value": g.Value, "slug": g.Slug})
	}
	return out
}

// tnMockGenreFor assigns a genre to a generated post, rotating through the list
// so any page carries a mix.
func tnMockGenreFor(n int) map[string]any {
	g := tnMockGenres[n%len(tnMockGenres)]
	return map[string]any{"name": g.Name, "value": g.Value, "slug": g.Slug}
}
