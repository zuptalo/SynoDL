package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"synodl/server/internal/httpx"
	"synodl/server/internal/source"
	"synodl/server/internal/store"
)

// Multi-source plumbing (spec 0007).
//
// The pre-0007 routes still address the lowest-id source so an older client is
// unaffected; everything here is about addressing sources explicitly, fanning
// out across them, and keeping one source's material away from another's hosts.

// toSession converts stored material into the shape a driver receives. The bag
// is copied rather than shared: a driver must never be able to reach back into
// the store's map, and each ref carries only its OWN source's material.
func toSession(s *store.SourceSession) source.Session {
	out := source.Session{Fields: map[string]string{}, UserAgent: s.UserAgent}
	for k, v := range s.Fields {
		out.Fields[k] = v
	}
	return out
}

// sourceRefs builds a callable ref per enabled, usable source, in the operator's
// display order. Sources whose driver is unknown or whose session is missing or
// unreadable are skipped with a reason, so the caller can report them as
// degraded rather than pretending they do not exist.
func (d Deps) sourceRefs() (refs []source.SourceRef, skipped []source.DegradedSource) {
	providers, err := d.Store.ListProviders()
	if err != nil {
		return nil, nil
	}
	for _, p := range providers {
		if !p.Enabled {
			continue
		}
		drv, has := source.Get(p.Kind)
		if !has {
			// An unknown kind means a driver was removed from the build while its
			// config survived. Report it; never fail startup or the whole query.
			skipped = append(skipped, source.DegradedSource{
				SourceID: p.ID, Name: p.DisplayName, Reason: source.ReasonUnreachable,
			})
			continue
		}
		sess, err := d.Store.LoadProviderSession(p.ID)
		if err != nil {
			// Unreadable sealed material: report it, leave it stored, and do NOT
			// treat it as "never configured" — that would strand the operator
			// (FR-035).
			skipped = append(skipped, source.DegradedSource{
				SourceID: p.ID, Name: p.DisplayName, Reason: source.ReasonNeedsRefresh,
			})
			continue
		}
		if sess == nil {
			skipped = append(skipped, source.DegradedSource{
				SourceID: p.ID, Name: p.DisplayName, Reason: source.ReasonNeedsRefresh,
			})
			continue
		}
		refs = append(refs, source.SourceRef{
			ID: p.ID, Name: p.DisplayName, Driver: drv,
			Cfg:  source.Config{APIHosts: p.APIHosts, DownloadHosts: p.DownloadHosts},
			Sess: toSession(sess),
		})
	}
	return refs, skipped
}

// selectRefs narrows to one source when the client asked for one. An unknown or
// disabled selection is NOT an error: it degrades to all sources, because a
// source can be removed while a user is browsing it and the user should land
// somewhere sensible rather than on a dead view.
func selectRefs(refs []source.SourceRef, want string) ([]source.SourceRef, bool) {
	want = strings.TrimSpace(want)
	if want == "" {
		return refs, false
	}
	id, err := strconv.ParseInt(want, 10, 64)
	if err != nil {
		return refs, false
	}
	for _, r := range refs {
		if r.ID == id {
			return []source.SourceRef{r}, true
		}
	}
	return refs, false
}

// refByID finds one ref for a source-qualified request.
func refByID(refs []source.SourceRef, id int64) (source.SourceRef, bool) {
	for _, r := range refs {
		if r.ID == id {
			return r, true
		}
	}
	return source.SourceRef{}, false
}

// resolveTitleID splits a wire id and resolves it to a configured source. It
// returns ok=false for a malformed id, an id naming a source the caller does not
// have, or one that fails the central safety checks — all client errors, never
// silent misses (FR-033, FR-034).
func (d Deps) resolveTitleID(wire string) (ref source.SourceRef, titleID string, ok bool) {
	refs, _ := d.sourceRefs()
	pid, tid, valid := source.SplitID(wire)
	if !valid {
		// An id containing a colon was MEANT to be qualified, so a bad provider
		// portion ("0:x", "abc:x") is an error — not an invitation to fall back.
		// Falling back there would quietly redirect a malformed request at whatever
		// source happens to be first.
		//
		// Only a genuinely unqualified id gets the pre-0007 treatment, addressed to
		// the lowest-id source, so a client holding an old stored id keeps working.
		if strings.Contains(wire, ":") {
			return source.SourceRef{}, "", false
		}
		if source.ValidateTitleID(wire) && len(refs) > 0 {
			return refs[0], wire, true
		}
		return source.SourceRef{}, "", false
	}
	r, has := refByID(refs, pid)
	if !has {
		return source.SourceRef{}, "", false
	}
	return r, tid, true
}

