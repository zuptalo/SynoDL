package source

import "testing"

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
