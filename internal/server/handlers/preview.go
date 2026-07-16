package handlers

import (
	"archive/zip"
	"bytes"
	"strings"

	"github.com/DasDarki/filez/internal/server/db"
	"github.com/DasDarki/filez/internal/server/web"
	"github.com/gofiber/fiber/v3"
)

// getPreview renders the preview page for /p/<id>.<ext>.
func (h *Handlers) getPreview(c fiber.Ctx) error {
	f, err := h.files.Get(fileID(c))
	if err != nil {
		return h.fileError(c, err)
	}

	kind := classify(f.MIME, f.Ext)
	data := web.PreviewData{
		ID:          f.ID,
		Name:        dispName(f),
		Ext:         f.Ext,
		MIME:        f.MIME,
		Size:        f.Size,
		Kind:        kind,
		HasPassword: f.HasPassword(),
		Limited:     f.Mode == db.ModeLimited,
		DownloadURL: downloadPath(f),
	}

	// Only list archive contents when the file isn't password protected.
	if kind == "archive" && !f.HasPassword() {
		data.Entries = h.archiveEntries(f)
	}

	var buf bytes.Buffer
	if err := web.RenderPreview(&buf, data); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("render error")
	}
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.Send(buf.Bytes())
}

// archiveEntries lists files in a zip archive (best effort; other formats return nil).
func (h *Handlers) archiveEntries(f *db.File) []web.ArchiveEntry {
	if strings.ToLower(f.Ext) != "zip" && !strings.Contains(f.MIME, "zip") {
		return nil
	}
	file, err := h.files.Store().Open(f.StoragePath)
	if err != nil {
		return nil
	}
	defer file.Close()

	zr, err := zip.NewReader(file, f.Size)
	if err != nil {
		return nil
	}
	out := make([]web.ArchiveEntry, 0, len(zr.File))
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		out = append(out, web.ArchiveEntry{Name: zf.Name, Size: int64(zf.UncompressedSize64)})
		if len(out) >= 1000 {
			break
		}
	}
	return out
}
