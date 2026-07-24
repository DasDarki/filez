package db

import (
	"database/sql"
	"errors"
)

// InsertFile stores a new file metadata row.
func (d *DB) InsertFile(f *File) error {
	_, err := d.write.Exec(`
INSERT INTO files (id, ext, orig_name, size, mime, mode, expires_at, downloads_left, download_count, password_hash, storage_path, created_at, last_accessed_at, keep)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.Ext, f.OrigName, f.Size, f.MIME, string(f.Mode),
		f.ExpiresAt, f.DownloadsLeft, f.DownloadCount, f.PasswordHash, f.StoragePath, f.CreatedAt,
		f.LastAccessedAt, boolToInt(f.Keep))
	return err
}

func scanFile(row interface{ Scan(...any) error }) (*File, error) {
	var f File
	var mode string
	var keep int
	err := row.Scan(&f.ID, &f.Ext, &f.OrigName, &f.Size, &f.MIME, &mode,
		&f.ExpiresAt, &f.DownloadsLeft, &f.DownloadCount, &f.PasswordHash, &f.StoragePath, &f.CreatedAt,
		&f.LastAccessedAt, &keep)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	f.Mode = FileMode(mode)
	f.Keep = keep != 0
	return &f, nil
}

const fileCols = `id, ext, orig_name, size, mime, mode, expires_at, downloads_left, download_count, password_hash, storage_path, created_at, last_accessed_at, keep`

// GetFile returns the file with the given id, or ErrNotFound.
func (d *DB) GetFile(id string) (*File, error) {
	return scanFile(d.read.QueryRow(`SELECT `+fileCols+` FROM files WHERE id = ?`, id))
}

// DeleteFile removes the metadata row. The caller is responsible for the bytes.
func (d *DB) DeleteFile(id string) error {
	_, err := d.write.Exec(`DELETE FROM files WHERE id = ?`, id)
	return err
}

// IDExists reports whether a file id is already taken (used for collision retry).
func (d *DB) IDExists(id string) (bool, error) {
	var one int
	err := d.read.QueryRow(`SELECT 1 FROM files WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ConsumeLimitedDownload atomically decrements downloads_left (and bumps
// download_count) for a limited file, returning the remaining count. ok is
// false when the file is already exhausted or missing.
func (d *DB) ConsumeLimitedDownload(id string) (remaining int64, ok bool, err error) {
	row := d.write.QueryRow(`
UPDATE files
SET downloads_left = downloads_left - 1, download_count = download_count + 1
WHERE id = ? AND downloads_left > 0
RETURNING downloads_left`, id)
	err = row.Scan(&remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return remaining, true, nil
}

// BumpDownloadCount increments the download counter for stats (non-limited files).
func (d *DB) BumpDownloadCount(id string) error {
	_, err := d.write.Exec(`UPDATE files SET download_count = download_count + 1 WHERE id = ?`, id)
	return err
}

// ExpiredFiles returns files that should be cleaned up at time now (unix seconds):
// temp files past expiry and limited files with no downloads left.
func (d *DB) ExpiredFiles(now int64) ([]*File, error) {
	rows, err := d.read.Query(`SELECT `+fileCols+`
FROM files
WHERE (expires_at IS NOT NULL AND expires_at <= ?)
   OR (mode = 'limited' AND downloads_left IS NOT NULL AND downloads_left <= 0)`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// TouchAccess records that a file was accessed at time now (for idle cleanup).
func (d *DB) TouchAccess(id string, now int64) error {
	_, err := d.write.Exec(`UPDATE files SET last_accessed_at = ? WHERE id = ?`, now, id)
	return err
}

// IdlePermanentFiles returns permanent, non-kept files whose last access is at or
// before idleBefore (unix seconds) — candidates for idle cleanup.
func (d *DB) IdlePermanentFiles(idleBefore int64) ([]*File, error) {
	rows, err := d.read.Query(`SELECT `+fileCols+`
FROM files
WHERE mode = 'permanent' AND keep = 0 AND last_accessed_at <= ?`, idleBefore)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
