package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// SyncBucket is the /api/sync create response.
type SyncBucket struct {
	Code       string `json:"code"`
	OwnerToken string `json:"owner_token"`
	URL        string `json:"url"`
}

// SyncCreate creates a new sync bucket on the server.
func (c *Client) SyncCreate() (*SyncBucket, error) {
	req, _ := http.NewRequest(http.MethodPost, c.BaseURL+"/api/sync", nil)
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
		return nil, fmt.Errorf("could not create sync bucket (status %d)", resp.StatusCode)
	}
	var b SyncBucket
	if err := json.Unmarshal(body, &b); err != nil {
		return nil, fmt.Errorf("invalid server response")
	}
	return &b, nil
}

// SyncClose closes a sync bucket using its owner token.
func (c *Client) SyncClose(code, ownerToken string) error {
	req, _ := http.NewRequest(http.MethodDelete, c.BaseURL+"/api/sync/"+code, nil)
	req.Header.Set("X-Sync-Owner", ownerToken)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		if msg := errorMessage(body); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return fmt.Errorf("could not close bucket (status %d)", resp.StatusCode)
	}
	return nil
}

// SyncUpload adds a file to a sync bucket (anyone with the code may do this).
func (c *Client) SyncUpload(code, path string) error {
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

	body := io.MultiReader(bytes.NewReader(prefix.Bytes()), f, bytes.NewReader(suffix))
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/sync/"+code, body)
	if err != nil {
		return err
	}
	req.ContentLength = int64(prefix.Len()) + stat.Size() + int64(len(suffix))
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if msg := errorMessage(respBody); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return fmt.Errorf("upload failed (status %d)", resp.StatusCode)
	}
	return nil
}
