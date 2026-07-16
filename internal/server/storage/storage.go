// Package storage keeps file bytes on disk and accelerates hot reads with an
// in-memory cache (ristretto). SQLite (the db package) only holds metadata.
package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dgraph-io/ristretto/v2"
)

// ErrTooLarge is returned when an upload exceeds the configured maximum.
var ErrTooLarge = errors.New("storage: file exceeds maximum size")

// Store writes files under root and caches small hot files in memory.
type Store struct {
	root    string
	cache   *ristretto.Cache[string, []byte]
	itemMax int64 // largest single file kept in the cache
}

// New creates a Store rooted at dataDir/files with a byte cache of at most
// cacheBytes total cost.
func New(dataDir string, cacheBytes int64) (*Store, error) {
	root := filepath.Join(dataDir, "files")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("storage: mkdir root: %w", err)
	}

	if cacheBytes < 1<<20 {
		cacheBytes = 1 << 20
	}
	cache, err := ristretto.NewCache(&ristretto.Config[string, []byte]{
		NumCounters: 1e6,
		MaxCost:     cacheBytes,
		BufferItems: 64,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: cache: %w", err)
	}

	// Never let a single item evict most of the cache; also cap absolute size.
	itemMax := cacheBytes / 4
	if itemMax > 8<<20 {
		itemMax = 8 << 20
	}

	return &Store{root: root, cache: cache, itemMax: itemMax}, nil
}

// Close releases cache resources.
func (s *Store) Close() {
	if s.cache != nil {
		s.cache.Close()
	}
}

// relPath shards files by the first two id characters to avoid huge directories.
func relPath(id string) string {
	shard := id
	if len(id) >= 2 {
		shard = id[:2]
	}
	return filepath.Join(shard, id)
}

// AbsPath returns the absolute path for a stored relative path.
func (s *Store) AbsPath(rel string) string { return filepath.Join(s.root, rel) }

// Save streams r to disk under an id-derived path, enforcing maxSize (0 = unlimited).
// It returns the relative path and the number of bytes written.
func (s *Store) Save(id string, r io.Reader, maxSize int64) (rel string, size int64, err error) {
	rel = relPath(id)
	abs := filepath.Join(s.root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", 0, err
	}

	f, err := os.OpenFile(abs, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	var src io.Reader = r
	if maxSize > 0 {
		src = io.LimitReader(r, maxSize+1)
	}
	n, err := io.Copy(f, src)
	if err != nil {
		_ = os.Remove(abs)
		return "", 0, err
	}
	if maxSize > 0 && n > maxSize {
		_ = os.Remove(abs)
		return "", 0, ErrTooLarge
	}
	return rel, n, nil
}

// ReadFull loads a whole file into memory (used for cacheable/small files).
func (s *Store) ReadFull(rel string) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.root, rel))
}

// Open returns a seekable handle for range serving of large files.
func (s *Store) Open(rel string) (*os.File, error) {
	return os.Open(filepath.Join(s.root, rel))
}

// Delete removes the bytes for a stored file (missing file is not an error).
func (s *Store) Delete(rel string) error {
	err := os.Remove(filepath.Join(s.root, rel))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Cacheable reports whether a file of the given size is eligible for the memory cache.
func (s *Store) Cacheable(size int64) bool {
	return size > 0 && size <= s.itemMax
}

// CacheGet returns cached bytes for a file id.
func (s *Store) CacheGet(id string) ([]byte, bool) {
	return s.cache.Get(id)
}

// CachePut stores bytes for a file id, using length as the cache cost.
func (s *Store) CachePut(id string, data []byte) {
	s.cache.Set(id, data, int64(len(data)))
}

// CacheDel drops a file id from the cache (on delete/expiry).
func (s *Store) CacheDel(id string) {
	s.cache.Del(id)
}
