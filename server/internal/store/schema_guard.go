package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
)

// Guarding against schema drift (spec 2018).
//
// A migration inserted into the MIDDLE of the list never runs on an installation
// already past that position: the counter says it has been applied, so it is
// skipped forever. This has now happened twice — spec 2012 (a source column,
// which took the source list down with a 500) and spec 2017 (the hide-owned
// column, which failed quietly for months) — and both were found by their
// symptoms rather than by the append-only guard that exists to prevent them.
//
// That guard cannot find them. It pins the list as it already stands, so it stops
// the NEXT insertion while blessing any already present. What it never asks is
// the question that actually matters: does a database that upgraded through this
// list end up with the same schema as one built from it today?
//
// This file asks that question — as a test across every historical stopping
// point, and again at start-up, where a column that is missing anyway is simply
// added.

// schemaShape is what a database actually looks like: table name -> its columns,
// plus the indexes. It is deliberately just names. Types and constraints would
// make the comparison stricter, and also make it fail on differences SQLite
// itself introduces (a column added by ALTER cannot carry every constraint the
// same column has in a CREATE), which would train everyone to ignore it.
type schemaShape struct {
	tables  map[string][]string
	indexes []string
}

// shapeOf reads the live schema.
func shapeOf(db *sql.DB) (schemaShape, error) {
	out := schemaShape{tables: map[string][]string{}}
	rows, err := db.Query(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return out, err
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			_ = rows.Close()
			return out, err
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return out, err
	}
	_ = rows.Close()

	for _, table := range names {
		cols, err := columnsOf(db, table)
		if err != nil {
			return out, err
		}
		out.tables[table] = cols
	}

	irows, err := db.Query(
		`SELECT name FROM sqlite_master WHERE type = 'index' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return out, err
	}
	defer func() { _ = irows.Close() }()
	for irows.Next() {
		var n string
		if err := irows.Scan(&n); err != nil {
			return out, err
		}
		out.indexes = append(out.indexes, n)
	}
	return out, irows.Err()
}

// columnsOf returns a table's column names, sorted, so comparison does not depend
// on the order columns happen to have been added in.
func columnsOf(db *sql.DB, table string) ([]string, error) {
	// table comes from sqlite_master or from the migration list, never from a
	// request, and PRAGMA takes no bind parameters.
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%q)`, table))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var cols []string
	for rows.Next() {
		var (
			cid, notnull, pk int
			name, typ        string
			dflt             any
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(cols)
	return cols, nil
}

// referenceShape is the schema the migration list produces from nothing — what
// every installation is supposed to end up with.
func referenceShape() (schemaShape, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return schemaShape{}, err
	}
	defer func() { _ = db.Close() }()
	ref := &Store{db: db}
	if err := ref.migrate(); err != nil {
		return schemaShape{}, err
	}
	return shapeOf(db)
}

// schemaDrift lists what this database is missing against the reference, in a
// form safe to log: identifiers from the source, never anything from a request
// or from the NAS.
func schemaDrift(have, want schemaShape) []string {
	var out []string
	for table, wantCols := range want.tables {
		haveCols, ok := have.tables[table]
		if !ok {
			out = append(out, "missing table "+table)
			continue
		}
		set := make(map[string]bool, len(haveCols))
		for _, c := range haveCols {
			set[c] = true
		}
		for _, c := range wantCols {
			if !set[c] {
				out = append(out, "missing column "+table+"."+c)
			}
		}
	}
	haveIdx := make(map[string]bool, len(have.indexes))
	for _, i := range have.indexes {
		haveIdx[i] = true
	}
	for _, i := range want.indexes {
		if !haveIdx[i] {
			out = append(out, "missing index "+i)
		}
	}
	sort.Strings(out)
	return out
}

// reAddColumn finds the ADD COLUMN statements in the migration list, so one whose
// column is missing can be run again.
var reAddColumn = regexp.MustCompile(
	`(?is)ALTER\s+TABLE\s+["']?(\w+)["']?\s+ADD\s+COLUMN\s+["']?(\w+)["']?[^;]*;`)

// repairSchema adds columns this database should have and does not.
//
// Only ADD COLUMN, deliberately. It is the one repair that is precisely
// idempotent, cannot lose data, and is exactly the class that has actually gone
// wrong — twice. A missing TABLE or INDEX is reported and left alone: creating a
// table implies deciding what belongs in it, and a database missing one has a
// problem an automatic repair should not paper over.
//
// Returns what it repaired, for logging.
func (s *Store) repairSchema() ([]string, error) {
	want, err := referenceShape()
	if err != nil {
		return nil, err
	}
	return s.repairSchemaAgainst(want)
}

