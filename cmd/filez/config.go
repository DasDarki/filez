package main

import (
	"fmt"
	"strings"

	"github.com/DasDarki/filez/internal/client/api"
	"github.com/DasDarki/filez/internal/client/config"
	"github.com/DasDarki/filez/internal/client/ui"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cfgCmd := &cobra.Command{
		Use:           "config",
		Short:         "Manage Filez CLI configuration",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	hosts := &cobra.Command{
		Use:   "hosts",
		Short: "List and manage configured hosts",
		RunE:  func(cmd *cobra.Command, args []string) error { return runHostsList() },
	}
	hosts.AddCommand(&cobra.Command{
		Use:   "add",
		Short: "Add a host interactively",
		RunE:  func(cmd *cobra.Command, args []string) error { return runHostsAdd() },
	})
	hosts.AddCommand(&cobra.Command{
		Use:   "delete [name]",
		Short: "Delete a host",
		RunE:  func(cmd *cobra.Command, args []string) error { return runHostsDelete(args) },
	})
	hosts.AddCommand(&cobra.Command{
		Use:   "primary [name]",
		Short: "Set the primary host",
		RunE:  func(cmd *cobra.Command, args []string) error { return runHostsPrimary(args) },
	})
	cfgCmd.AddCommand(hosts)

	cfgCmd.AddCommand(&cobra.Command{
		Use:   "askhost [true|false]",
		Short: "Whether to ask for a host on every upload",
		RunE:  func(cmd *cobra.Command, args []string) error { return runAskHost(args) },
	})
	cfgCmd.AddCommand(&cobra.Command{
		Use:   "default [permanent|temp:1d]",
		Short: "Get or set the default upload mode",
		RunE:  func(cmd *cobra.Command, args []string) error { return runDefault(args) },
	})

	return cfgCmd
}

func runHostsList() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(cfg.Hosts) == 0 {
		info("No hosts configured. Add one with: filez config hosts add")
		return nil
	}
	fmt.Println(ui.Title.Render("Configured hosts"))
	for i := range cfg.Hosts {
		h := &cfg.Hosts[i]
		marker := "  "
		if h.Primary {
			marker = ui.KeyStyle.Render("★ ")
		}
		access := ui.Subtle.Render("public")
		if h.AccessKey != "" {
			access = ui.KeyStyle.Render("keyed")
		}
		fmt.Printf("%s%s  %s  %s\n", marker, ui.Label.Render(h.Name), ui.Subtle.Render(h.URL), access)
	}
	fmt.Println()
	info(fmt.Sprintf("askhost: %v · default: %s", cfg.AskHost, defaultOr(cfg.DefaultUpload)))
	return nil
}

func runHostsAdd() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	baseURL, name, err := config.NormalizeURL(ask("Domain:"))
	if err != nil {
		return err
	}

	info("Checking instance…")
	inf, err := api.New(baseURL, "").Info()
	if err != nil {
		return fmt.Errorf("%s does not look like a Filez instance (%v)", baseURL, err)
	}
	okLine(fmt.Sprintf("Found Filez %s — %s", inf.Version, publicStr(inf.Public)))

	var key string
	if inf.Public {
		if confirm("Instance is public. Store an access key anyway?", false) {
			key = askSecret("Access Key:")
		}
	} else {
		key = askSecret("Access Key:")
		if key == "" {
			return fmt.Errorf("this instance is private and requires an access key")
		}
	}

	if key != "" {
		info("Verifying access key…")
		ok, err := api.New(baseURL, key).AuthCheck()
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("the server rejected this access key")
		}
		okLine("Access key verified")
	}

	primary := confirm("Set as primary host?", len(cfg.Hosts) == 0)
	cfg.Upsert(config.Host{Name: name, URL: baseURL, AccessKey: key, Primary: primary})
	if err := cfg.Save(); err != nil {
		return err
	}
	okLine("Added host " + ui.KeyStyle.Render(name))
	return nil
}

func runHostsDelete(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(cfg.Hosts) == 0 {
		info("No hosts configured.")
		return nil
	}
	name := ""
	if len(args) > 0 {
		name = args[0]
	} else {
		_ = runHostsList()
		name = ask("Host to delete:")
	}
	if !cfg.Remove(name) {
		return fmt.Errorf("unknown host %q", name)
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	okLine("Deleted host " + name)
	return nil
}

func runHostsPrimary(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	name := ""
	if len(args) > 0 {
		name = args[0]
	} else {
		_ = runHostsList()
		name = ask("Host to make primary:")
	}
	if !cfg.SetPrimary(name) {
		return fmt.Errorf("unknown host %q", name)
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	okLine(name + " is now the primary host")
	return nil
}

func runAskHost(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		info(fmt.Sprintf("askhost is currently: %v", cfg.AskHost))
		return nil
	}
	v, err := parseBool(args[0])
	if err != nil {
		return err
	}
	cfg.AskHost = v
	if err := cfg.Save(); err != nil {
		return err
	}
	okLine(fmt.Sprintf("askhost set to %v", v))
	return nil
}

func runDefault(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		info("default upload: " + defaultOr(cfg.DefaultUpload))
		return nil
	}
	if _, _, _, err := parseDefault(args[0]); err != nil {
		return err
	}
	cfg.DefaultUpload = args[0]
	if err := cfg.Save(); err != nil {
		return err
	}
	okLine("default upload set to " + args[0])
	return nil
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "on", "y":
		return true, nil
	case "false", "0", "no", "off", "n":
		return false, nil
	}
	return false, fmt.Errorf("expected true or false, got %q", s)
}

func publicStr(public bool) string {
	if public {
		return "public instance"
	}
	return "private instance"
}

func defaultOr(s string) string {
	if s == "" {
		return "permanent"
	}
	return s
}
