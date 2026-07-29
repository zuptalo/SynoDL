package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"strconv"

	"synodl/server/internal/auth"
	"synodl/server/internal/authz"
	"synodl/server/internal/httpx"
	"synodl/server/internal/store"
)

// Admin-only user management + per-user folder grants (spec 0003, Increment 3).
// All handlers are wrapped in requireAdmin at the router.

func userAdminView(u store.User, ownerID int64) map[string]any {
	return map[string]any{
		"id": u.ID, "username": u.Username, "isAdmin": u.IsAdmin, "isEnabled": u.IsEnabled,
		"contentRating": u.ContentRating, "dailyDownloadLimit": u.DailyDownloadLimit,
		"isOwner": u.ID == ownerID,
	}
}

func handleListUsers(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		users, err := d.Store.ListUsers()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		ownerID, _ := d.Store.OwnerID()
		out := make([]map[string]any, 0, len(users))
		for _, u := range users {
			out = append(out, userAdminView(u, ownerID))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"users": out})
	})
}

func handleCreateUser(d Deps) http.Handler {
	type req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		IsAdmin  bool   `json:"isAdmin"`
	}
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		var body req
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		if body.Username == "" || len(body.Password) < 8 {
			httpx.Error(w, http.StatusBadRequest, "username and an 8+ character password are required")
			return
		}
		hash, err := auth.HashPassword(body.Password)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		id, err := d.Store.CreateUser(body.Username, hash, body.IsAdmin)
		if err != nil {
			httpx.Error(w, http.StatusConflict, "username already exists")
			return
		}
		u, _ := d.Store.GetUserByID(id)
		ownerID, _ := d.Store.OwnerID()
		httpx.JSON(w, http.StatusCreated, userAdminView(*u, ownerID))
	})
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func handleUpdateUser(d Deps) http.Handler {
	type req struct {
		IsEnabled          *bool   `json:"isEnabled"`
		IsAdmin            *bool   `json:"isAdmin"`
		Password           *string `json:"password"`
		ContentRating      *string `json:"contentRating"`
		DailyDownloadLimit *int    `json:"dailyDownloadLimit"`
	}
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, actor *store.User) {
		id, ok := pathID(r)
		if !ok {
			httpx.Error(w, http.StatusBadRequest, "bad user id")
			return
		}
		var body req
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		if _, err := d.Store.GetUserByID(id); errors.Is(err, store.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "no such user")
			return
		}
		// The owner (the first user created) is protected: only the owner can
		// change their own account. This stops another admin from disabling,
		// demoting, or resetting the owner and locking them out.
		ownerID, _ := d.Store.OwnerID()
		if id == ownerID && actor.ID != ownerID {
			httpx.Error(w, http.StatusForbidden, "only the owner can change the owner account")
			return
		}
		// The owner can't be demoted or disabled at all — not even by themselves —
		// so the instance always keeps a full-access admin.
		if id == ownerID && ((body.IsEnabled != nil && !*body.IsEnabled) || (body.IsAdmin != nil && !*body.IsAdmin)) {
			httpx.Error(w, http.StatusBadRequest, "the owner account cannot be demoted or disabled")
			return
		}
		// An admin can't disable or demote their own account — that could lock
		// out the last admin.
		if id == actor.ID && ((body.IsEnabled != nil && !*body.IsEnabled) || (body.IsAdmin != nil && !*body.IsAdmin)) {
			httpx.Error(w, http.StatusBadRequest, "you cannot disable or demote your own account")
			return
		}
		if body.Password != nil {
			if len(*body.Password) < 8 {
				httpx.Error(w, http.StatusBadRequest, "password must be at least 8 characters")
				return
			}
			hash, err := auth.HashPassword(*body.Password)
			if err != nil {
				httpx.Error(w, http.StatusInternalServerError, "server")
				return
			}
			if err := d.Store.SetUserPassword(id, hash); err != nil {
				httpx.Error(w, http.StatusInternalServerError, "server")
				return
			}
		}
		if body.IsEnabled != nil {
			if err := d.Store.SetUserEnabled(id, *body.IsEnabled); err != nil {
				httpx.Error(w, http.StatusInternalServerError, "server")
				return
			}
		}
		if body.IsAdmin != nil {
			if err := d.Store.SetUserAdmin(id, *body.IsAdmin); err != nil {
				httpx.Error(w, http.StatusInternalServerError, "server")
				return
			}
		}
		if body.ContentRating != nil {
			if err := d.Store.SetUserContentRating(id, strings.TrimSpace(*body.ContentRating)); err != nil {
				httpx.Error(w, http.StatusInternalServerError, "server")
				return
			}
		}
		if body.DailyDownloadLimit != nil {
			if err := d.Store.SetUserDailyDownloadLimit(id, *body.DailyDownloadLimit); err != nil {
				httpx.Error(w, http.StatusInternalServerError, "server")
				return
			}
		}
		u, _ := d.Store.GetUserByID(id)
		httpx.JSON(w, http.StatusOK, userAdminView(*u, ownerID))
	})
}

func handleDeleteUser(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, actor *store.User) {
		id, ok := pathID(r)
		if !ok {
			httpx.Error(w, http.StatusBadRequest, "bad user id")
			return
		}
		// The owner account can never be deleted — by anyone, including itself.
		if ownerID, _ := d.Store.OwnerID(); id == ownerID {
			httpx.Error(w, http.StatusForbidden, "the owner account cannot be deleted")
			return
		}
		if id == actor.ID {
			httpx.Error(w, http.StatusBadRequest, "you cannot delete your own account")
			return
		}
		if err := d.Store.DeleteUser(id); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func handleGetUserFolders(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		id, ok := pathID(r)
		if !ok {
			httpx.Error(w, http.StatusBadRequest, "bad user id")
			return
		}
		grants, err := d.Store.ListFolderGrants(id)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		if grants == nil {
			grants = []string{}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"folders": grants})
	})
}

func handleSetUserFolders(d Deps) http.Handler {
	type req struct {
		Folders []string `json:"folders"`
	}
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		id, ok := pathID(r)
		if !ok {
			httpx.Error(w, http.StatusBadRequest, "bad user id")
			return
		}
		var body req
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		// Normalize + drop anything invalid (traversal, empty) so only clean
		// paths are stored.
		clean := make([]string, 0, len(body.Folders))
		for _, f := range body.Folders {
			if c, ok := authz.Normalize(f); ok {
				clean = append(clean, c)
			}
		}
		if err := d.Store.SetFolderGrants(id, clean); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"folders": clean})
	})
}
