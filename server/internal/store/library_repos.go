package store

import (
	"database/sql"
	"encoding/json"
	"sort"
	"strconv"
	"time"
)

// Persistence for the reading of what is on the NAS (spec 0011).
//
// Until this shipped the reading lived only in memory, so it was lost on every
// restart and replaced with an empty one on every NAS blip — and an empty
// reading is indistinguishable, to a user, from "you own nothing". These two
// tables are what keeps it. See migrations 30 and 31.
//
// What is stored is deliberately narrow (spec 0011 FR-011): the parent folders
// the operator configured, the folder names read under them, and — per title
// folder — whether it holds video, which seasons and episodes were found, and
// the release tokens and file-identity keys already reduced out of the file
// names. Never a file name, never a size, never a path outside a configured
// parent. Nothing here is a secret, and nothing here is logged.

// LibraryParent is one configured parent folder and what it serves. Movies and
// TV are not exclusive: an operator is free to point both at one folder.
type LibraryParent struct {
	Path   string
	Movies bool
	TV     bool
}

// LibraryFolders is one whole reading of the configured parents — exactly the
// two inputs the matcher builds its index from, and nothing derived. Storing the
// inputs rather than the index means a change to the matching rules needs no
// migration and leaves the rules in one place.
type LibraryFolders struct {
	Parents   []LibraryParent
	Names     map[string][]string // parent path -> folder names under it
	ScannedAt time.Time
}

// SeasonPresence is what one season was found to hold. It mirrors the shape the
// API already serves; it is redeclared here rather than imported so the store
// stays a leaf package and cannot pull the source/library packages in behind it.
//
// There is no "complete" field, deliberately: VideoFiles > 0 with no Episodes is
// valid and means "present, numbering unreadable".
type SeasonPresence struct {
	Season     int   `json:"season"`
	Episodes   []int `json:"episodes,omitempty"`
	VideoFiles int   `json:"videoFiles"`
}

// ReleaseToken is which encode a file is, as read out of its name. The name
// itself is discarded — these three fields are all that is kept.
type ReleaseToken struct {
	Resolution string `json:"resolution"`
	Group      string `json:"group"`
	Key        string `json:"key"`
}

// LibraryEvidence is what one title folder was found to contain. Season 0 is the
// folder itself, which is where a movie's release lives.
type LibraryEvidence struct {
	Path      string
	HasVideo  bool
	Seasons   []SeasonPresence
	Releases  map[int][]ReleaseToken
	FileKeys  map[int][]string
	CheckedAt time.Time
}

