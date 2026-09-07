package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/git-hulk/clime/internal/prompt"
	"github.com/git-hulk/clime/internal/skill"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallFromPluginSkillsUsesLockedCacheOffline(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	binDir := t.TempDir()
	// No git or gh on PATH: browsing and installing must both use the cache.
	t.Setenv("PATH", binDir)
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "clime-cached"), []byte("#!/bin/sh\necho owner/repo\n"), 0o755))
	cache := filepath.Join(home, ".clime", "sources", "owner", "repo@v1.0.0")
	require.NoError(t, os.MkdirAll(filepath.Join(cache, "alpha"), 0o755))
	for path, content := range map[string]string{
		"skills.yaml":    "skills:\n  - name: beta\n    path: beta\n  - name: alpha\n    path: alpha\n",
		"alpha/SKILL.md": "# cached alpha",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(cache, path), []byte(content), 0o644))
	}
	manifest := &skill.Manifest{Sources: []skill.SourceRecord{{Repo: "owner/repo", Version: "v1.0.0"}}}
	defer stubSkillPrompts(t)()
	multiSelectPrompt = func(config prompt.SelectConfig) ([]int, error) {
		require.Equal(t, []string{"alpha — cached", "beta — cached"}, config.Options)
		return []int{0}, nil
	}
	require.NoError(t, installFromPluginSkills(manifest))
	content, err := os.ReadFile(filepath.Join(home, ".agents", "skills", "alpha", "SKILL.md"))
	require.NoError(t, err)
	require.Equal(t, "# cached alpha", string(content))

	record, _ := manifest.GetSource(skill.Source{Repo: "owner/repo"})
	require.Equal(t, "v1.0.0", record.Version)
}

func TestTryInstallPluginSkillsInstallsFromSource(t *testing.T) {
	// Set up a fake skill repo with a skills.yaml and a SKILL.md.
	repoDir := t.TempDir()
	skillDir := filepath.Join(repoDir, "skills", "test-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	skillsYAML := `skills:
  - name: test-skill
    description: A test skill
    path: skills/test-skill
`
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "skills.yaml"), []byte(skillsYAML), 0o644))

	skillMD := `---
name: test-skill
description: A test skill
---
This is a test skill.
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644))

	// Create a fake plugin binary that outputs the repo dir as the skill source.
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "clime-withskills")
	script := "#!/bin/sh\necho '" + repoDir + "'\n"
	require.NoError(t, os.WriteFile(binPath, []byte(script), 0o755))

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath)

	// Set up a temp home directory so skill installs don't touch the real home.
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Create both agent directories to verify only Claude gets a skill link.
	for _, dir := range []string{".claude", ".codex"} {
		require.NoError(t, os.MkdirAll(filepath.Join(homeDir, dir), 0o755))
	}

	tryInstallPluginSkills("withskills")

	// Verify skill files were installed.
	for _, dir := range []string{".agents", ".claude"} {
		installed := filepath.Join(homeDir, dir, "skills", "test-skill", "SKILL.md")

		_, err := os.Stat(installed)
		assert.NoError(t, err)
	}
	shared := filepath.Join(homeDir, ".agents", "skills", "test-skill")

	got, err := os.Readlink(filepath.Join(homeDir, ".claude", "skills", "test-skill"))
	require.NoError(t, err)
	require.Equal(t, shared, got)

	_, err = os.Lstat(filepath.Join(homeDir, ".codex", "skills"))
	require.ErrorIs(t, err, os.ErrNotExist)

	// Verify skill manifest was updated.
	manifest, err := skill.LoadManifest("")
	require.NoError(t, err)

	_, found := manifest.GetSkill("test-skill")
	assert.True(t, found, "expected test-skill to be in the skill manifest")
}

func TestTryInstallPluginSkillsSkipsAlreadyInstalled(t *testing.T) {
	// Set up a fake skill repo.
	repoDir := t.TempDir()
	skillDir := filepath.Join(repoDir, "skills", "existing-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	skillsYAML := `skills:
  - name: existing-skill
    description: Already installed
    path: skills/existing-skill
`
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "skills.yaml"), []byte(skillsYAML), 0o644))

	skillMD := `---
name: existing-skill
description: Already installed
---
Test.
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644))

	// Create a fake plugin binary.
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "clime-skipskills")
	script := "#!/bin/sh\necho '" + repoDir + "'\n"
	require.NoError(t, os.WriteFile(binPath, []byte(script), 0o755))

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath)

	// Set up temp home with .claude dir.
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".claude"), 0o755))

	// Pre-populate the skill manifest with the skill already installed.
	manifest := &skill.Manifest{
		Skills: []skill.InstalledSkill{
			{Name: "existing-skill", Source: repoDir},
		},
	}
	require.NoError(t, manifest.Save())

	// Run tryInstallPluginSkills — it should skip the already-installed skill.
	tryInstallPluginSkills("skipskills")

	// The skill directory under .claude should NOT exist since it was skipped.
	installed := filepath.Join(homeDir, ".claude", "skills", "existing-skill", "SKILL.md")

	_, err := os.Stat(installed)
	assert.Error(t, err, "expected skill file NOT to be written for already-installed skill")
}
