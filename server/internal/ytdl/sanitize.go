package ytdl

import (
	"strings"
	"unicode"
)

// Making a source-derived name safe to use as a folder (spec 0013, FR-038a/b).
//
// This is the highest-risk thing in the feature, and it is worth being precise
// about why, because the obvious reading is wrong.
//
// A playlist or channel name is passed to a worker as album metadata, and the
// constitution's rule for user-influenced worker input is that it must reach the
// worker as a discrete argv element, never interpolated into a shell string.
// That rule is about COMMAND INJECTION, and it is satisfied — Container.Args is
// an array and nothing here concatenates.
//
// But the same value also becomes a FOLDER NAME inside the mounted media
// library, because the output template files a track under its album. Discrete
// argv passing does nothing whatsoever about a path. A source that names itself
// "../../../etc" is not injecting a command; it is choosing a directory. Those
// are two different problems with two different mitigations, and spec 0013's
// checklist caught that the spec had answered only one of them.
//
// The extractor does sanitise output-template values itself. That is a reason to
// expect this to be belt AND braces rather than a reason to omit the belt: "the
// dependency probably handles it" is not the standard for the one thing that
// would put files outside the library a request was aimed at.

// MaxNameLength bounds a source-derived name.
//
// Filesystem components are usually capped at 255 bytes, and a name is only one
// component of a path that already has a library root and a track name in it.
// This leaves room and keeps a pathological title from producing an argument
// list nobody can read.
const MaxNameLength = 120

// SanitizeName makes a source-derived name safe to use as a path component.
//
// Returns "" when nothing usable survives, which callers treat as "no name" and
// fall back accordingly — an empty result is a real answer here, not a failure.
func SanitizeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	// 1. Anything that could make this more than one path component goes.
	//    Separators for both conventions, and NUL, which terminates a path early
	//    in anything that reaches C.
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '/', r == '\\', r == 0:
			b.WriteRune(' ')
		case r < 0x20 || r == 0x7f:
			// Control characters: never useful in a name, and they make a
			// directory listing lie about what is in it.
			b.WriteRune(' ')
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	// 2. Drop any token that is ONLY dots.
	//
	//    Step 1 turned every separator into a space, so "../../../etc" is by now
	//    ".. .. .. etc" — the traversal is gone as a path, but ".." is still
	//    sitting at the front, and trimming leading dots would stop at the first
	//    space and leave it there. Removing dot-only tokens outright is both
	//    simpler and more honest about the rule: a token made of nothing but
	//    dots carries no name, whatever it meant to the filesystem.
	fields := strings.Fields(b.String())
	kept := fields[:0]
	for _, f := range fields {
		if strings.Trim(f, ".") == "" {
			continue
		}
		kept = append(kept, f)
	}
	out := strings.Join(kept, " ")
	if out == "" {
		return ""
	}
	// A leading dot makes a hidden folder on unix, which is not what a media
	// server will look in.
	out = strings.TrimLeft(out, ".")
	// A trailing dot or space is silently dropped by some filesystems, so two
	// names that differ only there would collide.
	out = strings.TrimRight(out, ". ")

	// 3. Bound the length. Cut on a rune boundary, not a byte one, or a
	//    multi-byte name ends in half a character.
	if len(out) > MaxNameLength {
		runes := []rune(out)
		for len(string(runes)) > MaxNameLength {
			runes = runes[:len(runes)-1]
		}
		out = strings.TrimRight(string(runes), ". ")
	}
	return strings.TrimSpace(out)
}
