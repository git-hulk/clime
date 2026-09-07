package cmd

import (
	"os"
	"path/filepath"
	"testing"
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
					if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
						t.Fatal(err)
					}
				}
			}
			oldContent, oldYAML := SkillContent, AgentYAML
			SkillContent, AgentYAML = "# Clime", "display_name: Clime\n"
			t.Cleanup(func() { SkillContent, AgentYAML = oldContent, oldYAML })

			if err := installSkillCmd.RunE(installSkillCmd, nil); err != nil {
				t.Fatalf("install skill: %v", err)
			}
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
					if err != nil || string(data) != want {
						t.Fatalf("%s/%s = %q, %v, want %q", dir, rel, data, err, want)
					}
				}
			}
			if withAgents {
				shared := filepath.Join(home, ".agents", "skills", skillDirName)
				if got, err := os.Readlink(filepath.Join(home, ".claude", "skills", skillDirName)); err != nil || got != shared {
					t.Fatalf("Claude link = %q, %v, want %q", got, err, shared)
				}
			}
			if _, err := os.Lstat(filepath.Join(home, ".codex", "skills")); !os.IsNotExist(err) {
				t.Fatalf("install must not create ~/.codex/skills: %v", err)
			}
		})
	}
}
