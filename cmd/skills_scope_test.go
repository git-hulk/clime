package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/git-hulk/clime/internal/prompt"
	"github.com/git-hulk/clime/internal/skill"
	"github.com/stretchr/testify/require"
)

func TestSkillsCommandsUseManifestScope(t *testing.T) {
	for _, scope := range []string{"default", "global", "nested global", "relative global", "project", "relative project", "similar prefix"} {
		t.Run(scope, func(t *testing.T) {
			home := t.TempDir()
			project := filepath.Join(home, "project")
			if scope == "similar prefix" {
				project = filepath.Join(home, ".clime-project")
			}
			t.Setenv("HOME", home)
			t.Chdir(home)
			defer stubSkillPrompts(t)()
			t.Cleanup(func() {
				rootCmd.SetArgs(nil)
				skillsManifestPath = ""
				skillsCmd.PersistentFlags().Lookup("manifest").Changed = false
			})
			multiSelectPrompt = func(prompt.SelectConfig) ([]int, error) {
				return []int{0}, nil
			}

			path := ""
			baseDir, untouchedDir := home, project
			switch scope {
			case "global":
				path = filepath.Join(home, ".clime", "skills.yaml")
			case "nested global":
				path = filepath.Join(home, ".clime", "team", "skills.yaml")
			case "relative global":
				path = filepath.Join(".clime", "skills.yaml")
			case "project", "relative project", "similar prefix":
				path = filepath.Join(project, "skills.yaml")
				baseDir, untouchedDir = project, home
				if scope == "relative project" {
					path = filepath.Join("project", "skills.yaml")
				}
			}
			if baseDir == home {
				require.NoError(t, os.MkdirAll(filepath.Join(baseDir, ".claude"), 0o755))
			}
			for _, target := range []string{".agents", ".claude"} {
				dir := filepath.Join(untouchedDir, target, "skills", "alpha")
				require.NoError(t, os.MkdirAll(dir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Untouched"), 0o644))
			}

			sourceDir := t.TempDir()
			sourceSkill := filepath.Join(sourceDir, "skills", "alpha", "SKILL.md")
			require.NoError(t, os.MkdirAll(filepath.Dir(sourceSkill), 0o755))
			for _, command := range []string{"install", "sync", "update", "uninstall"} {
				content := "# " + command
				require.NoError(t, os.WriteFile(sourceSkill, []byte(content), 0o644))
				args := []string{"skills", command}
				if command == "install" {
					args = append(args, sourceDir)
				} else if command == "uninstall" {
					args = append(args, "alpha")
				}
				if path != "" {
					args = append(args, "--manifest", path)
				}
				rootCmd.SetArgs(args)
				captureStdout(t, func() {
					require.NoError(t, rootCmd.Execute())
				})

				for _, target := range []string{".agents", ".claude"} {
					dir := filepath.Join(baseDir, target, "skills", "alpha")
					if command == "uninstall" {
						_, err := os.Lstat(dir)
						require.ErrorIs(t, err, os.ErrNotExist)
					} else {
						data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
						require.NoError(t, err)
						require.Equal(t, content, string(data))
						if target == ".claude" {
							link, err := os.Readlink(dir)
							require.NoError(t, err)
							require.Equal(t, filepath.Join(baseDir, ".agents", "skills", "alpha"), link)
						}
					}
					data, err := os.ReadFile(filepath.Join(untouchedDir, target, "skills", "alpha", "SKILL.md"))
					require.NoError(t, err)
					require.Equal(t, "# Untouched", string(data))
				}
			}
			manifest, err := skill.LoadManifest(path)
			require.NoError(t, err)
			require.Empty(t, manifest.Skills)
		})
	}
}
