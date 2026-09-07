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
	Short: "Install the clime-cli skill into ~/.agents/skills with a Claude symlink",
	RunE: func(cmd *cobra.Command, args []string) error {
		targets, err := skill.Targets()
		if err != nil {
			return err
		}

		for _, t := range targets {
			if t.Name != "agents" && !t.Exists() {
				terminal.Warningf("Skipping %s (directory not found)", t.Dir)
				continue
			}

			files := map[string][]byte{skillFileName: []byte(SkillContent)}
			// Include the bundled agent metadata in the shared skill.
			if t.Name == "agents" {
				files[filepath.Join("agents", "openai.yaml")] = []byte(AgentYAML)
			}
			if err := t.Install(skillDirName, files); err != nil {
				return fmt.Errorf("failed to install skill to %s: %w", t.Dir, err)
			}

			terminal.Successf("Installed skill to %s", filepath.Join(t.Dir, "skills", skillDirName, skillFileName))
		}
		return nil
	},
}
