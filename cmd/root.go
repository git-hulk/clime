package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/git-hulk/clime/internal/plugin"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "clime",
	Short:         "Unified CLI for organization tools",
	Long:          "clime is a unified command-line interface that discovers and dispatches to plugin CLIs (clime-<name> binaries on your PATH).",
	SilenceErrors: true,
	SilenceUsage:  true,
}

var builtinCommands = map[string]bool{
	"version":    true,
	"update":     true,
	"plugin":     true,
	"init":       true,
	"help":       true,
	"completion": true,
}

func Execute() error {
	// Before Cobra handles args, check if the subcommand is a plugin
	if len(os.Args) > 1 {
		sub := os.Args[1]
		if !builtinCommands[sub] && !strings.HasPrefix(sub, "-") {
			if p, found := plugin.Find(sub); found {
				plugin.Exec(p, os.Args[2:])
				// Exec replaces the process; reaching here means it failed
				fmt.Fprintf(os.Stderr, "failed to execute plugin: %s\n", p)
				os.Exit(1)
			}
			return fmt.Errorf("unknown command %q for \"clime\"", sub)
		}
	}

	return rootCmd.Execute()
}
