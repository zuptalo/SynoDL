package authz

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"movie", "movie", true},
		{"/movie/", "movie", true},
		{"tv-show/Friends", "tv-show/Friends", true},
		{"  home/Downloads  ", "home/Downloads", true},
		{"", "", false},
		{"/", "", false},
		{"..", "", false},
		{"movie/../home", "", false},
		{"../secret", "", false},
		{"a//b", "", false},
		{"a/./b", "", false},
	}
	for _, c := range cases {
		got, ok := Normalize(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("Normalize(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestAllowedForCreate(t *testing.T) {
	grants := []string{"movie", "tv-show/Friends"}

	// Admin: anywhere valid, nowhere invalid.
	if !AllowedForCreate(true, nil, "anything/here") {
		t.Error("admin should be allowed anywhere")
	}
	if AllowedForCreate(true, nil, "../escape") {
		t.Error("admin still rejected for traversal")
	}

	allowed := []string{"movie", "movie/4K", "tv-show/Friends", "tv-show/Friends/S01"}
	for _, d := range allowed {
		if !AllowedForCreate(false, grants, d) {
			t.Errorf("non-admin should be allowed %q", d)
		}
	}
	denied := []string{"tv-show", "tv-show/TheWire", "home", "music/x", "movieextra", "../movie", "tv-show/Friends/../../home"}
	for _, d := range denied {
		if AllowedForCreate(false, grants, d) {
			t.Errorf("non-admin should be DENIED %q", d)
		}
	}

	// No grants ⇒ nothing allowed.
	if AllowedForCreate(false, nil, "movie") {
		t.Error("non-admin with no grants should be denied everything")
	}
}

func TestVisibleInPicker(t *testing.T) {
	grants := []string{"tv-show/Friends"}

	if !VisibleInPicker(true, nil, "anything") {
		t.Error("admin sees everything")
	}
	// Ancestor of the grant is navigable; the grant + descendants are visible.
	for _, p := range []string{"tv-show", "tv-show/Friends", "tv-show/Friends/S01"} {
		if !VisibleInPicker(false, grants, p) {
			t.Errorf("should be visible: %q", p)
		}
	}
	// Siblings / unrelated are hidden.
	for _, p := range []string{"tv-show/TheWire", "movie", "home"} {
		if VisibleInPicker(false, grants, p) {
			t.Errorf("should be hidden: %q", p)
		}
	}
}
