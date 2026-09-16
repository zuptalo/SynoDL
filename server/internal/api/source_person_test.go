package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"synodl/server/internal/people"
)

// The endpoint's whole defence is the shape of the one value a client supplies.
// A URL parameter would have been an open-relay shape with a host check bolted
// on; an id cannot name a host at all (FR-019).
func TestPersonPhotoRejectsAnythingButAPersonID(t *testing.T) {
	// A resolver that fails the test if it is ever reached: a bad id must be
	// refused BEFORE any outbound request exists.
	r := people.New(nil)
	r.SetLookupForTest(func(string) (string, error) {
		t.Fatal("a rejected id reached the lookup")
		return "", nil
	})
	h := handlePersonPhoto(Deps{people: r})

	for _, bad := range []string{
		"tt2948372",           // a title id
		"nm12",                // too short
		"nm00006211234567890", // too long
		"NM0000621",           // the route is lowercase
		"nm0000621x",
		"..",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/source/person/"+bad+"/photo", nil)
		req.SetPathValue("imdbId", bad)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%q -> %d, want 400", bad, rec.Code)
		}
		if strings.Contains(rec.Body.String(), bad) {
			t.Fatalf("the rejected value was echoed back: %q", rec.Body.String())
		}
	}
}

// A person nobody has a picture of is a 404, and a 404 is an ordinary answer the
// client draws initials on — never an error, and never a 5xx (FR-023).
func TestPersonPhotoMissIs404(t *testing.T) {
	r := people.New(nil)
	r.SetLookupForTest(func(string) (string, error) { return "", nil })
	h := handlePersonPhoto(Deps{people: r})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/source/person/nm0000621/photo", nil)
	req.SetPathValue("imdbId", "nm0000621")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// An upstream that is blocked, slow or broken is the same 404. Nothing about the
// failure reaches the user, and no upstream body ever does.
func TestPersonPhotoUpstreamFailureIs404(t *testing.T) {
	r := people.New(nil)
	r.SetLookupForTest(func(string) (string, error) {
		return "", errBlockedForTest
	})
	h := handlePersonPhoto(Deps{people: r})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/source/person/nm0000002/photo", nil)
	req.SetPathValue("imdbId", "nm0000002")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if body := rec.Body.String(); strings.Contains(body, "blocked") {
		t.Fatalf("an upstream failure leaked into the response: %q", body)
	}
}

var errBlockedForTest = errTest("blocked by upstream")

type errTest string

func (e errTest) Error() string { return string(e) }
