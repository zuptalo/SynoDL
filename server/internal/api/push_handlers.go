package api

import (
	"encoding/json"
	"net/http"

	"synodl/server/internal/httpx"
	"synodl/server/internal/store"
)

// Web Push endpoints (spec 0003, Increment 4). All require a SynoDL session; the
// VAPID public key is not secret but is only handed to authenticated clients.

func handlePushKey(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		v, err := d.Store.GetVAPID()
		if err != nil {
			httpx.Error(w, http.StatusServiceUnavailable, "push_unavailable")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]string{"publicKey": v.Public})
	})
}

func handleSaveSubscription(d Deps) http.Handler {
	type req struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
		OptedIn bool `json:"optedIn"`
	}
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		var body req
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		if body.Endpoint == "" || body.Keys.P256dh == "" || body.Keys.Auth == "" {
			httpx.Error(w, http.StatusBadRequest, "endpoint and keys required")
			return
		}
		if err := d.Store.SaveSubscription(u.ID, body.Endpoint, body.Keys.P256dh, body.Keys.Auth, body.OptedIn); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func handleDeleteSubscription(d Deps) http.Handler {
	type req struct {
		Endpoint string `json:"endpoint"`
	}
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		var body req
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		if err := d.Store.DeleteSubscription(body.Endpoint); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
