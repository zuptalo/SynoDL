package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"synodl/server/internal/httpx"
	"synodl/server/internal/store"
	"synodl/server/internal/syno"
)

// maxFavorites caps how many favorite destinations a user may keep.
const maxFavorites = 4

// destPrefsView is the JSON shape for a user's destination preferences: a
// default folder (empty = root) and an ordered list of favorites (no leading
// slash, e.g. "tv-show/Friends").
type destPrefsView struct {
	Default   string   `json:"default"`
	Favorites []string `json:"favorites"`
}

// handleGetDestinationPrefs returns the user's default + favorites, first
// dropping any that access has been revoked from or that no longer exist on the
// NAS (the default resets to root when it's gone). The cleaned set is persisted
// so the removal sticks.
func handleGetDestinationPrefs(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		def, favs, err := d.Store.GetDestinationPrefs(u.ID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		all := append(append([]string{}, favs...), nonEmpty(def)...)
		keptSet := map[string]bool{}
		for _, p := range d.validDestinations(r.Context(), u, all) {
			keptSet[p] = true
		}
		newFavs := make([]string, 0, len(favs))
		for _, f := range favs {
			if keptSet[f] {
				newFavs = append(newFavs, f)
			}
		}
		newDef := def
		if def != "" && !keptSet[def] {
			newDef = "" // gone / revoked → reset the default to root
		}
		if newDef != def || len(newFavs) != len(favs) {
			_ = d.Store.SaveDestinationPrefs(u.ID, newDef, newFavs, time.Now().Unix())
		}
		httpx.JSON(w, http.StatusOK, destPrefsView{Default: newDef, Favorites: newFavs})
	})
}

// handleSetDestinationPrefs saves the user's default + favorites, normalizing,
// de-duplicating, capping favorites, and dropping any the user isn't allowed to
// use (so the UI can't smuggle a forbidden folder in).
func handleSetDestinationPrefs(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		var body destPrefsView
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		favs := make([]string, 0, maxFavorites)
		seen := map[string]bool{}
		for _, f := range body.Favorites {
			f = strings.Trim(strings.TrimSpace(f), "/")
			if f == "" || seen[f] || !d.destinationAllowed(u, f) {
				continue
			}
			seen[f] = true
			favs = append(favs, f)
			if len(favs) >= maxFavorites {
				break
			}
		}
		def := strings.Trim(strings.TrimSpace(body.Default), "/")
		if def != "" && !d.destinationAllowed(u, def) {
			def = ""
		}
		if err := d.Store.SaveDestinationPrefs(u.ID, def, favs, time.Now().Unix()); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		httpx.JSON(w, http.StatusOK, destPrefsView{Default: def, Favorites: favs})
	})
}

// validDestinations filters paths to those the user may still use AND that still
// exist on the NAS. Access checks are in-memory (so revoked access always
// drops); existence is checked by listing each distinct parent once. On a NAS
// error it keeps the access-allowed set (a transient outage never wipes prefs
// by existence).
func (d Deps) validDestinations(ctx context.Context, u *store.User, paths []string) []string {
	allowed := make([]string, 0, len(paths))
	for _, p := range paths {
		if d.destinationAllowed(u, p) {
			allowed = append(allowed, p)
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	parents := map[string]struct{}{}
	for _, p := range allowed {
		parents[parentOf(p)] = struct{}{}
	}
	children := map[string]map[string]bool{}
	err := d.NAS.Do(ctx, func(c syno.Client, sid string) error {
		for parent := range parents {
			var folders []syno.Folder
			var e error
			if parent == "" {
				folders, e = c.ListShares(ctx, sid)
			} else {
				folders, e = c.ListFolder(ctx, sid, "/"+parent)
			}
			if e != nil {
				return e
			}
			set := make(map[string]bool, len(folders))
			for _, f := range folders {
				set[f.Name] = true
			}
			children[parent] = set
		}
		return nil
	})
	if err != nil {
		return allowed // NAS unreachable (or a parent gone) — don't over-prune
	}
	kept := make([]string, 0, len(allowed))
	for _, p := range allowed {
		if set := children[parentOf(p)]; set != nil && set[leafOf(p)] {
			kept = append(kept, p)
		}
	}
	return kept
}

func parentOf(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return "" // top-level folder — its parent is the share root
}
func leafOf(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}
func nonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}
