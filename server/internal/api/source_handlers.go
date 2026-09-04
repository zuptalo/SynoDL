package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"synodl/server/internal/httpx"
	"synodl/server/internal/library"
	"synodl/server/internal/source"
	_ "synodl/server/internal/source/providers" // register provider drivers
	"synodl/server/internal/store"
	"synodl/server/internal/syno"
)

// fileNameFromURI extracts the download's file name (the last path segment,
// percent-decoded) from a link — this is the name Download Station reports for
// the task, so download history can be correlated to it on completion.
func fileNameFromURI(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	name := path.Base(u.Path)
	if dec, err := url.PathUnescape(name); err == nil {
		name = dec
	}
	if name == "." || name == "/" {
		return ""
	}
	return name
}

// sourceHTTP is the shared outbound client for provider calls (HTTP/2, host-
// allowlisted). Safe for concurrent use; one per process.
var sourceHTTP = source.NewClient()

// --- request/response shapes (no secret is ever serialized back) ---

type sourceStatusView struct {
	Configured     bool   `json:"configured"`
	Enabled        bool   `json:"enabled"`
	State          string `json:"state"`
	ProviderName   string `json:"providerName"`
	Kind           string `json:"kind"`
	MoviesParent   string `json:"moviesParent"`
	TVParent       string `json:"tvParent"`
	MaxDownloadMB  int    `json:"maxDownloadMB"`
	LastVerifiedAt int64  `json:"lastVerifiedAt"`
	CanManage      bool   `json:"canManage"`
}

type sourceSessionReq struct {
	Kind         string `json:"kind"`
	DisplayName  string `json:"displayName"`
	MoviesParent string `json:"moviesParent"`
	TVParent     string `json:"tvParent"`
	Session      struct {
		CFClearance string `json:"cfClearance"`
		CAPIKey     string `json:"cApiKey"`
		CToken      string `json:"cToken"`
		UserAgent   string `json:"userAgent"`
		CPlatform   string `json:"cPlatform"`
		CAppVersion string `json:"cAppVersion"`
	} `json:"session"`
}

type sourceSearchReq struct {
	Query string `json:"query"`
	Page  int    `json:"page"`
	Sort  string `json:"sort"`
	Order string `json:"order"` // "asc" / "desc"; "" = provider default (descending)
	// Source narrows the query to one configured source. "" or absent means every
	// enabled source, combined — which is the default (spec 0007 FR-006). An
	// unknown or removed id degrades to "all" rather than erroring, so a user
	// browsing a source an admin deletes lands somewhere sensible.
	Source  string `json:"source"`
	Filters struct {
		Type     string   `json:"type"`
		Quality  string   `json:"quality"`
		Language string   `json:"language"`
		Country  string   `json:"country"`
		Score    string   `json:"score"`
		Genre    []string `json:"genre"`
		Age      string   `json:"age"`
		Channel  string   `json:"channel"`
		Encoder  string   `json:"encoder"`
		X265     string   `json:"x265"`
		ThreeD   string   `json:"threeD"`
		Cast     string   `json:"cast"`
		Director string   `json:"director"`
		Creator  string   `json:"creator"`
		YearFrom string   `json:"yearFrom"`
		YearTo   string   `json:"yearTo"`
	} `json:"filters"`
}

type sourceSendReq struct {
	TitleID   string `json:"titleId"`
	QualityID string `json:"qualityId"`
	Title     string `json:"title"`
	Type      string `json:"type"` // movie/series/anime — picks the parent folder
	// Episodes, 1-based, select which episodes of a series season pack to send.
	// Empty means everything (a movie, or the whole season).
	Episodes []int `json:"episodes,omitempty"`
	// Catalog metadata carried through so the Tasks list can label the download,
	// show a poster thumbnail, and reopen the title in Discover (spec 1013, 1016).
	Year      string  `json:"year,omitempty"`
	IMDbScore float64 `json:"imdbScore,omitempty"`
	PosterURL string  `json:"posterUrl,omitempty"`
}

