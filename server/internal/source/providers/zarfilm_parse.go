package providers

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// HTML parsing for the zarfilm driver (spec 0007).
//
// This site publishes no API — no REST, no JSON, no separate API host — so its
// catalog has to be read out of the pages a browser gets. The markup is regular
// (stable class names, predictable nesting), but it is deeply nested, and
// pattern-matching nested markup with regexps is the classic way to ship a
// parser that is subtly wrong on the one title with an unusual field. So this
// uses a real tokenizer.
//
// Everything here is pure: it takes bytes and returns structs, with no HTTP and
// no session material, so it is tested directly against captured pages.

// ---------- small DOM helpers ----------

// node attribute lookup.
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// hasClass reports whether n carries the exact class token.
func hasClass(n *html.Node, class string) bool {
	for _, f := range strings.Fields(attr(n, "class")) {
		if f == class {
			return true
		}
	}
	return false
}

// walk calls fn for every element node, depth first. Returning false from fn
// prunes that subtree.
func walk(n *html.Node, fn func(*html.Node) bool) {
	if n.Type == html.ElementNode && !fn(n) {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, fn)
	}
}

// findAll collects every element matching pred.
func findAll(root *html.Node, pred func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	walk(root, func(n *html.Node) bool {
		if pred(n) {
			out = append(out, n)
		}
		return true
	})
	return out
}

// findFirst returns the first element matching pred (nil when none).
func findFirst(root *html.Node, pred func(*html.Node) bool) *html.Node {
	for _, n := range findAll(root, pred) {
		return n
	}
	return nil
}

func byClass(class string) func(*html.Node) bool {
	return func(n *html.Node) bool { return hasClass(n, class) }
}

func byTag(tag string) func(*html.Node) bool {
	return func(n *html.Node) bool { return n.Data == tag }
}

