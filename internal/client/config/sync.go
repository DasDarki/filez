package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// SyncMarker records a sync bucket created from this machine so that
// `filez sync close` can close it. It lives next to config.json as .filez_sync.
type SyncMarker struct {
	Code       string `json:"code"`
	URL        string `json:"url"`         // host base URL
	OwnerToken string `json:"owner_token"` // secret allowing the bucket to be closed
	Host       string `json:"host"`
	ViewerURL  string `json:"viewer_url"`
}

func syncMarkerPath() (string, error) {
	cfgPath, err := Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(cfgPath), ".filez_sync"), nil
}

// ReadSyncMarker returns the last-created sync bucket, or nil if none.
func ReadSyncMarker() (*SyncMarker, error) {
	path, err := syncMarkerPath()
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
	var m SyncMarker
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// WriteSyncMarker records a created sync bucket (mode 0600 — it holds the token).
func WriteSyncMarker(m *SyncMarker) error {
	path, err := syncMarkerPath()
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

// RemoveSyncMarker clears the recorded sync bucket.
func RemoveSyncMarker() error {
	path, err := syncMarkerPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
