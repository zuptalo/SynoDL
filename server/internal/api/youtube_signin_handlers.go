package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"synodl/server/internal/httpx"
	"synodl/server/internal/store"
	"synodl/server/internal/ytsignin"
)

// The admin's side of the YouTube sign-in (spec 1055).
//
// Every response here is METADATA. There is no path from this file to a cookie
// value going out: the pasted text is parsed, sealed and dropped, and the only
// read of the plaintext is the worker fetch (youtube_signin_grants.go).

type signinView struct {
	Available     bool     `json:"available"`
	Saved         bool     `json:"saved"`
	CookieCount   int      `json:"cookieCount"`
	LoginCookies  []string `json:"loginCookies"`
	MissingLogin  []string `json:"missingLogin"`
	SavedAt       *int64   `json:"savedAt"`
	SavedBy       string   `json:"savedBy"`
	LastRefusedAt *int64   `json:"lastRefusedAt"`
	LastOKAt      *int64   `json:"lastOkAt"`
	Warning       string   `json:"warning,omitempty"`
}

func (d Deps) signinView() (signinView, error) {
	v := signinView{Available: d.Stateful && d.Store != nil, LoginCookies: []string{}, MissingLogin: []string{}}
	y, err := d.Store.GetYoutubeSignIn()
	if err != nil {
		return v, err
	}
	if y == nil {
		v.MissingLogin = append(v.MissingLogin, ytsignin.LoginCookies...)
		return v, nil
	}
	have := map[string]bool{}
	for _, n := range y.LoginFound {
		have[n] = true
		v.LoginCookies = append(v.LoginCookies, n)
	}
	for _, n := range ytsignin.LoginCookies {
		if !have[n] {
			v.MissingLogin = append(v.MissingLogin, n)
		}
	}
	v.Saved, v.CookieCount, v.SavedBy = true, y.CookieCount, y.SavedByName
	at := y.SavedAt
	v.SavedAt, v.LastRefusedAt, v.LastOKAt = &at, y.LastRefusedAt, y.LastOKAt
	return v, nil
}

func signinError(w http.ResponseWriter, status int, code, message string) {
	httpx.JSON(w, status, map[string]any{"error": code, "message": message})
}

func handleGetYoutubeSignIn(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, _ *http.Request, _ *store.User) {
		v, err := d.signinView()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		httpx.JSON(w, http.StatusOK, v)
	})
}

func handlePutYoutubeSignIn(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		var body struct {
			Text string `json:"text"`
		}
		// The bound is on the request as well as the text: JSON escaping can make a
		// 64 KiB paste a good deal larger on the wire.
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*ytsignin.MaxPasteBytes)).Decode(&body); err != nil {
			signinError(w, http.StatusBadRequest, "too_large", "That is too large to be a browser's cookies.")
			return
		}
		res, err := ytsignin.Parse(body.Text)
		body.Text = "" // the paste is not needed past here
		if err != nil {
			switch {
			case errors.Is(err, ytsignin.ErrTooLarge):
				signinError(w, http.StatusBadRequest, "too_large", "That is too large to be a browser's cookies.")
			case errors.Is(err, ytsignin.ErrTooFew):
				signinError(w, http.StatusBadRequest, "too_few_cookies", "That has too few cookies to be a signed-in session.")
			case errors.Is(err, ytsignin.ErrInvalid):
				signinError(w, http.StatusBadRequest, "invalid", "Some of that is not a valid cookie.")
			default:
				signinError(w, http.StatusBadRequest, "unrecognised", "That does not look like a Cookie header or a cookies file.")
			}
			return
		}
		if err := d.Store.SaveYoutubeSignIn(res.Netscape, res.Count, res.Found, u.Username, d.nowUnix()); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		v, err := d.signinView()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		v.Warning = res.Warning()
		httpx.JSON(w, http.StatusOK, v)
	})
}

func handleDeleteYoutubeSignIn(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, _ *http.Request, _ *store.User) {
		if err := d.Store.DeleteYoutubeSignIn(); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
