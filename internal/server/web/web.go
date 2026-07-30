// Package web holds the embedded frontend: static HTML/CSS/JS, the locally
// hosted Inter font (GDPR — no external CDN), and the server-rendered preview
// template.
package web

import (
	"embed"
	"encoding/json"
	"html/template"
	"io"
)

//go:embed index.html admin.html preview.html live.html assets
var Assets embed.FS

// Brand is the footer credit shown across the UI.
const Brand = "Filez — made with ♥ by DasDarki (github.com/DasDarki)"

var previewTmpl = template.Must(template.ParseFS(Assets, "preview.html"))

// ArchiveEntry is one file inside a previewed archive.
type ArchiveEntry struct {
	Name string
	Size int64
}

// PreviewData is the model for the preview page.
type PreviewData struct {
	ID          string
	Name        string
	Ext         string
	MIME        string
	Size        int64
	Kind        string // text | image | audio | video | pdf | archive | binary
	HasPassword bool
	Limited     bool
	DownloadURL string
	Entries     []ArchiveEntry
	EntriesJSON string
	Brand       string
}

// RenderPreview writes the preview page for data to w.
func RenderPreview(w io.Writer, data PreviewData) error {
	data.Brand = Brand
	if len(data.Entries) > 0 {
		if b, err := json.Marshal(data.Entries); err == nil {
			data.EntriesJSON = string(b)
		}
	}
	if data.EntriesJSON == "" {
		data.EntriesJSON = "[]"
	}
	return previewTmpl.Execute(w, data)
}
