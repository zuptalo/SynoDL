package store

import (
	"database/sql"
	"errors"
	"strings"
)

// The YouTube sign-in (spec 1055): one instance-wide set of cookies, sealed.
//
// Two read paths, and the difference is the point. GetYoutubeSignIn returns
// METADATA ONLY and is what every API handler uses; OpenYoutubeSignInCookies
// returns the plaintext and has exactly one caller, the worker fetch handler.

// YoutubeSignIn is the non-secret description of the saved sign-in.
type YoutubeSignIn struct {
	CookieCount   int
	LoginFound    []string
	SavedAt       int64
	SavedByName   string
	LastRefusedAt *int64
	LastOKAt      *int64
}

// SaveYoutubeSignIn replaces the saved sign-in. A new sign-in starts with no
// recorded outcomes: a refusal of the OLD session says nothing about this one.
func (s *Store) SaveYoutubeSignIn(cookies []byte, count int, loginFound []string, savedBy string, now int64) error {
	sealed, err := s.cipher.Seal(cookies)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO youtube_signin (id, cookies_sealed, cookie_count, login_found, saved_at, saved_by_name, last_refused_at, last_ok_at)
		VALUES (1, ?, ?, ?, ?, ?, NULL, NULL)
		ON CONFLICT(id) DO UPDATE SET
		  cookies_sealed = excluded.cookies_sealed, cookie_count = excluded.cookie_count,
		  login_found = excluded.login_found, saved_at = excluded.saved_at,
		  saved_by_name = excluded.saved_by_name, last_refused_at = NULL, last_ok_at = NULL`,
		sealed, count, strings.Join(loginFound, ","), now, savedBy)
	return err
}

// GetYoutubeSignIn returns the metadata, or (nil, nil) when none is saved.
// A row the current key cannot open is reported as absent, so the admin is asked
// to paste again instead of being shown a failure they cannot act on.
func (s *Store) GetYoutubeSignIn() (*YoutubeSignIn, error) {
	var (
		y      YoutubeSignIn
		found  string
		sealed []byte
		refus  sql.NullInt64
		ok     sql.NullInt64
	)
	err := s.db.QueryRow(`SELECT cookies_sealed, cookie_count, login_found, saved_at, saved_by_name, last_refused_at, last_ok_at
		FROM youtube_signin WHERE id = 1`).Scan(&sealed, &y.CookieCount, &found, &y.SavedAt, &y.SavedByName, &refus, &ok)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := s.cipher.Open(sealed); err != nil {
		return nil, nil
	}
	if found != "" {
		y.LoginFound = strings.Split(found, ",")
	}
	if refus.Valid {
		v := refus.Int64
		y.LastRefusedAt = &v
	}
	if ok.Valid {
		v := ok.Int64
		y.LastOKAt = &v
	}
	return &y, nil
}

// OpenYoutubeSignInCookies returns the plaintext cookies file. Its only caller
// is the worker fetch handler; nothing that answers a browser may call it.
func (s *Store) OpenYoutubeSignInCookies() ([]byte, bool, error) {
	var sealed []byte
	err := s.db.QueryRow(`SELECT cookies_sealed FROM youtube_signin WHERE id = 1`).Scan(&sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	plain, err := s.cipher.Open(sealed)
	if err != nil {
		return nil, false, nil
	}
	return plain, true, nil
}

// DeleteYoutubeSignIn removes the sign-in, leaving nothing of it stored.
func (s *Store) DeleteYoutubeSignIn() error {
	_, err := s.db.Exec(`DELETE FROM youtube_signin WHERE id = 1`)
	return err
}

// NoteYoutubeSignInOutcome stamps the outcome of a FINISHED download that used the
// sign-in. A success clears the refusal: the session works again.
func (s *Store) NoteYoutubeSignInOutcome(refused bool, now int64) error {
	if refused {
		_, err := s.db.Exec(`UPDATE youtube_signin SET last_refused_at = ? WHERE id = 1`, now)
		return err
	}
	_, err := s.db.Exec(`UPDATE youtube_signin SET last_ok_at = ?, last_refused_at = NULL WHERE id = 1`, now)
	return err
}