// activeSource loads the enabled provider, its driver, outbound config, and the
// decrypted session for a runtime call. ok=false means "no usable provider" —
// the caller returns an unavailable state.
func (d Deps) activeSource() (p *store.SourceProvider, drv source.Provider, cfg source.Config, sess source.Session, ok bool) {
	pr, err := d.Store.GetProvider()
	if err != nil || pr == nil || !pr.Enabled {
		return nil, nil, source.Config{}, source.Session{}, false
	}
	driver, has := source.Get(pr.Kind)
	if !has {
		return nil, nil, source.Config{}, source.Session{}, false
	}
	s, err := d.Store.LoadProviderSession(pr.ID)
	if err != nil || s == nil {
		return nil, nil, source.Config{}, source.Session{}, false
	}
	return pr, driver, source.Config{APIHosts: pr.APIHosts, DownloadHosts: pr.DownloadHosts},
		toSession(s), true
}

// sourceFailThreshold is how many CONSECUTIVE provider auth failures we tolerate
// before declaring the session dead. A single failure is almost always transient
// — e.g. the provider's Cloudflare rate-limiting/challenging one request during a
// burst — and other requests keep succeeding, so nuking the whole source on the
// first blip is wrong. A genuine expiry fails every request, so it crosses this
// threshold quickly.
const sourceFailThreshold = 4

// sourceFailStreak counts consecutive auth failures PER SOURCE. It used to be one
// global counter, which was fine with a single provider and wrong the moment
// there are several: one flaky source would otherwise condemn a healthy one.
var (
	sourceFailMu     sync.Mutex
	sourceFailStreak = map[int64]int{}
)

// noteSourceFailure records one auth failure for a source and returns the new
// consecutive count.
func noteSourceFailure(id int64) int {
	sourceFailMu.Lock()
	defer sourceFailMu.Unlock()
	sourceFailStreak[id]++
	return sourceFailStreak[id]
}

// sourceFailureCount reports a source's current consecutive-failure streak.
func sourceFailureCount(id int64) int {
	sourceFailMu.Lock()
	defer sourceFailMu.Unlock()
	return sourceFailStreak[id]
}

// resetSourceFailures clears every source's streak. Used by tests, and whenever
// an admin re-pastes material — they have just fixed the thing that was failing.
func resetSourceFailures() {
	sourceFailMu.Lock()
	defer sourceFailMu.Unlock()
	sourceFailStreak = map[int64]int{}
}

// clearSourceFailures resets a source's streak after a healthy call.
func clearSourceFailures(id int64) {
	sourceFailMu.Lock()
	defer sourceFailMu.Unlock()
	delete(sourceFailStreak, id)
}

// sourceCallOK records a healthy provider call: clears the failure streak and, if
// the stored state had drifted, re-confirms the session as active.
func (d Deps) sourceCallOK(p *store.SourceProvider) {
	if p == nil {
		return
	}
	clearSourceFailures(p.ID)
	if p.State != store.SourceActive {
		now := time.Now().Unix()
		_ = d.Store.SetProviderState(p.ID, store.SourceActive, now, now)
	}
}

// writeSourceRuntimeErr maps a provider runtime error to the client contract. An
// auth failure only flips the stored state to needs_refresh after several
// CONSECUTIVE failures — a lone transient failure returns a retryable "busy" and
// leaves the session intact. It never echoes secrets or raw upstream bodies.
func (d Deps) writeSourceRuntimeErr(w http.ResponseWriter, providerID int64, err error) {
	if nr, ok := source.AsNeedsRefresh(err); ok {
		if noteSourceFailure(providerID) < sourceFailThreshold {
			// Likely transient (rate-limit/challenge on a burst) — keep the session;
			// the client can retry a moment later.
			httpx.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "source_busy"})
			return
		}
		_ = d.Store.SetProviderState(providerID, store.SourceNeedsRefresh, 0, time.Now().Unix())
		httpx.JSON(w, http.StatusConflict, map[string]any{"error": "source_needs_refresh", "layer": nr.Layer})
		return
	}
	if err == source.ErrHostNotAllowed {
		httpx.JSON(w, http.StatusBadGateway, map[string]any{"error": "send_failed", "reason": "download_host_unreachable"})
		return
	}
	httpx.Error(w, http.StatusBadGateway, "provider_error")
}

