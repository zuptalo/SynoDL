package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"synodl/server/internal/source"
	"synodl/server/internal/store"
)

// fakeAddrSrc is the fake driver with no address of its own — the shape the
// zarfilm driver has since spec 1045.
type fakeAddrSrc struct{ fakeSrc }

func init() { source.Register(fakeAddrSrc{}) }

func (fakeAddrSrc) Kind() string          { return "fakeaddr" }
func (fakeAddrSrc) RequiresAddress() bool { return true }
func (fakeAddrSrc) Hosts() source.Config {
	return source.Config{DownloadHosts: []string{"dl.fake"}}
}

func addrBody(extra string) string {
	return `{"kind":"fakeaddr","displayName":"Addr","moviesParent":"movie","tvParent":"tv-show",` +
		extra + `"session":{"c_token":"TOK","user_agent":"UA"}}`
}

// Spec 1045. A source whose driver has no address of its own cannot be saved
// without one — it would point nowhere.
func TestAnAddressIsRequiredWhereTheDriverHasNone(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	rec := do(t, h, "POST", "/v1/source/providers", addrBody(""), admin)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "address_required") {
		t.Fatalf("create with no address = %d %s, want 422 address_required", rec.Code, rec.Body.String())
	}

	rec = do(t, h, "POST", "/v1/source/providers", addrBody(`"mainBase":"https://zhomis.example",`), admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create with an address = %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		MainBase string `json:"mainBase"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.MainBase != "https://zhomis.example" {
		t.Fatalf("mainBase = %q", created.MainBase)
	}
}

// The only address given is the one meant, whichever field it went in.
func TestAnAlternateAloneBecomesTheAddress(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	rec := do(t, h, "POST", "/v1/source/providers", addrBody(`"altBase":"https://zhomis.example",`), admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		MainBase string `json:"mainBase"`
		AltBase  string `json:"altBase"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.MainBase != "https://zhomis.example" || created.AltBase != "" {
		t.Fatalf("addresses = main %q alt %q, want the alternate promoted", created.MainBase, created.AltBase)
	}
	// And verification saw it.
	if fakeLastSession.Fields["c_token"] != "TOK" {
		t.Fatal("the source was not verified")
	}
}

// The form learns it must ask for the address.
func TestTheKindSaysItNeedsAnAddress(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	rec := do(t, h, "GET", "/v1/source/providers", "", admin)
	var got struct {
		Kinds []struct {
			Kind            string `json:"kind"`
			AddressRequired bool   `json:"addressRequired"`
		} `json:"kinds"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	seen := map[string]bool{}
	for _, k := range got.Kinds {
		seen[k.Kind] = k.AddressRequired
	}
	if !seen["fakeaddr"] || seen["faketest"] {
		t.Fatalf("addressRequired = %v, want only fakeaddr", seen)
	}
}

// The older single-source route has no address fields, so it refuses such a
// driver rather than saving a source that points nowhere.
func TestTheLegacyRouteRefusesADriverWithNoAddress(t *testing.T) {
	resetFake()
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	rec := do(t, h, "PUT", "/v1/source/session",
		`{"kind":"fakeaddr","moviesParent":"movie","tvParent":"tv-show","session":{"c_token":"TOK"}}`, admin)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("legacy save = %d %s, want 422", rec.Code, rec.Body.String())
	}
}

// A source saved before the address was required, with only an alternate one,
// keeps working: that address — and its own sign-in material — stand in as the
// main ones.
func TestAStoredAlternateStandsInAsTheAddress(t *testing.T) {
	resetFake()
	_, st := newStatefulRouter(t)
	id, err := st.CreateProvider(store.SourceProvider{
		Kind: "fakeaddr", DisplayName: "Old", Enabled: true, State: store.SourceActive,
		AltBase: "https://zhomis.example",
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	_ = st.SaveProviderSession(id, store.SourceSession{
		Fields: map[string]string{"c_token": "MAIN"},
		Alt:    &store.SourceSession{Fields: map[string]string{"c_token": "ALT"}},
	}, 1)

	refs, _ := Deps{Store: st}.sourceRefs()
	if len(refs) != 1 {
		t.Fatalf("refs = %d", len(refs))
	}
	cfg := refs[0].Cfg
	if cfg.MainBase != "https://zhomis.example" || cfg.AltBase != "" {
		t.Fatalf("addresses = main %q alt %q", cfg.MainBase, cfg.AltBase)
	}
	if refs[0].Sess.Fields["c_token"] != "ALT" {
		t.Fatalf("session = %v, want the alternate address's own", refs[0].Sess.Fields)
	}
	if !source.HostAllowed("zhomis.example", cfg.APIHosts) {
		t.Fatalf("the address is not reachable: %v", cfg.APIHosts)
	}
}
