package cmd

import (
	"fmt"

	uicli "github.com/alperdrsnn/clime"
	"github.com/git-hulk/clime/internal/version"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("%s %s %s\n",
			uicli.BrightCyanColor.Sprint("clime"),
			uicli.BrightWhiteColor.Sprint(version.Version),
			uicli.DimColor.Sprintf("(commit: %s, built: %s)", version.GitCommit, version.BuildDate),
		)
	},
}
