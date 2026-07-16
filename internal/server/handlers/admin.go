package handlers

import (
	"encoding/json"
	"time"

	"github.com/DasDarki/filez/internal/server/db"
	"github.com/DasDarki/filez/internal/server/idgen"
	"github.com/DasDarki/filez/internal/timefmt"
	"github.com/gofiber/fiber/v3"
)

// listKeys returns all access keys as JSON.
func (h *Handlers) listKeys(c fiber.Ctx) error {
	keys, err := h.db.ListAccessKeys()
	if err != nil {
		return jsonErr(c, fiber.StatusInternalServerError, "could not list keys")
	}
	out := make([]fiber.Map, 0, len(keys))
	for _, k := range keys {
		out = append(out, keyJSON(k))
	}
	return c.JSON(out)
}

// createKey creates a new access key with an optional label and expiry.
func (h *Handlers) createKey(c fiber.Ctx) error {
	var req struct {
		Label  string `json:"label"`
		Expiry string `json:"expiry"`
	}
	if body := c.Body(); len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			return jsonErr(c, fiber.StatusBadRequest, "invalid JSON")
		}
	}

	now := time.Now().Unix()
	k := &db.AccessKey{
		Key:       idgen.NewKey(),
		Label:     req.Label,
		CreatedAt: now,
	}
	if req.Expiry != "" {
		d, err := timefmt.Parse(req.Expiry)
		if err != nil {
			return jsonErr(c, fiber.StatusBadRequest, "invalid expiry: "+err.Error())
		}
		exp := now + int64(d.Seconds())
		k.ExpiresAt = &exp
	}

	if err := h.db.InsertAccessKey(k); err != nil {
		return jsonErr(c, fiber.StatusInternalServerError, "could not create key")
	}
	return c.Status(fiber.StatusCreated).JSON(keyJSON(k))
}

// deleteKey removes an access key.
func (h *Handlers) deleteKey(c fiber.Ctx) error {
	if err := h.db.DeleteAccessKey(c.Params("key")); err != nil {
		return jsonErr(c, fiber.StatusInternalServerError, "could not delete key")
	}
	return c.JSON(fiber.Map{"ok": true})
}

func keyJSON(k *db.AccessKey) fiber.Map {
	return fiber.Map{
		"key":        k.Key,
		"label":      k.Label,
		"expires_at": k.ExpiresAt,
		"revoked":    k.Revoked,
		"created_at": k.CreatedAt,
	}
}
