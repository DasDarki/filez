package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// ErrLiveGone means the live session no longer exists on the server.
var ErrLiveGone = errors.New("live session not found")

// LiveSession is the /api/live start response.
type LiveSession struct {
	SessionID string `json:"session_id"`
	ViewerURL string `json:"viewer_url"`
}

// LiveStart creates a new live session on the server.
func (c *Client) LiveStart() (*LiveSession, error) {
	req, _ := http.NewRequest(http.MethodPost, c.BaseURL+"/api/live", nil)
	if c.AccessKey != "" {
		req.Header.Set("X-Access-Key", c.AccessKey)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if msg := errorMessage(body); msg != "" {
			return nil, fmt.Errorf("%s", msg)
		}
		return nil, fmt.Errorf("could not start live session (status %d)", resp.StatusCode)
	}
	var s LiveSession
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, fmt.Errorf("invalid server response")
	}
	return &s, nil
}

// LivePush replaces the current frame of a live session with the given file.
// It returns ErrLiveGone if the session has ended server-side.
func (c *Client) LivePush(sessionID, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		return fmt.Errorf("cannot read %s", path)
	}

	var prefix bytes.Buffer
	mw := multipart.NewWriter(&prefix)
	boundary := mw.Boundary()
	if _, err := mw.CreateFormFile("file", filepath.Base(path)); err != nil {
		return err
	}
	suffix := []byte("\r\n--" + boundary + "--\r\n")
	contentLen := int64(prefix.Len()) + stat.Size() + int64(len(suffix))

	body := io.MultiReader(bytes.NewReader(prefix.Bytes()), f, bytes.NewReader(suffix))
	req, err := http.NewRequest(http.MethodPut, c.BaseURL+"/api/live/"+sessionID, body)
	if err != nil {
		return err
	}
	req.ContentLength = contentLen
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if c.AccessKey != "" {
		req.Header.Set("X-Access-Key", c.AccessKey)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return ErrLiveGone
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if msg := errorMessage(respBody); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return fmt.Errorf("live push failed (status %d)", resp.StatusCode)
	}
	return nil
}

// LiveStop ends a live session on the server (idempotent).
func (c *Client) LiveStop(sessionID string) error {
	req, _ := http.NewRequest(http.MethodDelete, c.BaseURL+"/api/live/"+sessionID, nil)
	if c.AccessKey != "" {
		req.Header.Set("X-Access-Key", c.AccessKey)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
