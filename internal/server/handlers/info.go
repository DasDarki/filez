package handlers

import (
	"github.com/DasDarki/filez/internal/server/auth"
	"github.com/gofiber/fiber/v3"
)

// getInfo advertises that this is a Filez instance and its public settings.
// Used by the CLI to detect a host and by the web gate.
func (h *Handlers) getInfo(c fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"filez":           true,
		"version":         h.version,
		"public":          h.cfg.Public,
		"admin_enabled":   h.cfg.AdminEnabled(),
		"default_upload":  h.cfg.DefaultUpload.Raw,
		"max_upload_size": h.cfg.MaxUploadSize,
		"cleanup":         h.cfg.CleanupEnabled,
		"cleanup_after":   h.cfg.CleanupRaw,
	})
}

// getAuthCheck validates an access key (or confirms open access on a public
// instance) and reports whether that key may create permanent files.
func (h *Handlers) getAuthCheck(c fiber.Ctx) error {
	key := auth.ExtractAccessKey(c)
	if h.guard.ValidKey(key) {
		allowPerm := false
		if ak, err := h.db.GetAccessKey(key); err == nil {
			allowPerm = ak.AllowPermanent
		}
		return c.JSON(fiber.Map{"ok": true, "public": h.cfg.Public, "allow_permanent": allowPerm})
	}
	if h.cfg.Public {
		// On a public instance permanent (kept) files are never allowed when
		// cleanup is active.
		return c.JSON(fiber.Map{"ok": true, "public": true, "allow_permanent": !h.cfg.CleanupEnabled})
	}
	return jsonErr(c, fiber.StatusUnauthorized, "invalid access key")
}
