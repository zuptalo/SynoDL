package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"synodl/server/internal/httpx"
	"synodl/server/internal/source"
	_ "synodl/server/internal/source/providers" // register provider drivers
	"synodl/server/internal/store"
	"synodl/server/internal/syno"
)

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
	Query   string `json:"query"`
	Page    int    `json:"page"`
	Filters struct {
		Type     string   `json:"type"`
		Quality  string   `json:"quality"`
		Language string   `json:"language"`
		Country  string   `json:"country"`
		Genre    []string `json:"genre"`
	} `json:"filters"`
}

type sourceSendReq struct {
	TitleID   string `json:"titleId"`
	QualityID string `json:"qualityId"`
	Title     string `json:"title"`
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
		source.Session{
			CFClearance: s.CFClearance, CAPIKey: s.CAPIKey, CToken: s.CToken,
			UserAgent: s.UserAgent, CPlatform: s.CPlatform, CAppVersion: s.CAppVersion,
		}, true
}

// writeSourceRuntimeErr maps a provider runtime error to the client contract and
// flips the stored state to needs_refresh on an expiry. It never echoes secrets
// or raw upstream bodies.
func (d Deps) writeSourceRuntimeErr(w http.ResponseWriter, providerID int64, err error) {
	if nr, ok := source.AsNeedsRefresh(err); ok {
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
		p, err := d.Store.GetProvider()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		view := sourceStatusView{State: store.SourceNotConfigured, CanManage: u.IsAdmin}
		if p != nil {
			view.Configured = true
			view.Enabled = p.Enabled
			view.State = p.State
			view.ProviderName = p.DisplayName
			view.Kind = p.Kind
			view.MoviesParent = p.MoviesParent
			view.TVParent = p.TVParent
			view.LastVerifiedAt = p.LastVerifiedAt
		}
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
		sess := source.Session{
			CFClearance: body.Session.CFClearance, CAPIKey: body.Session.CAPIKey,
			CToken: body.Session.CToken, UserAgent: body.Session.UserAgent,
			CPlatform: body.Session.CPlatform, CAppVersion: body.Session.CAppVersion,
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
			CFClearance: sess.CFClearance, CAPIKey: sess.CAPIKey, CToken: sess.CToken,
			UserAgent: sess.UserAgent, CPlatform: sess.CPlatform, CAppVersion: sess.CAppVersion,
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

// handleSourceSearch runs a catalog search for any signed-in user.
func handleSourceSearch(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		p, drv, cfg, sess, ok := d.activeSource()
		if !ok {
			httpx.JSON(w, http.StatusConflict, map[string]any{"error": "source_unavailable"})
			return
		}
		var body sourceSearchReq
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		res, err := drv.Search(r.Context(), sourceHTTP, cfg, sess, source.SearchQuery{
			Query: body.Query, Page: body.Page,
			Filters: source.SearchFilters{
				Type: body.Filters.Type, Quality: body.Filters.Quality,
				Language: body.Filters.Language, Country: body.Filters.Country, Genre: body.Filters.Genre,
			},
		})
		if err != nil {
			d.writeSourceRuntimeErr(w, p.ID, err)
			return
		}
		// A successful call re-confirms the session is healthy.
		if p.State != store.SourceActive {
			_ = d.Store.SetProviderState(p.ID, store.SourceActive, time.Now().Unix(), time.Now().Unix())
		}
		if res.Items == nil {
			res.Items = []source.CatalogTitle{}
		}
		httpx.JSON(w, http.StatusOK, res)
	})
}

// handleSourceTitle returns a title's qualities (movies are sendable).
func handleSourceTitle(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		p, drv, cfg, sess, ok := d.activeSource()
		if !ok {
			httpx.JSON(w, http.StatusConflict, map[string]any{"error": "source_unavailable"})
			return
		}
		id := r.PathValue("id")
		td, err := drv.Title(r.Context(), sourceHTTP, cfg, sess, id)
		if err != nil {
			d.writeSourceRuntimeErr(w, p.ID, err)
			return
		}
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
		p, drv, cfg, sess, ok := d.activeSource()
		if !ok {
			httpx.JSON(w, http.StatusConflict, map[string]any{"error": "source_unavailable"})
			return
		}
		var body sourceSendReq
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		if body.TitleID == "" || body.QualityID == "" {
			httpx.Error(w, http.StatusBadRequest, "titleId and qualityId required")
			return
		}

		relParent := strings.Trim(strings.TrimSpace(p.MoviesParent), "/")
		if relParent == "" {
			httpx.JSON(w, http.StatusConflict, map[string]any{"error": "movies_parent_unset"})
			return
		}
		folderName := sanitizeFolderName(body.Title)
		if !validFolderName(folderName) {
			folderName = "title-" + sanitizeFolderName(body.TitleID)
		}
		dest := relParent + "/" + folderName
		if !d.destinationAllowed(u, dest) {
			httpx.JSON(w, http.StatusForbidden, map[string]any{"error": "destination_forbidden"})
			return
		}

		// Resolve the signed link at send time (never cached).
		link, err := drv.ResolveDownload(r.Context(), sourceHTTP, cfg, sess, body.TitleID, body.QualityID)
		if err != nil {
			d.writeSourceRuntimeErr(w, p.ID, err)
			return
		}

		absParent := "/" + relParent
		if err := d.NAS.Do(r.Context(), func(c syno.Client, sid string) error {
			if e := ensureSubfolder(r.Context(), c, sid, absParent, folderName); e != nil {
				return e
			}
			return c.CreateTaskURIs(r.Context(), sid, []string{link}, syno.CreateOpts{Destination: dest})
		}); err != nil {
			writeNASError(w, err)
			return
		}
		_ = d.Store.AddTaskClaim(u.ID, folderName, time.Now().Unix())
		httpx.JSON(w, http.StatusOK, map[string]any{"destination": dest, "created": true, "taskAdded": true})
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
