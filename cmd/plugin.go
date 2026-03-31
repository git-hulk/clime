package cmd

import (
	"fmt"
	"sort"
	"strings"

	uicli "github.com/alperdrsnn/clime"
	"github.com/git-hulk/clime/internal/installer"
	"github.com/git-hulk/clime/internal/plugin"
	"github.com/spf13/cobra"
)

var (
	pluginInstall     plugin.Plugin
	pluginUpdateRepo  string
	pluginUpdateForce bool
)

func init() {
	pluginInstallCmd.Flags().StringVar(&pluginInstall.Repo, "repo", "", "GitHub repo (owner/name) to install from, overrides the default convention")
	pluginInstallCmd.Flags().StringVar(&pluginInstall.Npm, "npm", "", "npm package name to install globally")
	pluginInstallCmd.Flags().StringVar(&pluginInstall.Script, "script", "", "URL of an install script to run (curl | sh)")
	pluginInstallCmd.Flags().StringVar(&pluginInstall.BinaryPath, "binary-path", "", "path to the binary after the install script runs (required with --script)")
	pluginInstallCmd.Flags().StringVar(&pluginInstall.Description, "description", "", "short description shown in help output")
	pluginUpdateCmd.Flags().StringVar(&pluginUpdateRepo, "repo", "", "GitHub repo (owner/name) to update from, overrides manifest/default convention")
	pluginUpdateCmd.Flags().BoolVar(&pluginUpdateForce, "force", false, "Update even if current version matches latest release")

	pluginCmd.AddCommand(pluginListCmd)
	pluginCmd.AddCommand(pluginInstallCmd)
	pluginCmd.AddCommand(pluginUpdateCmd)
	pluginCmd.AddCommand(pluginRemoveCmd)
	rootCmd.AddCommand(pluginCmd)
}

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage clime plugins",
	RunE:  pluginListCmd.RunE,
}

var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "List discovered and installed plugins",
	RunE: func(cmd *cobra.Command, args []string) error {
		discovered := plugin.Discover()
		manifest, err := plugin.LoadManifest()
		if err != nil {
			manifest = &plugin.Manifest{}
		}

		if len(discovered) == 0 && len(manifest.Plugins) == 0 {
			terminal.Warning("No plugins found.")
			terminal.Info("Install a plugin with: clime plugin install <name>")
			return nil
		}

		const (
			maxNameWidth    = 20
			maxDescWidth    = 40
			maxVersionWidth = 12
			maxPathWidth    = 30
		)

		table := uicli.NewTable().
			AutoResize(false).
			AddColumnWithWidth("NAME", maxNameWidth).
			AddColumnWithWidth("DESCRIPTION", maxDescWidth).
			AddColumnWithWidth("VERSION", maxVersionWidth).
			AddColumnWithWidth("PATH", maxPathWidth).
			WithHeaderColor(uicli.CyanColor).
			WithBorderColor(uicli.BlueColor).
			WithStyle(uicli.TableStyleRounded).
			SetColumnColor(0, uicli.BrightCyanColor).
			SetColumnColor(1, uicli.DimColor).
			SetColumnColor(2, uicli.GreenColor).
			SetColumnColor(3, uicli.DimColor)

		for _, p := range discovered {
			version := ""
			if entry, ok := manifest.Get(p.Name); ok {
				version = entry.Version
			}
			table.AddRow(
				uicli.TruncateString(p.Name, maxNameWidth),
				uicli.TruncateString(p.Description, maxDescWidth),
				uicli.TruncateString(version, maxVersionWidth),
				uicli.TruncateString(p.Path, maxPathWidth),
			)
		}
		table.Println()
		return nil
	},
}

