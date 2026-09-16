package api

import (
	"context"
	"net/http"
	"time"

	"synodl/server/internal/people"
	"synodl/server/internal/source"
)

// A person's photograph, served same-origin (spec 0014).
//
// This is the THIRD image proxy in this codebase, and the duplication is
// deliberate for exactly the reason spec 1034 gives for the second one: the
// catalog poster proxy's allowlist is assembled from the download-source drivers
// plus the operator's configured mirrors, and IMDb is neither. Sharing one list
// would mean an operator editing a source could change what this may fetch, and
// a change here could widen where source posters may come from.
//
// It differs from both of the others in one way that matters. They proxy a URL
// the caller already had; this one performs a LOOKUP, so a caller could use it
// to make the server talk to a third party. That is why it takes a person ID
// rather than a URL — a shape that cannot name a host at all (FR-019) — why the
// lookups behind it are capped instance-wide, and why the route is rate-limited.
//
// Unauthenticated, like both of the others, and for the same unavoidable reason:
// an <img> tag cannot send the session header. Gating it would mean a token in a
// URL, where it lands in logs and history, to protect something public.

// personPhotos holds ~32 MB of faces. Its own cache, not the poster cache, so a
// busy Discover grid cannot evict every face and a large cast cannot evict every
// poster.
var personPhotos = newImageCache(32 << 20)

// handlePersonPhoto serves one person's photograph, or 404 when there is none.
//
// A 404 here is an ordinary answer, not an error: it is what a person nobody has
// a picture of looks like, and it is what a blocked or unreachable IMDb looks
// like too. The client draws initials on it (FR-023, FR-031). Nothing about the
// failure reaches the user, and no upstream body ever does.
func handlePersonPhoto(d Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("imdbId")
		if !source.PersonIDRe.MatchString(id) {
			// Rejected before any outbound request exists — not after one came
			// back (FR-019).
			http.Error(w, "bad_person_id", http.StatusBadRequest)
			return
		}
		if d.people == nil {
			http.NotFound(w, r)
			return
		}
		// A face is as immutable as a poster for caching purposes, and the client
		// asks for the same handful on every sheet.
		w.Header().Set("Cache-Control", "public, max-age=604800, immutable")

		if img, ok := personPhotos.get(id); ok {
			if len(img.body) == 0 {
				// A remembered "there is no photograph of this person". Cached as
				// a 404 so a missing face costs nothing on the next sheet either.
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", img.contentType)
			w.Header().Set("X-Cache", "HIT")
			_, _ = w.Write(img.body)
			return
		}

		// Bounded by the caller's own timeout: a slow third party must never hold
		// a server goroutine open for longer than the client is waiting.
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()

		url := d.people.Photo(ctx, id)
		if url == "" {
			personPhotos.put(id, cachedImage{})
			http.NotFound(w, r)
			return
		}
		body, ct, err := people.FetchImage(ctx, url)
		if err != nil || len(body) == 0 {
			// The location was known but the image would not come. Not cached as
			// "none": the answer we have is still good, only this fetch failed.
			http.NotFound(w, r)
			return
		}
		personPhotos.put(id, cachedImage{body: body, contentType: ct})
		w.Header().Set("Content-Type", ct)
		w.Header().Set("X-Cache", "MISS")
		_, _ = w.Write(body)
	})
}
