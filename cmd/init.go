package cmd

import (
	"fmt"
	"net/url"
	"strings"

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

If a URL is provided, the plugin list is fetched from that remote YAML file and
remembered for future runs.
If a local file path is provided, the plugin list is loaded from that YAML file.
Otherwise, the remembered URL is used, falling back to the built-in default
plugin list when no URL has been recorded.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := plugin.LoadManifest()
		if err != nil {
			manifest = &plugin.Manifest{}
		}

		source := manifest.InitURL
		shouldRecord := false
		if len(args) > 0 {
			source = args[0]
			shouldRecord = isURL(source)
		}

		plugins, err := resolvePlugins(source)
		if err != nil {
			return err
		}

		if shouldRecord && source != manifest.InitURL {
			manifest.InitURL = source
			if err := manifest.Save(); err != nil {
				return fmt.Errorf("failed to save init URL: %w", err)
			}
		}

		plugins = plugin.FilterByTags(plugins, initTags)

		if len(plugins) == 0 {
			terminal.Warning("No default plugins configured.")
			return nil
		}

		toInstall, toReinstall, skipped := plugin.CategorizeForInit(plugins, manifest)

		if len(skipped) > 0 {
			terminal.Infof("Skipping %d already installed plugin(s): %s", len(skipped), formatNames(skipped))
		}

		if len(toInstall) == 0 && len(toReinstall) == 0 {
			terminal.Success("All plugins are already installed.")
			return nil
		}

		if len(toInstall) > 0 {
			terminal.Infof("Installing %d new plugin(s)...", len(toInstall))
		}
		if len(toReinstall) > 0 {
			terminal.Infof("Reinstalling %d plugin(s) due to install URL changes:", len(toReinstall))
			for _, pluginConfig := range toReinstall {
				if entry, ok := manifest.Get(pluginConfig.Name); ok {
					fmt.Printf("  • %s: %s → %s\n", pluginConfig.Name, entry.Source, pluginConfig.Script)
				}
			}
		}
		fmt.Println()

		var failed []string

		runInstall := func(pluginConfig plugin.Plugin, reinstall bool) {
			verb := "Installing"
			if reinstall {
				verb = "Reinstalling"
			}

			spinner := uicli.NewSpinner().
				WithStyle(uicli.SpinnerDots).
				WithColor(uicli.CyanColor).
				WithMessage(fmt.Sprintf("%s %q...", verb, pluginConfig.Name)).
				Start()

			pluginInstaller, err := installer.FromPlugin(pluginConfig)
			if err != nil {
				spinner.Error(fmt.Sprintf("Failed to install %q: %v", pluginConfig.Name, err))
				failed = append(failed, fmt.Sprintf("%s (%v)", pluginConfig.Name, err))
				return
			}

			version, installErr := pluginInstaller.Install(pluginConfig.Name)
			if installErr != nil {
				spinner.Error(fmt.Sprintf("Failed to install %q: %v", pluginConfig.Name, installErr))
				failed = append(failed, fmt.Sprintf("%s (%v)", pluginConfig.Name, installErr))
				return
			}

			manifest.Add(pluginConfig.Name, version, pluginInstaller.PluginType(), pluginInstaller.Source(), "")
			if pluginConfig.Description != "" {
				manifest.SetDescription(pluginConfig.Name, pluginConfig.Description)
			}

			doneVerb := "Installed"
			if reinstall {
				doneVerb = "Reinstalled"
			}
			if path, ok := plugin.Find(pluginConfig.Name); ok {
				spinner.Success(fmt.Sprintf("%s %q (%s)", doneVerb, pluginConfig.Name, path))
			} else {
				spinner.Success(fmt.Sprintf("%s %q", doneVerb, pluginConfig.Name))
			}
		}

		for _, pluginConfig := range toInstall {
			runInstall(pluginConfig, false)
		}
		for _, pluginConfig := range toReinstall {
			runInstall(pluginConfig, true)
		}

		if err := manifest.Save(); err != nil {
			return fmt.Errorf("failed to save manifest: %w", err)
		}

		if len(failed) > 0 {
			fmt.Println()
			return fmt.Errorf("%d plugin(s) failed to install", len(failed))
		}

		fmt.Println()
		total := len(toInstall) + len(toReinstall)
		terminal.Successf("All %d plugin(s) processed!", total)
		return nil
	},
}

// formatNames joins a slice of names into a comma-separated string.
func formatNames(names []string) string {
	return strings.Join(names, ", ")
}

// isURL returns true if the given string looks like an HTTP(S) URL.
func isURL(source string) bool {
	parsedURL, err := url.Parse(source)
	return err == nil && (parsedURL.Scheme == "http" || parsedURL.Scheme == "https")
}

// resolvePlugins returns the plugin list to install. URLs are fetched remotely,
// local paths are loaded from disk, and an empty source uses built-in defaults.
func resolvePlugins(source string) ([]plugin.Plugin, error) {
	if source == "" {
		return defaultPlugins, nil
	}

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
