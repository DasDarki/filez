// Package handlers wires the Filez HTTP routes onto a Fiber app.
package handlers

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/DasDarki/filez/internal/server/auth"
	"github.com/DasDarki/filez/internal/server/bucket"
	"github.com/DasDarki/filez/internal/server/config"
	"github.com/DasDarki/filez/internal/server/db"
	"github.com/DasDarki/filez/internal/server/files"
	"github.com/DasDarki/filez/internal/server/live"
	"github.com/DasDarki/filez/internal/server/web"
	"github.com/gofiber/fiber/v3"
)

// Handlers bundles the dependencies shared by all route handlers.
type Handlers struct {
	cfg     *config.Config
	files   *files.Service
	db      *db.DB
	guard   *auth.Guard
	live    *live.Store
	buckets *bucket.Store
	version string
}

// New creates the handler set.
func New(cfg *config.Config, svc *files.Service, database *db.DB, guard *auth.Guard, liveStore *live.Store, buckets *bucket.Store, version string) *Handlers {
	return &Handlers{cfg: cfg, files: svc, db: database, guard: guard, live: liveStore, buckets: buckets, version: version}
}

// Register mounts all routes on app.
func (h *Handlers) Register(app *fiber.App) {
	// Public discovery + auth check (used by the CLI and the web gate).
	app.Get("/api/info", h.getInfo)
	app.Get("/api/auth/check", h.getAuthCheck)

	// Static frontend assets (embedded).
	app.Get("/assets/*", h.getAsset)

	// Index page (renders the gate itself when non-public).
	app.Get("/", h.getIndex)

	// Instance-gated routes. In Fiber the handler chain runs left-to-right, so
	// the auth middleware MUST come before the actual handler.
	gated := h.guard.InstanceAuth()

	// Uploading always requires the access key on a private instance.
	app.Post("/api/upload", gated, h.postUpload)

	// Download and preview links: by default (PublicLinks) they are reachable
	// without an access key even on a private instance, so shared links work for
	// recipients — password-protected files stay gated by their password. Set
	// PUBLIC_LINKS=false to require the access key here too. Readable links keep
	// the original filename in the path; the id (first segment) is the
	// collision-free, unguessable lookup key. The legacy "<id>.<ext>" form works too.
	var linkGate []fiber.Handler
	if !h.cfg.PublicLinks {
		linkGate = []fiber.Handler{gated}
	}
	registerLink := func(path string, handler fiber.Handler) {
		if len(linkGate) > 0 {
			app.Get(path, linkGate[0], handler)
		} else {
			app.Get(path, handler)
		}
	}
	registerLink("/d/:id/:name", h.getDownload)
	registerLink("/d/:name", h.getDownload)
	registerLink("/p/:id/:name", h.getPreview)
	registerLink("/p/:name", h.getPreview)

	// Live sessions: start/push/stop require the access key (like uploads); the
	// viewer and its frame are public (the session id is the unguessable secret).
	app.Post("/api/live", gated, h.postLiveStart)
	app.Put("/api/live/:id", gated, h.putLiveImage)
	app.Delete("/api/live/:id", gated, h.deleteLive)
	app.Get("/l/:id", h.getLiveViewer)
	app.Get("/l/:id/image", h.getLiveImage)
	app.Get("/l/:id/rev", h.getLiveRev)

	// Sync buckets: creating one requires the access key (like uploads); once
	// created, anyone with the 4-digit code can upload/list/download. Only the
	// creator (owner token) can close it.
	app.Post("/api/sync", gated, h.postBucketCreate)
	app.Get("/s/:code", h.getBucketPage)
	app.Get("/api/sync/:code", h.getBucketList)
	app.Post("/api/sync/:code", h.postBucketUpload)
	app.Get("/api/sync/:code/:fileid", h.getBucketFile)
	app.Delete("/api/sync/:code", h.deleteBucket)

	// Admin area (only when an admin password is configured).
	if h.cfg.AdminEnabled() {
		admin := h.guard.AdminAuth()
		app.Get("/admin", admin, h.getAdminPage)
		app.Get("/api/admin/keys", admin, h.listKeys)
		app.Post("/api/admin/keys", admin, h.createKey)
		app.Patch("/api/admin/keys/:key", admin, h.updateKey)
		app.Delete("/api/admin/keys/:key", admin, h.deleteKey)
	}
}

// ---- helpers ----

// baseURL returns the external base URL for building links.
func (h *Handlers) baseURL(c fiber.Ctx) string {
	if h.cfg.BaseURL != "" {
		return h.cfg.BaseURL
	}
	return c.Scheme() + "://" + c.Host() // Host() includes the port
}

// splitName splits "<id>.<ext>" into id and extension (ext may be empty or contain dots, e.g. tar.gz).
func splitName(name string) (id, ext string) {
	if i := strings.IndexByte(name, '.'); i >= 0 {
		return name[:i], name[i+1:]
	}
	return name, ""
}

// fileID extracts the lookup id from either URL form: the readable
// "/d/<id>/<name>" (first segment) or the legacy "/d/<id>.<ext>".
func fileID(c fiber.Ctx) string {
	if id := c.Params("id"); id != "" {
		return id
	}
	id, _ := splitName(c.Params("name"))
	return id
}