var pluginInstallCmd = &cobra.Command{
	Use:   "install <name>",
	Short: "Install a plugin from GitHub Releases, npm, or an install script",
	Long:  "Downloads and installs a plugin. By default, looks for git-hulk/clime-<name> on GitHub. Use --npm to install from an npm package, or --script to run a remote install script.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		sources := 0
		if pluginInstall.Npm != "" {
			sources++
		}
		if pluginInstall.Repo != "" {
			sources++
		}
		if pluginInstall.Script != "" {
			sources++
		}
		if sources > 1 {
			return fmt.Errorf("--npm, --repo, and --script are mutually exclusive")
		}

		inst, err := installer.FromPlugin(pluginInstall)
		if err != nil {
			return err
		}

		spinner := uicli.NewSpinner().
			WithStyle(uicli.SpinnerDots).
			WithColor(uicli.CyanColor).
			WithMessage(fmt.Sprintf("Installing plugin %q...", name)).
			Start()

		version, err := inst.Install(name)
		if err != nil {
			spinner.Error(fmt.Sprintf("Failed to install plugin %q", name))
			return fmt.Errorf("failed to install plugin %q: %w", name, err)
		}

		manifest, err := plugin.LoadManifest()
		if err != nil {
			manifest = &plugin.Manifest{}
		}
		manifest.Add(name, version, inst.PluginType(), inst.Source(), "")
		if pluginInstall.Description != "" {
			manifest.SetDescription(name, pluginInstall.Description)
		}
		if err := manifest.Save(); err != nil {
			return fmt.Errorf("plugin installed but failed to update manifest: %w", err)
		}

		spinner.Success(fmt.Sprintf("Installed plugin %q (%s)", name, version))
		return nil
	},
}

var pluginRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an installed plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		spinner := uicli.NewSpinner().
			WithStyle(uicli.SpinnerDots).
			WithColor(uicli.CyanColor).
			WithMessage(fmt.Sprintf("Removing plugin %q...", name)).
			Start()

		manifest, err := plugin.LoadManifest()
		if err != nil {
			manifest = &plugin.Manifest{}
		}

		entry, _ := manifest.Get(name)
		inst, err := installer.FromManifest(entry)
		if err != nil {
			// If we can't determine the installer type, just remove the binary directly
			inst = installer.NewGitHubInstaller("")
		}
		if err := inst.Uninstall(name, entry); err != nil {
			spinner.Error(fmt.Sprintf("Failed to remove plugin %q", name))
			return fmt.Errorf("failed to remove plugin %q: %w", name, err)
		}

		manifest.Remove(name)
		if err := manifest.Save(); err != nil {
			return fmt.Errorf("plugin removed but failed to update manifest: %w", err)
		}

		spinner.Success(fmt.Sprintf("Removed plugin %q", name))
		return nil
	},
}

var pluginUpdateCmd = &cobra.Command{
	Use:   "update <name|all>",
	Short: "Update an installed plugin",
	Long:  "Updates a plugin from its configured source. GitHub-based plugins update to the latest release. Script-based plugins rerun their install script. Repo is resolved from --repo, manifest, or the default git-hulk/clime-<name> convention. Use '*' or 'all' to update all managed plugins.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if strings.EqualFold(name, "all") {
			return runPluginUpdateAll()
		}

		spinner := uicli.NewSpinner().
			WithStyle(uicli.SpinnerDots).
			WithColor(uicli.CyanColor).
			WithMessage(fmt.Sprintf("Checking updates for plugin %q...", name)).
			Start()

		manifest, err := plugin.LoadManifest()
		if err != nil {
			manifest = &plugin.Manifest{}
		}

		entry, _ := manifest.Get(name)

		var inst installer.Installer
		if repo := strings.TrimSpace(pluginUpdateRepo); repo != "" {
			inst = installer.NewGitHubInstaller(repo)
		} else {
			inst, err = installer.FromManifest(entry)
			if err != nil {
				if entry.Type == "" {
					spinner.Stop()
					return fmt.Errorf("no source configured for plugin %q; use --repo to specify one", name)
				}
				spinner.Error(fmt.Sprintf("Failed to update plugin %q", name))
				return err
			}
		}

		result, err := inst.Update(name, entry)
		if err != nil {
			spinner.Error(fmt.Sprintf("Failed to update plugin %q", name))
			return fmt.Errorf("failed to update plugin %q: %w", name, err)
		}

		if !result.Updated {
			spinner.Info(fmt.Sprintf("Plugin %q is already up to date (%s)", name, result.LatestVersion))
			return nil
		}

		manifest.Add(name, result.LatestVersion, inst.PluginType(), inst.Source(), entry.BinaryPath)
		if err := manifest.Save(); err != nil {
			return fmt.Errorf("plugin updated but failed to update manifest: %w", err)
		}

		if result.CurrentVersion == "" {
			spinner.Success(fmt.Sprintf("Updated plugin %q to %s", name, result.LatestVersion))
		} else {
			spinner.Success(fmt.Sprintf("Updated plugin %q: %s → %s", name, result.CurrentVersion, result.LatestVersion))
		}
		terminal.Infof("Installed binary: %s", result.Path)
		return nil
	},
}

