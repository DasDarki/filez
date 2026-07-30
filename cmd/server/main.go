// Command server runs the Filez HTTP server: web UI, uploads, direct links,
// previews and admin access-key management.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/DasDarki/filez/internal/server/auth"
	"github.com/DasDarki/filez/internal/server/config"
	"github.com/DasDarki/filez/internal/server/db"
	"github.com/DasDarki/filez/internal/server/files"
	"github.com/DasDarki/filez/internal/server/handlers"
	"github.com/DasDarki/filez/internal/server/live"
	"github.com/DasDarki/filez/internal/server/storage"
	"github.com/gofiber/fiber/v3"
)

const version = "1.0.0"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatalf("data dir: %v", err)
	}

	database, err := db.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	store, err := storage.New(cfg.DataDir, cfg.CacheSize)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	defer store.Close()

	var cleanupAfter time.Duration
	if cfg.CleanupEnabled {
		cleanupAfter = cfg.CleanupAfter
	}
	svc := files.New(database, store, cfg.MaxUploadSize, cleanupAfter)
	guard := auth.New(cfg, database)

	// Live sessions hold one frame each in memory; cap a single frame to protect memory.
	liveMax := cfg.MaxUploadSize
	if liveMax > 64<<20 {
		liveMax = 64 << 20
	}
	liveStore := live.New(liveMax)

	app := fiber.New(fiber.Config{
		AppName:           "Filez " + version,
		ServerHeader:      "Filez",
		BodyLimit:         int(cfg.MaxUploadSize) + 16<<20, // upload + multipart overhead
		StreamRequestBody: true,
		// Behind a reverse proxy (Coolify/Traefik/nginx) honor X-Forwarded-Proto
		// and X-Forwarded-Host so generated links use the public https URL.
		// Only loopback/private/link-local proxies are trusted, so directly
		// exposed instances (public client IPs) are unaffected.
		TrustProxy: cfg.TrustProxy,
		TrustProxyConfig: fiber.TrustProxyConfig{
			Loopback:  true,
			Private:   true,
			LinkLocal: true,
		},
	})

	handlers.New(cfg, svc, database, guard, liveStore, version).Register(app)

	// Background cleanup of expired/limited files and abandoned live sessions.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go cleanupLoop(ctx, svc, liveStore)

	logStartup(cfg)
	if err := app.Listen(":"+strconv.Itoa(cfg.Port), fiber.ListenConfig{
		DisableStartupMessage: true,
		GracefulContext:       ctx, // Ctrl-C / SIGTERM triggers a graceful shutdown.
	}); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

func cleanupLoop(ctx context.Context, svc *files.Service, liveStore *live.Store) {
	// Abandoned live sessions (no push) are dropped after this long.
	const liveIdle = 2 * time.Hour

	sweep := func() {
		if n, err := svc.CleanupExpired(); err != nil {
			log.Printf("cleanup error: %v", err)
		} else if n > 0 {
			log.Printf("cleanup: removed %d expired file(s)", n)
		}
		if n := liveStore.Cleanup(time.Now().Add(-liveIdle).Unix()); n > 0 {
			log.Printf("cleanup: removed %d idle live session(s)", n)
		}
	}

	sweep() // once at startup
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}

func logStartup(cfg *config.Config) {
	mode := "public"
	if !cfg.Public {
		mode = "private (access key required)"
	}
	admin := "disabled"
	if cfg.AdminEnabled() {
		admin = "enabled (/admin)"
	}
	cleanup := "off"
	if cfg.CleanupEnabled {
		cleanup = "idle > " + cfg.CleanupRaw
	}
	log.Printf("Filez %s listening on :%d — %s — admin: %s — cleanup: %s — data: %s",
		version, cfg.Port, mode, admin, cleanup, cfg.DataDir)
}