// providerView is the admin's view of one configured source. It carries no
// session material of any kind — the fields are write-only, so an admin can
// replace them but never read them back.
type providerView struct {
	ID             int64  `json:"id"`
	Kind           string `json:"kind"`
	DisplayName    string `json:"displayName"`
	Enabled        bool   `json:"enabled"`
	State          string `json:"state"`
	LastVerifiedAt int64  `json:"lastVerifiedAt"`
	LastError      string `json:"lastError,omitempty"`
	SortOrder      int64  `json:"sortOrder"`
	MoviesParent   string `json:"moviesParent"`
	TVParent       string `json:"tvParent"`
}

func toProviderView(p store.SourceProvider) providerView {
	return providerView{
		ID: p.ID, Kind: p.Kind, DisplayName: p.DisplayName, Enabled: p.Enabled,
		State: p.State, LastVerifiedAt: p.LastVerifiedAt, LastError: p.LastError,
		SortOrder: p.SortOrder, MoviesParent: p.MoviesParent, TVParent: p.TVParent,
	}
}

// kindView advertises a driver an admin can add, including the fields it needs.
// The admin form is generated from this, so a new driver needs no client change.
type kindView struct {
	Kind          string                `json:"kind"`
	Name          string                `json:"name"`
	SessionFields []source.SessionField `json:"sessionFields"`
}

// handleListProviders lists configured sources plus the kinds available to add.
func handleListProviders(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, _ *http.Request, _ *store.User) {
		providers, err := d.Store.ListProviders()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		views := make([]providerView, 0, len(providers))
		for _, p := range providers {
			views = append(views, toProviderView(p))
		}
		kinds := make([]kindView, 0)
		for _, k := range source.Kinds() {
			if drv, ok := source.Get(k); ok {
				kinds = append(kinds, kindView{
					Kind: k, Name: drv.DisplayName(), SessionFields: drv.SessionFields(),
				})
			}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"providers": views, "kinds": kinds})
	})
}

type providerWriteReq struct {
	Kind         string            `json:"kind"`
	DisplayName  string            `json:"displayName"`
	MoviesParent string            `json:"moviesParent"`
	TVParent     string            `json:"tvParent"`
	SortOrder    int64             `json:"sortOrder"`
	Enabled      *bool             `json:"enabled"`
	Session      map[string]string `json:"session"`
}

// verifyAndMap runs the driver's verification and maps the outcome onto a stored
// state. The distinction that matters: a session that WORKS but has no download
// entitlement is "unsubscribed", never "needs refresh" — telling an operator to
// re-paste working material sends them in circles (FR-019).
func verifyAndMap(r *http.Request, drv source.Provider, hosts source.Config, sess source.Session) (state, reason string, ok bool) {
	err := drv.VerifySession(r.Context(), sourceHTTP, hosts, sess)
	if err == nil {
		return store.SourceActive, "", true
	}
	var ve *source.ErrProviderVerify
	reason = source.ReasonUnreachable
	if asProviderVerify(err, &ve) {
		reason = ve.Reason
	}
	if reason == source.ReasonUnsubscribed {
		// Configured and authenticated, just not entitled. Stored as usable-ish so
		// the admin sees the real problem rather than a login error.
		return store.SourceUnsubscribed, reason, true
	}
	return "", reason, false
}

// handleCreateProvider adds a source, verifying before anything is persisted.
func handleCreateProvider(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		var body providerWriteReq
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		drv, ok := source.Get(strings.TrimSpace(body.Kind))
		if !ok {
			httpx.Error(w, http.StatusBadRequest, "unknown_provider")
			return
		}
		sess := source.Session{Fields: map[string]string{}, UserAgent: body.Session["user_agent"]}
		for k, v := range body.Session {
			if k != "user_agent" {
				sess.Fields[k] = v
			}
		}
		hosts := drv.Hosts()
		state, reason, ok := verifyAndMap(r, drv, hosts, sess)
		if !ok {
			httpx.JSON(w, http.StatusUnprocessableEntity,
				map[string]any{"error": "verify_failed", "reason": reason})
			return
		}
		now := time.Now().Unix()
		id, err := d.Store.CreateProvider(store.SourceProvider{
			Kind: drv.Kind(), DisplayName: orElse(body.DisplayName, drv.DisplayName()),
			APIHosts: hosts.APIHosts, DownloadHosts: hosts.DownloadHosts,
			MoviesParent: body.MoviesParent, TVParent: body.TVParent,
			Enabled: true, State: state, SortOrder: body.SortOrder, LastError: reason,
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
		_ = d.Store.SetProviderStateErr(id, state, reason, now, now)
		source.ResetBreakers()
		p, _ := d.Store.GetProviderByID(id)
		if p == nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		httpx.JSON(w, http.StatusCreated, toProviderView(*p))
	})
}

