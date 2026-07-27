// Package authz enforces per-user NAS folder access (spec 0003, Increment 3).
// Because all SynoDL users share one NAS account, DSM cannot express per-user
// permissions — SynoDL enforces them here, server-side, on every folder listing
// and task creation. Paths are DSM destination paths without a leading slash
// (e.g. "movie", "tv-show/Friends"); traversal is rejected.
package authz

import "strings"

// Normalize cleans a folder path to "a/b/c" form (no leading/trailing slashes)
// and rejects traversal. ok is false for empty results or any ".", ".." or
// empty segment — so a crafted "../secret" or "movie/../home" never validates.
func Normalize(path string) (clean string, ok bool) {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, "/")
	if path == "" {
		return "", false
	}
	segs := strings.Split(path, "/")
	for _, s := range segs {
		if s == "" || s == "." || s == ".." {
			return "", false
		}
	}
	return strings.Join(segs, "/"), true
}

func normalizeGrants(grants []string) []string {
	out := make([]string, 0, len(grants))
	for _, g := range grants {
		if c, ok := Normalize(g); ok {
			out = append(out, c)
		}
	}
	return out
}

// within reports whether dest is the folder g itself or lives under it.
func within(g, dest string) bool {
	return dest == g || strings.HasPrefix(dest, g+"/")
}

// AllowedForCreate reports whether a task may target dest. Admins may target
// anywhere; a non-admin may target a folder only if it equals or is under one of
// their grants. A non-admin with no grants can target nothing (safe default).
func AllowedForCreate(isAdmin bool, grants []string, dest string) bool {
	if isAdmin {
		_, ok := Normalize(dest)
		return ok
	}
	clean, ok := Normalize(dest)
	if !ok {
		return false
	}
	for _, g := range normalizeGrants(grants) {
		if within(g, clean) {
			return true
		}
	}
	return false
}

// VisibleInPicker reports whether a folder should appear in the destination
// picker for the user. Admins see everything. A non-admin sees a folder if it is
// within a grant OR an ancestor of a grant (ancestors must be navigable to reach
// a granted subfolder).
func VisibleInPicker(isAdmin bool, grants []string, path string) bool {
	if isAdmin {
		return true
	}
	clean, ok := Normalize(path)
	if !ok {
		return false
	}
	for _, g := range normalizeGrants(grants) {
		if within(g, clean) || strings.HasPrefix(g, clean+"/") {
			return true
		}
	}
	return false
}