// handleSourceStatus returns non-secret provider status for any signed-in user.
func handleSourceStatus(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		providers, err := d.Store.ListProviders()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		view := sourceStatusView{State: store.SourceNotConfigured, CanManage: u.IsAdmin}
		view.MaxDownloadMB, _ = d.Store.GetMaxDownloadMB()
		if len(providers) == 0 {
			httpx.JSON(w, http.StatusOK, view)
			return
		}

		// This status drives a WHOLE-VIEW gate in the client ("the source needs
		// refreshing"), so with several sources it has to be an aggregate. Reporting
		// the first source's health would let one unhealthy source blank a Discover
		// view that the others could fill perfectly well — the exact opposite of
		// degrading gracefully (spec 0007 FR-012).
		//
		// So: healthy if ANY enabled source is usable; needs-refresh only when every
		// one of them is unusable.
		healthy := providers[0]
		state := ""
		for _, p := range providers {
			if !p.Enabled {
				continue
			}
			if state == "" || p.State == store.SourceActive {
				state = p.State
				healthy = p
			}
			if p.State == store.SourceActive {
				break
			}
		}
		if state == "" {
			// Every source is disabled: configured, but nothing to browse.
			state = providers[0].State
		}
		view.Configured = true
		view.Enabled = healthy.Enabled
		view.State = state
		view.ProviderName = healthy.DisplayName
		view.Kind = healthy.Kind
		view.MoviesParent = healthy.MoviesParent
		view.TVParent = healthy.TVParent
		view.LastVerifiedAt = healthy.LastVerifiedAt
		httpx.JSON(w, http.StatusOK, view)
	})
}

// handleSourceSession verifies then stores provider session material (admin
// only). Nothing is stored if verification fails. Secrets are never returned.
func handleSourceSession(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		var body sourceSessionReq
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		drv, ok := source.Get(strings.TrimSpace(body.Kind))
		if !ok {
			httpx.Error(w, http.StatusBadRequest, "unknown_provider")
			return
		}
		// The pre-0007 request shape carries 30nama's field names directly; map them
		// into the provider-neutral bag under the same keys the store has always
		// sealed, so an existing client and an existing database both keep working.
		sess := source.Session{Fields: map[string]string{}, UserAgent: body.Session.UserAgent}
		for k, v := range map[string]string{
			"cf_clearance":  body.Session.CFClearance,
			"c_api_key":     body.Session.CAPIKey,
			"c_token":       body.Session.CToken,
			"c_platform":    body.Session.CPlatform,
			"c_app_version": body.Session.CAppVersion,
		} {
			if v != "" {
				sess.Fields[k] = v
			}
		}
		// If this provider is already configured, blank secret fields mean "keep the
		// stored value" — so the admin can re-verify or edit only the destination
		// folders without re-pasting every cookie/token. Merge before verifying.
		if pr, err := d.Store.GetProvider(); err == nil && pr != nil && pr.Kind == drv.Kind() {
			if stored, e := d.Store.LoadProviderSession(pr.ID); e == nil && stored != nil {
				for k, v := range stored.Fields {
					if sess.Fields[k] == "" {
						sess.Fields[k] = v
					}
				}
				if sess.UserAgent == "" {
					sess.UserAgent = stored.UserAgent
				}
			}
		}
		hosts := drv.Hosts()

		// Verify against the provider BEFORE persisting anything.
		if err := drv.VerifySession(r.Context(), sourceHTTP, hosts, sess); err != nil {
			reason := "unreachable"
			var ve *source.ErrProviderVerify
			if ok := asProviderVerify(err, &ve); ok {
				reason = ve.Reason
			}
			httpx.JSON(w, http.StatusBadGateway, map[string]any{"error": "provider_verify_failed", "reason": reason})
			return
		}

		now := time.Now().Unix()
		id, err := d.Store.SaveProviderConfig(store.SourceProvider{
			Kind: drv.Kind(), DisplayName: orElse(body.DisplayName, drv.Kind()),
			APIHosts: hosts.APIHosts, DownloadHosts: hosts.DownloadHosts,
			MoviesParent: body.MoviesParent, TVParent: body.TVParent,
			Enabled: true, State: store.SourceActive,
		}, now)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		if err := d.Store.SaveProviderSession(id, store.SourceSession{
			Fields: sess.Fields, UserAgent: sess.UserAgent,
		}, now); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		_ = d.Store.SetProviderState(id, store.SourceActive, now, now)
		httpx.JSON(w, http.StatusOK, map[string]any{"state": store.SourceActive, "lastVerifiedAt": now})
	})
}