// handleUpdateProvider edits one source. Omitted session fields keep their
// stored value, so an admin can rename a source or change its folders without
// re-pasting every secret — and without those values ever being read back to a
// client.
func handleUpdateProvider(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		p, err := d.Store.GetProviderByID(id)
		if err != nil || p == nil {
			httpx.Error(w, http.StatusNotFound, "not_found")
			return
		}
		var body providerWriteReq
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		drv, ok := source.Get(p.Kind)
		if !ok {
			httpx.Error(w, http.StatusBadRequest, "unknown_provider")
			return
		}
		stored, _ := d.Store.LoadProviderSession(id)
		sess := source.Session{Fields: map[string]string{}}
		if stored != nil {
			for k, v := range stored.Fields {
				sess.Fields[k] = v
			}
			sess.UserAgent = stored.UserAgent
		}
		for k, v := range body.Session {
			if strings.TrimSpace(v) == "" {
				continue // blank means "keep what is stored"
			}
			if k == "user_agent" {
				sess.UserAgent = v
			} else {
				sess.Fields[k] = v
			}
		}
		hosts := drv.Hosts()
		state, reason, ok := verifyAndMap(r, drv, hosts, sess)
		if !ok {
			httpx.JSON(w, http.StatusUnprocessableEntity,
				map[string]any{"error": "verify_failed", "reason": reason})
			return
		}
		now := time.Now().Unix()
		p.DisplayName = orElse(body.DisplayName, p.DisplayName)
		p.MoviesParent = orElse(body.MoviesParent, p.MoviesParent)
		p.TVParent = orElse(body.TVParent, p.TVParent)
		p.SortOrder = body.SortOrder
		p.APIHosts, p.DownloadHosts = hosts.APIHosts, hosts.DownloadHosts
		if body.Enabled != nil {
			p.Enabled = *body.Enabled
		}
		if err := d.Store.UpdateProvider(*p, now); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		if err := d.Store.SaveProviderSession(id, store.SourceSession{
			Fields: sess.Fields, UserAgent: sess.UserAgent,
		}, now); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		_ = d.Store.SetProviderStateErr(id, state, reason, now, now)
		// The admin has just fixed whatever was failing; making them wait out a
		// cooling-off window would be absurd.
		source.ResetBreakers()
		updated, _ := d.Store.GetProviderByID(id)
		httpx.JSON(w, http.StatusOK, toProviderView(*updated))
	})
}

// handleDeleteProvider removes one source and, by cascade, its sealed session.
func handleDeleteProvider(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		if err := d.Store.DeleteProvider(id); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		source.ResetBreakers()
		httpx.JSON(w, http.StatusOK, map[string]any{"deleted": id})
	})
}

// gatherParameters returns the filter facets the UI should offer.
//
// For a single source that is simply its own facet set. For combined mode the
// facets are INTERSECTED across enabled sources, so every filter shown actually
// applies to everything on screen (FR-014) — offering a filter only one source
// understands would silently leave the other source's results unfiltered, which
// looks like the filter is broken.
func (d Deps) gatherParameters(r *http.Request, refs []source.SourceRef, single bool) (source.SearchParameters, error) {
	if len(refs) == 0 {
		return source.SearchParameters{}, nil
	}
	if single || len(refs) == 1 {
		p, err := refs[0].Driver.Parameters(r.Context(), sourceHTTP, refs[0].Cfg, refs[0].Sess)
		if err != nil {
			return source.SearchParameters{}, err
		}
		return p, nil
	}
	var sets []source.SearchParameters
	var firstErr error
	for _, ref := range refs {
		p, err := ref.Driver.Parameters(r.Context(), sourceHTTP, ref.Cfg, ref.Sess)
		if err != nil {
			// A source that can't report its facets is skipped rather than failing
			// the sheet — the intersection just gets narrower.
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		sets = append(sets, p)
	}
	if len(sets) == 0 {
		return source.SearchParameters{}, firstErr
	}
	return source.IntersectParameters(sets), nil
}
