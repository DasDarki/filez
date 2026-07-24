package db

// FileMode is the lifecycle policy of a stored file.
type FileMode string

const (
	ModePermanent FileMode = "permanent"
	ModeTemp      FileMode = "temp"
	ModeLimited   FileMode = "limited"
)

// File is a stored file's metadata. Bytes live on disk (StoragePath); only this
// record lives in SQLite.
type File struct {
	ID             string
	Ext            string // without leading dot, may be empty
	OrigName       string
	Size           int64
	MIME           string
	Mode           FileMode
	ExpiresAt      *int64 // unix seconds, nil = never (permanent/limited)
	DownloadsLeft  *int64 // nil unless Mode == ModeLimited
	DownloadCount  int64
	PasswordHash   string // "" = no password
	StoragePath    string // path relative to DataDir
	CreatedAt      int64
	LastAccessedAt int64 // unix seconds of the last /d or /p access
	Keep           bool  // exempt from idle cleanup (truly permanent)
}

// HasPassword reports whether the file is password protected.
func (f *File) HasPassword() bool { return f.PasswordHash != "" }

// IsExpired reports whether the file should no longer be served at time now (unix seconds).
func (f *File) IsExpired(now int64) bool {
	if f.ExpiresAt != nil && *f.ExpiresAt <= now {
		return true
	}
	if f.Mode == ModeLimited && f.DownloadsLeft != nil && *f.DownloadsLeft <= 0 {
		return true
	}
	return false
}

// AccessKey grants access to a non-public instance.
type AccessKey struct {
	Key            string
	Label          string
	ExpiresAt      *int64 // nil = permanent
	Revoked        bool
	CreatedAt      int64
	AllowPermanent bool // may upload files exempt from idle cleanup (--keep)
}

// Valid reports whether the key currently grants access at time now.
func (k *AccessKey) Valid(now int64) bool {
	if k.Revoked {
		return false
	}
	if k.ExpiresAt != nil && *k.ExpiresAt <= now {
		return false
	}
	return true
}
