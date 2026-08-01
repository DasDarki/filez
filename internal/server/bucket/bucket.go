// Package bucket holds ephemeral "sync buckets": temporary, in-memory, shared
// file drops identified by a short 4-digit code. Anyone with the code can upload,
// list and download; only the creator (holding the owner token) can close it.
// Nothing is persisted.
package bucket

import (
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/DasDarki/filez/internal/server/idgen"
)

// Errors returned by the store.
var (
	ErrNotFound = errors.New("bucket: not found")
	ErrTooLarge = errors.New("bucket: file too large")
	ErrFull     = errors.New("bucket: full")
	ErrNoCode   = errors.New("bucket: no free code")
)

// File is one item in a bucket.
type File struct {
	ID         string
	Name       string
	MIME       string
	Size       int64
	Data       []byte
	UploadedAt int64
}

// Bucket is a temporary shared drop.
type Bucket struct {
	Code       string
	OwnerToken string
	Files      []*File
	CreatedAt  int64
	UpdatedAt  int64
	totalSize  int64
}

// Store is a concurrency-safe collection of buckets.
type Store struct {
	mu       sync.RWMutex
	buckets  map[string]*Bucket
	maxFile  int64
	maxTotal int64
	maxFiles int
	now      func() int64
}

// New creates a Store. maxFile caps a single file, maxTotal caps a bucket's total
// bytes, maxFiles caps the number of files per bucket. Any cap given as 0 (or
// negative) means unlimited.
func New(maxFile, maxTotal int64, maxFiles int) *Store {
	return &Store{
		buckets:  make(map[string]*Bucket),
		maxFile:  maxFile,
		maxTotal: maxTotal,
		maxFiles: maxFiles,
		now:      func() int64 { return time.Now().Unix() },
	}
}

// MaxFile returns the per-file size cap.
func (s *Store) MaxFile() int64 { return s.maxFile }

// Create allocates a new bucket with a free 4-digit code.
func (s *Store) Create() (*Bucket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := 0; i < 50; i++ {
		code := randCode()
		if _, taken := s.buckets[code]; taken {
			continue
		}
		now := s.now()
		b := &Bucket{Code: code, OwnerToken: idgen.NewKey(), CreatedAt: now, UpdatedAt: now}
		s.buckets[code] = b
		return b, nil
	}
	return nil, ErrNoCode
}

// Get returns a bucket by code.
func (s *Store) Get(code string) (*Bucket, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.buckets[code]
	return b, ok
}

// Add stores a file in a bucket, enforcing the size and count caps (0 = unlimited).
func (s *Store) Add(code, name, mime string, data []byte) (*File, error) {
	if s.maxFile > 0 && int64(len(data)) > s.maxFile {
		return nil, ErrTooLarge
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.buckets[code]
	if b == nil {
		return nil, ErrNotFound
	}
	if s.maxFiles > 0 && len(b.Files) >= s.maxFiles {
		return nil, ErrFull
	}
	if s.maxTotal > 0 && b.totalSize+int64(len(data)) > s.maxTotal {
		return nil, ErrFull
	}
	f := &File{ID: idgen.New(8), Name: name, MIME: mime, Size: int64(len(data)), Data: data, UploadedAt: s.now()}
	b.Files = append(b.Files, f)
	b.totalSize += f.Size
	b.UpdatedAt = s.now()
	return f, nil
}

// List returns a metadata snapshot (no bytes) of a bucket's files.
func (s *Store) List(code string) ([]File, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b := s.buckets[code]
	if b == nil {
		return nil, false
	}
	out := make([]File, 0, len(b.Files))
	for _, f := range b.Files {
		out = append(out, File{ID: f.ID, Name: f.Name, MIME: f.MIME, Size: f.Size, UploadedAt: f.UploadedAt})
	}
	return out, true
}

// FileData returns a file's bytes.
func (s *Store) FileData(code, fileID string) (data []byte, mime, name string, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b := s.buckets[code]
	if b == nil {
		return nil, "", "", false
	}
	for _, f := range b.Files {
		if f.ID == fileID {
			return f.Data, f.MIME, f.Name, true
		}
	}
	return nil, "", "", false
}

// Close removes a bucket if the owner token matches. Returns false if the bucket
// is missing or the token is wrong.
func (s *Store) Close(code, ownerToken string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.buckets[code]
	if b == nil || b.OwnerToken != ownerToken {
		return false
	}
	delete(s.buckets, code)
	return true
}

// Cleanup removes buckets idle since idleBefore (unix seconds), returning the count.
func (s *Store) Cleanup(idleBefore int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for code, b := range s.buckets {
		if b.UpdatedAt <= idleBefore {
			delete(s.buckets, code)
			n++
		}
	}
	return n
}

func randCode() string {
	n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(10000))
	if err != nil {
		return "0000"
	}
	return fmt.Sprintf("%04d", n.Int64())
}
