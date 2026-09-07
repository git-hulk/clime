package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/git-hulk/clime/internal/plugin"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "clime",
	Short: "Unified CLI manager that lets you install, discover, and update CLI plugins from one place",
	Long: "As more agents move from MCP servers to CLIs, it gets hard to track what's installed and keep things up to date." +
		"\nFor organizations with many internal tools, there's often no single place for employees to discover and download them." +
		"\nclime solves these problems by providing a unified CLI manager that lets you install, discover, and update CLI plugins from one place.",
	SilenceErrors:     true,
	SilenceUsage:      true,
	CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
}

var builtinCommands = map[string]bool{
	"version":    true,
	"update":     true,
	"plugin":     true,
	"init":       true,
	"help":       true,
	"completion": true,
	"install":    true,
	"skills":     true,
}

func Execute() error {
	// Before Cobra handles args, check if the subcommand is a plugin
	if len(os.Args) > 1 {
		subcommand := os.Args[1]
		if !builtinCommands[subcommand] && !strings.HasPrefix(subcommand, "-") && !strings.HasPrefix(subcommand, "__") {
			if binaryPath, found := plugin.Find(subcommand); found {
				plugin.Exec(binaryPath, os.Args[2:])
				// Exec replaces the process; reaching here means it failed
				fmt.Fprintf(os.Stderr, "Failed to execute plugin: %s\n", binaryPath)
				os.Exit(1)
			}
			return fmt.Errorf("unknown command %q for \"clime\"", subcommand)
		}
	}

	// Register discovered plugins so they appear in help output.
	registerPlugins()

	return rootCmd.Execute()
}

func registerPlugins() {
	plugins := plugin.Discover()
	if len(plugins) == 0 {
		return
	}

	rootCmd.AddGroup(
		&cobra.Group{ID: "builtin", Title: "Available Commands:"},
		&cobra.Group{ID: "plugin", Title: "Plugins:"},
	)
	// Assign all existing (builtin) commands to the "builtin" group.
	for _, cmd := range rootCmd.Commands() {
		cmd.GroupID = "builtin"
	}
	for _, discoveredPlugin := range plugins {
		short := discoveredPlugin.Name + " plugin"
		if discoveredPlugin.Description != "" {
			short = discoveredPlugin.Description
		}
		rootCmd.AddCommand(&cobra.Command{
			Use:                discoveredPlugin.Name,
			Short:              short,
			GroupID:            "plugin",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return fmt.Errorf("plugin %q dispatch failed", cmd.Name())
			},
		})
	}
}
