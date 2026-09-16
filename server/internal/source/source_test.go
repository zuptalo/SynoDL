package source

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// Spec 0009: material belongs to an ADDRESS, not to a source. A challenge cookie
// is issued per domain and a login cookie is tied to the address that issued it,
// so sending the main address's material to the mirror is how an outage turned
// into a catalog that answered every request with a login page.
func TestConfigSendsEachAddressItsOwnMaterial(t *testing.T) {
	mainSess := Session{Fields: map[string]string{"cf_clearance": "for-main"}}
	altSess := Session{Fields: map[string]string{"cf_clearance": "for-alt"}}
	cfg := Config{MainBase: "https://main.example", AltBase: "https://mirror.example", AltSession: &altSess}

	if got := cfg.SessionFor("https://mirror.example", mainSess); got.Fields["cf_clearance"] != "for-alt" {
		t.Errorf("mirror got %q, want its own material", got.Fields["cf_clearance"])
	}
	if got := cfg.SessionFor("https://main.example", mainSess); got.Fields["cf_clearance"] != "for-main" {
		t.Errorf("main got %q, want its own material", got.Fields["cf_clearance"])
	}
	// A trailing slash is the same address.
	if got := cfg.SessionFor("https://mirror.example/", mainSess); got.Fields["cf_clearance"] != "for-alt" {
		t.Error("a trailing slash must not change which address this is")
	}
}

// FR-006: one set keeps working exactly as it does today.
func TestConfigWithOneSetSendsItEverywhere(t *testing.T) {
	only := Session{Fields: map[string]string{"cf_clearance": "the-only-one"}}
	cfg := Config{MainBase: "https://main.example", AltBase: "https://mirror.example"}

	for _, base := range []string{"https://main.example", "https://mirror.example"} {
		if got := cfg.SessionFor(base, only); got.Fields["cf_clearance"] != "the-only-one" {
			t.Errorf("%s got %q, want the source's single set", base, got.Fields["cf_clearance"])
		}
	}
}

// Values about a person all arrive from outside — a name, a character, an image
// URL — so they are bounded before they can reach the operator's volume or a
// viewer's DOM (FR-030a). A clipped name must still be valid UTF-8: cutting a
// multi-byte rune in half would put a replacement character in somebody's name.
func TestPersonClampBoundsExternalValues(t *testing.T) {
	long := strings.Repeat("ن", 500) // 500 runes, 1000 bytes
	p, ok := Person{
		Name:      long,
		Character: long,
		PhotoURL:  "https://x.invalid/" + strings.Repeat("a", 5000),
		IMDbID:    "nm0000621",
	}.Clamp()
	if !ok {
		t.Fatal("a person with a name should be worth showing")
	}
	if len(p.Name) > maxPersonName || len(p.Character) > maxPersonName {
		t.Fatalf("name/character not bounded: %d/%d", len(p.Name), len(p.Character))
	}
	if !utf8.ValidString(p.Name) {
		t.Fatal("clipped name is not valid UTF-8")
	}
	if len(p.PhotoURL) > maxPersonURL {
		t.Fatalf("photo URL not bounded: %d", len(p.PhotoURL))
	}
	if p.IMDbID != "nm0000621" {
		t.Fatalf("a valid id was altered: %q", p.IMDbID)
	}
}

// Anything that is not an IMDb person id is dropped rather than carried: it is
// about to become an href and a cache key.
func TestPersonClampRejectsNonPersonIDs(t *testing.T) {
	for _, bad := range []string{
		"tt2948372", // a TITLE id
		"javascript:alert(1)",
		"nm123", // too short
		"../../etc/passwd",
		"https://imdb.com.evil.example/name/nm0000621/",
	} {
		p, _ := Person{Name: "X", IMDbID: bad}.Clamp()
		if p.IMDbID != "" {
			t.Fatalf("%q survived as an id", bad)
		}
	}
	if got := PersonID("https://www.imdb.com/name/nm0000621/"); got != "nm0000621" {
		t.Fatalf("canonical profile URL not normalised: %q", got)
	}
	if got := PersonID("https://imdb.com.evil.example/name/nm0000621/"); got != "" {
		t.Fatalf("lookalike host accepted: %q", got)
	}
}

// A person with no name is not worth a tile; a role is capped; and a person
// listed twice in ONE role appears once — while two different people who happen
// to share a name do not collapse into each other.
func TestClampPeopleDeduplicatesAndCaps(t *testing.T) {
	in := []Person{
		{Name: "A", IMDbID: "nm0000001"},
		{Name: "A", IMDbID: "nm0000001"}, // same person, twice
		{Name: "B", IMDbID: "nm0000002"},
		{Name: "B", IMDbID: "nm0000003"}, // different people, same name
		{Name: "   "},                    // no name at all
	}
	got := ClampPeople(in, MaxCrew)
	if len(got) != 3 {
		t.Fatalf("want 3 people, got %d: %+v", len(got), got)
	}
	many := make([]Person, 50)
	for i := range many {
		many[i] = Person{Name: "P", IMDbID: fmt.Sprintf("nm%07d", i)}
	}
	if got := ClampPeople(many, MaxCast); len(got) != MaxCast {
		t.Fatalf("cast cap not applied: %d", len(got))
	}
	// Nothing in means nothing out — nil, never an empty slice, because the wire
	// contract says an absent field is how "the source publishes none" is said.
	if got := ClampPeople(nil, MaxCast); got != nil {
		t.Fatalf("want nil, got %+v", got)
	}
}
