package source

import (
	"strconv"
	"strings"
)

// Source-qualified title identifiers (spec 0007).
//
// In combined mode the client holds titles from several sources in one list and
// must be able to hand one back unambiguously. So an id on the wire is
// "<providerID>:<providerTitleID>".
//
// The split is on the FIRST colon only, because a driver's own id is opaque and
// may contain almost anything — zarfilm's are URL paths like
// "series/the-loyalty-game", which contain slashes. It may not contain a colon,
// which is the one character this scheme reserves.
//
// Security note: the trailing portion arrives from a client and is passed to a
// driver. A driver MUST treat it as a value to encode into its own URL
// construction, never as a URL or a path to join blindly — otherwise a crafted
// id could change which host is contacted. ValidateTitleID enforces the part of
// that which can be checked centrally; the rest is a driver obligation, covered
// by each driver's tests (FR-033).

// QualifyID renders a driver-side id as a wire id for one source.
func QualifyID(providerID int64, titleID string) string {
	return strconv.FormatInt(providerID, 10) + ":" + titleID
}

// SplitID parses a wire id back into its provider and driver-side parts.
// ok=false for anything malformed: no separator, a non-numeric or non-positive
// provider portion, an empty title portion, or a title portion that fails
// ValidateTitleID. A malformed id is a client error, never a silent miss
// (FR-034).
func SplitID(wire string) (providerID int64, titleID string, ok bool) {
	i := strings.IndexByte(wire, ':')
	if i <= 0 || i == len(wire)-1 {
		return 0, "", false
	}
	id, err := strconv.ParseInt(wire[:i], 10, 64)
	if err != nil || id <= 0 {
		return 0, "", false
	}
	rest := wire[i+1:]
	if !ValidateTitleID(rest) {
		return 0, "", false
	}
	return id, rest, true
}

// ValidateTitleID rejects driver-side ids that could escape the source's own
// site if a driver were careless about how it builds a URL. This is defence in
// depth: a correct driver encodes the id as a path segment or query value and is
// unaffected, but the ids come from a client, and one central check is cheaper
// than trusting every present and future driver to be careful.
//
// Rejected:
//   - a scheme or protocol-relative prefix ("http://", "//evil.example") — the
//     classic way a joined "path" silently becomes a different host
//   - "..", which can climb out of an intended prefix
//   - a leading "/", which makes a join absolute and discards the base path
//   - control characters, including the CR/LF that enable header and request
//     splitting, and the NUL that can truncate in downstream C-backed code
//   - anything implausibly long
func ValidateTitleID(id string) bool {
	if id == "" || len(id) > 512 {
		return false
	}
	if strings.HasPrefix(id, "/") || strings.HasPrefix(id, "\\") {
		return false
	}
	if strings.Contains(id, "://") || strings.HasPrefix(id, "//") {
		return false
	}
	// Any ".." segment, not just a literal "../": "a/../../b" and a bare ".."
	// both climb.
	for _, seg := range strings.Split(id, "/") {
		if seg == ".." {
			return false
		}
	}
	for _, r := range id {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}
