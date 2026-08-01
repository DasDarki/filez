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
