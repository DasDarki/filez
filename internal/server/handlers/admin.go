package handlers

import (
	"encoding/json"
	"strings"
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
		Label          string `json:"label"`
		Expiry         string `json:"expiry"`
		AllowPermanent bool   `json:"allow_permanent"`
	}
	if body := c.Body(); len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			return jsonErr(c, fiber.StatusBadRequest, "invalid JSON")
		}
	}

	now := time.Now().Unix()
	k := &db.AccessKey{
		Key:            idgen.NewKey(),
		Label:          req.Label,
		CreatedAt:      now,
		AllowPermanent: req.AllowPermanent,
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

// updateKey edits a key's label, permanent permission and (optionally) expiry.
// Fields are partial: an omitted field is left unchanged. For expiry, "" is
// ignored (unchanged) while "never"/"nie"/"-" clears it.
func (h *Handlers) updateKey(c fiber.Ctx) error {
	k, err := h.db.GetAccessKey(c.Params("key"))
	if err != nil {
		return jsonErr(c, fiber.StatusNotFound, "key not found")
	}

	var req struct {
		Label          *string `json:"label"`
		AllowPermanent *bool   `json:"allow_permanent"`
		Expiry         *string `json:"expiry"`
	}
	if body := c.Body(); len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			return jsonErr(c, fiber.StatusBadRequest, "invalid JSON")
		}
	}

	if req.Label != nil {
		k.Label = *req.Label
	}
	if req.AllowPermanent != nil {
		k.AllowPermanent = *req.AllowPermanent
	}
	if req.Expiry != nil {
		switch v := strings.TrimSpace(*req.Expiry); strings.ToLower(v) {
		case "":
			// keep current expiry unchanged
		case "never", "nie", "-", "0", "off":
			k.ExpiresAt = nil
		default:
			d, err := timefmt.Parse(v)
			if err != nil {
				return jsonErr(c, fiber.StatusBadRequest, "invalid expiry: "+err.Error())
			}
			exp := time.Now().Unix() + int64(d.Seconds())
			k.ExpiresAt = &exp
		}
	}

	if err := h.db.UpdateAccessKey(k.Key, k.Label, k.ExpiresAt, k.AllowPermanent); err != nil {
		return jsonErr(c, fiber.StatusInternalServerError, "could not update key")
	}
	return c.JSON(keyJSON(k))
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
		"key":             k.Key,
		"label":           k.Label,
		"expires_at":      k.ExpiresAt,
		"revoked":         k.Revoked,
		"created_at":      k.CreatedAt,
		"allow_permanent": k.AllowPermanent,
	}
}
