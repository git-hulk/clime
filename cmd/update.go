package cmd

import (
	"fmt"

	"github.com/git-hulk/clime/internal/selfupdate"
	"github.com/git-hulk/clime/internal/version"
	"github.com/spf13/cobra"
)

const defaultCLIRepo = "git-hulk/clime"

var (
	updateForce bool
	updateRepo  string
)

func init() {
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "Update even if current version matches latest release")
	updateCmd.Flags().StringVar(&updateRepo, "repo", defaultCLIRepo, "GitHub repo (owner/name) to update from")
	rootCmd.AddCommand(updateCmd)
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update clime to the latest release",
	RunE: func(cmd *cobra.Command, args []string) error {
		terminal.Infof("Checking for updates in %s...", updateRepo)
		result, err := selfupdate.Update(selfupdate.Options{
			Repo:           updateRepo,
			CurrentVersion: version.Version,
			Force:          updateForce,
		})
		if err != nil {
			return fmt.Errorf("self-update failed: %w", err)
		}

		if !result.Updated {
			terminal.Infof("Already up to date (version %s).", result.LatestVersion)
			return nil
		}

		terminal.Successf("Updated clime from %s to %s", result.CurrentVersion, result.LatestVersion)
		terminal.Infof("Installed binary: %s", result.Path)
		return nil
	},
}
