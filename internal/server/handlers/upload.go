package handlers

import (
	"errors"
	"mime"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DasDarki/filez/internal/server/auth"
	"github.com/DasDarki/filez/internal/server/db"
	"github.com/DasDarki/filez/internal/server/files"
	"github.com/DasDarki/filez/internal/server/storage"
	"github.com/DasDarki/filez/internal/timefmt"
	"github.com/gofiber/fiber/v3"
)

// postUpload accepts a multipart upload and stores it with the requested policy.
func (h *Handlers) postUpload(c fiber.Ctx) error {
	fh, err := c.FormFile("file")
	if err != nil {
		return jsonErr(c, fiber.StatusBadRequest, "no file provided")
	}

	opts := files.CreateOptions{
		Mode:     db.ModePermanent,
		OrigName: fh.Filename,
		Password: c.FormValue("password"),
	}

	// Extension (without the leading dot), preserving multi-part like tar.gz poorly
	// is unnecessary — a single trailing extension is enough for the URL.
	opts.Ext = strings.TrimPrefix(strings.ToLower(filepath.Ext(fh.Filename)), ".")

	// MIME: trust the browser-provided type, else derive from the extension.
	opts.MIME = fh.Header.Get("Content-Type")
	if opts.MIME == "" || opts.MIME == "application/octet-stream" {
		if byExt := mime.TypeByExtension("." + opts.Ext); byExt != "" {
			opts.MIME = byExt
		}
	}
	if opts.MIME == "" {
		opts.MIME = "application/octet-stream"
	}
	if i := strings.IndexByte(opts.MIME, ';'); i >= 0 {
		opts.MIME = strings.TrimSpace(opts.MIME[:i])
	}

	switch strings.ToLower(c.FormValue("mode", "permanent")) {
	case "", "permanent", "perma":
		opts.Mode = db.ModePermanent
	case "temp", "temporary":
		opts.Mode = db.ModeTemp
		ttl, err := timefmt.Parse(c.FormValue("ttl"))
		if err != nil {
			return jsonErr(c, fiber.StatusBadRequest, "invalid or missing ttl: "+err.Error())
		}
		opts.TTL = ttl
	case "limited":
		opts.Mode = db.ModeLimited
		n := int64(1)
		if v := c.FormValue("downloads"); v != "" {
			parsed, err := strconv.ParseInt(v, 10, 64)
			if err != nil || parsed < 1 {
				return jsonErr(c, fiber.StatusBadRequest, "invalid downloads count")
			}
			n = parsed
		}
		opts.Downloads = n
	default:
		return jsonErr(c, fiber.StatusBadRequest, "unknown mode")
	}

	// --keep exempts a permanent file from idle cleanup. When cleanup is active
	// this requires authorization: never allowed on a public instance, and on a
	// private instance only for access keys the admin has granted it.
	if v := c.FormValue("keep"); v == "true" || v == "1" || v == "on" {
		if h.cfg.CleanupEnabled && opts.Mode == db.ModePermanent {
			if h.cfg.Public {
				return jsonErr(c, fiber.StatusForbidden,
					"permanent files are not allowed on this public instance (idle cleanup is active)")
			}
			ak, err := h.db.GetAccessKey(auth.ExtractAccessKey(c))
			if err != nil || !ak.AllowPermanent {
				return jsonErr(c, fiber.StatusForbidden,
					"this access key is not allowed to create permanent files")
			}
		}
		opts.Keep = true
	}

	src, err := fh.Open()
	if err != nil {
		return jsonErr(c, fiber.StatusInternalServerError, "could not read upload")
	}
	defer src.Close()

	f, err := h.files.Create(src, opts)
	if err != nil {
		if errors.Is(err, storage.ErrTooLarge) {
			return jsonErr(c, fiber.StatusRequestEntityTooLarge, "file exceeds maximum size")
		}
		return jsonErr(c, fiber.StatusInternalServerError, "upload failed")
	}

	base := h.baseURL(c)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":          f.ID,
		"ext":         f.Ext,
		"url":         base + downloadPath(f),
		"preview_url": base + previewPath(f),
	})
}
