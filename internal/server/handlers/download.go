package handlers

import (
	"errors"

	"github.com/DasDarki/filez/internal/server/auth"
	"github.com/DasDarki/filez/internal/server/db"
	"github.com/DasDarki/filez/internal/server/files"
	"github.com/gofiber/fiber/v3"
)

// getDownload serves the raw file bytes for /d/<id>.<ext>.
func (h *Handlers) getDownload(c fiber.Ctx) error {
	f, err := h.files.Get(fileID(c))
	if err != nil {
		return h.fileError(c, err)
	}

	if f.HasPassword() && !h.files.VerifyPassword(f, auth.FilePassword(c)) {
		c.Set("WWW-Authenticate", `Basic realm="Filez File"`)
		return c.Status(fiber.StatusUnauthorized).SendString("password required")
	}

	initial := isInitialRequest(c)
	if initial {
		h.files.TouchAccess(f.ID) // refresh idle-cleanup timer
	}

	// Limited files spend one download on the initial (non-range-continuation) GET.
	if f.Mode == db.ModeLimited {
		if initial {
			_, ok, err := h.files.ConsumeLimited(f.ID)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).SendString("internal error")
			}
			if !ok {
				_ = h.files.Delete(f)
				return c.Status(fiber.StatusGone).SendString("no downloads left")
			}
		}
	} else if initial {
		h.files.BumpCount(f.ID)
	}

	// Small/hot files: serve from memory with range support.
	if h.files.Store().Cacheable(f.Size) {
		data, err := h.files.Bytes(f)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("read error")
		}
		return serveBytes(c, data, f.MIME, dispName(f))
	}

	// Large files: let Fiber stream from disk with byte-range support.
	if f.MIME != "" {
		c.Set("Content-Type", f.MIME)
	}
	c.Set("Content-Disposition", "inline; filename=\""+sanitizeFilename(dispName(f))+"\"")
	return c.SendFile(h.files.Store().AbsPath(f.StoragePath), fiber.SendFile{ByteRange: true})
}

// dispName is the filename used in Content-Disposition.
func dispName(f *db.File) string {
	if f.OrigName != "" {
		return f.OrigName
	}
	if f.Ext != "" {
		return f.ID + "." + f.Ext
	}
	return f.ID
}

// fileError maps service errors to HTTP status codes for the file routes.
func (h *Handlers) fileError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, files.ErrNotFound):
		return c.Status(fiber.StatusNotFound).SendString("file not found")
	case errors.Is(err, files.ErrGone):
		return c.Status(fiber.StatusGone).SendString("file expired")
	default:
		return c.Status(fiber.StatusInternalServerError).SendString("internal error")
	}
}
