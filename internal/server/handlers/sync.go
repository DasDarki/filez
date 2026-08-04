package handlers

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"

	"github.com/DasDarki/filez/internal/server/bucket"
	"github.com/gofiber/fiber/v3"
)

// postBucketCreate creates a new sync bucket (requires the access key).
func (h *Handlers) postBucketCreate(c fiber.Ctx) error {
	b, err := h.buckets.Create()
	if err != nil {
		return jsonErr(c, fiber.StatusServiceUnavailable, "no free bucket code, try again")
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"code":        b.Code,
		"owner_token": b.OwnerToken,
		"url":         h.baseURL(c) + "/s/" + b.Code,
	})
}

// getBucketPage serves the shared bucket UI.
func (h *Handlers) getBucketPage(c fiber.Ctx) error {
	return h.serveEmbedded(c, "bucket.html")
}

// getBucketList returns the files currently in a bucket.
func (h *Handlers) getBucketList(c fiber.Ctx) error {
	files, ok := h.buckets.List(c.Params("code"))
	if !ok {
		return c.JSON(fiber.Map{"alive": false})
	}
	out := make([]fiber.Map, 0, len(files))
	for _, f := range files {
		out = append(out, fiber.Map{
			"id": f.ID, "name": f.Name, "size": f.Size, "mime": f.MIME, "uploaded_at": f.UploadedAt,
		})
	}
	return c.JSON(fiber.Map{"alive": true, "code": c.Params("code"), "files": out})
}

// postBucketUpload adds a file to a bucket (open to anyone with the code).
func (h *Handlers) postBucketUpload(c fiber.Ctx) error {
	fh, err := c.FormFile("file")
	if err != nil {
		return jsonErr(c, fiber.StatusBadRequest, "no file provided")
	}
	if max := h.buckets.MaxFile(); max > 0 && fh.Size > max {
		return jsonErr(c, fiber.StatusRequestEntityTooLarge, "file too large")
	}

	src, err := fh.Open()
	if err != nil {
		return jsonErr(c, fiber.StatusInternalServerError, "could not read file")
	}
	defer src.Close()
	data, err := io.ReadAll(src)
	if err != nil {
		return jsonErr(c, fiber.StatusInternalServerError, "could not read file")
	}

	ctype := fh.Header.Get("Content-Type")
	if ctype == "" || ctype == "application/octet-stream" {
		if byExt := mime.TypeByExtension(filepath.Ext(fh.Filename)); byExt != "" {
			ctype = byExt
		}
	}
	if i := strings.IndexByte(ctype, ';'); i >= 0 {
		ctype = strings.TrimSpace(ctype[:i])
	}
	if ctype == "" {
		ctype = "application/octet-stream"
	}

	f, err := h.buckets.Add(c.Params("code"), fh.Filename, ctype, data)
	if err != nil {
		switch {
		case errors.Is(err, bucket.ErrNotFound):
			return jsonErr(c, fiber.StatusNotFound, "bucket closed or unknown")
		case errors.Is(err, bucket.ErrTooLarge):
			return jsonErr(c, fiber.StatusRequestEntityTooLarge, "file too large")
		case errors.Is(err, bucket.ErrFull):
			return jsonErr(c, fiber.StatusConflict, "bucket is full")
		default:
			return jsonErr(c, fiber.StatusInternalServerError, "upload failed")
		}
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id": f.ID, "name": f.Name, "size": f.Size, "mime": f.MIME, "uploaded_at": f.UploadedAt,
	})
}

// getBucketZip streams all files in a bucket as a single zip download.
func (h *Handlers) getBucketZip(c fiber.Ctx) error {
	code := c.Params("code")
	files, ok := h.buckets.Snapshot(code)
	if !ok || len(files) == 0 {
		return c.Status(fiber.StatusNotFound).SendString("nothing to download")
	}

	c.Set("Content-Type", "application/zip")
	c.Set("Content-Disposition", `attachment; filename="filez-`+sanitizeFilename(code)+`.zip"`)

	pr, pw := io.Pipe()
	go func() {
		zw := zip.NewWriter(pw)
		seen := map[string]int{}
		for _, f := range files {
			w, err := zw.Create(zipEntryName(f.Name, seen))
			if err != nil {
				_ = pw.CloseWithError(err)
				return
			}
			if _, err := w.Write(f.Data); err != nil {
				_ = pw.CloseWithError(err)
				return
			}
		}
		if err := zw.Close(); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		_ = pw.Close()
	}()
	return c.SendStream(pr)
}

// zipEntryName deduplicates repeated filenames within the archive.
func zipEntryName(name string, seen map[string]int) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" {
		name = "file"
	}
	n := seen[name]
	seen[name]++
	if n == 0 {
		return name
	}
	ext := filepath.Ext(name)
	return fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(name, ext), n, ext)
}

// getBucketFile downloads a file from a bucket.
func (h *Handlers) getBucketFile(c fiber.Ctx) error {
	data, ctype, name, ok := h.buckets.FileData(c.Params("code"), c.Params("fileid"))
	if !ok {
		return c.Status(fiber.StatusNotFound).SendString("not found")
	}
	if ctype != "" {
		c.Set("Content-Type", ctype)
	}
	c.Set("Content-Disposition", "inline; filename=\""+sanitizeFilename(name)+"\"")
	return c.Send(data)
}

// deleteBucket closes a bucket; only the owner token may do so.
func (h *Handlers) deleteBucket(c fiber.Ctx) error {
	token := c.Get("X-Sync-Owner")
	if token == "" {
		token = c.Query("owner")
	}
	if !h.buckets.Close(c.Params("code"), token) {
		return jsonErr(c, fiber.StatusForbidden, "not allowed (only the creator can close this bucket)")
	}
	return c.JSON(fiber.Map{"ok": true})
}
