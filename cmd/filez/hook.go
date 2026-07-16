package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DasDarki/filez/internal/client/api"
	"github.com/DasDarki/filez/internal/client/config"
	"github.com/DasDarki/filez/internal/client/desktop"
	"github.com/DasDarki/filez/internal/client/ui"
	"github.com/DasDarki/filez/internal/timefmt"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
)

const serviceName = "filez-hook.service"

func newHookCmd() *cobra.Command {
	hook := &cobra.Command{
		Use:           "hook",
		Short:         "Auto-upload files dropped into a folder (e.g. screenshots)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	var (
		hHost string
		hTemp string
		hPerm bool
		hDl   int
	)
	add := &cobra.Command{
		Use:   "add <dir>",
		Short: "Watch a directory and auto-upload files that appear in it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHookAdd(args[0], hHost, hTemp, hPerm, hDl)
		},
	}
	af := add.Flags()
	af.StringVarP(&hHost, "host", "H", "", "host to upload to (base domain)")
	af.StringVarP(&hTemp, "temp", "t", "", "upload as temporary with this TTL (e.g. 30d)")
	af.BoolVarP(&hPerm, "permanent", "p", false, "upload as permanent")
	af.IntVarP(&hDl, "downloads", "d", 0, "upload as limited-download")
	af.SetNormalizeFunc(normalizeFlags)
	hook.AddCommand(add)

	hook.AddCommand(&cobra.Command{
		Use: "list", Short: "List configured hooks",
		RunE: func(cmd *cobra.Command, args []string) error { return runHookList() },
	})
	hook.AddCommand(&cobra.Command{
		Use: "remove [dir]", Short: "Remove a hook", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return runHookRemove(args) },
	})
	hook.AddCommand(&cobra.Command{
		Use: "watch", Short: "Run the upload watcher in the foreground",
		RunE: func(cmd *cobra.Command, args []string) error { return runHookWatch() },
	})
	hook.AddCommand(&cobra.Command{
		Use: "install", Short: "Install & start the watcher as a systemd user service (autostart)",
		RunE: func(cmd *cobra.Command, args []string) error { return runHookInstall() },
	})
	hook.AddCommand(&cobra.Command{
		Use: "uninstall", Short: "Stop & remove the systemd user service",
		RunE: func(cmd *cobra.Command, args []string) error { return runHookUninstall() },
	})
	return hook
}

func runHookAdd(dir, host, temp string, perm bool, dl int) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
		return fmt.Errorf("not a directory: %s", abs)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if host != "" {
		if cfg.Find(host) == nil {
			return fmt.Errorf("unknown host %q — configured: %s", host, hostNames(cfg))
		}
	} else if cfg.Primary() == nil {
		return fmt.Errorf("no host configured — run: filez config hosts add")
	}

	h := config.Hook{Dir: abs, Host: host}
	switch {
	case perm:
		h.Mode = "permanent"
	case temp != "":
		if _, e := timefmt.Parse(temp); e != nil {
			return fmt.Errorf("invalid --temp duration: %w", e)
		}
		h.Mode, h.TTL = "temp", temp
	case dl > 0:
		h.Mode, h.Downloads = "limited", dl
	}

	cfg.AddHook(h)
	if err := cfg.Save(); err != nil {
		return err
	}
	okLine("Watching " + ui.KeyStyle.Render(abs))
	info("Start now with: filez hook watch  ·  autostart at login: filez hook install")
	return nil
}

func runHookList() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(cfg.Hooks) == 0 {
		info("No hooks configured. Add one with: filez hook add <dir>")
		return nil
	}
	fmt.Println(ui.Title.Render("Configured hooks"))
	for i := range cfg.Hooks {
		h := &cfg.Hooks[i]
		host := h.Host
		if host == "" {
			host = "primary"
		}
		mode := h.Mode
		if mode == "" {
			mode = "default"
		} else if h.Mode == "temp" {
			mode = "temp:" + h.TTL
		} else if h.Mode == "limited" {
			mode = fmt.Sprintf("limited:%d", h.Downloads)
		}
		fmt.Printf("  %s  %s  %s\n", ui.Label.Render(h.Dir),
			ui.Subtle.Render("→ "+host), ui.Subtle.Render(mode))
	}
	return nil
}

