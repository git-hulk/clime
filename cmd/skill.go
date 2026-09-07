package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/git-hulk/clime/internal/skill"
	"github.com/spf13/cobra"
)

// SkillContent is set by main.go with the embedded SKILL.md content.
var SkillContent string

// AgentYAML is set by main.go with the embedded agents/openai.yaml content.
var AgentYAML string

const skillDirName = "clime-cli"
const skillFileName = "SKILL.md"

func init() {
	installCmd.AddCommand(installSkillCmd)
	rootCmd.AddCommand(installCmd)
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install clime components",
}

var installSkillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Install the clime-cli skill into ~/.claude/skills and ~/.codex/skills",
	RunE: func(cmd *cobra.Command, args []string) error {
		targets, err := skill.Targets()
		if err != nil {
			return err
		}

		installed := 0
		for _, t := range targets {
			if !t.Exists() {
				terminal.Warningf("Skipping %s (directory not found)", t.Dir)
				continue
			}

			files := map[string][]byte{skillFileName: []byte(SkillContent)}
			// Codex discovers skills through agents/openai.yaml.
			if t.Name == "codex" {
				files[filepath.Join("agents", "openai.yaml")] = []byte(AgentYAML)
			}
			if err := t.Install(skillDirName, files); err != nil {
				return fmt.Errorf("failed to install skill to %s: %w", t.Dir, err)
			}

			terminal.Successf("Installed skill to %s", filepath.Join(t.Dir, "skills", skillDirName, skillFileName))
			installed++
		}

		if installed == 0 {
			terminal.Warning("No skill directories were installed. Neither ~/.claude nor ~/.codex was found.")
		}
		return nil
	},
}
