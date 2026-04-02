package main

import (
	_ "embed"
	"os"

	"github.com/git-hulk/clime/cmd"
	"github.com/git-hulk/clime/internal/version"
)

//go:embed SKILL.md
var skillContent string

//go:embed agents/openai.yaml
var agentYAML string

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
	cmd.SkillContent = skillContent
	cmd.AgentYAML = agentYAML

	if err := cmd.Execute(); err != nil {
		terminal.Errorf("Error: %v", err)
		os.Exit(1)
	}
}
