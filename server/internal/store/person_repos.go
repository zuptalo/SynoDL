package store

import (
	"database/sql"
	"sync/atomic"
	"time"
)

// Remembered faces (spec 0014). See migration 39.
//
// Two tables, and they are a CACHE in the strict sense: losing them costs
// lookups, never data. They exist because the alternative — re-deriving after
// every restart — means re-reading a third party's pages for hundreds of people,
// which is both slow for the user and the behaviour most likely to get an
// instance blocked.
//
// What they hold is derived public fact: a person's public identity, their name,
// and a public image URL. Deliberately NOT here, and never to be added: a user
// id, a title id, or anything else that would turn a cache of faces into a
// record of who looked at what (FR-030). Nothing here is a secret, so nothing
// here is encrypted — and nothing that would deserve to be may be put in it.

// Lifetimes. A found photograph is remembered far longer than a missing one:
// publicity stills change rarely, whereas "no photograph" is also what a BLOCK
// looks like, and a blocked instance must recover on its own rather than caching
// its own outage for a month.
const (
	personPhotoTTL   = 30 * 24 * time.Hour
	personMissingTTL = 3 * 24 * time.Hour
	sourcePersonTTL  = 30 * 24 * time.Hour
)

// Bounds. Both tables are pruned on write — expired rows first, then the oldest
// beyond the cap. At roughly a hundred bytes a row this is a couple of megabytes
// at worst, and it cannot grow past that however long the instance runs.
const (
	personRowCap    = 20000
	personPruneEach = 256
)

// personWrites counts writes across both tables so the prune runs occasionally
// rather than on every insert. A cache that swept itself on every write would
// spend more time pruning than looking things up.
var personWrites atomic.Uint64

// PersonPhoto is a remembered answer to "what does this person look like".
// Found is false for a person no photograph could be found for — which is a real
// answer, cached so it is not asked again on every view, and distinct from
// having never asked.
type PersonPhoto struct {
	URL   string
	Found bool
}

// GetPersonPhoto returns what is remembered about a person's photograph, and
// whether that memory is still fresh. A stale row reads as a miss: the caller
// re-resolves and overwrites it, so expiry needs no sweeper.
func (s *Store) GetPersonPhoto(imdbID string) (PersonPhoto, bool) {
	var url string
	var checked int64
	err := s.db.QueryRow(
		`SELECT photo_url, checked_at FROM person_photos WHERE imdb_id = ?`, imdbID,
	).Scan(&url, &checked)
	if err != nil {
		return PersonPhoto{}, false
	}
	ttl := personPhotoTTL
	if url == "" {
		ttl = personMissingTTL
	}
	if time.Since(time.Unix(checked, 0)) > ttl {
		return PersonPhoto{}, false
	}
	return PersonPhoto{URL: url, Found: url != ""}, true
}

// PutPersonPhoto records the outcome of a lookup, found or not.
func (s *Store) PutPersonPhoto(imdbID, photoURL string) error {
	_, err := s.db.Exec(`
		INSERT INTO person_photos (imdb_id, photo_url, checked_at) VALUES (?, ?, ?)
		ON CONFLICT(imdb_id) DO UPDATE SET photo_url = excluded.photo_url, checked_at = excluded.checked_at`,
		imdbID, photoURL, time.Now().Unix())
	if err != nil {
		return err
	}
	s.maybePrunePeople()
	return nil
}

// SourcePerson is a source's own handle for somebody, resolved once: who they
// are, and the portrait that source hosts of them.
type SourcePerson struct {
	IMDbID   string
	PhotoURL string
	Name     string
}

// GetSourcePerson returns a resolved source handle, and whether it is fresh.
func (s *Store) GetSourcePerson(kind, ref string) (SourcePerson, bool) {
	var p SourcePerson
	var checked int64
	err := s.db.QueryRow(
		`SELECT imdb_id, photo_url, name, checked_at FROM source_people WHERE source_kind = ? AND ref = ?`,
		kind, ref,
	).Scan(&p.IMDbID, &p.PhotoURL, &p.Name, &checked)
	if err != nil {
		return SourcePerson{}, false
	}
	if time.Since(time.Unix(checked, 0)) > sourcePersonTTL {
		return SourcePerson{}, false
	}
	return p, true
}

// PutSourcePerson records a resolved source handle. A resolution that found
// nothing is stored too — that is the answer, and re-asking for it on every view
// of every title they appear in is exactly what this table exists to prevent.
func (s *Store) PutSourcePerson(kind, ref string, p SourcePerson) error {
	_, err := s.db.Exec(`
		INSERT INTO source_people (source_kind, ref, imdb_id, photo_url, name, checked_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_kind, ref) DO UPDATE SET
			imdb_id = excluded.imdb_id, photo_url = excluded.photo_url,
			name = excluded.name, checked_at = excluded.checked_at`,
		kind, ref, p.IMDbID, p.PhotoURL, p.Name, time.Now().Unix())
	if err != nil {
		return err
	}
	s.maybePrunePeople()
	return nil
}

// maybePrunePeople bounds both tables, occasionally (FR-029). Failures are
// ignored on purpose: pruning a cache is housekeeping, and a failed sweep must
// never turn into a failed page.
func (s *Store) maybePrunePeople() {
	if personWrites.Add(1)%personPruneEach != 0 {
		return
	}
	s.PrunePeople()
}

// PrunePeople drops what has expired and then what is oldest beyond the cap.
// Exported so a test can ask for it rather than writing 256 rows to trigger it.
func (s *Store) PrunePeople() {
	now := time.Now().Unix()
	// Expired first — a row past its life is worthless, whatever the cap says.
	exec(s.db, `DELETE FROM person_photos WHERE (photo_url <> '' AND checked_at < ?) OR (photo_url = '' AND checked_at < ?)`,
		now-int64(personPhotoTTL.Seconds()), now-int64(personMissingTTL.Seconds()))
	exec(s.db, `DELETE FROM source_people WHERE checked_at < ?`, now-int64(sourcePersonTTL.Seconds()))
	// Then the oldest beyond the cap. Oldest rather than least-used: a row's age
	// is the only thing this table knows, and re-resolving one costs one lookup.
	exec(s.db, `DELETE FROM person_photos WHERE imdb_id IN (
		SELECT imdb_id FROM person_photos ORDER BY checked_at DESC LIMIT -1 OFFSET ?)`, personRowCap)
	exec(s.db, `DELETE FROM source_people WHERE rowid IN (
		SELECT rowid FROM source_people ORDER BY checked_at DESC LIMIT -1 OFFSET ?)`, personRowCap)
}

func exec(db *sql.DB, q string, args ...any) {
	_, _ = db.Exec(q, args...)
}
