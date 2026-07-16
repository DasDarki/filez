package db

import (
	"database/sql"
	"errors"
)

// InsertAccessKey stores a new access key.
func (d *DB) InsertAccessKey(k *AccessKey) error {
	_, err := d.write.Exec(`
INSERT INTO access_keys (key, label, expires_at, revoked, created_at)
VALUES (?, ?, ?, ?, ?)`,
		k.Key, k.Label, k.ExpiresAt, boolToInt(k.Revoked), k.CreatedAt)
	return err
}

// GetAccessKey returns the key row, or ErrNotFound.
func (d *DB) GetAccessKey(key string) (*AccessKey, error) {
	var k AccessKey
	var revoked int
	err := d.read.QueryRow(`SELECT key, label, expires_at, revoked, created_at FROM access_keys WHERE key = ?`, key).
		Scan(&k.Key, &k.Label, &k.ExpiresAt, &revoked, &k.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	k.Revoked = revoked != 0
	return &k, nil
}

// ListAccessKeys returns all keys, newest first.
func (d *DB) ListAccessKeys() ([]*AccessKey, error) {
	rows, err := d.read.Query(`SELECT key, label, expires_at, revoked, created_at FROM access_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*AccessKey
	for rows.Next() {
		var k AccessKey
		var revoked int
		if err := rows.Scan(&k.Key, &k.Label, &k.ExpiresAt, &revoked, &k.CreatedAt); err != nil {
			return nil, err
		}
		k.Revoked = revoked != 0
		out = append(out, &k)
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
