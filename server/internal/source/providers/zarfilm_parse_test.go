package providers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These run against trimmed captures of the real site, so the parser is
// exercised on the markup it will actually meet rather than on something
// hand-written to match the parser.

func zarFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "zarfilm", name))
	if err != nil {
		t.Fatalf("read zarfilm fixture %s: %v", name, err)
	}
	return b
}

func TestParseListingReadsEveryCard(t *testing.T) {
	items, err := parseListing(zarFixture(t, "archive_page1.html"), "https://zarfilm.com")
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	// The real archive serves 21 items per page.
	if len(items) != 21 {
		t.Fatalf("got %d items, want 21", len(items))
	}
	for i, it := range items {
		if it.ID == "" || it.Title == "" {
			t.Fatalf("item %d incomplete: %+v", i, it)
		}
		if strings.HasPrefix(it.ID, "http") || strings.HasPrefix(it.ID, "/") {
			t.Fatalf("item %d id must be a bare path, got %q", i, it.ID)
		}
	}
	first := items[0]
	if first.Title != "The Sheep Detectives" {
		t.Fatalf("first title = %q", first.Title)
	}
	if first.ID != "the-sheep-detectives-2026" {
		t.Fatalf("first id = %q", first.ID)
	}
	if first.Year != "2026" {
		t.Fatalf("first year = %q", first.Year)
	}
	// The rating element nests a "/10" child; only the number belongs to it.
	if first.Rating != 7.6 {
		t.Fatalf("first rating = %v, want 7.6 (nested /10 must not leak in)", first.Rating)
	}
	if !strings.Contains(first.PosterURL, "zarfilm.com/wp-content/uploads/") {
		t.Fatalf("first poster = %q", first.PosterURL)
	}
	if len(first.Genres) == 0 {
		t.Fatal("genres missing")
	}
}

// A listing must never be able to hand back an off-site URL as a title id — that
// is where a crafted id would otherwise come from.
func TestParseListingIgnoresOffsiteLinks(t *testing.T) {
	page := []byte(`<div class="inner_item_body_widget">
	  <a class="bgbackitem" href="https://evil.example/pwned"><img src="x.jpg"></a>
	  <div class="item-foot-title"><h3 class="movie-title">Hostile</h3></div>
	</div>`)
	items, err := parseListing(page, "https://zarfilm.com")
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("off-site link became a title: %+v", items)
	}
}

