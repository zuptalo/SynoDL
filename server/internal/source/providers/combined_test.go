package providers

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"synodl/server/internal/source"
)

// Spec 1047, FR-006. The two real drivers, each answering from what its site
// actually serves (30nama's captured parameters, ZarFilm's captured dialog),
// intersected the way the combined sheet intersects them. This is the check
// that would have caught the empty sheet: every kind below was dropping out.
func TestTheTwoRealVocabulariesIntersectToAUsableSheet(t *testing.T) {
	cfg, done := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "advanced_search_parametres") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write(fixture(t, "advanced_search_parametres.json"))
	})
	defer done()
	tn, err := nama30{}.Parameters(context.Background(), source.NewClient(), cfg, source.Session{})
	if err != nil {
		t.Fatalf("30nama parameters: %v", err)
	}
	site := newZarFakeSite(t)
	zar, err := zarfilm{}.Parameters(context.Background(), source.NewClient(), zarCfg(site), zarSession("abc"))
	if err != nil {
		t.Fatalf("zarfilm parameters: %v", err)
	}

	for _, order := range [][]source.SearchParameters{{tn, zar}, {zar, tn}} {
		got := source.IntersectParameters(order)
		want := map[string][]source.FacetOption{
			"types": got.Types, "genres": got.Genres, "languages": got.Languages,
			"countries": got.Countries, "scores": got.Scores, "sorts": got.Sorts,
		}
		for kind, opts := range want {
			if len(opts) == 0 {
				t.Errorf("combined %s is empty", kind)
			}
		}
		has := func(opts []source.FacetOption, slug string) bool {
			for _, o := range opts {
				if o.Slug == slug {
					return true
				}
			}
			return false
		}
		if !has(got.Types, "movie") || !has(got.Types, "series") {
			t.Errorf("types = %+v", got.Types)
		}
		if !has(got.Genres, "drama") {
			t.Errorf("genres = %+v", got.Genres)
		}
		if !has(got.Languages, "en") {
			t.Errorf("languages = %+v", got.Languages)
		}
		if !has(got.Scores, "score-9") || !has(got.Scores, "score-under-5") {
			t.Errorf("scores = %+v", got.Scores)
		}
		if !has(got.Sorts, "imdb") || !has(got.Sorts, "date") || has(got.Sorts, "year") {
			t.Errorf("sorts = %+v", got.Sorts)
		}
		var japan bool
		for _, c := range got.Countries {
			if c.Name == "ژاپن" {
				japan = true
			}
		}
		if !japan {
			t.Errorf("countries = %+v", got.Countries)
		}
		if got.MinYear == 0 || got.MaxYear == 0 {
			t.Errorf("years = %d–%d", got.MinYear, got.MaxYear)
		}
	}
}