// text returns the concatenated, whitespace-collapsed text of a subtree.
func text(n *html.Node) string {
	var b strings.Builder
	var rec func(*html.Node)
	rec = func(x *html.Node) {
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
	}
	rec(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// ownText returns only the node's DIRECT text children, so a wrapper's own value
// isn't polluted by a nested element (the rating span nests "/10" inside it).
func ownText(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func parseHTML(body []byte) (*html.Node, error) {
	return html.Parse(strings.NewReader(string(body)))
}

// ---------- session state ----------

// zarLoginState is what a page reveals about the caller's session.
type zarLoginState struct {
	LoggedIn bool
	UserID   string
}

var (
	// Every page inlines `var ajax_var = {...}` carrying the login flag and user
	// id. Read from the raw bytes rather than the DOM: it is script content, and
	// this avoids parsing an entire page just to answer "am I logged in".
	reAjaxLogged = regexp.MustCompile(`"logged"\s*:\s*"([^"]*)"`)
	reAjaxUser   = regexp.MustCompile(`"u"\s*:\s*"([^"]*)"`)
)

func parseLoginState(body []byte) zarLoginState {
	s := string(body)
	st := zarLoginState{}
	if m := reAjaxLogged.FindStringSubmatch(s); m != nil {
		st.LoggedIn = m[1] == "1"
	}
	if m := reAjaxUser.FindStringSubmatch(s); m != nil {
		st.UserID = m[1]
		if m[1] == "0" {
			st.LoggedIn = false
		}
	}
	return st
}

// ---------- catalog listings ----------

// zarListItem is one card in an archive or search listing.
type zarListItem struct {
	ID        string // URL path, e.g. "the-whisper-man-2026" or "series/the-loyalty-game"
	Title     string
	Year      string
	Rating    float64
	PosterURL string
	Genres    []string
	IsSeries  bool
}

var reYear = regexp.MustCompile(`\b((?:19|20)\d{2})\b`)

// parseListing reads every result card on an archive or search page.
func parseListing(body []byte, bases ...string) ([]zarListItem, error) {
	doc, err := parseHTML(body)
	if err != nil {
		return nil, err
	}
	var out []zarListItem
	for _, card := range findAll(doc, byClass("inner_item_body_widget")) {
		link := findFirst(card, byClass("bgbackitem"))
		if link == nil {
			continue
		}
		id := pathFromURL(attr(link, "href"), bases...)
		if id == "" {
			continue
		}
		it := zarListItem{ID: id, IsSeries: strings.HasPrefix(id, "series/")}
		if t := findFirst(card, byClass("movie-title")); t != nil {
			it.Title = text(t)
		}
		if y := findFirst(card, byClass("year")); y != nil {
			it.Year = text(y)
		}
		if r := findFirst(card, byClass("rate")); r != nil {
			// The rating span nests a "/10" child, so take only its own text.
			it.Rating, _ = strconv.ParseFloat(strings.TrimSpace(ownText(r)), 64)
		}
		if img := findFirst(card, byTag("img")); img != nil {
			it.PosterURL = attr(img, "src")
		}
		if g := findFirst(card, byClass("genres_links")); g != nil {
			for _, h3 := range findAll(g, byTag("h3")) {
				if v := text(h3); v != "" {
					it.Genres = append(it.Genres, v)
				}
			}
		}
		// A card without a title is a template or a placeholder, not a result.
		if it.Title == "" {
			continue
		}
		if it.Year == "" {
			// Some cards omit the year element; the link title carries it.
			if m := reYear.FindStringSubmatch(attr(link, "title")); m != nil {
				it.Year = m[1]
			}
		}
		out = append(out, it)
	}
	return out, nil
}

// pathFromURL turns an absolute site URL into the driver-side id: the path with
// no leading or trailing slash. Anything off-site yields "" so a listing can
// never smuggle in a foreign host as a title id.
//
// Several bases are accepted because a page's links are absolute and always name
// the canonical host, even when the driver reached the page by another address
// (a test server, or the site behind a different name). Off-site links are still
// rejected — a base has to be one this driver actually talks to.
func pathFromURL(href string, bases ...string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	for _, base := range bases {
		if base == "" || !strings.HasPrefix(href, base) {
			continue
		}
		p := strings.Trim(strings.TrimPrefix(href, base), "/")
		if p == "" || strings.Contains(p, "?") || strings.Contains(p, "#") {
			return ""
		}
		return p
	}
	return ""
}

// parsePageCount reads the highest page number from a listing's pagination.
func parsePageCount(body []byte) int {
	re := regexp.MustCompile(`/page/(\d+)/`)
	max := 1
	for _, m := range re.FindAllStringSubmatch(string(body), -1) {
		if n, err := strconv.Atoi(m[1]); err == nil && n > max {
			max = n
		}
	}
	return max
}

// ---------- title pages ----------

// zarDownloadRow is one downloadable release on a title page.
type zarDownloadRow struct {
	URL        string // "" when the row is a paywall upsell rather than a link
	Paywalled  bool
	Encoder    string
	Size       string
	Subtitle   string // e.g. "SoftSub"
	Dubbed     bool
	Resolution string
	Season     string
	Episode    int
	GroupLabel string
}

// parseTitlePage reads every download row on a movie or series page.
//
// Rows are grouped by release variant (Persian-subtitled vs Persian-dubbed),
// each group headed by an <h3> with a badge class naming which it is. A series
// nests one level deeper: season blocks containing per-episode rows.
func parseTitlePage(body []byte) ([]zarDownloadRow, error) {
	doc, err := parseHTML(body)
	if err != nil {
		return nil, err
	}
	var out []zarDownloadRow
	for _, group := range findAll(doc, byClass("inner_dl_box_n_single")) {
		label, dubbed := groupVariant(group)
		season := ""
		if sn := findFirst(group, byClass("season_name")); sn != nil {
			season = text(sn)
		}
		episode := 0
		for _, row := range findAll(group, byClass("item_row_dl")) {
			r := zarDownloadRow{GroupLabel: label, Dubbed: dubbed, Season: season}
			link := findFirst(row, byClass("dllink"))
			if link == nil {
				continue
			}
			href := attr(link, "href")
			// A visitor with no download entitlement gets an upsell link to the
			// pricing page in place of a real one. That is a distinct, reportable
			// state — not a broken session (FR-019).
			if hasClass(link, "vip_link") || strings.Contains(href, "/pricing") {
				r.Paywalled = true
			} else {
				r.URL = href
			}
			for _, meta := range findAll(row, byClass("item_meta_n_dl")) {
				k := findFirst(meta, byClass("label"))
				v := findFirst(meta, byClass("value"))
				if k == nil || v == nil {
					continue
				}
				assignMeta(&r, text(k), text(v))
			}
			r.Resolution = resolutionFromURL(href)
			if season != "" {
				episode++
				r.Episode = episode
			}
			out = append(out, r)
		}
	}
	return out, nil
}

// groupVariant reads a group's heading badge: subtitled or dubbed.
func groupVariant(group *html.Node) (label string, dubbed bool) {
	if h := findFirst(group, byTag("h3")); h != nil {
		label = text(h)
	}
	if b := findFirst(group, byClass("label_dl_row")); b != nil {
		// The badge class is the reliable signal; its text is Persian.
		dubbed = hasClass(b, "double_row")
	}
	return label, dubbed
}

// assignMeta maps one label/value pair from a row's metadata. The labels are
// Persian, so they are matched by content rather than by a translated name:
// انکود = encoder, حجم = size, نوع زیرنویس = subtitle type.
func assignMeta(r *zarDownloadRow, label, value string) {
	switch {
	case strings.Contains(label, "انکود"):
		r.Encoder = value
	case strings.Contains(label, "حجم"):
		r.Size = value
	case strings.Contains(label, "زیرنویس"):
		r.Subtitle = value
	}
}

// resolutionFromURL pulls the resolution out of the release filename. It is not
// in the row metadata — the site only puts it in the file name.
var reResolution = regexp.MustCompile(`(?i)\b(2160p|1080p|720p|480p|360p)\b`)

func resolutionFromURL(u string) string {
	if m := reResolution.FindStringSubmatch(u); m != nil {
		return strings.ToLower(m[1])
	}
	return ""
}

// ---------- title metadata ----------

var (
	reIMDbID    = regexp.MustCompile(`imdb\.com/title/(tt\d{7,10})`)
	reIMDbIDAlt = regexp.MustCompile(`\b(tt\d{7,10})\b`)
)

// parseIMDbID finds the title's IMDb id: from an IMDb link where present, and
// otherwise from a poster filename, which embeds it.
func parseIMDbID(body []byte) string {
	s := string(body)
	if m := reIMDbID.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	if m := reIMDbIDAlt.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

// parsePlot reads the title's synopsis.
//
// The page prints the same text twice — once in the block beside the cover and
// again in a separate block the stylesheet shows only on narrow screens — so the
// two are alternatives, never pieces to be joined: taking both would show every
// synopsis doubled. The first non-empty wins.
//
// ZarFilm publishes the synopsis in Persian only. It is returned as published;
// deciding what to do with a right-to-left string is the client's job, not the
// parser's.
func parsePlot(body []byte) string {
	doc, err := parseHTML(body)
	if err != nil {
		return ""
	}
	for _, class := range []string{"plot", "mobile_plot"} {
		if n := findFirst(doc, byClass(class)); n != nil {
			if s := strings.TrimSpace(text(n)); hasWords(s) {
				return s
			}
		}
	}
	return ""
}

// hasWords reports whether s says anything. A synopsis the site has not written
// yet is not always an empty element: it can be a dash or an ellipsis holding the
// space, which would render as a synopsis consisting of punctuation.
func hasWords(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// ---------- series pages ----------

// Series pages are shaped differently from movie pages, not merely nested one
// level deeper: a season block holds quality options, and each quality option
// holds one link per episode. So they get their own parser rather than being
// forced through the movie one, which would misread both.
//
//	row_season_n_dl            one season, headed by .season_name
//	  item_quality_n_row       one quality of that season
//	    .item_meta_qu_r_n      quality / episode count / size / subtitle type
//	    .inner_parts_holder
//	      .item_part > a.dllinkhref     one episode link

// zarSeasonQuality is one downloadable quality of one season.
type zarSeasonQuality struct {
	Season     string // the site's own label, e.g. "فصل 1"
	SeasonNum  int    // parsed where possible, for a stable id
	Quality    string // e.g. "WEB-DL 1080p - ZarFilm"
	Resolution string // from the release filenames
	Size       string // per-episode size as the site reports it
	Subtitle   string // e.g. "Soft"
	Dubbed     bool
	Episodes   []string // one signed URL per episode, in order
	Paywalled  bool
}

var (
	reSeasonNum = regexp.MustCompile(`(\d+)`)
	// Persian-Indic digits appear in episode labels; the season heading uses
	// western digits, but be tolerant either way.
	persianDigits = strings.NewReplacer(
		"۰", "0", "۱", "1", "۲", "2", "۳", "3", "۴", "4",
		"۵", "5", "۶", "6", "۷", "7", "۸", "8", "۹", "9")
)

// parseSeriesPage reads every season/quality/episode-list on a series page.
func parseSeriesPage(body []byte) ([]zarSeasonQuality, error) {
	doc, err := parseHTML(body)
	if err != nil {
		return nil, err
	}
	var out []zarSeasonQuality
	for _, season := range findAll(doc, byClass("row_season_n_dl")) {
		label := ""
		if sn := findFirst(season, byClass("season_name")); sn != nil {
			label = text(sn)
		}
		num := 0
		if m := reSeasonNum.FindStringSubmatch(persianDigits.Replace(label)); m != nil {
			num, _ = strconv.Atoi(m[1])
		}
		for _, q := range findAll(season, byClass("item_quality_n_row")) {
			sq := zarSeasonQuality{Season: label, SeasonNum: num}
			for _, meta := range findAll(q, byClass("item_meta_qu_r_n")) {
				assignSeriesMeta(&sq, meta)
			}
			for _, part := range findAll(q, byClass("item_part")) {
				a := findFirst(part, byTag("a"))
				if a == nil {
					continue
				}
				href := attr(a, "href")
				if hasClass(a, "vip_link") || strings.Contains(href, "/pricing") {
					sq.Paywalled = true
					continue
				}
				if href != "" {
					sq.Episodes = append(sq.Episodes, href)
				}
			}
			if len(sq.Episodes) > 0 {
				sq.Resolution = resolutionFromURL(sq.Episodes[0])
			}
			if sq.Resolution == "" {
				sq.Resolution = resolutionFromURL(sq.Quality)
			}
			// A quality row with neither links nor a paywall marker is a layout
			// artifact, not an offer.
			if len(sq.Episodes) == 0 && !sq.Paywalled {
				continue
			}
			out = append(out, sq)
		}
	}
	return out, nil
}

// assignSeriesMeta reads one metadata block on a season's quality row. As on
// movie pages the labels are Persian and are matched by content:
// کیفیت = quality, تعداد قسمت = episode count, حجم = size.
func assignSeriesMeta(sq *zarSeasonQuality, meta *html.Node) {
	label, value := "", ""
	if l := findFirst(meta, byClass("label_meta_qu")); l != nil {
		label = text(l)
	}
	if v := findFirst(meta, byClass("value_meta_qu")); v != nil {
		value = text(v)
	}
	switch {
	case strings.Contains(label, "کیفیت"):
		sq.Quality = value
	case strings.Contains(label, "حجم"):
		sq.Size = value
	case hasClass(meta, "sub_type_item_meta"):
		sq.Subtitle = value
	}
	// The dubbed/subtitled badge sits beside the quality rather than in a labelled
	// pair. "دوبله" is the site's word for dubbed.
	if strings.Contains(text(meta), "دوبله") {
		sq.Dubbed = true
	}
}