// handleSourceDelete removes the provider config + secrets (admin only).
func handleSourceDelete(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		p, err := d.Store.GetProvider()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		if p != nil {
			if err := d.Store.DeleteProvider(p.ID); err != nil {
				httpx.Error(w, http.StatusInternalServerError, "server")
				return
			}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"state": store.SourceNotConfigured})
	})
}

// handleSourcePolicy sets the instance-wide max download size (admin only).
func handleSourcePolicy(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		var body struct {
			MaxDownloadMB int `json:"maxDownloadMB"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		if err := d.Store.SetMaxDownloadMB(body.MaxDownloadMB); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		mb, _ := d.Store.GetMaxDownloadMB()
		httpx.JSON(w, http.StatusOK, map[string]any{"maxDownloadMB": mb})
	})
}

// handleSourceSearch runs a catalog search for any signed-in user.
func handleSourceSearch(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		refs, skipped := d.sourceRefs()
		if len(refs) == 0 && len(skipped) == 0 {
			httpx.JSON(w, http.StatusConflict, map[string]any{"error": "source_unavailable"})
			return
		}
		var body sourceSearchReq
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		filters := source.SearchFilters{
			Type: body.Filters.Type, Quality: body.Filters.Quality,
			Language: body.Filters.Language, Country: body.Filters.Country,
			Score: body.Filters.Score, Genre: body.Filters.Genre,
			Age: body.Filters.Age, Channel: body.Filters.Channel, Encoder: body.Filters.Encoder,
			X265: body.Filters.X265, ThreeD: body.Filters.ThreeD,
			Cast: body.Filters.Cast, Director: body.Filters.Director, Creator: body.Filters.Creator,
			YearFrom: body.Filters.YearFrom, YearTo: body.Filters.YearTo,
		}
		// Parental control: a user with a content-rating cap can ONLY see that
		// rating. Enforced server-side (the client can't relax or remove it) — it
		// overrides any age the client asked for.
		if u.ContentRating != "" {
			filters.Age = u.ContentRating
		}
		// "" (or an unknown/removed source) means every enabled source, combined.
		selected, _ := selectRefs(refs, body.Source)
		res := source.SearchAll(r.Context(), sourceHTTP, selected, source.SearchQuery{
			Query: body.Query, Page: body.Page, Sort: body.Sort, Order: body.Order, Filters: filters,
		})
		// Sources that couldn't even be built (unknown driver, unreadable session)
		// are degraded too — the user is told, rather than quietly shown less.
		res.Degraded = append(res.Degraded, skipped...)

		// Persist health transitions the fan-out observed, so the admin view and the
		// refresh prompt reflect reality. Reason categories only, never upstream text.
		//
		// A single auth failure is almost always transient — the provider
		// rate-limiting or challenging one request during a burst — so a source is
		// only condemned after several CONSECUTIVE failures. Flipping it on the
		// first blip would tell the operator to re-paste a session that is fine.
		now := time.Now().Unix()
		degradedIDs := map[int64]string{}
		for _, dg := range res.Degraded {
			degradedIDs[dg.SourceID] = dg.Reason
		}
		transient := 0
		for _, ref := range selected {
			reason, bad := degradedIDs[ref.ID]
			if !bad {
				clearSourceFailures(ref.ID)
				_ = d.Store.SetProviderStateErr(ref.ID, store.SourceActive, "", now, now)
				continue
			}
			// An entitlement problem is immediate and definite — no streak needed,
			// and re-pasting would not help.
			if reason == source.ReasonUnsubscribed {
				_ = d.Store.SetProviderStateErr(ref.ID, store.SourceUnsubscribed, reason, 0, now)
				continue
			}
			if noteSourceFailure(ref.ID) < sourceFailThreshold {
				transient++
				continue // keep the stored state; this may well be a blip
			}
			_ = d.Store.SetProviderStateErr(ref.ID, store.SourceNeedsRefresh, reason, 0, now)
		}

		// Mark the titles already sitting on the NAS (spec 0008). Read from the
		// cached snapshot, so a warm cache adds no NAS round-trip to browsing; a
		// snapshot that could not be built simply matches nothing, leaving the
		// search untouched (FR-009). Same shape as decorateTasks in
		// task_ownership.go: decorate on the way out, never in a driver.
		if ix := d.libraryIndex(r.Context()); !ix.IsEmpty() {
			// Read once for the whole response rather than per title: it is a
			// snapshot copy, and a page can carry forty items.
			active := d.activeDestinations()
			for i := range res.Items {
				res.Items[i].Ownership = d.ownershipOf(
					r.Context(), ix, res.Items[i].Title, mediaKind(res.Items[i].Type), active)
			}
		}

		allFailed := len(res.Items) == 0 && len(selected) > 0 && len(res.Degraded) >= len(selected)
		// Everything we asked failed, but not yet often enough to call it dead:
		// answer retryable rather than sending the user to the login prompt.
		if allFailed && transient > 0 {
			httpx.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "source_busy"})
			return
		}
		// Every source failed for real: report once, not once per source.
		if allFailed {
			out := map[string]any{"error": "source_needs_refresh", "degraded": res.Degraded}
			if len(res.Degraded) == 1 && res.Degraded[0].Reason == source.ReasonNeedsRefresh {
				// Single-source callers have always been told which layer expired.
				out["layer"] = source.LayerToken
			}
			httpx.JSON(w, http.StatusConflict, out)
			return
		}
		if res.Items == nil {
			res.Items = []source.CatalogTitle{}
		}
		httpx.JSON(w, http.StatusOK, res)
	})
}

// handleSourceParameters returns the provider's live filter facets (genres,
// types, qualities, languages, countries, score bands, year range) so the filter
// UI stays in step with the source. The client refreshes this on open/foreground.
func handleSourceParameters(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		refs, _ := d.sourceRefs()
		if len(refs) == 0 {
			httpx.JSON(w, http.StatusConflict, map[string]any{"error": "source_unavailable"})
			return
		}
		selected, single := selectRefs(refs, r.URL.Query().Get("source"))
		params, err := d.gatherParameters(r, selected, single)
		if err != nil {
			d.writeSourceRuntimeErr(w, selected[0].ID, err)
			return
		}
		httpx.JSON(w, http.StatusOK, params)
	})
}

// handleSourceTitle returns a title's qualities (movies are sendable).
func handleSourceTitle(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		ref, id, ok := d.resolveTitleID(r.PathValue("id"))
		if !ok {
			// Malformed, hostile, or naming a source this caller doesn't have — a
			// client error, never a silent miss (FR-034).
			httpx.Error(w, http.StatusBadRequest, "bad_title_id")
			return
		}
		td, err := ref.Driver.Title(r.Context(), sourceHTTP, ref.Cfg, ref.Sess, id)
		if err != nil {
			d.writeSourceRuntimeErr(w, ref.ID, err)
			return
		}
		d.sourceCallOK(&store.SourceProvider{ID: ref.ID, State: store.SourceActive})
		td.ID = source.QualifyID(ref.ID, td.ID)
		if td.Qualities == nil {
			td.Qualities = []source.QualityOption{}
		}
		httpx.JSON(w, http.StatusOK, td)
	})
}

// handleSourceSend resolves a signed link at send time and creates the task in a
// per-title subfolder under the movies parent, honoring folder grants.
func handleSourceSend(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		var body sourceSendReq
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		// The title id names its own source, so a send always addresses the source
		// the user was actually looking at — not whichever one happens to be first.
		ref, titleID, okID := d.resolveTitleID(body.TitleID)
		if !okID {
			httpx.Error(w, http.StatusBadRequest, "bad_title_id")
			return
		}
		p, err := d.Store.GetProviderByID(ref.ID)
		if err != nil || p == nil {
			httpx.JSON(w, http.StatusConflict, map[string]any{"error": "source_unavailable"})
			return
		}
		drv, cfg, sess := ref.Driver, ref.Cfg, ref.Sess
		if body.TitleID == "" || body.QualityID == "" {
			httpx.Error(w, http.StatusBadRequest, "titleId and qualityId required")
			return
		}

		// Series/anime land under the TV parent; everything else under movies.
		parent := p.MoviesParent
		if body.Type == source.TypeSeries || body.Type == source.TypeAnime {
			parent = p.TVParent
		}
		relParent := strings.Trim(strings.TrimSpace(parent), "/")
		if relParent == "" {
			httpx.JSON(w, http.StatusConflict, map[string]any{"error": "parent_unset"})
			return
		}
		// "Despicable Me 4 2024" -> "Despicable Me 4 (2024)": the shape a media
		// server scrapes, and the same one scripts/library_tidy.py applies to a
		// library retrospectively (spec 1021). Keeping the two in step is what
		// stops every new download re-introducing the mess a tidy just removed.
		folderName := sanitizeFolderName(library.PlexName(body.Title))
		if !validFolderName(folderName) {
			folderName = "title-" + sanitizeFolderName(titleID)
		}
		titleDest := relParent + "/" + folderName
		// Checked here, before any link is resolved, so a forbidden destination
		// costs nothing. The final path is checked again below, once the season
		// subfolder is known.
		if !d.destinationAllowed(u, titleDest) {
			httpx.JSON(w, http.StatusForbidden, map[string]any{"error": "destination_forbidden"})
			return
		}
		dest := titleDest

		// Resolve the signed link(s) (+ per-file size) at send time (never cached):
		// one for a movie, one per episode for a series season pack.
		links, size, err := drv.ResolveDownload(r.Context(), sourceHTTP, cfg, sess, titleID, body.QualityID)
		if err != nil {
			d.writeSourceRuntimeErr(w, p.ID, err)
			return
		}

		// Narrow a series to the picked episodes (1-based). Empty selection, a
		// single-file title, or a movie sends everything.
		selected, err := selectEpisodes(links, body.Episodes)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad episode selection")
			return
		}

		// Instance-wide max size: refuse a file larger than the admin's cap so the
		// user picks a smaller one (for a series this is the per-episode size).
		if maxMB, _ := d.Store.GetMaxDownloadMB(); maxMB > 0 {
			if mb := parseSizeMB(size); mb > 0 && mb > maxMB {
				httpx.JSON(w, http.StatusRequestEntityTooLarge,
					map[string]any{"error": "download_too_large", "maxMB": maxMB, "sizeMB": mb})
				return
			}
		}

		// Daily download quota: a per-user rolling-24h cap on how many downloads
		// they may start (protects the provider's daily download limit). The whole
		// selection must fit the remaining allowance — so a big season can't blow
		// past the cap in one send. The client turns this into an offer to send
		// just the first `remaining`.
		if u.DailyDownloadLimit > 0 {
			since := time.Now().Add(-24 * time.Hour).Unix()
			used, _ := d.Store.CountUserDownloadsSince(u.ID, since)
			remaining := u.DailyDownloadLimit - used
			if remaining < 0 {
				remaining = 0
			}
			if len(selected) > remaining {
				httpx.JSON(w, http.StatusTooManyRequests, map[string]any{
					"error": "daily_limit_exceeded", "limit": u.DailyDownloadLimit,
					"used": used, "remaining": remaining, "requested": len(selected),
				})
				return
			}
		}

		// A series' episodes belong in "Show (Year)/Season NN". The season is read
		// from the files being downloaded rather than taken from the client, so it
		// can never disagree with what lands on disk; a set spanning seasons, or
		// one with no season in its names, goes straight in the show's folder,
		// which a scraper still reads correctly (FR-006, FR-007).
		seasonFolder := ""
		if body.Type == source.TypeSeries || body.Type == source.TypeAnime {
			if n, ok := library.SeasonOfFiles(selected); ok {
				seasonFolder = sanitizeFolderName(library.SeasonFolder(n))
				dest = titleDest + "/" + seasonFolder
			}
		}
		// The deeper path is checked too: grants are prefix-based, so this passes
		// whenever the title folder did, but it is verified rather than assumed.
		if !d.destinationAllowed(u, dest) {
			httpx.JSON(w, http.StatusForbidden, map[string]any{"error": "destination_forbidden"})
			return
		}

		absParent := "/" + relParent
		if err := d.NAS.Do(r.Context(), func(c syno.Client, sid string) error {
			if e := ensureSubfolder(r.Context(), c, sid, absParent, folderName); e != nil {
				return e
			}
			if seasonFolder != "" {
				if e := ensureSubfolder(r.Context(), c, sid, absParent+"/"+folderName, seasonFolder); e != nil {
					return e
				}
			}
			return c.CreateTaskURIs(r.Context(), sid, selected, syno.CreateOpts{Destination: dest})
		}); err != nil {
			writeNASError(w, err)
			return
		}
		// Record the downloads durably (daily-limit count) and one claim per file
		// (notification attribution) — a whole season counts as many.
		now := time.Now().Unix()
		_ = d.Store.AddDownloadEvents(u.ID, len(selected), now)
		for range selected {
			// The TITLE folder, never the season one: a claim named "Season 01"
			// would match nothing useful.
			_ = d.Store.AddTaskClaim(u.ID, folderName, now)
		}
		// Remember the catalog metadata + WHO sent it for this title's folder, so
		// the Tasks list can label it and attribute ownership by destination (the
		// name-based claim can't match a source send — DSM names the task after the
		// file, not the folder).
		_ = d.Store.SaveSourceDownload(store.SourceDownload{
			Destination: dest, MediaType: body.Type, Title: body.Title,
			Year: strings.TrimSpace(body.Year), IMDbScore: body.IMDbScore,
			PosterURL: strings.TrimSpace(body.PosterURL), CatalogID: strings.TrimSpace(body.TitleID),
			OwnerID: u.ID,
		}, now)
		// Record durable per-file history for the Statistics section: one row per
		// selected file (size unknown until the watcher sees it finish). The
		// task_name is the file the watcher will report, so it can backfill the size
		// by (destination, name). Category comes from the catalog type.
		cat := body.Type
		if !store.ValidCategory(cat) {
			cat = store.CategoryOther
		}
		for _, link := range selected {
			_ = d.Store.AddDownloadHistory(store.DownloadHistory{
				UserID: u.ID, Source: store.SourceCatalog, Category: cat,
				Destination: dest, TaskName: fileNameFromURI(link), CreatedAt: now,
			})
		}
		// The title now has a folder on the NAS, so the ownership snapshot behind
		// Discover's markers is stale. Drop it rather than waiting out the TTL:
		// a user who just sent something and scrolls back expects to see it
		// marked (FR-008, spec 0008).
		d.invalidateLibrary()
		httpx.JSON(w, http.StatusOK,
			map[string]any{"destination": dest, "created": true, "taskAdded": true, "count": len(selected)})
	})
}

// handleGetSourceQuota reports the signed-in user's daily download allowance so
// the client can warn before a big season blows past it. limit 0 = unlimited
// (remaining is then -1).
func handleGetSourceQuota(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, _ *http.Request, u *store.User) {
		limit := u.DailyDownloadLimit
		used, remaining := 0, -1
		if limit > 0 {
			used, _ = d.Store.CountUserDownloadsSince(u.ID, time.Now().Add(-24*time.Hour).Unix())
			remaining = limit - used
			if remaining < 0 {
				remaining = 0
			}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"limit": limit, "used": used, "remaining": remaining})
	})
}

// handleGetSourcePrefs / handleSetSourcePrefs — per-user preferred quality.
func handleGetSourcePrefs(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		q, err := d.Store.GetSourcePref(u.ID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"preferredQuality": q})
	})
}

func handleSetSourcePrefs(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		var body struct {
			PreferredQuality string `json:"preferredQuality"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		q := strings.TrimSpace(body.PreferredQuality)
		if err := d.Store.SaveSourcePref(u.ID, q, time.Now().Unix()); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"preferredQuality": q})
	})
}

