package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DasDarki/filez/internal/client/api"
	"github.com/DasDarki/filez/internal/client/config"
	"github.com/DasDarki/filez/internal/client/desktop"
	"github.com/DasDarki/filez/internal/client/ui"
	"github.com/spf13/cobra"
)

func newSyncCmd() *cobra.Command {
	var host string
	sync := &cobra.Command{
		Use:   "sync",
		Short: "Create a temporary sync bucket — a shared drop with a 4-digit code",
		Long: "Create a temporary, in-memory sync bucket. Anyone with the link can upload,\n" +
			"view and download files; only you (the creator) can close it. Run without a\n" +
			"subcommand to create one; 'filez sync close' closes it.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(cmd *cobra.Command, args []string) error { return runSyncCreate(host) },
	}
	sync.Flags().StringVarP(&host, "host", "H", "", "host for the bucket (base domain)")
	sync.Flags().SetNormalizeFunc(normalizeFlags)

	sync.AddCommand(&cobra.Command{
		Use: "close", Short: "Close the sync bucket you created",
		RunE: func(cmd *cobra.Command, args []string) error { return runSyncClose() },
	})
	sync.AddCommand(&cobra.Command{
		Use: "add <file>...", Short: "Upload files to the sync bucket you created",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return runSyncAdd(args) },
	})

	var dlHost string
	dl := &cobra.Command{
		Use:     "download [code]",
		Aliases: []string{"dl"},
		Short:   "Download all files from a bucket into the current folder",
		Long: "Download every file of a sync bucket into the current directory. Pass a code\n" +
			"to download that bucket, or omit it to use the bucket you created locally.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			code := ""
			if len(args) > 0 {
				code = args[0]
			}
			return runSyncDownload(code, dlHost)
		},
	}
	dl.Flags().StringVarP(&dlHost, "host", "H", "", "host to download from (when passing a code)")
	dl.Flags().SetNormalizeFunc(normalizeFlags)
	sync.AddCommand(dl)
	return sync
}

func runSyncCreate(host string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	h, err := liveHost(cfg, host)
	if err != nil {
		return err
	}

	b, err := api.FromHost(h).SyncCreate()
	if err != nil {
		return err
	}
	marker := &config.SyncMarker{Code: b.Code, URL: h.URL, OwnerToken: b.OwnerToken, Host: h.Name, ViewerURL: b.URL}
	if err := config.WriteSyncMarker(marker); err != nil {
		return err
	}

	fmt.Println(ui.Logo())
	okLine("Sync bucket created on " + h.Name)
	fmt.Println("  " + ui.Label.Render("Code: ") + ui.KeyStyle.Render(b.Code))
	fmt.Println("  " + ui.Label.Render("URL:  ") + ui.KeyStyle.Render(b.URL))
	if err := desktop.Clipboard(b.URL); err == nil {
		info("Link copied to clipboard")
	}
	info("Anyone with the link can upload & download. Close it with 'filez sync close'.")
	return nil
}

func runSyncClose() error {
	m, _ := config.ReadSyncMarker()
	if m == nil {
		return fmt.Errorf("no sync bucket to close — create one with 'filez sync'")
	}
	err := api.New(m.URL, "").SyncClose(m.Code, m.OwnerToken)
	_ = config.RemoveSyncMarker()
	if err != nil {
		info("Bucket " + m.Code + " was already closed or expired.")
		return nil
	}
	okLine("Sync bucket " + m.Code + " closed")
	return nil
}

func runSyncDownload(code, host string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	var baseURL, bucketCode string
	if code != "" {
		h, err := liveHost(cfg, host)
		if err != nil {
			return err
		}
		baseURL, bucketCode = h.URL, code
	} else {
		m, _ := config.ReadSyncMarker()
		if m == nil {
			return fmt.Errorf("no local sync bucket — pass a code: filez sync download <code>")
		}
		baseURL, bucketCode = m.URL, m.Code
	}

	client := api.New(baseURL, "")
	files, err := client.SyncList(bucketCode)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		info("Bucket " + bucketCode + " is empty.")
		return nil
	}

	cwd, _ := os.Getwd()
	fmt.Println(ui.Logo())
	info(fmt.Sprintf("Downloading %d file(s) from bucket %s into %s", len(files), bucketCode, cwd))

	downloaded := 0
	for _, f := range files {
		dest := uniqueLocalPath(safeName(f.Name, f.ID))
		n, err := client.SyncDownload(bucketCode, f.ID, dest)
		if err != nil {
			failLine(f.Name + ": " + err.Error())
			continue
		}
		okLine("↓ " + filepath.Base(dest) + "  " + ui.Subtle.Render(ui.HumanBytes(n)))
		downloaded++
	}
	okLine(fmt.Sprintf("Done — %d/%d file(s) downloaded", downloaded, len(files)))
	return nil
}

// safeName strips any path components so a bucket file can only land in the
// current directory (never escape it).
func safeName(name, fallback string) string {
	base := filepath.Base(filepath.FromSlash(name))
	if base == "" || base == "." || base == ".." || base == string(filepath.Separator) {
		return fallback
	}
	return base
}

// uniqueLocalPath avoids overwriting an existing local file by adding a suffix.
func uniqueLocalPath(name string) string {
	if _, err := os.Stat(name); os.IsNotExist(err) {
		return name
	}
	ext := filepath.Ext(name)
	base := name[:len(name)-len(ext)]
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func runSyncAdd(files []string) error {
	m, _ := config.ReadSyncMarker()
	if m == nil {
		return fmt.Errorf("no active sync bucket — create one with 'filez sync'")
	}
	client := api.New(m.URL, "")
	uploaded := 0
	for _, f := range files {
		if fi, err := os.Stat(f); err != nil || fi.IsDir() {
			failLine("skipping " + f)
			continue
		}
		if err := client.SyncUpload(m.Code, f); err != nil {
			failLine(filepath.Base(f) + ": " + err.Error())
			continue
		}
		okLine("added " + filepath.Base(f))
		uploaded++
	}
	if uploaded > 0 {
		info("View them at " + m.ViewerURL)
	}
	return nil
}
