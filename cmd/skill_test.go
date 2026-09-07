package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInstallBundledSkillUsesSharedDirectory(t *testing.T) {
	for _, withAgents := range []bool{false, true} {
		name := "empty home"
		if withAgents {
			name = "Claude and Codex"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			if withAgents {
				for _, dir := range []string{".claude", ".codex"} {
					require.NoError(t, os.MkdirAll(filepath.Join(home, dir), 0o755))
				}
			}
			oldContent, oldYAML := SkillContent, AgentYAML
			SkillContent, AgentYAML = "# Clime", "display_name: Clime\n"
			t.Cleanup(func() { SkillContent, AgentYAML = oldContent, oldYAML })

			require.NoError(t, installSkillCmd.RunE(installSkillCmd, nil))
			dirs := []string{".agents"}
			if withAgents {
				dirs = append(dirs, ".claude")
			}
			for _, dir := range dirs {
				for rel, want := range map[string]string{
					"SKILL.md":                             SkillContent,
					filepath.Join("agents", "openai.yaml"): AgentYAML,
				} {
					data, err := os.ReadFile(filepath.Join(home, dir, "skills", skillDirName, rel))
					require.NoError(t, err)
					require.Equal(t, want, string(data))
				}
			}
			if withAgents {
				shared := filepath.Join(home, ".agents", "skills", skillDirName)

				got, err := os.Readlink(filepath.Join(home, ".claude", "skills", skillDirName))
				require.NoError(t, err)
				require.Equal(t, shared, got)
			}

			_, err := os.Lstat(filepath.Join(home, ".codex", "skills"))
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}