func runHookRemove(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(cfg.Hooks) == 0 {
		info("No hooks configured.")
		return nil
	}
	dir := ""
	if len(args) > 0 {
		dir = args[0]
	} else {
		_ = runHookList()
		dir = ask("Directory to remove:")
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if !cfg.RemoveHook(dir) {
		return fmt.Errorf("no hook for %q", dir)
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	okLine("Removed hook " + dir)
	return nil
}

// runHookWatch is the long-running watcher daemon.
func runHookWatch() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(cfg.Hooks) == 0 {
		return fmt.Errorf("no hooks configured — add one with: filez hook add <dir>")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	dirHook := map[string]config.Hook{}
	for _, h := range cfg.Hooks {
		if fi, err := os.Stat(h.Dir); err != nil || !fi.IsDir() {
			failLine("skipping missing directory: " + h.Dir)
			continue
		}
		if err := watcher.Add(h.Dir); err != nil {
			failLine("cannot watch " + h.Dir + ": " + err.Error())
			continue
		}
		dirHook[filepath.Clean(h.Dir)] = h
		okLine("watching " + h.Dir)
	}
	if len(dirHook) == 0 {
		return fmt.Errorf("no valid directories to watch")
	}
	if !desktop.HasClipboard() {
		failLine("no clipboard tool found — links won't be copied (install wl-clipboard, xclip or xsel)")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var mu sync.Mutex
	timers := map[string]*time.Timer{}

	for {
		select {
		case <-ctx.Done():
			info("shutting down")
			return nil
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			failLine("watch error: " + err.Error())
		case ev, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if ev.Op&(fsnotify.Create|fsnotify.Write) == 0 {
				continue
			}
			if !isUploadCandidate(ev.Name) {
				continue
			}
			hook, ok := dirHook[filepath.Clean(filepath.Dir(ev.Name))]
			if !ok {
				continue
			}
			// Debounce: coalesce the burst of Create/Write events for one file,
			// then process once it stops changing.
			name := ev.Name
			mu.Lock()
			if t := timers[name]; t != nil {
				t.Stop()
			}
			timers[name] = time.AfterFunc(900*time.Millisecond, func() {
				mu.Lock()
				delete(timers, name)
				mu.Unlock()
				processHookFile(cfg, hook, name)
			})
			mu.Unlock()
		}
	}
}

// isUploadCandidate filters out hidden files and in-progress temp files.
func isUploadCandidate(path string) bool {
	base := filepath.Base(path)
	if strings.HasPrefix(base, ".") {
		return false
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".part", ".tmp", ".crdownload", ".swp", ".swx", ".ktmp":
		return false
	}
	return true
}

// processHookFile waits for the file to finish being written, uploads it, copies
// the link to the clipboard and shows a notification.
func processHookFile(cfg *config.Config, hook config.Hook, path string) {
	if !waitStable(path) {
		return // vanished or never stabilized
	}

	host := hook.Host
	var h *config.Host
	if host == "" {
		h = cfg.Primary()
	} else {
		h = cfg.Find(host)
	}
	if h == nil {
		failLine("no host for hook " + hook.Dir)
		_ = desktop.Notify("Filez — upload failed", "No host configured")
		return
	}

	name := filepath.Base(path)
	info("uploading " + name)
	res, err := api.FromHost(h).Upload(path, hookOptions(cfg, hook), nil)
	if err != nil {
		failLine("upload failed: " + err.Error())
		_ = desktop.Notify("Filez — upload failed", name+": "+err.Error())
		return
	}

	copied := ""
	if err := desktop.Clipboard(res.URL); err == nil {
		copied = "\n(link copied to clipboard)"
	}
	okLine(res.URL)
	_ = desktop.Notify("Filez — uploaded "+name, res.URL+copied)
}

// waitStable blocks until the file size stops changing (best effort).
func waitStable(path string) bool {
	var last int64 = -1
	for i := 0; i < 40; i++ {
		fi, err := os.Stat(path)
		if err != nil {
			return false
		}
		if fi.IsDir() {
			return false
		}
		if size := fi.Size(); size > 0 && size == last {
			return true
		} else {
			last = size
		}
		time.Sleep(250 * time.Millisecond)
	}
	return last > 0
}

func hookOptions(cfg *config.Config, hook config.Hook) api.UploadOptions {
	switch hook.Mode {
	case "temp":
		return api.UploadOptions{Mode: "temp", TTL: hook.TTL}
	case "limited":
		return api.UploadOptions{Mode: "limited", Downloads: hook.Downloads}
	case "permanent":
		return api.UploadOptions{Mode: "permanent"}
	}
	raw := os.Getenv("DEFAULT_UPLOAD")
	if raw == "" {
		raw = cfg.DefaultUpload
	}
	mode, ttl, dl, _ := parseDefault(raw)
	return api.UploadOptions{Mode: mode, TTL: ttl, Downloads: dl}
}

// ---- systemd user service ----

const unitTemplate = `[Unit]
Description=Filez auto-upload watcher
After=graphical-session.target

[Service]
Type=simple
ExecStart=%s hook watch
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
`

func unitPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfgDir, "systemd", "user", serviceName), nil
}

func runHookInstall() error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemctl not found; run 'filez hook watch' manually or add it to your desktop autostart")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	path, err := unitPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(fmt.Sprintf(unitTemplate, exe)), 0o644); err != nil {
		return err
	}

	// Make the graphical-session env (Wayland display, X display) available to
	// the user service so wl-copy and notify-send work.
	_ = run("systemctl", "--user", "import-environment", "WAYLAND_DISPLAY", "DISPLAY")
	if err := run("systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	if err := run("systemctl", "--user", "enable", "--now", serviceName); err != nil {
		return err
	}

	okLine("Installed and started " + serviceName)
	info("logs:   journalctl --user -u filez-hook -f")
	info("status: systemctl --user status filez-hook")
	cfg, _ := config.Load()
	if cfg == nil || len(cfg.Hooks) == 0 {
		info("No hooks yet — add your screenshot folder: filez hook add ~/Bilder")
	}
	return nil
}

func runHookUninstall() error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemctl not found")
	}
	_ = run("systemctl", "--user", "disable", "--now", serviceName)
	if path, err := unitPath(); err == nil {
		_ = os.Remove(path)
	}
	_ = run("systemctl", "--user", "daemon-reload")
	okLine("Removed " + serviceName)
	return nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
