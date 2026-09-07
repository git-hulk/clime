package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectTargets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	targets, err := DetectTargets()
	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.Equal(t, "agents", targets[0].Name)

	require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude"), 0o755))
	targets, err = DetectTargets()
	require.NoError(t, err)
	require.Len(t, targets, 2)
	require.Equal(t, "agents", targets[0].Name)
	require.Equal(t, "claude", targets[1].Name)

	require.NoError(t, os.MkdirAll(filepath.Join(home, ".codex"), 0o755))
	targets, err = DetectTargets()
	require.NoError(t, err)
	require.Len(t, targets, 2)
	require.Equal(t, "agents", targets[0].Name)
	require.Equal(t, "claude", targets[1].Name)
}

func TestTargetInstallAndRemove(t *testing.T) {
	target := Target{Name: "agents", Dir: filepath.Join(t.TempDir(), ".agents")}
	require.NoError(t, os.MkdirAll(target.Dir, 0o755))

	files := map[string][]byte{
		"SKILL.md":                       []byte("# Skill"),
		"helper.sh":                      []byte("#!/bin/bash\necho hello"),
		filepath.Join("sub", "nest.txt"): []byte("nested"),
	}
	require.NoError(t, target.Install("test-skill", files))

	for rel, want := range files {
		got, err := os.ReadFile(filepath.Join(target.Dir, "skills", "test-skill", rel))
		require.NoError(t, err)
		require.Equal(t, string(want), string(got))
	}

	removed, err := target.Remove("test-skill")
	require.NoError(t, err)
	require.True(t, removed, "Remove() = false, want true for an installed skill")

	_, err = os.Stat(filepath.Join(target.Dir, "skills", "test-skill"))
	require.ErrorIs(t, err, os.ErrNotExist, "skill directory still exists after Remove")

	removed, err = target.Remove("test-skill")
	require.NoError(t, err)
	require.False(t, removed, "Remove() = true for a skill that is not installed")
}

func TestClaudeInstallLinksSharedSkill(t *testing.T) {
	for _, existing := range []string{"absent", "directory", "shared link", "dangling link", "other link"} {
		t.Run(existing, func(t *testing.T) {
			home := t.TempDir()
			shared := Target{Name: "agents", Dir: filepath.Join(home, ".agents")}
			claude := Target{Name: "claude", Dir: filepath.Join(home, ".claude")}
			files := map[string][]byte{"SKILL.md": []byte("# Shared")}
			require.NoError(t, shared.Install("test-skill", files))
			link := claude.skillDir("test-skill")
			require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
			other := filepath.Join(home, "other")
			switch existing {
			case "directory":
				writeFile(t, filepath.Join(link, "SKILL.md"), "# Old")
			case "shared link", "dangling link", "other link":
				dest := shared.skillDir("test-skill")
				if existing != "shared link" {
					dest = other
				}
				if existing == "other link" {
					writeFile(t, filepath.Join(other, "SKILL.md"), "# Other")
				}
				require.NoError(t, os.Symlink(dest, link))
			}

			require.NoError(t, claude.Install("test-skill", files))

			got, err := os.Readlink(link)
			require.NoError(t, err)
			require.Equal(t, shared.skillDir("test-skill"), got)

			files["SKILL.md"] = []byte("# Updated")
			require.NoError(t, shared.Install("test-skill", files))
			require.Equal(t, "# Updated", readInstalledSkill(t, home, "test-skill"))
			if existing == "other link" {
				data, err := os.ReadFile(filepath.Join(other, "SKILL.md"))
				require.NoError(t, err)
				require.Equal(t, "# Other", string(data))
			}

			removed, err := claude.Remove("test-skill")
			require.NoError(t, err)
			require.True(t, removed)

			_, err = os.Stat(shared.skillDir("test-skill"))
			require.NoError(t, err)
		})
	}
}