func runPluginUpdateAll() error {
	if pluginUpdateRepo != "" {
		return fmt.Errorf("--repo cannot be used with \"all\"; repos are resolved per plugin from manifest/default convention")
	}

	manifest, err := plugin.LoadManifest()
	if err != nil {
		return fmt.Errorf("failed to load plugin manifest: %w", err)
	}

	seen := map[string]bool{}
	var names []string
	for _, p := range manifest.Plugins {
		name := strings.TrimSpace(p.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	sort.Strings(names)

	if len(names) == 0 {
		terminal.Warning("No managed plugins found.")
		terminal.Info("Install a plugin with: clime plugin install <name>")
		return nil
	}

	terminal.Infof("Checking updates for %d plugin(s)...", len(names))
	fmt.Println()

	var (
		updated int
		skipped int
		failed  []string
	)

	for _, name := range names {
		spinner := uicli.NewSpinner().
			WithStyle(uicli.SpinnerDots).
			WithColor(uicli.CyanColor).
			WithMessage(fmt.Sprintf("Checking %q...", name)).
			Start()

		entry, _ := manifest.Get(name)
		inst, err := installer.FromManifest(entry)
		if err != nil {
			spinner.Error(fmt.Sprintf("Failed: %s", name))
			failed = append(failed, fmt.Sprintf("%s (%v)", name, err))
			continue
		}

		result, err := inst.Update(name, entry)
		if err != nil {
			spinner.Error(fmt.Sprintf("Failed: %s", name))
			failed = append(failed, fmt.Sprintf("%s (%v)", name, err))
			continue
		}

		if !result.Updated {
			spinner.Info(fmt.Sprintf("%s is up to date", name))
			skipped++
			continue
		}

		manifest.Add(name, result.LatestVersion, inst.PluginType(), inst.Source(), entry.BinaryPath)
		if err := manifest.Save(); err != nil {
			spinner.Error(fmt.Sprintf("Failed: %s (manifest save)", name))
			failed = append(failed, fmt.Sprintf("%s (manifest save: %v)", name, err))
			continue
		}

		spinner.Success(fmt.Sprintf("Updated %s: %s → %s", name, result.CurrentVersion, result.LatestVersion))
		updated++
	}

	fmt.Println()
	fmt.Printf("  %s %s, %s, %s\n",
		uicli.BoldColor.Sprint("Summary:"),
		uicli.GreenColor.Sprintf("%d updated", updated),
		uicli.DimColor.Sprintf("%d up to date", skipped),
		uicli.RedColor.Sprintf("%d failed", len(failed)),
	)
	if len(failed) > 0 {
		return fmt.Errorf("%d plugin(s) failed to update: %s", len(failed), strings.Join(failed, "; "))
	}
	return nil
}
