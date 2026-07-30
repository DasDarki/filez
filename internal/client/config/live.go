package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// LiveMarker records the active live session so every filez process (uploads and
// the hook watcher) streams into it instead of creating links. It lives next to
// config.json as .filez_live.
type LiveMarker struct {
	SessionID string `json:"session_id"`
	URL       string `json:"url"`        // host base URL to push frames to
	AccessKey string `json:"access_key"` // may be empty for public instances
	Host      string `json:"host"`       // host name, for display
	ViewerURL string `json:"viewer_url"`
}

func liveMarkerPath() (string, error) {
	cfgPath, err := Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(cfgPath), ".filez_live"), nil
}

// ReadLiveMarker returns the active live session, or nil if none.
func ReadLiveMarker() (*LiveMarker, error) {
	path, err := liveMarkerPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var m LiveMarker
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// WriteLiveMarker starts a live session marker (mode 0600 — it holds the key).
func WriteLiveMarker(m *LiveMarker) error {
	path, err := liveMarkerPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// RemoveLiveMarker ends the local live session marker.
func RemoveLiveMarker() error {
	path, err := liveMarkerPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
