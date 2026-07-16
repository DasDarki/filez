// Package auth implements the two Filez access layers as Fiber middleware:
//  1. instance access via access keys (only when the instance is not public)
//  2. the admin area via HTTP Basic Auth (username "admin")
//
// It also provides helpers to extract per-file passwords.
package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"strings"
	"time"

	"github.com/DasDarki/filez/internal/server/config"
	"github.com/DasDarki/filez/internal/server/db"
	"github.com/gofiber/fiber/v3"
)

// CookieName is where the web UI stores the access key for direct-link navigations.
const CookieName = "filez_key"

// Guard holds the dependencies shared by the auth middlewares.
type Guard struct {
	cfg *config.Config
	db  *db.DB
	now func() int64
}

// New builds a Guard.
func New(cfg *config.Config, database *db.DB) *Guard {
	return &Guard{cfg: cfg, db: database, now: func() int64 { return time.Now().Unix() }}
}

// ExtractAccessKey pulls an access key from (in order) the Authorization header
// ("Access-Key <key>" or Basic password), the X-Access-Key header, a cookie, or
// a query parameter.
func ExtractAccessKey(c fiber.Ctx) string {
	authz := c.Get("Authorization")
	if authz != "" {
		if k, ok := stripPrefixFold(authz, "Access-Key "); ok {
			return strings.TrimSpace(k)
		}
		if _, pass, ok := parseBasic(authz); ok && pass != "" {
			return pass
		}
	}
	if k := c.Get("X-Access-Key"); k != "" {
		return k
	}
	if k := c.Cookies(CookieName); k != "" {
		return k
	}
	return c.Query("key")
}

// ValidKey reports whether the given key currently grants instance access.
func (g *Guard) ValidKey(key string) bool {
	if key == "" {
		return false
	}
	k, err := g.db.GetAccessKey(key)
	if err != nil {
		return false
	}
	return k.Valid(g.now())
}

// InstanceAuth guards routes that require instance access when the instance is
// not public. Public instances pass everything through.
func (g *Guard) InstanceAuth() fiber.Handler {
	return func(c fiber.Ctx) error {
		if g.cfg.Public {
			return c.Next()
		}
		if g.ValidKey(ExtractAccessKey(c)) {
			return c.Next()
		}
		// Prompt browsers (direct /d and /p links) for Basic Auth credentials.
		c.Set("WWW-Authenticate", `Basic realm="Filez"`)
		return fiber.NewError(fiber.StatusUnauthorized, "access key required")
	}
}

// AdminAuth guards the admin area with HTTP Basic Auth (username must be "admin").
func (g *Guard) AdminAuth() fiber.Handler {
	return func(c fiber.Ctx) error {
		user, pass, ok := parseBasic(c.Get("Authorization"))
		if !ok || user != "admin" || !constEq(pass, g.cfg.AdminPassword) {
			c.Set("WWW-Authenticate", `Basic realm="Filez Admin"`)
			return fiber.NewError(fiber.StatusUnauthorized, "admin credentials required")
		}
		return c.Next()
	}
}

// FilePassword extracts a per-file password from ?pw=, the X-File-Password
// header, or (as a fallback) HTTP Basic Auth.
func FilePassword(c fiber.Ctx) string {
	if pw := c.Query("pw"); pw != "" {
		return pw
	}
	if pw := c.Get("X-File-Password"); pw != "" {
		return pw
	}
	if _, pass, ok := parseBasic(c.Get("Authorization")); ok {
		return pass
	}
	return ""
}

func stripPrefixFold(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):], true
	}
	return "", false
}

func parseBasic(authz string) (user, pass string, ok bool) {
	raw, ok := stripPrefixFold(authz, "Basic ")
	if !ok {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return "", "", false
	}
	user, pass, found := strings.Cut(string(decoded), ":")
	if !found {
		return "", "", false
	}
	return user, pass, true
}

func constEq(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