// SaveLibraryFolders replaces the stored parent reading wholesale.
//
// Wholesale is the point, and it is what satisfies FR-009 for parents: a parent
// the operator has stopped configuring simply is not in the new set, so its rows
// go with it rather than lingering to answer for a folder that is no longer
// connected. The caller must therefore only ever pass a COMPLETE reading — a
// partial one would silently delete the rest.
func (s *Store) SaveLibraryFolders(f LibraryFolders) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM library_folders`); err != nil {
		return err
	}
	at := f.ScannedAt.Unix()
	for _, p := range f.Parents {
		for _, name := range f.Names[p.Path] {
			if _, err := tx.Exec(
				`INSERT OR REPLACE INTO library_folders (parent, name, movies, tv, scanned_at)
				 VALUES (?, ?, ?, ?, ?)`,
				p.Path, name, boolToInt(p.Movies), boolToInt(p.TV), at,
			); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// GetLibraryFolders returns the stored reading. A store that has never been
// scanned yields a zero value with no error — "nothing stored" is an answer, not
// a failure, and the caller degrades to today's behaviour.
//
// A parent that was configured but held no folders is not represented, because
// it contributes nothing to matching either way.
func (s *Store) GetLibraryFolders() (LibraryFolders, error) {
	rows, err := s.db.Query(
		`SELECT parent, name, movies, tv, scanned_at FROM library_folders ORDER BY parent, name`)
	if err != nil {
		return LibraryFolders{}, err
	}
	defer func() { _ = rows.Close() }()

	out := LibraryFolders{Names: map[string][]string{}}
	byPath := map[string]*LibraryParent{}
	var order []string
	var newest int64
	for rows.Next() {
		var parent, name string
		var movies, tv int
		var at int64
		if err := rows.Scan(&parent, &name, &movies, &tv, &at); err != nil {
			return LibraryFolders{}, err
		}
		p, seen := byPath[parent]
		if !seen {
			p = &LibraryParent{Path: parent}
			byPath[parent] = p
			order = append(order, parent)
		}
		// Folded rather than overwritten: a parent serving both kinds has both
		// flags set on every one of its rows, but folding is what keeps this
		// correct if that ever stops being true.
		p.Movies = p.Movies || movies != 0
		p.TV = p.TV || tv != 0
		out.Names[parent] = append(out.Names[parent], name)
		if at > newest {
			newest = at
		}
	}
	if err := rows.Err(); err != nil {
		return LibraryFolders{}, err
	}
	for _, path := range order {
		out.Parents = append(out.Parents, *byPath[path])
	}
	if newest > 0 {
		out.ScannedAt = time.Unix(newest, 0)
	}
	if len(out.Names) == 0 {
		out.Names = nil
	}
	return out, nil
}

// SaveLibraryEvidence records what one title folder was found to hold.
func (s *Store) SaveLibraryEvidence(e LibraryEvidence) error {
	seasons, err := json.Marshal(nonNilSeasons(e.Seasons))
	if err != nil {
		return err
	}
	// Season numbers are map keys, and JSON object keys must be strings — so they
	// are written as decimal strings and read back the same way.
	releases, err := json.Marshal(keyedByString(e.Releases))
	if err != nil {
		return err
	}
	fileKeys, err := json.Marshal(keyedByString(e.FileKeys))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT OR REPLACE INTO library_evidence
		   (path, has_video, seasons, releases, file_keys, checked_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		e.Path, boolToInt(e.HasVideo), string(seasons), string(releases), string(fileKeys),
		e.CheckedAt.Unix(),
	)
	return err
}

// GetLibraryEvidence returns the stored reading for one title folder.
// found is false when there is none — again an answer, not a failure.
func (s *Store) GetLibraryEvidence(path string) (LibraryEvidence, bool, error) {
	var (
		hasVideo                    int
		seasons, releases, fileKeys string
		checkedAt                   int64
	)
	err := s.db.QueryRow(
		`SELECT has_video, seasons, releases, file_keys, checked_at
		   FROM library_evidence WHERE path = ?`, path,
	).Scan(&hasVideo, &seasons, &releases, &fileKeys, &checkedAt)
	if err == sql.ErrNoRows {
		return LibraryEvidence{}, false, nil
	}
	if err != nil {
		return LibraryEvidence{}, false, err
	}

	out := LibraryEvidence{
		Path:      path,
		HasVideo:  hasVideo != 0,
		CheckedAt: time.Unix(checkedAt, 0),
	}
	// A row whose JSON will not parse is treated as no row at all: the caller then
	// re-reads the NAS, which is always recoverable. Failing the whole lookup would
	// not be.
	if err := json.Unmarshal([]byte(seasons), &out.Seasons); err != nil {
		return LibraryEvidence{}, false, nil
	}
	var rel map[string][]ReleaseToken
	if err := json.Unmarshal([]byte(releases), &rel); err != nil {
		return LibraryEvidence{}, false, nil
	}
	var keys map[string][]string
	if err := json.Unmarshal([]byte(fileKeys), &keys); err != nil {
		return LibraryEvidence{}, false, nil
	}
	out.Releases = keyedByInt(rel)
	out.FileKeys = keyedByInt(keys)
	if len(out.Seasons) == 0 {
		out.Seasons = nil
	}
	return out, true, nil
}

// StaleLibraryEvidence returns up to limit folder paths, least-recently-read
// first. That ordering is what lets a bounded scan converge over a large library
// instead of re-reading the same handful every cycle.
func (s *Store) StaleLibraryEvidence(limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.Query(
		`SELECT path FROM library_evidence ORDER BY checked_at ASC, path ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PruneLibraryEvidence drops readings for folders that are no longer present
// under a configured parent (FR-009).
//
// An EMPTY keep set is a no-op, not a wipe. Empty is what a failed or partial
// scan produces, and treating it as "the NAS holds nothing" would delete the
// fallback that the whole feature exists to provide — turning one bad scan into
// exactly the blank-ownership failure being fixed.
func (s *Store) PruneLibraryEvidence(keep []string) error {
	if len(keep) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// A temp table rather than a giant IN (...): a library can hold hundreds of
	// folders and SQLite has a bound on how many parameters one statement takes.
	if _, err := tx.Exec(`CREATE TEMP TABLE IF NOT EXISTS library_keep (path TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM library_keep`); err != nil {
		return err
	}
	for _, p := range keep {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO library_keep (path) VALUES (?)`, p); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(
		`DELETE FROM library_evidence WHERE path NOT IN (SELECT path FROM library_keep)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE library_keep`); err != nil {
		return err
	}
	return tx.Commit()
}

// nonNilSeasons keeps the stored JSON as `[]` rather than `null`, so a reader
// that never unmarshals into a fresh value still sees an empty list.
func nonNilSeasons(s []SeasonPresence) []SeasonPresence {
	if s == nil {
		return []SeasonPresence{}
	}
	return s
}

func keyedByString[V any](m map[int]V) map[string]V {
	out := make(map[string]V, len(m))
	for k, v := range m {
		out[strconv.Itoa(k)] = v
	}
	return out
}

func keyedByInt[V any](m map[string]V) map[int]V {
	if len(m) == 0 {
		return nil
	}
	out := make(map[int]V, len(m))
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		n, err := strconv.Atoi(k)
		if err != nil {
			continue // an unreadable key is dropped, never fatal
		}
		out[n] = m[k]
	}
	return out
}
