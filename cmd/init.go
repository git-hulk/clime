package cmd

import (
	"fmt"
	"strings"

	"net/url"

	uicli "github.com/alperdrsnn/clime"
	"github.com/git-hulk/clime/internal/plugin"
	"github.com/spf13/cobra"
)

// defaultPlugin defines a plugin to install during `clime init`.
type defaultPlugin struct {
	Name        string // subcommand name (e.g. "account")
	Description string // short description shown in help output

	// For GitHub Releases-based plugins (leave empty to use script-based install):
	Repo string // GitHub repo (e.g. "git-hulk/clime-hr")

	// For script-based plugins:
	ScriptURL  string // URL of the install script (curl | sh)
	BinaryPath string // where the script installs the binary (supports ~/)

	// For npm-based plugins:
	NpmPackage string // npm package name (e.g. "@myorg/clime-deploy")

	// Tags for selective installation (empty = always install).
	Tags []string
}

// defaultPlugins is the list of plugins installed by `clime init`.
var defaultPlugins = []defaultPlugin{}

var initTags []string

func init() {
	initCmd.Flags().StringSliceVar(&initTags, "tags", nil,
		"Install tagged plugins matching these tags (comma-separated); untagged plugins are always installed, tagged plugins require matching tags")
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init [url|path]",
	Short: "Install default plugins",
	Long: `Downloads and installs the organization's default set of plugins.

If a URL is provided, the plugin list is fetched from that remote YAML file.
If a local file path is provided, the plugin list is loaded from that YAML file.
Otherwise, the built-in default plugin list is used.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		plugins, err := resolvePlugins(args)
		if err != nil {
			return err
		}

		plugins = filterPluginsByTags(plugins, initTags)

		if len(plugins) == 0 {
			terminal.Warning("No default plugins configured.")
			return nil
		}

		terminal.Infof("Installing %d default plugin(s)...", len(plugins))
		fmt.Println()

		type installRow struct {
			name   string
			source string
			tags   string
			status string
		}

		var (
			rows   []installRow
			failed []string
		)

		for _, p := range plugins {
			spinner := uicli.NewSpinner().
				WithStyle(uicli.SpinnerDots).
				WithColor(uicli.CyanColor).
				WithMessage(fmt.Sprintf("Installing %q...", p.Name)).
				Start()

			var (
				installErr error
				source     string
			)
			if p.NpmPackage != "" {
				source = p.NpmPackage
				installErr = plugin.InstallFromNpm(p.Name, p.NpmPackage)
			} else if p.ScriptURL != "" {
				source = p.ScriptURL
				installErr = plugin.InstallFromScript(p.Name, p.ScriptURL, p.BinaryPath)
			} else {
				repo := p.Repo
				if repo == "" {
					repo = fmt.Sprintf("git-hulk/clime-%s", p.Name)
				}
				source = repo
				_, installErr = plugin.InstallFromRepo(p.Name, repo)
			}

			if installErr != nil {
				spinner.Error(fmt.Sprintf("Failed to install %q", p.Name))
				failed = append(failed, p.Name)
				rows = append(rows, installRow{name: p.Name, source: source, tags: strings.Join(p.Tags, ", "), status: "Failed"})
				continue
			}
			if p.Description != "" {
				if m, err := plugin.LoadManifest(); err == nil {
					m.SetDescription(p.Name, p.Description)
					_ = m.Save()
				}
			}
			spinner.Success(fmt.Sprintf("Installed %q", p.Name))
			rows = append(rows, installRow{name: p.Name, source: source, tags: strings.Join(p.Tags, ", "), status: "Installed"})
		}

		fmt.Println()
		table := uicli.NewTable().
			WithSmartWidth(1).
			AddColumnWithWidth("NAME", 20).
			AddColumnWithWidth("SOURCE", 60).
			AddColumnWithWidth("TAGS", 20).
			AddColumnWithWidth("STATUS", 20).
			WithHeaderColor(uicli.CyanColor).
			WithBorderColor(uicli.BlueColor).
			WithStyle(uicli.TableStyleRounded).
			SetColumnColor(0, uicli.BrightCyanColor).
			SetColumnColor(1, uicli.DimColor).
			SetColumnColor(2, uicli.DimColor)
		for _, r := range rows {
			coloredStatus := r.status
			switch r.status {
			case "Installed":
				coloredStatus = uicli.GreenColor.Sprint(r.status)
			case "Failed":
				coloredStatus = uicli.RedColor.Sprint(r.status)
			}
			table.AddRow(r.name, r.source, r.tags, coloredStatus)
		}
		table.Println()

		if len(failed) > 0 {
			fmt.Println()
			return fmt.Errorf("%d plugin(s) failed to install: %v", len(failed), failed)
		}

		fmt.Println()
		terminal.Successf("All %d plugins installed!", len(plugins))
		return nil
	},
}

// isURL returns true if the given string looks like an HTTP(S) URL.
func isURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
}

// resolvePlugins returns the plugin list to install. If a URL argument is
// provided, the list is fetched from that remote YAML file. If a local file
// path is provided, the list is loaded from that file. Otherwise the built-in
// defaultPlugins slice is returned.
func resolvePlugins(args []string) ([]defaultPlugin, error) {
	if len(args) == 0 {
		return defaultPlugins, nil
	}

	source := args[0]
	var (
		defaults *plugin.DefaultPlugins
		err      error
	)

	if isURL(source) {
		terminal.Infof("Fetching plugin list from %s...", source)
		defaults, err = plugin.FetchDefaultPlugins(source)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch plugin list: %w", err)
		}
	} else {
		terminal.Infof("Loading plugin list from %s...", source)
		defaults, err = plugin.LoadDefaultPluginsFromFile(source)
		if err != nil {
			return nil, fmt.Errorf("failed to load plugin list: %w", err)
		}
	}

	plugins := make([]defaultPlugin, 0, len(defaults.Plugins))
	for _, p := range defaults.Plugins {
		plugins = append(plugins, defaultPlugin{
			Name:        p.Name,
			Description: p.Description,
			Repo:        p.Repo,
			ScriptURL:   p.Script,
			BinaryPath:  p.BinaryPath,
			NpmPackage:  p.Npm,
			Tags:        p.Tags,
		})
	}
	return plugins, nil
}

// filterPluginsByTags returns plugins that should be installed based on the
// given tags. Untagged plugins are always included. Tagged plugins are only
// included if tags are provided and they share at least one tag with the
// provided list. If no tags are specified, tagged plugins are skipped.
func filterPluginsByTags(plugins []defaultPlugin, tags []string) []defaultPlugin {
	tagSet := make(map[string]bool, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t != "" {
			tagSet[t] = true
		}
	}

	filtered := make([]defaultPlugin, 0, len(plugins))
	for _, p := range plugins {
		if len(p.Tags) == 0 {
			filtered = append(filtered, p)
			continue
		}
		for _, pt := range p.Tags {
			if tagSet[strings.TrimSpace(pt)] {
				filtered = append(filtered, p)
				break
			}
		}
	}
	return filtered
}
