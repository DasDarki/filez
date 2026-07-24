package db

import (
	"database/sql"
	"errors"
)

// InsertAccessKey stores a new access key.
func (d *DB) InsertAccessKey(k *AccessKey) error {
	_, err := d.write.Exec(`
INSERT INTO access_keys (key, label, expires_at, revoked, created_at, allow_permanent)
VALUES (?, ?, ?, ?, ?, ?)`,
		k.Key, k.Label, k.ExpiresAt, boolToInt(k.Revoked), k.CreatedAt, boolToInt(k.AllowPermanent))
	return err
}

const keyCols = `key, label, expires_at, revoked, created_at, allow_permanent`

func scanKey(row interface{ Scan(...any) error }) (*AccessKey, error) {
	var k AccessKey
	var revoked, allowPerm int
	if err := row.Scan(&k.Key, &k.Label, &k.ExpiresAt, &revoked, &k.CreatedAt, &allowPerm); err != nil {
		return nil, err
	}
	k.Revoked = revoked != 0
	k.AllowPermanent = allowPerm != 0
	return &k, nil
}

// GetAccessKey returns the key row, or ErrNotFound.
func (d *DB) GetAccessKey(key string) (*AccessKey, error) {
	k, err := scanKey(d.read.QueryRow(`SELECT `+keyCols+` FROM access_keys WHERE key = ?`, key))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return k, err
}

// ListAccessKeys returns all keys, newest first.
func (d *DB) ListAccessKeys() ([]*AccessKey, error) {
	rows, err := d.read.Query(`SELECT ` + keyCols + ` FROM access_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*AccessKey
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// DeleteAccessKey removes a key permanently.
func (d *DB) DeleteAccessKey(key string) error {
	_, err := d.write.Exec(`DELETE FROM access_keys WHERE key = ?`, key)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