// urlName makes a filename safe to embed as a single URL path segment.
func urlName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	if name == "" {
		name = "file"
	}
	return url.PathEscape(name)
}

// downloadPath and previewPath build readable, name-bearing paths for a file.
// The name segment is cosmetic — lookups always go by f.ID — so it never affects
// uniqueness.
func downloadPath(f *db.File) string { return "/d/" + f.ID + "/" + urlName(dispName(f)) }
func previewPath(f *db.File) string  { return "/p/" + f.ID + "/" + urlName(dispName(f)) }

var textExts = map[string]bool{
	"txt": true, "md": true, "markdown": true, "json": true, "xml": true, "yml": true, "yaml": true,
	"csv": true, "tsv": true, "log": true, "ini": true, "toml": true, "env": true, "conf": true,
	"js": true, "ts": true, "jsx": true, "tsx": true, "go": true, "py": true, "rb": true, "rs": true,
	"c": true, "h": true, "cpp": true, "cc": true, "hpp": true, "java": true, "kt": true, "cs": true,
	"php": true, "sh": true, "bash": true, "fish": true, "zsh": true, "sql": true, "html": true,
	"css": true, "scss": true, "less": true, "vue": true, "svelte": true, "lua": true, "pl": true,
}

var archiveExts = map[string]bool{
	"zip": true, "tar": true, "gz": true, "tgz": true, "rar": true, "7z": true, "bz2": true, "xz": true,
}

// classify chooses a preview viewer kind from the MIME type and extension.
func classify(mime, ext string) string {
	ext = strings.ToLower(ext)
	switch {
	case strings.HasPrefix(mime, "image/"):
		return "image"
	case strings.HasPrefix(mime, "video/"):
		return "video"
	case strings.HasPrefix(mime, "audio/"):
		return "audio"
	case mime == "application/pdf" || ext == "pdf":
		return "pdf"
	case strings.HasPrefix(mime, "text/") || textExts[ext]:
		return "text"
	case archiveExts[ext] || strings.Contains(mime, "zip"):
		return "archive"
	default:
		return "binary"
	}
}

// isInitialRequest reports whether the request should count as a download
// (a GET that is not the continuation of a byte-range read).
func isInitialRequest(c fiber.Ctx) bool {
	if c.Method() != fiber.MethodGet {
		return false
	}
	r := c.Get("Range")
	return r == "" || strings.HasPrefix(r, "bytes=0-")
}

// serveBytes serves an in-memory file with single-range support.
func serveBytes(c fiber.Ctx, data []byte, mime, filename string) error {
	c.Set("Accept-Ranges", "bytes")
	if mime != "" {
		c.Set("Content-Type", mime)
	}
	if filename != "" {
		c.Set("Content-Disposition", "inline; filename=\""+sanitizeFilename(filename)+"\"")
	}

	total := int64(len(data))
	if rng := c.Get("Range"); rng != "" {
		start, end, ok := parseRange(rng, total)
		if !ok {
			c.Set("Content-Range", "bytes */"+strconv.FormatInt(total, 10))
			return c.SendStatus(fiber.StatusRequestedRangeNotSatisfiable)
		}
		c.Set("Content-Range", "bytes "+strconv.FormatInt(start, 10)+"-"+strconv.FormatInt(end, 10)+"/"+strconv.FormatInt(total, 10))
		c.Status(fiber.StatusPartialContent)
		return c.Send(data[start : end+1])
	}
	return c.Send(data)
}

// parseRange parses a single "bytes=start-end" header against total size.
func parseRange(header string, total int64) (start, end int64, ok bool) {
	const p = "bytes="
	if !strings.HasPrefix(header, p) || total == 0 {
		return 0, 0, false
	}
	spec := header[len(p):]
	if strings.Contains(spec, ",") {
		return 0, 0, false // multi-range unsupported
	}
	dash := strings.IndexByte(spec, '-')
	if dash < 0 {
		return 0, 0, false
	}
	startStr, endStr := spec[:dash], spec[dash+1:]

	if startStr == "" { // suffix range: last N bytes
		n, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		if n > total {
			n = total
		}
		return total - n, total - 1, true
	}

	start, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil || start >= total {
		return 0, 0, false
	}
	end = total - 1
	if endStr != "" {
		e, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil {
			return 0, 0, false
		}
		if e < end {
			end = e
		}
	}
	if end < start {
		return 0, 0, false
	}
	return start, end, true
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\"", "")
	name = strings.ReplaceAll(name, "\n", "")
	name = strings.ReplaceAll(name, "\r", "")
	return name
}

// jsonErr writes a JSON error response.
func jsonErr(c fiber.Ctx, code int, msg string) error {
	return c.Status(code).JSON(fiber.Map{"error": msg})
}

// getAsset serves an embedded static asset.
func (h *Handlers) getAsset(c fiber.Ctx) error {
	p := "assets/" + c.Params("*")
	return c.SendFile(p, fiber.SendFile{FS: web.Assets})
}

// getIndex serves the index page.
func (h *Handlers) getIndex(c fiber.Ctx) error {
	return c.SendFile("index.html", fiber.SendFile{FS: web.Assets})
}

// getAdminPage serves the admin page.
func (h *Handlers) getAdminPage(c fiber.Ctx) error {
	return c.SendFile("admin.html", fiber.SendFile{FS: web.Assets})
}
