package cmd

import (
	"fmt"
	"net/url"

	uicli "github.com/alperdrsnn/clime"
	"github.com/git-hulk/clime/internal/installer"
	"github.com/git-hulk/clime/internal/plugin"
	"github.com/spf13/cobra"
)

// defaultPlugins is the list of plugins installed by `clime init`.
var defaultPlugins = []plugin.Plugin{}

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

		plugins = plugin.FilterByTags(plugins, initTags)

		if len(plugins) == 0 {
			terminal.Warning("No default plugins configured.")
			return nil
		}

		terminal.Infof("Installing %d default plugin(s)...", len(plugins))
		fmt.Println()

		manifest, err := plugin.LoadManifest()
		if err != nil {
			manifest = &plugin.Manifest{}
		}

		var failed []string

		for _, p := range plugins {
			spinner := uicli.NewSpinner().
				WithStyle(uicli.SpinnerDots).
				WithColor(uicli.CyanColor).
				WithMessage(fmt.Sprintf("Installing %q...", p.Name)).
				Start()

			inst, err := installer.FromPlugin(p)
			if err != nil {
				spinner.Error(fmt.Sprintf("Failed to install %q: %v", p.Name, err))
				failed = append(failed, fmt.Sprintf("%s (%v)", p.Name, err))
				continue
			}

			version, installErr := inst.Install(p.Name)
			if installErr != nil {
				spinner.Error(fmt.Sprintf("Failed to install %q: %v", p.Name, installErr))
				failed = append(failed, fmt.Sprintf("%s (%v)", p.Name, installErr))
				continue
			}

			manifest.Add(p.Name, version, inst.PluginType(), inst.Source(), "")
			if p.Description != "" {
				manifest.SetDescription(p.Name, p.Description)
			}

			if path, ok := plugin.Find(p.Name); ok {
				spinner.Success(fmt.Sprintf("Installed %q (%s)", p.Name, path))
			} else {
				spinner.Success(fmt.Sprintf("Installed %q", p.Name))
			}
		}

		if err := manifest.Save(); err != nil {
			return fmt.Errorf("failed to save manifest: %w", err)
		}

		if len(failed) > 0 {
			fmt.Println()
			return fmt.Errorf("%d plugin(s) failed to install", len(failed))
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
func resolvePlugins(args []string) ([]plugin.Plugin, error) {
	if len(args) == 0 {
		return defaultPlugins, nil
	}

	source := args[0]
	var (
		defaults *plugin.PluginList
		err      error
	)

	if isURL(source) {
		terminal.Infof("Fetching plugin list from %s...", source)
		defaults, err = plugin.FetchPlugins(source)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch plugin list: %w", err)
		}
	} else {
		terminal.Infof("Loading plugin list from %s...", source)
		defaults, err = plugin.LoadPluginsFromFile(source)
		if err != nil {
			return nil, fmt.Errorf("failed to load plugin list: %w", err)
		}
	}

	return defaults.Plugins, nil
}