// repairSchemaAgainst is repairSchema with the reference already in hand, so a
// start-up that both repairs and re-checks builds it once rather than twice.
func (s *Store) repairSchemaAgainst(want schemaShape) ([]string, error) {
	have, err := shapeOf(s.db)
	if err != nil {
		return nil, err
	}
	drift := schemaDrift(have, want)
	if len(drift) == 0 {
		return nil, nil
	}

	missing := map[string]bool{}
	for _, d := range drift {
		if rest, ok := strings.CutPrefix(d, "missing column "); ok {
			missing[rest] = true
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}

	var repaired []string
	for _, m := range migrations {
		for _, stmt := range reAddColumn.FindAllStringSubmatch(m, -1) {
			key := stmt[1] + "." + stmt[2]
			if !missing[key] {
				continue
			}
			if _, err := s.db.Exec(stmt[0]); err != nil {
				if !isDuplicateColumn(err) {
					return repaired, fmt.Errorf("repair %s: %w", key, err)
				}
			}
			delete(missing, key)
			repaired = append(repaired, key)
		}
	}
	return repaired, nil
}

// checkSchema repairs what it safely can and reports whatever is left.
//
// It never fails start-up. An instance with a drifted schema is already running
// in production; refusing to open its database would turn a partial problem into
// a total outage, which is the opposite of the point.
func (s *Store) checkSchema() {
	want, err := referenceShape()
	if err != nil {
		slog.Error("schema reference", "err", err)
		return
	}
	repaired, err := s.repairSchemaAgainst(want)
	if err != nil {
		slog.Error("schema repair", "err", err)
	}
	if len(repaired) > 0 {
		// Worth a line: it means this database had drifted, and knowing which
		// columns is how the cause gets found.
		slog.Warn("schema drift repaired", "columns", strings.Join(repaired, ","))
	}
	have, err := shapeOf(s.db)
	if err != nil {
		return
	}
	if rest := schemaDrift(have, want); len(rest) > 0 {
		slog.Error("schema drift remains", "items", strings.Join(rest, ","))
	}
	// Rows whose parent is gone, left behind while foreign keys were off on most
	// connections (spec 2019). Driven by what SQLite reports as broken rather than
	// by a migration having run, because on the reporting instance that migration
	// was recorded without taking effect.
	if swept, err := s.sweepOrphans(); err != nil {
		slog.Error("orphan sweep", "err", err)
	} else if len(swept) > 0 {
		slog.Warn("orphaned rows cleaned", "rows", strings.Join(swept, ","))
	}
}

// sweepOrphans deletes rows SQLite itself reports as violating a foreign key.
//
// The migration that first did this cannot be relied on alone. On the reporting
// instance its version was recorded while the statements had no effect — the same
// "recorded as applied, never ran" shape that has now bitten three times — and a
// cleanup that silently does not happen is worse than none, because everyone
// believes it did.
//
// So it is driven by PRAGMA foreign_key_check instead of by bookkeeping. That
// asks the database what is actually broken rather than what should have been
// fixed, which means:
//   - it cannot act on a healthy database, because there is nothing to report
//   - it cannot remove a row that is still referenced, because SQLite would not
//     have named it
//   - it needs no version, no flag, and no memory of having run
//
// The action per row is the one the schema itself declares: CASCADE deletes,
// SET NULL clears the column. Anything else is left alone and reported.
func (s *Store) sweepOrphans() ([]string, error) {
	type violation struct {
		table string
		rowid int64
		fkid  int64
	}
	rows, err := s.db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return nil, err
	}
	var found []violation
	for rows.Next() {
		var v violation
		var parent sql.NullString
		var rowid sql.NullInt64
		if err := rows.Scan(&v.table, &rowid, &parent, &v.fkid); err != nil {
			_ = rows.Close()
			return nil, err
		}
		// A WITHOUT ROWID table cannot be addressed this way; skip rather than
		// guess at which row was meant.
		if !rowid.Valid {
			continue
		}
		v.rowid = rowid.Int64
		found = append(found, v)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()
	if len(found) == 0 {
		return nil, nil
	}

	cleaned := map[string]int{}
	for _, v := range found {
		action, column, err := fkAction(s.db, v.table, v.fkid)
		if err != nil {
			return nil, err
		}
		switch action {
		case "CASCADE":
			if _, err := s.db.Exec(
				fmt.Sprintf(`DELETE FROM %q WHERE rowid = ?`, v.table), v.rowid); err != nil {
				return nil, fmt.Errorf("sweep %s: %w", v.table, err)
			}
			cleaned[v.table+" (deleted)"]++
		case "SET NULL":
			if _, err := s.db.Exec(
				fmt.Sprintf(`UPDATE %q SET %q = NULL WHERE rowid = ?`, v.table, column), v.rowid); err != nil {
				return nil, fmt.Errorf("sweep %s: %w", v.table, err)
			}
			cleaned[v.table+"."+column+" (cleared)"]++
		default:
			// NO ACTION / RESTRICT: the schema does not say to remove it, so we do
			// not. The caller reports it instead.
			cleaned[v.table+" (left: "+action+")"]++
		}
	}
	out := make([]string, 0, len(cleaned))
	for k, n := range cleaned {
		out = append(out, fmt.Sprintf("%s x%d", k, n))
	}
	sort.Strings(out)
	return out, nil
}

// fkAction reports what the schema says to do when the parent of this foreign key
// disappears, and which column holds it.
func fkAction(db *sql.DB, table string, fkid int64) (action, column string, err error) {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA foreign_key_list(%q)`, table))
	if err != nil {
		return "", "", err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			id, seq                             int64
			parent, from, to, onUpd, onDel, mat string
		)
		if err := rows.Scan(&id, &seq, &parent, &from, &to, &onUpd, &onDel, &mat); err != nil {
			return "", "", err
		}
		if id == fkid {
			return strings.ToUpper(onDel), from, nil
		}
	}
	return "", "", rows.Err()
}
