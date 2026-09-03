package source

import "testing"

func TestQualifyAndSplitRoundTrip(t *testing.T) {
	for _, id := range []string{
		"the-whisper-man-2026",
		"series/the-loyalty-game", // zarfilm ids are URL paths containing slashes
		"12345",
		"a b c",
	} {
		wire := QualifyID(7, id)
		pid, got, ok := SplitID(wire)
		if !ok || pid != 7 || got != id {
			t.Fatalf("round trip %q -> %q -> (%d, %q, %v)", id, wire, pid, got, ok)
		}
	}
}

// The split is on the FIRST colon only, so a driver-side id may contain colons
// in principle without the provider portion being misread.
func TestSplitIDUsesFirstColonOnly(t *testing.T) {
	pid, id, ok := SplitID("3:some:thing")
	if !ok || pid != 3 || id != "some:thing" {
		t.Fatalf("got (%d, %q, %v)", pid, id, ok)
	}
}

// FR-034: a malformed id is a client error, never a silent miss.
func TestSplitIDRejectsMalformed(t *testing.T) {
	for _, bad := range []string{
		"", "nocolon", ":leading", "7:", "abc:x", "-1:x", "0:x", "1.5:x",
	} {
		if _, _, ok := SplitID(bad); ok {
			t.Fatalf("accepted malformed id %q", bad)
		}
	}
}

// FR-033: an id arriving from a client must not be able to change which host is
// contacted or climb out of the source's own site, even if a driver were
// careless about how it builds a URL.
func TestValidateTitleIDRejectsEscapes(t *testing.T) {
	hostile := []string{
		"http://evil.example/x",   // absolute URL
		"https://evil.example/x",  // absolute URL, TLS
		"//evil.example/x",        // protocol-relative: joins as a new host
		"/etc/passwd",             // absolute path discards the base
		"\\evil",                  // backslash variant
		"..",                      // bare climb
		"../../etc/passwd",        // classic traversal
		"a/../../b",               // climb hidden mid-path
		"a\r\nHost: evil.example", // header / request splitting
		"a\x00b",                  // NUL truncation
		"a\nb",
	}
	for _, h := range hostile {
		if ValidateTitleID(h) {
			t.Fatalf("accepted hostile id %q", h)
		}
		if _, _, ok := SplitID("1:" + h); ok {
			t.Fatalf("SplitID accepted hostile id %q", h)
		}
	}
}

func TestValidateTitleIDAcceptsRealIDs(t *testing.T) {
	for _, good := range []string{
		"the-whisper-man-2026",
		"series/the-loyalty-game",
		"movie.with.dots-2019",
		"a..b", // dots inside a segment are fine; only a ".." SEGMENT climbs
	} {
		if !ValidateTitleID(good) {
			t.Fatalf("rejected legitimate id %q", good)
		}
	}
}

func TestValidateTitleIDBoundsLength(t *testing.T) {
	long := make([]byte, 513)
	for i := range long {
		long[i] = 'a'
	}
	if ValidateTitleID(string(long)) {
		t.Fatal("accepted an implausibly long id")
	}
}
