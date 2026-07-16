// Package config loads Filez server configuration from the environment
// (optionally seeded from a .env file). Every value has a sensible default so
// the server runs out of the box as a public instance.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/DasDarki/filez/internal/timefmt"
	"github.com/joho/godotenv"
)

// DefaultUpload describes the upload mode a client should use when the user
// does not specify one. It is parsed from DEFAULT_UPLOAD, e.g. "permanent" or
// "temp:1d".
type DefaultUpload struct {
	Mode string        // "permanent" or "temp"
	TTL  time.Duration // only meaningful for temp
	Raw  string        // original string, echoed to CLI clients
}

// Config holds all server settings.
type Config struct {
	Port          int
	Public        bool
	AdminPassword string // empty => /admin disabled
	DataDir       string
	MaxUploadSize int64  // bytes
	CacheSize     int64  // bytes, ristretto max cost
	BaseURL       string // optional; if empty, derived from request
	TrustProxy    bool   // trust X-Forwarded-* from loopback/private proxies (e.g. Coolify/Traefik)
	PublicLinks   bool   // on a private instance, still serve /d and /p without an access key
	DefaultUpload DefaultUpload
}

// Load reads the .env file (if present) and then the environment.
func Load() (*Config, error) {
	// Best-effort: a missing .env is not an error.
	_ = godotenv.Load()

	cfg := &Config{
		Port:          envInt("PORT", 8080),
		Public:        envBool("PUBLIC", true),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
		DataDir:       envStr("DATA_DIR", "./data"),
		MaxUploadSize: envBytes("MAX_UPLOAD_SIZE", 1<<30), // 1 GiB
		CacheSize:     envBytes("CACHE_SIZE", 256<<20),    // 256 MiB
		BaseURL:       strings.TrimRight(os.Getenv("BASE_URL"), "/"),
		TrustProxy:    envBool("TRUST_PROXY", true),
		PublicLinks:   envBool("PUBLIC_LINKS", true),
	}

	du, err := parseDefaultUpload(envStr("DEFAULT_UPLOAD", "permanent"))
	if err != nil {
		return nil, err
	}
	cfg.DefaultUpload = du

	return cfg, nil
}

// AdminEnabled reports whether the /admin area and access-key management are active.
func (c *Config) AdminEnabled() bool { return c.AdminPassword != "" }

func parseDefaultUpload(raw string) (DefaultUpload, error) {
	raw = strings.TrimSpace(raw)
	du := DefaultUpload{Raw: raw}
	switch {
	case raw == "" || strings.EqualFold(raw, "permanent") || strings.EqualFold(raw, "perma"):
		du.Mode = "permanent"
		du.Raw = "permanent"
	case strings.HasPrefix(strings.ToLower(raw), "temp"):
		parts := strings.SplitN(raw, ":", 2)
		du.Mode = "temp"
		if len(parts) == 2 {
			d, err := timefmt.Parse(parts[1])
			if err != nil {
				return du, fmt.Errorf("DEFAULT_UPLOAD: %w", err)
			}
			du.TTL = d
		} else {
			du.TTL = 24 * time.Hour
			du.Raw = "temp:1d"
		}
	default:
		return du, fmt.Errorf("DEFAULT_UPLOAD: unknown mode %q (use 'permanent' or 'temp:1d')", raw)
	}
	return du, nil
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

// envBytes parses a size that may carry a unit suffix (KB, MB, GB, KiB, ...).
// A bare number is interpreted as bytes.
func envBytes(key string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := parseBytes(v)
	if err != nil {
		return def
	}
	return n
}

func parseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	upper := strings.ToUpper(s)
	mult := int64(1)
	for _, u := range []struct {
		suffix string
		size   int64
	}{
		{"GIB", 1 << 30}, {"MIB", 1 << 20}, {"KIB", 1 << 10},
		{"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10},
		{"G", 1 << 30}, {"M", 1 << 20}, {"K", 1 << 10}, {"B", 1},
	} {
		if strings.HasSuffix(upper, u.suffix) {
			mult = u.size
			s = strings.TrimSpace(s[:len(s)-len(u.suffix)])
			break
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return int64(f * float64(mult)), nil
}
