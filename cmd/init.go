package cmd

import (
	"fmt"

	uicli "github.com/alperdrsnn/clime"
	"github.com/git-hulk/clime/internal/plugin"
	"github.com/spf13/cobra"
)

// defaultPlugin defines a plugin to install during `clime init`.
type defaultPlugin struct {
	Name string // subcommand name (e.g. "account")

	// For GitHub Releases-based plugins (leave empty to use script-based install):
	Repo string // GitHub repo (e.g. "git-hulk/clime-hr")

	// For script-based plugins:
	ScriptURL  string // URL of the install script (curl | sh)
	BinaryPath string // where the script installs the binary (supports ~/)

	// For npm-based plugins:
	NpmPackage string // npm package name (e.g. "@myorg/clime-deploy")
}

// defaultPlugins is the list of plugins installed by `clime init`.
var defaultPlugins = []defaultPlugin{}

func init() {
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init [url]",
	Short: "Install default plugins",
	Long: `Downloads and installs the organization's default set of plugins.

If a URL is provided, the plugin list is fetched from that remote YAML file.
Otherwise, the built-in default plugin list is used.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		plugins, err := resolvePlugins(args)
		if err != nil {
			return err
		}

		if len(plugins) == 0 {
			terminal.Warning("No default plugins configured.")
			return nil
		}

		terminal.Infof("Installing %d default plugin(s)...", len(plugins))
		fmt.Println()

		type installRow struct {
			name   string
			source string
			status string
		}

		var (
			rows   []installRow
			failed []string
		)

		for _, p := range plugins {
			terminal.Infof("Installing %q...", p.Name)

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
				terminal.Errorf("FAILED: %v", installErr)
				failed = append(failed, p.Name)
				rows = append(rows, installRow{name: p.Name, source: source, status: "Failed"})
				continue
			}
			rows = append(rows, installRow{name: p.Name, source: source, status: "Installed"})
		}

		fmt.Println()
		table := uicli.NewTable().
			AddColumn("NAME").
			AddColumn("SOURCE").
			AddColumn("STATUS").
			WithHeaderColor(uicli.CyanColor).
			WithBorderColor(uicli.BlueColor).
			SetColumnColor(1, uicli.CyanColor)
		for _, r := range rows {
			table.AddRow(r.name, r.source, r.status)
		}
		table.Println()

		fmt.Println()
		if len(failed) > 0 {
			return fmt.Errorf("%d plugin(s) failed to install: %v", len(failed), failed)
		}
		return nil
	},
}

// resolvePlugins returns the plugin list to install. If a URL argument is
// provided, the list is fetched from that remote YAML file; otherwise the
// built-in defaultPlugins slice is returned.
func resolvePlugins(args []string) ([]defaultPlugin, error) {
	if len(args) == 0 {
		return defaultPlugins, nil
	}

	url := args[0]
	terminal.Infof("Fetching plugin list from %s...", url)
	defaults, err := plugin.FetchDefaultPlugins(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch plugin list: %w", err)
	}

	plugins := make([]defaultPlugin, 0, len(defaults.Plugins))
	for _, p := range defaults.Plugins {
		plugins = append(plugins, defaultPlugin{
			Name:       p.Name,
			Repo:       p.Repo,
			ScriptURL:  p.Script,
			BinaryPath: p.BinaryPath,
			NpmPackage: p.Npm,
		})
	}
	return plugins, nil
}
