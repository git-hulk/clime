package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/git-hulk/clime/internal/prompt"
	"github.com/git-hulk/clime/internal/skill"
	"github.com/stretchr/testify/require"
)

func TestSkillListPagesNavigateAtBoundaries(t *testing.T) {
	for _, count := range []int{1, 10, 11, 20, 21} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			defer stubSkillPrompts(t)()
			var rows [][]string
			for i := 1; i <= count; i++ {
				rows = append(rows, []string{fmt.Sprintf("skill-%02d", i)})
			}
			pageCount := (count-1)/10 + 1
			page, backward := 0, false
			var starts []int
			selectPrompt = func(config prompt.SelectConfig) (int, error) {
				require.Equal(t, "Previous page", config.LeftOption, "left/right arrows must activate previous/next page")
				require.Equal(t, "Next page", config.RightOption, "left/right arrows must activate previous/next page")
				starts = append(starts, page*10)
				require.Equal(t, fmt.Sprintf("Page %d/%d", page+1, pageCount), config.Label)
				choice := "Next page"
				if page == pageCount-1 {
					backward = true
				}
				if backward {
					choice = "Previous page"
				}
				if backward && page == 0 {
					choice = "Done"
				}
				for i, option := range config.Options {
					require.False(t, page == 0 && option == "Previous page")
					require.False(t, page == pageCount-1 && option == "Next page")
					if option == choice {
						if choice == "Next page" {
							page++
						} else if choice == "Previous page" {
							page--
						}
						return i, nil
					}
				}
				require.FailNow(t, "navigation option missing", "option %q", choice)
				return 0, nil
			}
			output := captureStdout(t, func() {
				require.NoError(t, printSkillPages([]string{"NAME"}, rows))
			})
			if pageCount == 1 {
				require.Empty(t, starts, "a single page must not prompt")
				starts = []int{0}
			}
			pages := strings.Split(output, "NAME")[1:]
			require.Len(t, pages, len(starts))
			for i, page := range pages {
				got, want := strings.Count(page, "skill-"), min(10, count-starts[i])
				require.Equal(t, want, got)

				require.Contains(t, page, fmt.Sprintf("skill-%02d", starts[i]+1))
			}
		})
	}
}

func TestSkillListPipedOutputIncludesEverySkillInNameOrder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	manifest := &skill.Manifest{}
	for i := 21; i >= 1; i-- {
		manifest.AddSkill(skill.InstalledSkill{Name: fmt.Sprintf("skill-%02d", i), Source: "owner/repo"})
	}
	require.NoError(t, manifest.Save())
	output := captureStdout(t, func() {
		require.NoError(t, skillsListCmd.RunE(skillsListCmd, nil))
	})
	require.Equal(t, 21, strings.Count(output, "skill-"))
	previous := -1
	for i := 1; i <= 21; i++ {
		name := fmt.Sprintf("skill-%02d", i)
		index := strings.Index(output, name)
		require.Greater(t, index, previous)
		previous = index
	}
}

func TestSkillsCommandsUseSelectedManifest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "skills.yaml")
	require.NoError(t, os.WriteFile(path, []byte("skills: [invalid\n"), 0o644))
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		skillsManifestPath = ""
		skillsCmd.PersistentFlags().Lookup("manifest").Changed = false
	})
	for _, command := range []string{"", "list", "install", "update", "sync", "uninstall"} {
		t.Run(command, func(t *testing.T) {
			args := []string{"skills", "--manifest", path}
			if command != "" {
				args = append(args, command)
			}
			rootCmd.SetArgs(args)
			require.ErrorContains(t, rootCmd.Execute(), "failed to parse "+path)
		})
	}
}

func TestSkillsListAndUninstallWithCustomManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(t.TempDir(), "skills.yaml")
	manifest, err := skill.LoadManifest(path)
	require.NoError(t, err)
	manifest.AddSkill(skill.InstalledSkill{Name: "custom-skill", Source: "owner/repo"})
	manifest.SetSourceVersion(skill.Source{Repo: "owner/repo"}, "v1.2.3")
	require.NoError(t, manifest.Save())
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		skillsManifestPath = ""
		skillsCmd.PersistentFlags().Lookup("manifest").Changed = false
	})
	rootCmd.SetArgs([]string{"skills", "list", "--manifest", path})
	output := captureStdout(t, func() {
		require.NoError(t, rootCmd.Execute())
	})
	require.Contains(t, output, "custom-skill")
	require.Contains(t, output, "v1.2.3")
	rootCmd.SetArgs([]string{"skills", "uninstall", "custom-skill", "--manifest", path})
	captureStdout(t, func() {
		require.NoError(t, rootCmd.Execute())
	})
	manifest, err = skill.LoadManifest(path)
	require.NoError(t, err)
	require.Empty(t, manifest.Skills)
	_, err = os.Stat(filepath.Join(home, ".clime", "skills.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
}
