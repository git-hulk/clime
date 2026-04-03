package cmd

import (
	"fmt"
	"os"
	"path/filepath"

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
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}

		installed := 0
		targets := []string{".claude", ".codex"}
		for _, dir := range targets {
			base := filepath.Join(home, dir)
			info, err := os.Stat(base)
			if err != nil || !info.IsDir() {
				terminal.Warningf("Skipping %s (directory not found)", base)
				continue
			}

			skillDir := filepath.Join(base, "skills", skillDirName)
			if err := os.MkdirAll(skillDir, 0o755); err != nil {
				return fmt.Errorf("failed to create %s: %w", skillDir, err)
			}

			dest := filepath.Join(skillDir, skillFileName)
			if err := os.WriteFile(dest, []byte(SkillContent), 0o644); err != nil {
				return fmt.Errorf("failed to write %s: %w", dest, err)
			}

			// Write agents/openai.yaml for Codex skill discovery.
			if dir == ".codex" {
				agentsDir := filepath.Join(skillDir, "agents")
				if err := os.MkdirAll(agentsDir, 0o755); err != nil {
					return fmt.Errorf("failed to create %s: %w", agentsDir, err)
				}
				yamlDest := filepath.Join(agentsDir, "openai.yaml")
				if err := os.WriteFile(yamlDest, []byte(AgentYAML), 0o644); err != nil {
					return fmt.Errorf("failed to write %s: %w", yamlDest, err)
				}
			}

			terminal.Successf("Installed skill to %s", dest)
			installed++
		}

		if installed == 0 {
			terminal.Warning("No skill directories were installed. Neither ~/.claude nor ~/.codex was found.")
		}
		return nil
	},
}
