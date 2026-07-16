// Command filez is the Filez CLI: direct upload plus host configuration.
// A sibling command, filezui, offers the same flow as an interactive console UI.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/DasDarki/filez/internal/client/api"
	"github.com/DasDarki/filez/internal/client/config"
	"github.com/DasDarki/filez/internal/client/ui"
	"github.com/DasDarki/filez/internal/timefmt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	flagHost      string
	flagPermanent bool
	flagTemp      string
	flagPassword  string
	flagDownloads int
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		failLine(err.Error())
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "filez [file]",
		Short: "Share files from your terminal with a Filez server",
		Long: ui.Logo() + "\n\n" +
			"Upload a file:   filez report.pdf --temp 2d\n" +
			"Configure hosts: filez config hosts add\n\n" +
			ui.Subtle.Render(ui.Brand),
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return runUpload(args[0])
		},
	}

	f := root.Flags()
	f.StringVarP(&flagHost, "host", "H", "", "host to upload to (base domain)")
	f.BoolVarP(&flagPermanent, "permanent", "p", false, "store permanently")
	f.StringVarP(&flagTemp, "temp", "t", "", "store temporarily for a duration (e.g. 20m, 2d, 2d20m, 1M)")
	f.StringVarP(&flagPassword, "password", "P", "", "protect the file with a password")
	f.IntVarP(&flagDownloads, "downloads", "d", 0, "delete after this many downloads")
	f.SetNormalizeFunc(normalizeFlags)

	root.AddCommand(newConfigCmd())
	root.AddCommand(newHookCmd())
	root.AddCommand(newShareCmd())
	root.AddCommand(newMenuCmd())
	return root
}

// normalizeFlags maps the concept's two-letter aliases to canonical flag names.
func normalizeFlags(_ *pflag.FlagSet, name string) pflag.NormalizedName {
	switch name {
	case "pw":
		name = "password"
	case "dl":
		name = "downloads"
	case "h":
		name = "host"
	case "permant", "perma":
		name = "permanent"
	}
	return pflag.NormalizedName(name)
}

func runUpload(path string) error {
	if fi, err := os.Stat(path); err != nil {
		return fmt.Errorf("file not found: %s", path)
	} else if fi.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	host, err := resolveHost(cfg)
	if err != nil {
		return err
	}
	mode, ttl, downloads, err := resolveMode(cfg)
	if err != nil {
		return err
	}

	fmt.Println(ui.Logo())
	info(fmt.Sprintf("Host: %s", host.Name))
	info("Mode: " + describeMode(mode, ttl, downloads, flagPassword != ""))
	fmt.Println()

	client := api.FromHost(host)
	opts := api.UploadOptions{Mode: mode, TTL: ttl, Downloads: downloads, Password: flagPassword}

	res, err := client.Upload(path, opts, progressPrinter())
	fmt.Print("\r\033[K") // clear progress line
	if err != nil {
		return err
	}

	okLine("Upload complete")
	fmt.Println("  " + ui.Label.Render("Link:    ") + ui.KeyStyle.Render(res.URL))
	fmt.Println("  " + ui.Label.Render("Preview: ") + res.PreviewURL)
	return nil
}

// resolveHost picks the target host from flags/config.
func resolveHost(cfg *config.Config) (*config.Host, error) {
	if flagHost != "" {
		h := cfg.Find(flagHost)
		if h == nil {
			return nil, fmt.Errorf("unknown host %q — configured: %s", flagHost, hostNames(cfg))
		}
		return h, nil
	}
	if len(cfg.Hosts) == 0 {
		return nil, fmt.Errorf("no host configured — run: filez config hosts add")
	}
	if cfg.AskHost && len(cfg.Hosts) > 1 {
		return pickHost(cfg)
	}
	if p := cfg.Primary(); p != nil {
		return p, nil
	}
	return &cfg.Hosts[0], nil
}

// pickHost interactively selects a host, defaulting to the primary.
func pickHost(cfg *config.Config) (*config.Host, error) {
	fmt.Println(ui.Label.Render("Choose a host:"))
	def := 1
	for i := range cfg.Hosts {
		marker := " "
		if cfg.Hosts[i].Primary {
			marker = ui.KeyStyle.Render("★")
			def = i + 1
		}
		fmt.Printf("  %s %d) %s %s\n", marker, i+1, cfg.Hosts[i].Name, ui.Subtle.Render(cfg.Hosts[i].URL))
	}
	choice := askDefault("Number", strconv.Itoa(def))
	n, err := strconv.Atoi(choice)
	if err != nil || n < 1 || n > len(cfg.Hosts) {
		return nil, fmt.Errorf("invalid selection")
	}
	return &cfg.Hosts[n-1], nil
}

// resolveMode derives the upload mode from flags, falling back to the configured default.
func resolveMode(cfg *config.Config) (mode, ttl string, downloads int, err error) {
	switch {
	case flagPermanent:
		return "permanent", "", 0, nil
	case flagTemp != "":
		if _, e := timefmt.Parse(flagTemp); e != nil {
			return "", "", 0, fmt.Errorf("invalid --temp duration: %w", e)
		}
		return "temp", flagTemp, 0, nil
	case flagDownloads > 0:
		return "limited", "", flagDownloads, nil
	}

	// Default from env DEFAULT_UPLOAD, else config, else permanent.
	raw := os.Getenv("DEFAULT_UPLOAD")
	if raw == "" {
		raw = cfg.DefaultUpload
	}
	return parseDefault(raw)
}

func parseDefault(raw string) (mode, ttl string, downloads int, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "permanent") || strings.EqualFold(raw, "perma") {
		return "permanent", "", 0, nil
	}
	if strings.HasPrefix(strings.ToLower(raw), "temp") {
		parts := strings.SplitN(raw, ":", 2)
		ttl = "1d"
		if len(parts) == 2 && parts[1] != "" {
			ttl = parts[1]
		}
		if _, e := timefmt.Parse(ttl); e != nil {
			return "", "", 0, fmt.Errorf("invalid DEFAULT_UPLOAD ttl: %w", e)
		}
		return "temp", ttl, 0, nil
	}
	return "permanent", "", 0, nil
}

func describeMode(mode, ttl string, downloads int, hasPw bool) string {
	var s string
	switch mode {
	case "temp":
		s = "temporary (" + ttl + ")"
	case "limited":
		s = fmt.Sprintf("limited (%d downloads)", downloads)
	default:
		s = "permanent"
	}
	if hasPw {
		s += " + password"
	}
	return s
}

func hostNames(cfg *config.Config) string {
	names := make([]string, 0, len(cfg.Hosts))
	for i := range cfg.Hosts {
		names = append(names, cfg.Hosts[i].Name)
	}
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}

// progressPrinter returns a throttled progress callback that renders a bar.
func progressPrinter() func(sent, total int64) {
	last := -1
	return func(sent, total int64) {
		if total <= 0 {
			return
		}
		pct := int(sent * 100 / total)
		if pct == last {
			return
		}
		last = pct
		bar := ui.ProgressBar(float64(sent)/float64(total), 24)
		fmt.Printf("\r  %s %3d%% %s", bar, pct,
			ui.Subtle.Render(ui.HumanBytes(sent)+" / "+ui.HumanBytes(total)))
	}
}
