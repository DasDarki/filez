package files

import (
	"strings"
	"testing"

	"github.com/DasDarki/filez/internal/server/db"
	"github.com/DasDarki/filez/internal/server/storage"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(dir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	store, err := storage.New(dir, 8<<20)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(store.Close)
	return New(database, store, 1<<20)
}

func TestCreateGetBytes(t *testing.T) {
	svc := newTestService(t)
	content := "hello filez"
	f, err := svc.Create(strings.NewReader(content), CreateOptions{
		Mode: db.ModePermanent, OrigName: "hi.txt", Ext: "txt", MIME: "text/plain",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if f.Size != int64(len(content)) {
		t.Errorf("size = %d, want %d", f.Size, len(content))
	}

	got, err := svc.Get(f.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	b, err := svc.Bytes(got)
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if string(b) != content {
		t.Errorf("bytes = %q, want %q", b, content)
	}
}

func TestPassword(t *testing.T) {
	svc := newTestService(t)
	f, err := svc.Create(strings.NewReader("secret"), CreateOptions{
		Mode: db.ModePermanent, Password: "hunter2",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !f.HasPassword() {
		t.Fatal("expected password protection")
	}
	if svc.VerifyPassword(f, "wrong") {
		t.Error("wrong password accepted")
	}
	if !svc.VerifyPassword(f, "hunter2") {
		t.Error("correct password rejected")
	}
}

func TestLimitedDownloads(t *testing.T) {
	svc := newTestService(t)
	f, err := svc.Create(strings.NewReader("x"), CreateOptions{
		Mode: db.ModeLimited, Downloads: 2,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, ok, _ := svc.ConsumeLimited(f.ID); !ok {
		t.Fatal("first download should succeed")
	}
	if _, ok, _ := svc.ConsumeLimited(f.ID); !ok {
		t.Fatal("second download should succeed")
	}
	if _, ok, _ := svc.ConsumeLimited(f.ID); ok {
		t.Fatal("third download should be refused")
	}

	// Once exhausted, Get treats it as gone.
	if _, err := svc.Get(f.ID); err != ErrGone {
		t.Errorf("Get after exhaustion = %v, want ErrGone", err)
	}
}

func TestTempExpiryCleanup(t *testing.T) {
	svc := newTestService(t)
	// Force an already-expired temp file by pinning "now" into the future.
	f, err := svc.Create(strings.NewReader("temp"), CreateOptions{
		Mode: db.ModeTemp, TTL: 1,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	svc.now = func() int64 { return f.CreatedAt + 3600 } // an hour later

	if _, err := svc.Get(f.ID); err != ErrGone {
		t.Errorf("expired Get = %v, want ErrGone", err)
	}
}
