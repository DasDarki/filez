// Package db is the SQLite metadata store for Filez. It uses the pure-Go
// modernc.org/sqlite driver in WAL mode. Only metadata is stored here; file
// bytes live on disk (see the storage package).
package db

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a lookup finds no matching row.
var ErrNotFound = errors.New("db: not found")

// DB wraps two connection pools: a single-connection writer (SQLite allows only
// one writer) and a multi-connection reader, per modernc.org/sqlite guidance.
type DB struct {
	write *sql.DB
	read  *sql.DB
}

const dsnPragmas = "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)"

// Open opens (creating if needed) the SQLite database at dataDir/filez.db and
// applies the schema.
func Open(dataDir string) (*DB, error) {
	path := filepath.Join(dataDir, "filez.db")
	dsn := "file:" + path + "?" + dsnPragmas

	write, err := sql.Open("sqlite", dsn+"&_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("open writer: %w", err)
	}
	write.SetMaxOpenConns(1)

	read, err := sql.Open("sqlite", dsn)
	if err != nil {
		_ = write.Close()
		return nil, fmt.Errorf("open reader: %w", err)
	}
	read.SetMaxOpenConns(8)

	d := &DB{write: write, read: read}
	if err := d.migrate(); err != nil {
		_ = d.Close()
		return nil, err
	}
	return d, nil
}

// Close closes both pools.
func (d *DB) Close() error {
	err1 := d.read.Close()
	err2 := d.write.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

func (d *DB) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS files (
    id             TEXT PRIMARY KEY,
    ext            TEXT NOT NULL DEFAULT '',
    orig_name      TEXT NOT NULL DEFAULT '',
    size           INTEGER NOT NULL DEFAULT 0,
    mime           TEXT NOT NULL DEFAULT 'application/octet-stream',
    mode           TEXT NOT NULL,
    expires_at     INTEGER,
    downloads_left INTEGER,
    download_count INTEGER NOT NULL DEFAULT 0,
    password_hash  TEXT NOT NULL DEFAULT '',
    storage_path   TEXT NOT NULL,
    created_at     INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_files_expires ON files(expires_at);

CREATE TABLE IF NOT EXISTS access_keys (
    key        TEXT PRIMARY KEY,
    label      TEXT NOT NULL DEFAULT '',
    expires_at INTEGER,
    revoked    INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);
`
	_, err := d.write.Exec(schema)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}
