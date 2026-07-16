// Package config manages the Filez CLI's on-disk configuration: known hosts
// (with their access keys), whether to ask for a host each time, and the
// default upload mode. It lives at $XDG_CONFIG_HOME/filez/config.json.
package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Host is a configured Filez server.
type Host struct {
	Name      string `json:"name"`       // base domain, e.g. files.example.com
	URL       string `json:"url"`        // full base URL, e.g. https://files.example.com
	AccessKey string `json:"access_key"` // empty for public instances
	Primary   bool   `json:"primary"`
}

// Hook watches a directory and auto-uploads files that appear in it.
type Hook struct {
	Dir       string `json:"dir"`                 // absolute directory to watch
	Host      string `json:"host,omitempty"`      // host name; empty = primary
	Mode      string `json:"mode,omitempty"`      // permanent|temp|limited; empty = default
	TTL       string `json:"ttl,omitempty"`       // for temp mode, e.g. "30d"
	Downloads int    `json:"downloads,omitempty"` // for limited mode
}

// Config is the whole CLI configuration.
type Config struct {
	Hosts         []Host `json:"hosts"`
	AskHost       bool   `json:"ask_host"`
	DefaultUpload string `json:"default_upload"` // "permanent" or "temp:1d"; empty = permanent
	Hooks         []Hook `json:"hooks,omitempty"`
}

// FindHook returns the hook for a directory, or nil.
func (c *Config) FindHook(dir string) *Hook {
	for i := range c.Hooks {
		if c.Hooks[i].Dir == dir {
			return &c.Hooks[i]
		}
	}
	return nil
}

// AddHook adds or replaces a hook by directory.
func (c *Config) AddHook(h Hook) {
	if existing := c.FindHook(h.Dir); existing != nil {
		*existing = h
		return
	}
	c.Hooks = append(c.Hooks, h)
}

// RemoveHook deletes a hook by directory, returning whether it existed.
func (c *Config) RemoveHook(dir string) bool {
	for i := range c.Hooks {
		if c.Hooks[i].Dir == dir {
			c.Hooks = append(c.Hooks[:i], c.Hooks[i+1:]...)
			return true
		}
	}
	return false
}

// Path returns the config file path, creating parent dirs on Save.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "filez", "config.json"), nil
}

// Load reads the config, returning an empty config if the file does not exist.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return &c, nil
}

// Save writes the config with 0600 permissions (it holds access keys).
func (c *Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Primary returns the primary host, or the only host, or nil.
func (c *Config) Primary() *Host {
	for i := range c.Hosts {
		if c.Hosts[i].Primary {
			return &c.Hosts[i]
		}
	}
	if len(c.Hosts) == 1 {
		return &c.Hosts[0]
	}
	return nil
}

// Find returns the host with the given name (base domain), or nil.
func (c *Config) Find(name string) *Host {
	name = strings.ToLower(name)
	for i := range c.Hosts {
		if strings.ToLower(c.Hosts[i].Name) == name {
			return &c.Hosts[i]
		}
	}
	return nil
}

// Upsert adds or replaces a host by name. If host.Primary is true, all others
// are demoted. If it's the first host, it becomes primary automatically.
func (c *Config) Upsert(host Host) {
	if len(c.Hosts) == 0 {
		host.Primary = true
	}
	if host.Primary {
		for i := range c.Hosts {
			c.Hosts[i].Primary = false
		}
	}
	if existing := c.Find(host.Name); existing != nil {
		*existing = host
		return
	}
	c.Hosts = append(c.Hosts, host)
}

// Remove deletes a host by name, promoting another to primary if needed.
func (c *Config) Remove(name string) bool {
	name = strings.ToLower(name)
	for i := range c.Hosts {
		if strings.ToLower(c.Hosts[i].Name) == name {
			wasPrimary := c.Hosts[i].Primary
			c.Hosts = append(c.Hosts[:i], c.Hosts[i+1:]...)
			if wasPrimary && len(c.Hosts) > 0 {
				c.Hosts[0].Primary = true
			}
			return true
		}
	}
	return false
}

// SetPrimary marks the named host as primary.
func (c *Config) SetPrimary(name string) bool {
	h := c.Find(name)
	if h == nil {
		return false
	}
	for i := range c.Hosts {
		c.Hosts[i].Primary = false
	}
	h.Primary = true
	return true
}

// NormalizeURL turns user input like "files.example.com" or "http://host:8099"
// into (baseURL, name). Scheme defaults to https.
func NormalizeURL(input string) (baseURL, name string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", fmt.Errorf("empty host")
	}
	if !strings.Contains(input, "://") {
		input = "https://" + input
	}
	u, err := url.Parse(input)
	if err != nil {
		return "", "", err
	}
	if u.Host == "" {
		return "", "", fmt.Errorf("invalid host")
	}
	base := u.Scheme + "://" + u.Host
	return strings.TrimRight(base, "/"), u.Hostname(), nil
}
