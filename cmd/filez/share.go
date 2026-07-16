package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DasDarki/filez/internal/client/api"
	"github.com/DasDarki/filez/internal/client/config"
	"github.com/DasDarki/filez/internal/client/desktop"
	"github.com/DasDarki/filez/internal/client/ui"
	"github.com/spf13/cobra"
)

// ---- filez share (the action the KDE context menu calls) ----

func newShareCmd() *cobra.Command {
	var host string
	cmd := &cobra.Command{
		Use:           "share <file>...",
		Short:         "Upload files, copy the link(s) to the clipboard and notify (used by the KDE menu)",
		Args:          cobra.MinimumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShare(args, host)
		},
	}
	cmd.Flags().StringVarP(&host, "host", "H", "", "host to upload to (base domain)")
	cmd.Flags().SetNormalizeFunc(normalizeFlags)
	return cmd
}

func runShare(files []string, host string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	var h *config.Host
	if host != "" {
		h = cfg.Find(host)
	} else {
		h = cfg.Primary()
	}
	if h == nil {
		_ = desktop.Notify("Filez — cannot upload", "No host configured. Run: filez config hosts add")
		return fmt.Errorf("no host configured — run: filez config hosts add")
	}

	client := api.FromHost(h)
	opts := defaultOptions(cfg)

	var urls []string
	var firstErr error
	for _, f := range files {
		if fi, err := os.Stat(f); err != nil || fi.IsDir() {
			continue // skip directories and unreadable entries
		}
		res, err := client.Upload(f, opts, nil)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			failLine(filepath.Base(f) + ": " + err.Error())
			continue
		}
		urls = append(urls, res.URL)
		okLine(res.URL)
	}

	if len(urls) == 0 {
		msg := "Upload failed"
		if firstErr != nil {
			msg = firstErr.Error()
		}
		_ = desktop.Notify("Filez — upload failed", msg)
		return fmt.Errorf("nothing uploaded: %s", msg)
	}

	joined := strings.Join(urls, "\n")
	copied := ""
	if err := desktop.Clipboard(joined); err == nil {
		copied = "\n(copied to clipboard)"
	}
	if len(urls) == 1 {
		_ = desktop.Notify("Filez — uploaded "+filepath.Base(files[0]), urls[0]+copied)
	} else {
		_ = desktop.Notify(fmt.Sprintf("Filez — uploaded %d files", len(urls)), joined+copied)
	}
	return nil
}

// defaultOptions builds upload options from the CLI's default upload mode.
func defaultOptions(cfg *config.Config) api.UploadOptions {
	raw := os.Getenv("DEFAULT_UPLOAD")
	if raw == "" {
		raw = cfg.DefaultUpload
	}
	mode, ttl, dl, _ := parseDefault(raw)
	return api.UploadOptions{Mode: mode, TTL: ttl, Downloads: dl}
}

// ---- filez menu (KDE Plasma / Dolphin service menu) ----

const serviceMenuFile = "filez-share.desktop"

const serviceMenuTemplate = `[Desktop Entry]
Type=Service
MimeType=all/allfiles;
Actions=filezShare;
X-KDE-Priority=TopLevel
Icon=%[1]s

[Desktop Action filezShare]
Name=Share with Filez
Icon=%[1]s
Exec=%[2]s share %%F
`

func newMenuCmd() *cobra.Command {
	menu := &cobra.Command{
		Use:           "menu",
		Short:         "Install a 'Share with Filez' right-click menu in KDE Dolphin",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	menu.AddCommand(&cobra.Command{
		Use: "install", Short: "Add the 'Share with Filez' context-menu entry",
		RunE: func(cmd *cobra.Command, args []string) error { return runMenuInstall() },
	})
	menu.AddCommand(&cobra.Command{
		Use: "uninstall", Short: "Remove the context-menu entry",
		RunE: func(cmd *cobra.Command, args []string) error { return runMenuUninstall() },
	})
	return menu
}

func xdgDataHome() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share")
}

func iconInstallPath() string {
	return filepath.Join(xdgDataHome(), "icons", "hicolor", "128x128", "apps", "filez.png")
}

func serviceMenuPath() string {
	return filepath.Join(xdgDataHome(), "kio", "servicemenus", serviceMenuFile)
}

func runMenuInstall() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	// Extract the embedded icon into the hicolor icon theme.
	iconPath := iconInstallPath()
	if err := os.MkdirAll(filepath.Dir(iconPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(iconPath, desktop.IconPNG(), 0o644); err != nil {
		return err
	}

	// Write the Dolphin service menu. Plasma 6 requires it to be executable.
	menuPath := serviceMenuPath()
	if err := os.MkdirAll(filepath.Dir(menuPath), 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf(serviceMenuTemplate, iconPath, exe)
	if err := os.WriteFile(menuPath, []byte(content), 0o755); err != nil {
		return err
	}

	// Best-effort cache refresh so Dolphin and the icon show up immediately.
	refreshKDECaches()

	okLine("Installed context menu: right-click a file → " + ui.KeyStyle.Render("Share with Filez"))
	info("menu:  " + menuPath)
	info("icon:  " + iconPath)
	info("If it doesn't appear yet, restart Dolphin (or log out and back in).")
	return nil
}

func runMenuUninstall() error {
	removed := false
	for _, p := range []string{serviceMenuPath(), iconInstallPath()} {
		if err := os.Remove(p); err == nil {
			removed = true
		}
	}
	refreshKDECaches()
	if removed {
		okLine("Removed the 'Share with Filez' context menu")
	} else {
		info("Nothing to remove.")
	}
	return nil
}

// refreshKDECaches rebuilds the KDE service cache and icon cache if the tools
// are present (best effort — failures are ignored).
func refreshKDECaches() {
	for _, bin := range []string{"kbuildsycoca6", "kbuildsycoca5"} {
		if _, err := exec.LookPath(bin); err == nil {
			_ = exec.Command(bin).Run()
			break
		}
	}
	if _, err := exec.LookPath("gtk-update-icon-cache"); err == nil {
		_ = exec.Command("gtk-update-icon-cache", "-q", "-t",
			filepath.Join(xdgDataHome(), "icons", "hicolor")).Run()
	}
}
