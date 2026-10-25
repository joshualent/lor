// Package store is Lor's SQLite storage (modernc.org/sqlite, WAL mode) with
// embedded migrations. All timestamps are Unix seconds; zero means NULL.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

//go:embed migrations/*.sql
var migrations embed.FS

// ErrNotFound is returned when a row doesn't exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when a conditional update matched no row, for
// example because the grant changed state in the meantime.
var ErrConflict = errors.New("conflict: state changed")

// DB is the database handle.
type DB struct {
	db *sql.DB
	q
}

// Open opens (creating if needed) the database at path and runs migrations.
func Open(ctx context.Context, path string) (*DB, error) {
	v := url.Values{}
	v.Add("_pragma", "busy_timeout(10000)")
	v.Add("_pragma", "journal_mode(WAL)")
	v.Add("_pragma", "foreign_keys(1)")
	v.Add("_pragma", "synchronous(NORMAL)")
	v.Add("_txlock", "immediate")
	dsn := "file:" + filepath.ToSlash(path) + "?" + v.Encode()
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection: SQLite has a single writer anyway, and this avoids
	// SQLITE_BUSY between goroutines of this process.
	sqldb.SetMaxOpenConns(1)
	if err := sqldb.PingContext(ctx); err != nil {
		sqldb.Close()
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	d := &DB{db: sqldb, q: q{sqldb}}
	if err := d.migrate(ctx); err != nil {
		sqldb.Close()
		return nil, err
	}
	return d, nil
}

// Close closes the database.
func (d *DB) Close() error { return d.db.Close() }

func (d *DB) migrate(ctx context.Context) error {
	if _, err := d.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		return err
	}
	names, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		version := strings.TrimSuffix(filepath.Base(name), ".sql")
		var n int
		if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		body, err := migrations.ReadFile(name)
		if err != nil {
			return err
		}
		err = d.WithTx(ctx, func(tx *Tx) error {
			if _, err := tx.tx.ExecContext(ctx, string(body)); err != nil {
				return fmt.Errorf("migration %s: %w", version, err)
			}
			_, err := tx.tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, strftime('%s','now'))`, version)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Tx is a transaction with the same query methods as DB.
type Tx struct {
	tx *sql.Tx
	q
}

// WithTx runs fn in a transaction (BEGIN IMMEDIATE), committing if fn
// returns nil.
func (d *DB) WithTx(ctx context.Context, fn func(tx *Tx) error) error {
	sqltx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(&Tx{tx: sqltx, q: q{sqltx}}); err != nil {
		sqltx.Rollback()
		return err
	}
	return sqltx.Commit()
}

type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// q holds the query methods shared by DB and Tx.
type q struct{ db dbtx }

func nullInt(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullStr(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nullBytes(v []byte) any {
	if len(v) == 0 {
		return nil
	}
	return v
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}

// scanner abstracts *sql.Row and *sql.Rows.
type scanner interface{ Scan(dest ...any) error }