func TestParseMovieTitlePage(t *testing.T) {
	rows, err := parseTitlePage(zarFixture(t, "movie_subscribed.html"))
	if err != nil {
		t.Fatalf("parseTitlePage: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("no download rows parsed")
	}
	var subbed, dubbed int
	for _, r := range rows {
		if r.Paywalled {
			t.Fatalf("subscribed page yielded a paywalled row: %+v", r)
		}
		if r.URL == "" {
			t.Fatalf("row without a link: %+v", r)
		}
		if !strings.Contains(r.URL, "indllserver.info") {
			t.Fatalf("unexpected download host: %q", r.URL)
		}
		if r.Size == "" {
			t.Fatalf("row without a size: %+v", r)
		}
		if r.Dubbed {
			dubbed++
		} else {
			subbed++
		}
	}
	if subbed == 0 || dubbed == 0 {
		t.Fatalf("both release variants should be present: %d subbed, %d dubbed", subbed, dubbed)
	}
	// Resolution comes from the filename — the site never puts it in metadata.
	var res1080 bool
	for _, r := range rows {
		if r.Resolution == "1080p" {
			res1080 = true
		}
	}
	if !res1080 {
		t.Fatal("no 1080p row; resolution must be read from the release filename")
	}
	// Encoder is real metadata on the subtitled rows.
	var haveEncoder bool
	for _, r := range rows {
		if r.Encoder != "" {
			haveEncoder = true
		}
	}
	if !haveEncoder {
		t.Fatal("encoder metadata not parsed")
	}
}

// FR-019: no entitlement is a distinct state, not a broken session.
func TestParseUnsubscribedTitlePage(t *testing.T) {
	rows, err := parseTitlePage(zarFixture(t, "movie_unsubscribed.html"))
	if err != nil {
		t.Fatalf("parseTitlePage: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("no rows parsed from the paywalled page")
	}
	for _, r := range rows {
		if !r.Paywalled {
			t.Fatalf("paywalled page yielded a real link: %+v", r)
		}
		if r.URL != "" {
			t.Fatalf("paywalled row must carry no URL: %+v", r)
		}
	}
}

func TestParseSeriesPage(t *testing.T) {
	seasons, err := parseSeriesPage(zarFixture(t, "series_subscribed.html"))
	if err != nil {
		t.Fatalf("parseSeriesPage: %v", err)
	}
	if len(seasons) == 0 {
		t.Fatal("no season qualities parsed")
	}
	for i, sq := range seasons {
		if sq.Season == "" {
			t.Fatalf("season %d has no label: %+v", i, sq)
		}
		if sq.SeasonNum == 0 {
			t.Fatalf("season %d number not parsed from %q", i, sq.Season)
		}
		if len(sq.Episodes) == 0 {
			t.Fatalf("season %d has no episode links: %+v", i, sq)
		}
		for _, u := range sq.Episodes {
			if !strings.Contains(u, "indllserver.info") {
				t.Fatalf("unexpected episode host: %q", u)
			}
		}
		if sq.Size == "" {
			t.Fatalf("season %d has no size: %+v", i, sq)
		}
	}
	// Episodes must come back in broadcast order, not shuffled by the DOM walk.
	first := seasons[0]
	if !strings.Contains(first.Episodes[0], "S01E01") {
		t.Fatalf("first episode = %q, want S01E01", first.Episodes[0])
	}
	if len(first.Episodes) > 1 && !strings.Contains(first.Episodes[1], "S01E02") {
		t.Fatalf("second episode = %q, want S01E02", first.Episodes[1])
	}
	if first.Resolution != "1080p" {
		t.Fatalf("resolution = %q, want 1080p from the release filename", first.Resolution)
	}
}

func TestParseLoginState(t *testing.T) {
	for _, tc := range []struct {
		file   string
		logged bool
	}{
		{"logged_out.html", false},
		{"movie_subscribed.html", true},
		{"archive_page1.html", true},
	} {
		got := parseLoginState(zarFixture(t, tc.file))
		if got.LoggedIn != tc.logged {
			t.Fatalf("%s: LoggedIn = %v, want %v", tc.file, got.LoggedIn, tc.logged)
		}
	}
	// A user id of "0" is the anonymous marker even if the flag were malformed.
	if st := parseLoginState([]byte(`{"u":"0","logged":"1"}`)); st.LoggedIn {
		t.Fatal(`u="0" must be treated as anonymous`)
	}
}

func TestParseIMDbID(t *testing.T) {
	if got := parseIMDbID(zarFixture(t, "movie_subscribed.html")); got != "tt11561116" {
		t.Fatalf("imdb id = %q", got)
	}
	// The metadata block the site serves today: a real IMDb anchor, on both a
	// movie page and a series page (FR-003, FR-004).
	for _, tc := range []struct{ file, want string }{
		{"movie_meta.html", "tt1756855"},
		{"series_meta.html", "tt13210838"},
	} {
		if got := parseIMDbID(zarFixture(t, tc.file)); got != tc.want {
			t.Fatalf("%s: imdb id = %q, want %q", tc.file, got, tc.want)
		}
	}
}

func TestParsePlot(t *testing.T) {
	// Movie and series pages carry the synopsis identically (FR-002, FR-004).
	for _, file := range []string{"movie_meta.html", "series_meta.html"} {
		got := parsePlot(zarFixture(t, file))
		if got == "" {
			t.Fatalf("%s: no synopsis found", file)
		}
		// The page repeats the same text in a second block for its narrow layout.
		// One copy, never two concatenated (FR-005).
		if strings.Count(got, "خلاصه") != 1 {
			t.Fatalf("%s: synopsis looks doubled: %q", file, got)
		}
	}
	// A page with no synopsis block at all — the shape the site served when these
	// fixtures were captured. Absent is a normal outcome, not an error (FR-010).
	for _, file := range []string{"movie_subscribed.html", "series_subscribed.html", "archive_page1.html"} {
		if got := parsePlot(zarFixture(t, file)); got != "" {
			t.Fatalf("%s: synopsis = %q, want empty", file, got)
		}
	}
	// Empty, blank and placeholder-only synopses all count as absent (FR-006).
	for _, body := range []string{
		`<div class="plot"></div>`,
		"<div class=\"plot\">  \n\t </div>",
		`<div class="plot">-</div>`,
		`<div class="plot">—</div>`,
		`<div class="plot">...</div>`,
	} {
		if got := parsePlot([]byte(body)); got != "" {
			t.Fatalf("parsePlot(%q) = %q, want empty", body, got)
		}
	}
	// Falls back to the narrow-layout block when only that one is present, and
	// yields text rather than markup (FR-007).
	got := parsePlot([]byte(`<div class="mobile_plot">A <b>bold</b> claim &amp; then some.</div>`))
	if got != "A bold claim & then some." {
		t.Fatalf("plot = %q", got)
	}
}

func TestParsePageCount(t *testing.T) {
	if n := parsePageCount(zarFixture(t, "archive_page1.html")); n < 100 {
		t.Fatalf("page count = %d, want the archive's real depth", n)
	}
}

// A parse failure must degrade, never panic: the site can change its markup at
// any time and that must surface as a source-level error.
func TestParsersSurviveGarbage(t *testing.T) {
	for _, junk := range [][]byte{
		nil, []byte(""), []byte("not html at all"),
		[]byte("<div class=\"inner_item_body_widget\">"), // truncated mid-element
		[]byte("<html><body><div class='item_row_dl'><a class='dllink'></a></div>"),
	} {
		if _, err := parseListing(junk, "https://zarfilm.com"); err != nil {
			t.Fatalf("parseListing(%q) errored: %v", junk, err)
		}
		if _, err := parseTitlePage(junk); err != nil {
			t.Fatalf("parseTitlePage(%q) errored: %v", junk, err)
		}
		if _, err := parseSeriesPage(junk); err != nil {
			t.Fatalf("parseSeriesPage(%q) errored: %v", junk, err)
		}
		_ = parseLoginState(junk)
		_ = parseIMDbID(junk)
		_ = parsePlot(junk)
		_ = parsePageCount(junk)
	}
}

func TestParseFilterPanel(t *testing.T) {
	p := parseFilterPanel(zarFixture(t, "archive_filters.html"))

	// Groups are told apart by the SHAPE of their values, not by their Persian
	// headings — so a relabelled panel still parses.
	wantSorts := []string{"newest", "modified", "popular", "imdb_rate", "release"}
	if len(p.Sorts) != len(wantSorts) {
		t.Fatalf("sorts = %+v", p.Sorts)
	}
	for i, w := range wantSorts {
		if p.Sorts[i].Value != w {
			t.Fatalf("sort %d = %q, want %q", i, p.Sorts[i].Value, w)
		}
	}
	if got := len(p.Scores); got != 6 {
		t.Fatalf("scores = %d (%+v), want the six bands", got, p.Scores)
	}
	if p.Scores[0].Value != "9" {
		t.Fatalf("scores start at %q, want the highest band first", p.Scores[0].Value)
	}
	// 29 entries in the panel, one of which is the empty "all" marker.
	if got := len(p.Genres); got != 28 {
		t.Fatalf("genres = %d, want 28", got)
	}
	for _, g := range p.Genres {
		if g.Value == "" || g.Label == "" {
			t.Fatalf("blank genre entry: %+v", g)
		}
	}
	// The "all" entry is a UI affordance, not a filter value.
	for _, g := range append(append([]zarFacet{}, p.Genres...), p.Scores...) {
		if g.Value == "" {
			t.Fatal(`the empty "all" option must not be offered as a value`)
		}
	}
}

func TestParseFilterPanelSurvivesPagesWithout(t *testing.T) {
	for _, f := range []string{"archive_page1.html", "movie_subscribed.html", "logged_out.html"} {
		p := parseFilterPanel(zarFixture(t, f))
		if len(p.Sorts)+len(p.Scores)+len(p.Genres) != 0 {
			t.Fatalf("%s: found a panel where there is none: %+v", f, p)
		}
	}
}

func TestParseGenreSlugs(t *testing.T) {
	m := parseGenreSlugs(zarFixture(t, "archive_filters.html"))
	for label, want := range map[string]string{
		"کمدی":       "comedy",
		"درام":       "drama",
		"علمی تخیلی": "sci-fi",
		"نوآر":       "film-noir",
	} {
		if m[label] != want {
			t.Fatalf("%q -> %q, want %q", label, m[label], want)
		}
	}
	// Some routes are just the Persian label percent-encoded — no English slug at
	// all. Those must be dropped rather than offered as a slug, because a Persian
	// "slug" would join with nothing and would be shown to the user title-cased.
	for _, label := range []string{"معمایی", "فیلم نوآر", "مراسم تلویزیونی"} {
		if s, ok := m[label]; ok {
			t.Fatalf("%q kept a non-English slug %q", label, s)
		}
	}
}

// Spec 1026 US2: a season's quality names who encoded it after the final
// separator. Real values, captured from the live site.
func TestQualityEncoder(t *testing.T) {
	for _, tc := range []struct{ quality, want string }{
		{"WEB-DL 1080p FHD DDP5.1 - NHTFS", "NHTFS"},
		{"WEB-DL 1080p - ZarFilm", "ZarFilm"},
		{"WEB-DL 1080p x265 10bit - PSA", "PSA"},
		{"WEB-DL 720p - Pahe", "Pahe"},
		{"WEB-DL 480p - RMTeam", "RMTeam"},
		// "WEB-DL" is hyphenated without spaces and must not be mistaken for the
		// separator — otherwise every quality would report an encoder of "DL".
		{"WEB-DL 1080p", ""},
		{"BluRay 720p", ""},
		{"", ""},
		{" - ", ""},
	} {
		if got := qualityEncoder(tc.quality); got != tc.want {
			t.Errorf("qualityEncoder(%q) = %q, want %q", tc.quality, got, tc.want)
		}
	}
}

func TestFileNameFromURL(t *testing.T) {
	for _, tc := range []struct{ url, want string }{
		{"https://dl9.example.invalid/Series/The.Gentlemen.S01E01.720p.x264.Pahe-ZarFilm.mkv?md5=X&expires=1", "The.Gentlemen.S01E01.720p.x264.Pahe-ZarFilm.mkv"},
		{"https://dl9.example.invalid/Movies/Some.Movie.1080p.mkv", "Some.Movie.1080p.mkv"},
		// A percent-encoded name is decoded, because that is how it lands on disk.
		{"https://dl9.example.invalid/Movies/Some%20Movie%201080p.mkv", "Some Movie 1080p.mkv"},
		// An upsell link is not a release and names no file.
		{"https://zarfilm.com/pricing/", ""},
		{"", ""},
		{"::not a url::", ""},
	} {
		if got := fileNameFromURL(tc.url); got != tc.want {
			t.Errorf("fileNameFromURL(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}
