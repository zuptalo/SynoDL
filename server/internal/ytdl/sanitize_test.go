package ytdl

import (
	"strings"
	"testing"
)

// FR-038a. The property under test is not "the name looks tidy" — it is that NO
// input can put a file outside the library the request was aimed at.
func TestSanitizeName_NeverEscapesTheLibrary(t *testing.T) {
	hostile := []string{
		"../../../etc",
		"..",
		".",
		"...",
		"../",
		"/etc/passwd",
		"/",
		`..\..\windows`,
		`C:\Windows`,
		"a/../../b",
		"nested/path/name",
		"trailing/",
		"\x00etc",
		"name\x00/../../etc",
		"\n../\n..",
		"....//....//etc",
	}
	for _, in := range hostile {
		got := SanitizeName(in)
		if strings.ContainsAny(got, `/\`) {
			t.Errorf("SanitizeName(%q) = %q, which is more than one path component", in, got)
		}
		if got == "." || got == ".." || strings.Trim(got, ".") == "" && got != "" {
			t.Errorf("SanitizeName(%q) = %q, which names a directory rather than a folder", in, got)
		}
		if strings.HasPrefix(got, ".") {
			t.Errorf("SanitizeName(%q) = %q, a hidden folder the media server will not look in", in, got)
		}
		if strings.ContainsRune(got, 0) {
			t.Errorf("SanitizeName(%q) = %q, which contains NUL", in, got)
		}
	}
}

func TestSanitizeName_KeepsOrdinaryNamesIntact(t *testing.T) {
	// The other half of the job: a sanitiser that mangles real names is a
	// different bug, not a safer one.
	for _, in := range []string{
		"Lo-fi Beats",
		"Queen",
		"Rock & Roll, Vol. 2",
		"Sigur Rós",
		"ゆゆ式",
		"موسیقی",
		"AC/DC", // the one real name that contains a separator
	} {
		got := SanitizeName(in)
		if got == "" {
			t.Errorf("SanitizeName(%q) discarded a perfectly good name", in)
		}
		if strings.ContainsAny(got, `/\`) {
			t.Errorf("SanitizeName(%q) = %q still has a separator", in, got)
		}
	}
	if got := SanitizeName("Lo-fi Beats"); got != "Lo-fi Beats" {
		t.Errorf("SanitizeName kept-as-is check: %q", got)
	}
	// AC/DC becomes AC DC rather than being thrown away — recognisable, and one
	// component.
	if got := SanitizeName("AC/DC"); got != "AC DC" {
		t.Errorf("SanitizeName(%q) = %q, want the separator replaced not the name lost", "AC/DC", got)
	}
}

func TestSanitizeName_IsBounded(t *testing.T) {
	// FR-038b. A pathological title must not produce an unusable path or an
	// argument nobody can read.
	long := strings.Repeat("a", 5000)
	got := SanitizeName(long)
	if len(got) > MaxNameLength {
		t.Fatalf("length %d, want it capped at %d", len(got), MaxNameLength)
	}

	// Cut on a rune boundary: a multi-byte name must not end in half a
	// character, which would make the name invalid rather than merely long.
	multi := strings.Repeat("日", 5000)
	got = SanitizeName(multi)
	if len(got) > MaxNameLength {
		t.Fatalf("multi-byte length %d, want <= %d", len(got), MaxNameLength)
	}
	if !isValidUTF8(got) {
		t.Fatalf("truncation split a character: %q", got)
	}
}

func TestSanitizeName_EmptyIsARealAnswer(t *testing.T) {
	// Nothing usable surviving is a legitimate outcome, and callers fall back
	// rather than treating it as an error.
	for _, in := range []string{"", "   ", "...", "///", "\x00", "\n\t "} {
		if got := SanitizeName(in); got != "" {
			t.Errorf("SanitizeName(%q) = %q, want empty", in, got)
		}
	}
}

func TestSanitizeName_NoTrailingDotOrSpace(t *testing.T) {
	// Some filesystems silently drop these, so two names differing only there
	// would collide — and the collision would be invisible in the app.
	for _, in := range []string{"Album.", "Album ", "Album. ", "Album..."} {
		got := SanitizeName(in)
		if strings.HasSuffix(got, ".") || strings.HasSuffix(got, " ") {
			t.Errorf("SanitizeName(%q) = %q, which ends in a dot or space", in, got)
		}
	}
}

func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == 0xFFFD {
			return false
		}
	}
	return true
}
