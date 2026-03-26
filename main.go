package main

import (
	"os"

	"github.com/git-hulk/clime/cmd"
	"github.com/git-hulk/clime/internal/version"
)

var (
	ver       = "dev"
	gitCommit = "unknown"
	buildDate = "unknown"
)

func main() {
	var terminal cmd.Terminal
	version.Version = ver
	version.GitCommit = gitCommit
	version.BuildDate = buildDate

	if err := cmd.Execute(); err != nil {
		terminal.Errorf("Error: %v", err)
		os.Exit(1)
	}
}