// handleGetSourceView / handleSetSourceView — per-user Discover view (facet
// filters + sort) so it follows the user across devices. The server treats
// `filters` as an opaque JSON blob owned by the client.
func handleGetSourceView(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		filters, sort, order, selected, err := d.Store.GetSourceViewFull(u.ID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		// A source the user had selected may since have been removed or disabled.
		// Normalize that to "all sources" on READ rather than rewriting the row —
		// the user lands somewhere sensible and a re-enabled source comes back.
		if selected != "" {
			refs, _ := d.sourceRefs()
			if _, single := selectRefs(refs, selected); !single {
				selected = ""
			}
		}
		// Pass the stored filters JSON through verbatim (default {}), so the client
		// gets an object rather than a JSON-encoded string.
		httpx.JSON(w, http.StatusOK, map[string]any{
			"filters": json.RawMessage(orElse(filters, "{}")), "sort": sort, "order": order,
			"selectedSource": selected,
		})
	})
}

func handleSetSourceView(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		var body struct {
			Filters json.RawMessage `json:"filters"`
			Sort    string          `json:"sort"`
			Order   string          `json:"order"`
			// "" = all sources. Persisted with the rest of the view so the choice
			// follows the user across devices (FR-008).
			SelectedSource string `json:"selectedSource"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		filters := strings.TrimSpace(string(body.Filters))
		if filters == "" || filters == "null" {
			filters = "{}"
		}
		if err := d.Store.SaveSourceViewFull(u.ID, filters, strings.TrimSpace(body.Sort),
			strings.TrimSpace(body.Order), strings.TrimSpace(body.SelectedSource), time.Now().Unix()); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// ensureSubfolder creates name under absParent, tolerating "already exists" by
// confirming the folder is present (so a repeat send reuses it).
func ensureSubfolder(ctx context.Context, c syno.Client, sid, absParent, name string) error {
	if _, err := c.CreateFolder(ctx, sid, absParent, name); err == nil {
		return nil
	}
	folders, e := c.ListFolder(ctx, sid, absParent)
	if e != nil {
		return e
	}
	for _, f := range folders {
		if f.Name == name {
			return nil // already exists — reuse it
		}
	}
	// Create failed and it's still not there — surface a create error.
	_, err := c.CreateFolder(ctx, sid, absParent, name)
	return err
}

// sanitizeFolderName turns a title into a safe single-segment folder name.
func sanitizeFolderName(title string) string {
	repl := strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|':
			return ' '
		case r < 0x20:
			return ' '
		default:
			return r
		}
	}, title)
	repl = strings.Join(strings.Fields(repl), " ")
	repl = strings.Trim(repl, " .")
	if len(repl) > 120 {
		repl = strings.TrimSpace(repl[:120])
	}
	return repl
}

// parseSizeMB turns a provider size string ("37.55 GB", "700 MB", "1.2 TB") into
// whole megabytes; 0 when it can't be parsed (so the cap fails open rather than
// blocking a valid download on an odd label).
func parseSizeMB(s string) int {
	f := strings.Fields(strings.ToUpper(strings.TrimSpace(s)))
	if len(f) < 2 {
		return 0
	}
	v, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return 0
	}
	switch f[1] {
	case "TB":
		v *= 1024 * 1024
	case "GB":
		v *= 1024
	case "MB":
		// already MB
	case "KB":
		v /= 1024
	default:
		return 0
	}
	return int(v + 0.5)
}

func orElse(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func asProviderVerify(err error, target **source.ErrProviderVerify) bool {
	if v, ok := err.(*source.ErrProviderVerify); ok {
		*target = v
		return true
	}
	return false
}
