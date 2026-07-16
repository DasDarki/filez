// Package files is the service layer tying metadata (db) and bytes (storage)
// together: creating files with a lifecycle policy, resolving/serving them with
// lazy expiry, consuming limited downloads, and cleaning up expired files.
package files

import (
	"errors"
	"io"
	"time"

	"github.com/DasDarki/filez/internal/server/db"
	"github.com/DasDarki/filez/internal/server/idgen"
	"github.com/DasDarki/filez/internal/server/storage"
	"golang.org/x/crypto/bcrypt"
)

// ErrGone signals a file that existed but has expired or run out of downloads.
var ErrGone = errors.New("files: gone")

// ErrNotFound is re-exported for convenience.
var ErrNotFound = db.ErrNotFound

// Service coordinates the metadata store and byte storage.
type Service struct {
	db        *db.DB
	store     *storage.Store
	maxUpload int64
	now       func() int64
}

// New builds a file service.
func New(database *db.DB, store *storage.Store, maxUpload int64) *Service {
	return &Service{
		db:        database,
		store:     store,
		maxUpload: maxUpload,
		now:       func() int64 { return time.Now().Unix() },
	}
}

// CreateOptions describe the desired lifecycle and protection of an upload.
type CreateOptions struct {
	Mode      db.FileMode
	TTL       time.Duration // for ModeTemp
	Downloads int64         // for ModeLimited
	Password  string        // empty = none
	OrigName  string
	Ext       string
	MIME      string
}

// Create stores an uploaded file and returns its metadata.
func (s *Service) Create(r io.Reader, opts CreateOptions) (*db.File, error) {
	id, err := s.freshID()
	if err != nil {
		return nil, err
	}

	rel, size, err := s.store.Save(id, r, s.maxUpload)
	if err != nil {
		return nil, err
	}

	now := s.now()
	f := &db.File{
		ID:          id,
		Ext:         opts.Ext,
		OrigName:    opts.OrigName,
		Size:        size,
		MIME:        opts.MIME,
		Mode:        opts.Mode,
		StoragePath: rel,
		CreatedAt:   now,
	}

	switch opts.Mode {
	case db.ModeTemp:
		exp := now + int64(opts.TTL.Seconds())
		f.ExpiresAt = &exp
	case db.ModeLimited:
		n := opts.Downloads
		if n < 1 {
			n = 1
		}
		f.DownloadsLeft = &n
	}

	if opts.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(opts.Password), bcrypt.DefaultCost)
		if err != nil {
			_ = s.store.Delete(rel)
			return nil, err
		}
		f.PasswordHash = string(hash)
	}

	if err := s.db.InsertFile(f); err != nil {
		_ = s.store.Delete(rel)
		return nil, err
	}
	return f, nil
}

func (s *Service) freshID() (string, error) {
	for i := 0; i < 8; i++ {
		id := idgen.New(idgen.DefaultLength)
		exists, err := s.db.IDExists(id)
		if err != nil {
			return "", err
		}
		if !exists {
			return id, nil
		}
	}
	return "", errors.New("files: could not allocate unique id")
}

// Get resolves a file by id, deleting and reporting ErrGone if it has expired.
func (s *Service) Get(id string) (*db.File, error) {
	f, err := s.db.GetFile(id)
	if err != nil {
		return nil, err
	}
	if f.IsExpired(s.now()) {
		_ = s.Delete(f)
		return nil, ErrGone
	}
	return f, nil
}

// VerifyPassword checks a plaintext password against the file's hash.
func (s *Service) VerifyPassword(f *db.File, password string) bool {
	if !f.HasPassword() {
		return true
	}
	return bcrypt.CompareHashAndPassword([]byte(f.PasswordHash), []byte(password)) == nil
}

// ConsumeLimited atomically spends one download of a limited file. ok is false
// when the file just ran out (caller should treat it as Gone).
func (s *Service) ConsumeLimited(id string) (remaining int64, ok bool, err error) {
	return s.db.ConsumeLimitedDownload(id)
}

// BumpCount records a download for stats on non-limited files (best effort).
func (s *Service) BumpCount(id string) {
	_ = s.db.BumpDownloadCount(id)
}

// Bytes returns the file contents, using the memory cache for eligible sizes.
func (s *Service) Bytes(f *db.File) ([]byte, error) {
	if s.store.Cacheable(f.Size) {
		if b, ok := s.store.CacheGet(f.ID); ok {
			return b, nil
		}
		b, err := s.store.ReadFull(f.StoragePath)
		if err != nil {
			return nil, err
		}
		s.store.CachePut(f.ID, b)
		return b, nil
	}
	return s.store.ReadFull(f.StoragePath)
}

// Store exposes the underlying byte store (for large-file streaming in handlers).
func (s *Service) Store() *storage.Store { return s.store }

// Delete removes a file's bytes, cache entry and metadata.
func (s *Service) Delete(f *db.File) error {
	s.store.CacheDel(f.ID)
	_ = s.store.Delete(f.StoragePath)
	return s.db.DeleteFile(f.ID)
}

// CleanupExpired removes all currently-expired files, returning the count.
func (s *Service) CleanupExpired() (int, error) {
	expired, err := s.db.ExpiredFiles(s.now())
	if err != nil {
		return 0, err
	}
	for _, f := range expired {
		_ = s.Delete(f)
	}
	return len(expired), nil
}
