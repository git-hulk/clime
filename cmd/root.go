package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/git-hulk/clime/internal/plugin"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:               "clime",
	Short:             "Unified CLI manager that lets you install, discover, and update CLI plugins from one place",
	Long:              "As more agents move from MCP servers to CLIs, it gets hard to track what's installed and keep things up to date." +
						"\nFor organizations with many internal tools, there's often no single place for employees to discover and download them."+
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
}

func Execute() error {
	// Before Cobra handles args, check if the subcommand is a plugin
	if len(os.Args) > 1 {
		sub := os.Args[1]
		if !builtinCommands[sub] && !strings.HasPrefix(sub, "-") && !strings.HasPrefix(sub, "__") {
			if p, found := plugin.Find(sub); found {
				plugin.Exec(p, os.Args[2:])
				// Exec replaces the process; reaching here means it failed
				fmt.Fprintf(os.Stderr, "failed to execute plugin: %s\n", p)
				os.Exit(1)
			}
			return fmt.Errorf("unknown command %q for \"clime\"", sub)
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
	for _, p := range plugins {
		short := p.Name + " plugin"
		if p.Description != "" {
			short = p.Description
		}
		rootCmd.AddCommand(&cobra.Command{
			Use:                p.Name,
			Short:              short,
			GroupID:            "plugin",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return fmt.Errorf("plugin %q dispatch failed", cmd.Name())
			},
		})
	}
}
