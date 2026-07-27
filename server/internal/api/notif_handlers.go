package api

import (
	"encoding/json"
	"net/http"
	"time"

	"synodl/server/internal/httpx"
	"synodl/server/internal/store"
)

// notifPrefsView is the JSON shape for a user's notification preferences.
type notifPrefsView struct {
	NotifyAdded     bool   `json:"notifyAdded"`
	NotifyCompleted bool   `json:"notifyCompleted"`
	NotifyFailed    bool   `json:"notifyFailed"`
	Scope           string `json:"scope"` // "own" | "any"
}

// handleGetNotifPrefs returns the signed-in user's notification preferences
// (defaults when they haven't set any).
func handleGetNotifPrefs(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, _ *http.Request, u *store.User) {
		p, err := d.Store.GetNotificationPrefs(u.ID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		httpx.JSON(w, http.StatusOK, notifPrefsView{
			NotifyAdded:     p.NotifyAdded,
			NotifyCompleted: p.NotifyCompleted,
			NotifyFailed:    p.NotifyFailed,
			Scope:           p.Scope,
		})
	})
}

// handleSetNotifPrefs saves the signed-in user's notification preferences.
func handleSetNotifPrefs(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		var body notifPrefsView
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		scope := "own"
		if body.Scope == "any" {
			scope = "any"
		}
		err := d.Store.SaveNotificationPrefs(u.ID, store.NotificationPrefs{
			NotifyAdded:     body.NotifyAdded,
			NotifyCompleted: body.NotifyCompleted,
			NotifyFailed:    body.NotifyFailed,
			Scope:           scope,
		}, time.Now().Unix())
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
