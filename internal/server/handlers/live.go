package handlers

import (
	"io"
	"mime"
	"path/filepath"
	"strings"

	"github.com/DasDarki/filez/internal/server/web"
	"github.com/gofiber/fiber/v3"
)

// postLiveStart creates a new live session and returns its id and viewer URL.
func (h *Handlers) postLiveStart(c fiber.Ctx) error {
	s := h.live.Create()
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"session_id": s.ID,
		"viewer_url": h.baseURL(c) + "/l/" + s.ID,
	})
}

// putLiveImage replaces the current frame of a live session.
func (h *Handlers) putLiveImage(c fiber.Ctx) error {
	fh, err := c.FormFile("file")
	if err != nil {
		return jsonErr(c, fiber.StatusBadRequest, "no file provided")
	}
	if max := h.live.MaxImage(); max > 0 && fh.Size > max {
		return jsonErr(c, fiber.StatusRequestEntityTooLarge, "frame exceeds maximum size")
	}

	src, err := fh.Open()
	if err != nil {
		return jsonErr(c, fiber.StatusInternalServerError, "could not read frame")
	}
	defer src.Close()
	data, err := io.ReadAll(src)
	if err != nil {
		return jsonErr(c, fiber.StatusInternalServerError, "could not read frame")
	}

	ctype := fh.Header.Get("Content-Type")
	if ctype == "" || ctype == "application/octet-stream" {
		if byExt := mime.TypeByExtension(filepath.Ext(fh.Filename)); byExt != "" {
			ctype = byExt
		}
	}
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	if i := strings.IndexByte(ctype, ';'); i >= 0 {
		ctype = strings.TrimSpace(ctype[:i])
	}

	rev, ok := h.live.Put(c.Params("id"), fh.Filename, ctype, data)
	if !ok {
		return jsonErr(c, fiber.StatusNotFound, "live session not found")
	}
	return c.JSON(fiber.Map{"ok": true, "rev": rev})
}

// deleteLive ends a live session (idempotent).
func (h *Handlers) deleteLive(c fiber.Ctx) error {
	h.live.Delete(c.Params("id"))
	return c.JSON(fiber.Map{"ok": true})
}

// getLiveViewer serves the auto-refreshing viewer page.
func (h *Handlers) getLiveViewer(c fiber.Ctx) error {
	return c.SendFile("live.html", fiber.SendFile{FS: web.Assets})
}

// getLiveImage serves the current frame's bytes.
func (h *Handlers) getLiveImage(c fiber.Ctx) error {
	data, ctype, name, _, ok := h.live.Image(c.Params("id"))
	if !ok || len(data) == 0 {
		return c.Status(fiber.StatusNotFound).SendString("no frame yet")
	}
	c.Set("Cache-Control", "no-store")
	if ctype != "" {
		c.Set("Content-Type", ctype)
	}
	if name != "" {
		c.Set("Content-Disposition", "inline; filename=\""+sanitizeFilename(name)+"\"")
	}
	return c.Send(data)
}

// getLiveRev reports the current revision for lightweight polling.
func (h *Handlers) getLiveRev(c fiber.Ctx) error {
	rev, name, ctype, hasImage, ok := h.live.Meta(c.Params("id"))
	if !ok {
		return c.JSON(fiber.Map{"alive": false})
	}
	kind := "other"
	if strings.HasPrefix(ctype, "image/") {
		kind = "image"
	}
	return c.JSON(fiber.Map{
		"alive": true, "rev": rev, "name": name, "kind": kind, "has_image": hasImage,
	})
}
