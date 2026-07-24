// Package api is the Filez CLI's HTTP client to a Filez server.
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
	"strconv"
	"time"

	"github.com/DasDarki/filez/internal/client/config"
)

// Client talks to one Filez server.
type Client struct {
	BaseURL   string
	AccessKey string
	HTTP      *http.Client
}

// New builds a client for a base URL and optional access key.
func New(baseURL, accessKey string) *Client {
	return &Client{
		BaseURL:   baseURL,
		AccessKey: accessKey,
		HTTP:      &http.Client{Timeout: 0}, // uploads can be large; no overall timeout
	}
}

// FromHost builds a client from a configured host.
func FromHost(h *config.Host) *Client {
	return New(h.URL, h.AccessKey)
}

// Info is the /api/info response.
type Info struct {
	Filez         bool   `json:"filez"`
	Version       string `json:"version"`
	Public        bool   `json:"public"`
	AdminEnabled  bool   `json:"admin_enabled"`
	DefaultUpload string `json:"default_upload"`
	MaxUploadSize int64  `json:"max_upload_size"`
}

// Info fetches server metadata and verifies the host is a Filez instance.
func (c *Client) Info() (*Info, error) {
	req, _ := http.NewRequest(http.MethodGet, c.BaseURL+"/api/info", nil)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var info Info
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("not a Filez instance (invalid response)")
	}
	if !info.Filez {
		return nil, fmt.Errorf("not a Filez instance")
	}
	return &info, nil
}

// AuthCheck reports whether the client's access key is accepted (or the instance
// is public).
func (c *Client) AuthCheck() (bool, error) {
	req, _ := http.NewRequest(http.MethodGet, c.BaseURL+"/api/auth/check", nil)
	if c.AccessKey != "" {
		req.Header.Set("X-Access-Key", c.AccessKey)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return false, nil
	}
	return false, fmt.Errorf("unexpected status %d", resp.StatusCode)
}

// UploadOptions describe an upload request.
type UploadOptions struct {
	Mode      string // permanent | temp | limited
	TTL       string // for temp, e.g. "2d20m"
	Downloads int    // for limited
	Password  string // optional, any mode
	Keep      bool   // exempt from idle cleanup (needs permission on private)
}

// UploadResult is the /api/upload response.
type UploadResult struct {
	ID         string `json:"id"`
	Ext        string `json:"ext"`
	URL        string `json:"url"`
	PreviewURL string `json:"preview_url"`
}

// Upload streams a file to the server, reporting progress (bytes of the file
// sent) via the optional callback.
func (c *Client) Upload(path string, opts UploadOptions, progress func(sent, total int64)) (*UploadResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if stat.IsDir() {
		return nil, fmt.Errorf("%s is a directory", path)
	}
	total := stat.Size()

	// Build the multipart envelope up to (prefix) and after (suffix) the file
	// bytes so we can send an exact Content-Length instead of chunked encoding.
	// Chunked uploads leave a trailing terminator on keep-alive connections that
	// confuses the server; a fixed length avoids that entirely.
	var prefix bytes.Buffer
	mw := multipart.NewWriter(&prefix)
	boundary := mw.Boundary()

	fields := map[string]string{"mode": opts.Mode, "password": opts.Password}
	if opts.Mode == "temp" {
		fields["ttl"] = opts.TTL
	}
	if opts.Mode == "limited" {
		fields["downloads"] = strconv.Itoa(opts.Downloads)
	}
	if opts.Keep {
		fields["keep"] = "true"
	}
	for name, val := range fields {
		if val == "" {
			continue
		}
		if err := mw.WriteField(name, val); err != nil {
			return nil, err
		}
	}
	if _, err := mw.CreateFormFile("file", filepath.Base(path)); err != nil {
		return nil, err
	}
	// The closing boundary (mime/multipart writes exactly "\r\n--BOUNDARY--\r\n").
	suffix := []byte("\r\n--" + boundary + "--\r\n")
	contentLen := int64(prefix.Len()) + total + int64(len(suffix))

	body := io.MultiReader(
		bytes.NewReader(prefix.Bytes()),
		&countingReader{r: f, total: total, cb: progress},
		bytes.NewReader(suffix),
	)

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/upload", body)
	if err != nil {
		return nil, err
	}
	req.ContentLength = contentLen
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if c.AccessKey != "" {
		req.Header.Set("X-Access-Key", c.AccessKey)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if msg := errorMessage(respBody); msg != "" {
			return nil, fmt.Errorf("%s", msg)
		}
		return nil, fmt.Errorf("upload failed (status %d)", resp.StatusCode)
	}

	var res UploadResult
	if err := json.Unmarshal(respBody, &res); err != nil {
		return nil, fmt.Errorf("invalid server response")
	}
	return &res, nil
}

func errorMessage(body []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil {
		return e.Error
	}
	return ""
}

// countingReader reports cumulative bytes read via cb.
type countingReader struct {
	r     io.Reader
	sent  int64
	total int64
	cb    func(sent, total int64)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.sent += int64(n)
		if c.cb != nil {
			c.cb(c.sent, c.total)
		}
	}
	return n, err
}
