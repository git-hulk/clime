package cmd

import (
	"fmt"
	"sort"
	"strings"

	uicli "github.com/alperdrsnn/clime"
	"github.com/git-hulk/clime/internal/plugin"
	"github.com/spf13/cobra"
)

var pluginRepo string
var pluginNpm string
var pluginScript string
var pluginBinaryPath string
var pluginDescription string
var (
	pluginUpdateRepo  string
	pluginUpdateForce bool
)

func init() {
	pluginInstallCmd.Flags().StringVar(&pluginRepo, "repo", "", "GitHub repo (owner/name) to install from, overrides the default convention")
	pluginInstallCmd.Flags().StringVar(&pluginNpm, "npm", "", "npm package name to install globally")
	pluginInstallCmd.Flags().StringVar(&pluginScript, "script", "", "URL of an install script to run (curl | sh)")
	pluginInstallCmd.Flags().StringVar(&pluginBinaryPath, "binary-path", "", "path to the binary after the install script runs (required with --script)")
	pluginInstallCmd.Flags().StringVar(&pluginDescription, "description", "", "short description shown in help output")
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

		table := uicli.NewTable().
			AddColumn("NAME").
			AddColumn("DESCRIPTION").
			AddColumn("VERSION").
			AddColumn("PATH").
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
			table.AddRow(p.Name, p.Description, version, p.Path)
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
		if pluginNpm != "" {
			sources++
		}
		if pluginRepo != "" {
			sources++
		}
		if pluginScript != "" {
			sources++
		}
		if sources > 1 {
			return fmt.Errorf("--npm, --repo, and --script are mutually exclusive")
		}

		spinner := uicli.NewSpinner().
			WithStyle(uicli.SpinnerDots).
			WithColor(uicli.CyanColor).
			WithMessage(fmt.Sprintf("Installing plugin %q...", name)).
			Start()

		if pluginScript != "" {
			if err := plugin.InstallFromScript(name, pluginScript, pluginBinaryPath); err != nil {
				spinner.Error(fmt.Sprintf("Failed to install plugin %q", name))
				return fmt.Errorf("failed to install plugin %q: %w", name, err)
			}
			savePluginDescription(name, pluginDescription)
			spinner.Success(fmt.Sprintf("Installed plugin %q via install script", name))
			return nil
		}

		if pluginNpm != "" {
			if err := plugin.InstallFromNpm(name, pluginNpm); err != nil {
				spinner.Error(fmt.Sprintf("Failed to install plugin %q", name))
				return fmt.Errorf("failed to install plugin %q: %w", name, err)
			}
			savePluginDescription(name, pluginDescription)
			spinner.Success(fmt.Sprintf("Installed plugin %q via npm", name))
			return nil
		}

		var (
			version string
			err     error
		)
		if pluginRepo != "" {
			version, err = plugin.InstallFromRepo(name, pluginRepo)
		} else {
			version, err = plugin.Install(name)
		}
		if err != nil {
			spinner.Error(fmt.Sprintf("Failed to install plugin %q", name))
			return fmt.Errorf("failed to install plugin %q: %w", name, err)
		}

		savePluginDescription(name, pluginDescription)
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
		if err := plugin.Uninstall(name); err != nil {
			spinner.Error(fmt.Sprintf("Failed to remove plugin %q", name))
			return fmt.Errorf("failed to remove plugin %q: %w", name, err)
		}
		spinner.Success(fmt.Sprintf("Removed plugin %q", name))
		return nil
	},
}

var pluginUpdateCmd = &cobra.Command{
	Use:   "update <name|*|all>",
	Short: "Update an installed plugin",
	Long:  "Updates a plugin from its configured source. GitHub-based plugins update to the latest release. Script-based plugins rerun their install script. Repo is resolved from --repo, manifest, or the default git-hulk/clime-<name> convention. Use '*' or 'all' to update all managed plugins.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if name == "*" || strings.EqualFold(name, "all") {
			return runPluginUpdateAll()
		}

		spinner := uicli.NewSpinner().
			WithStyle(uicli.SpinnerDots).
			WithColor(uicli.CyanColor).
			WithMessage(fmt.Sprintf("Checking updates for plugin %q...", name)).
			Start()

		result, err := plugin.Update(plugin.UpdateOptions{
			Name:  name,
			Repo:  pluginUpdateRepo,
			Force: pluginUpdateForce,
		})
		if err != nil {
			spinner.Error(fmt.Sprintf("Failed to update plugin %q", name))
			return fmt.Errorf("failed to update plugin %q: %w", name, err)
		}

		if !result.Updated {
			spinner.Info(fmt.Sprintf("Plugin %q is already up to date (%s)", name, result.LatestVersion))
			return nil
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
		return fmt.Errorf("--repo cannot be used with \"*\"; repos are resolved per plugin from manifest/default convention")
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

	type updateRow struct {
		name   string
		from   string
		to     string
		status string
		errMsg string
	}

	var (
		rows    []updateRow
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

		result, err := plugin.Update(plugin.UpdateOptions{
			Name:  name,
			Force: pluginUpdateForce,
		})
		if err != nil {
			spinner.Error(fmt.Sprintf("Failed: %s", name))
			failed = append(failed, fmt.Sprintf("%s (%v)", name, err))
			rows = append(rows, updateRow{name: name, status: "Failed", errMsg: err.Error()})
			continue
		}

		if !result.Updated {
			spinner.Info(fmt.Sprintf("%s is up to date", name))
			skipped++
			rows = append(rows, updateRow{
				name:   name,
				from:   result.CurrentVersion,
				to:     result.LatestVersion,
				status: "Up to date",
			})
			continue
		}

		spinner.Success(fmt.Sprintf("Updated %s: %s → %s", name, result.CurrentVersion, result.LatestVersion))
		updated++
		rows = append(rows, updateRow{
			name:   name,
			from:   result.CurrentVersion,
			to:     result.LatestVersion,
			status: "Updated",
		})
	}

	table := uicli.NewTable().
		AddColumn("NAME").
		AddColumn("FROM").
		AddColumn("TO").
		AddColumn("STATUS").
		WithHeaderColor(uicli.CyanColor).
		WithBorderColor(uicli.BlueColor).
		WithStyle(uicli.TableStyleRounded).
		SetColumnColor(0, uicli.BrightCyanColor)

	for _, r := range rows {
		coloredStatus := r.status
		switch r.status {
		case "Updated":
			coloredStatus = uicli.GreenColor.Sprint(r.status)
		case "Up to date":
			coloredStatus = uicli.DimColor.Sprint(r.status)
		case "Failed":
			coloredStatus = uicli.RedColor.Sprint(r.status)
		}
		table.AddRow(r.name, r.from, r.to, coloredStatus)
	}
	table.Println()

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

func savePluginDescription(name, description string) {
	if description == "" {
		return
	}
	if m, err := plugin.LoadManifest(); err == nil {
		m.SetDescription(name, description)
		_ = m.Save()
	}
}
