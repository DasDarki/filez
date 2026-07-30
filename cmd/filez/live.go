package main

import (
	"errors"
	"fmt"

	"github.com/DasDarki/filez/internal/client/api"
	"github.com/DasDarki/filez/internal/client/config"
	"github.com/DasDarki/filez/internal/client/desktop"
	"github.com/DasDarki/filez/internal/client/ui"
	"github.com/spf13/cobra"
)

func newLiveCmd() *cobra.Command {
	var host string
	cmd := &cobra.Command{
		Use:   "live",
		Short: "Toggle a live session — uploads stream to a live view instead of creating links",
		Long: "Start or stop a live session.\n\n" +
			"While active, every upload (filez <file>, filez share, and the screenshot hook)\n" +
			"streams into the session instead of creating a link. Open the viewer URL on a\n" +
			"screen and it updates live with the most recently uploaded image.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(cmd *cobra.Command, args []string) error { return runLive(host) },
	}
	cmd.Flags().StringVarP(&host, "host", "H", "", "host for the live session (base domain)")
	cmd.Flags().SetNormalizeFunc(normalizeFlags)
	return cmd
}

func runLive(host string) error {
	// Toggle off when a session is already active.
	if m, _ := config.ReadLiveMarker(); m != nil {
		_ = api.New(m.URL, m.AccessKey).LiveStop(m.SessionID)
		_ = config.RemoveLiveMarker()
		okLine("Live session stopped on " + m.Host)
		info("Uploads create normal links again.")
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	h, err := liveHost(cfg, host)
	if err != nil {
		return err
	}

	sess, err := api.FromHost(h).LiveStart()
	if err != nil {
		return err
	}
	marker := &config.LiveMarker{
		SessionID: sess.SessionID,
		URL:       h.URL,
		AccessKey: h.AccessKey,
		Host:      h.Name,
		ViewerURL: sess.ViewerURL,
	}
	if err := config.WriteLiveMarker(marker); err != nil {
		return err
	}

	fmt.Println(ui.Logo())
	okLine("Live session started on " + h.Name)
	fmt.Println("  " + ui.Label.Render("Viewer: ") + ui.KeyStyle.Render(sess.ViewerURL))
	if err := desktop.Clipboard(sess.ViewerURL); err == nil {
		info("Link copied to clipboard")
	}
	info("Every upload (incl. screenshot hooks) now streams here until you run 'filez live' again.")
	return nil
}

func liveHost(cfg *config.Config, name string) (*config.Host, error) {
	if name != "" {
		h := cfg.Find(name)
		if h == nil {
			return nil, fmt.Errorf("unknown host %q — configured: %s", name, hostNames(cfg))
		}
		return h, nil
	}
	if p := cfg.Primary(); p != nil {
		return p, nil
	}
	if len(cfg.Hosts) == 1 {
		return &cfg.Hosts[0], nil
	}
	return nil, fmt.Errorf("no host configured — run: filez config hosts add")
}

// pushLiveIfActive streams path to the active live session, if any. handled=true
// means the file went to the live session and no normal upload should happen. If
// the session has ended server-side, the stale marker is removed and handled is
// false so the caller falls back to a normal upload.
func pushLiveIfActive(path string) (handled bool, viewerURL string, err error) {
	m, e := config.ReadLiveMarker()
	if e != nil || m == nil {
		return false, "", nil
	}
	if e := api.New(m.URL, m.AccessKey).LivePush(m.SessionID, path); e != nil {
		if errors.Is(e, api.ErrLiveGone) {
			_ = config.RemoveLiveMarker()
			return false, "", nil
		}
		return false, "", e
	}
	return true, m.ViewerURL, nil
}
