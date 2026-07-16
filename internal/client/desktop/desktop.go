// Package desktop provides small OS-integration helpers used by the hook
// watcher: copying text to the clipboard and showing a desktop notification.
// It shells out to whatever tools are available (Wayland/X11/macOS).
package desktop

import (
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed icon.png
var iconPNG []byte

// IconPNG returns the embedded Filez icon (128×128 PNG), used for notifications
// and desktop/context-menu integration.
func IconPNG() []byte { return iconPNG }

// Clipboard copies text to the system clipboard, trying the available tools in
// order: wl-copy (Wayland), xclip, xsel (X11), pbcopy (macOS).
func Clipboard(text string) error {
	type tool struct {
		bin  string
		args []string
	}
	candidates := []tool{
		{"wl-copy", nil},
		{"xclip", []string{"-selection", "clipboard"}},
		{"xsel", []string{"--clipboard", "--input"}},
		{"pbcopy", nil},
	}
	var lastErr error
	for _, t := range candidates {
		if _, err := exec.LookPath(t.bin); err != nil {
			continue
		}
		cmd := exec.Command(t.bin, t.args...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return errNoClipboardTool
}

// HasClipboard reports whether any clipboard tool is available.
func HasClipboard() bool {
	for _, bin := range []string{"wl-copy", "xclip", "xsel", "pbcopy"} {
		if _, err := exec.LookPath(bin); err == nil {
			return true
		}
	}
	return false
}

// Notify shows a desktop notification via notify-send (libnotify). It is a
// no-op (returning nil) when notify-send is not installed.
func Notify(title, body string) error {
	bin, err := exec.LookPath("notify-send")
	if err != nil {
		return nil
	}
	args := []string{"--app-name=Filez", "--expire-time=5000"}
	if icon := iconPath(); icon != "" {
		args = append(args, "--icon="+icon)
	}
	args = append(args, title, body)
	return exec.Command(bin, args...).Run()
}

// iconPath writes the embedded icon to the user cache dir once and returns its
// path, so notifications carry the Filez logo. Returns "" on failure.
func iconPath() string {
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(cache, "filez")
	path := filepath.Join(dir, "icon.png")
	if fi, err := os.Stat(path); err == nil && fi.Size() == int64(len(iconPNG)) {
		return path
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	if err := os.WriteFile(path, iconPNG, 0o644); err != nil {
		return ""
	}
	return path
}

type clipboardError string

func (e clipboardError) Error() string { return string(e) }

const errNoClipboardTool = clipboardError("no clipboard tool found (install wl-clipboard, xclip or xsel)")
