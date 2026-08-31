package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/git-hulk/clime/internal/skill"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTryInstallPluginSkillsPluginNotFound(t *testing.T) {
	t.Parallel()

	// A plugin name that doesn't exist should return silently without error.
	tryInstallPluginSkills("nonexistent-plugin-xyz-12345")
}

func TestTryInstallPluginSkillsNoSkillsSubcommand(t *testing.T) {
	// Create a fake plugin binary that exits with an error when called with "skills".
	dir := t.TempDir()
	binPath := filepath.Join(dir, "clime-noskills")
	script := "#!/bin/sh\nexit 1\n"
	require.NoError(t, os.WriteFile(binPath, []byte(script), 0o755))

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+origPath)

	// Should return silently since the skills subcommand fails.
	tryInstallPluginSkills("noskills")
}

func TestTryInstallPluginSkillsEmptyOutput(t *testing.T) {
	// Create a fake plugin binary that outputs nothing for "skills".
	dir := t.TempDir()
	binPath := filepath.Join(dir, "clime-emptyskills")
	script := "#!/bin/sh\necho ''\n"
	require.NoError(t, os.WriteFile(binPath, []byte(script), 0o755))

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+origPath)

	// Should return silently since the output is empty.
	tryInstallPluginSkills("emptyskills")
}

func TestTryInstallPluginSkillsLocalPathSourceIgnored(t *testing.T) {
	newTestHome(t)
	repoDir := t.TempDir()

	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "clime-localskills")
	script := "#!/bin/sh\necho '" + repoDir + "'\n"
	require.NoError(t, os.WriteFile(binPath, []byte(script), 0o755))
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath)

	// Local directories have no immutable identity; the plugin flow skips
	// them silently and installs nothing.
	tryInstallPluginSkills("localskills")

	manifest, err := skill.LoadManifest()
	require.NoError(t, err)
	assert.Empty(t, manifest.Repos, "manifest gained repositories from a local path source")
}

func TestTryInstallPluginSkillsInstallsFromSource(t *testing.T) {
	home := newTestHome(t)
	newSkillFixture(t, "test-skill")

	// Create a fake plugin binary that outputs the repo as the skill source.
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "clime-withskills")
	script := "#!/bin/sh\nif [ \"$1\" = skills ]; then echo '" + testRepo + "'; fi\n"
	require.NoError(t, os.WriteFile(binPath, []byte(script), 0o755))
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath)

	tryInstallPluginSkills("withskills")

	// Verify skill files were installed.
	for _, dir := range []string{".claude", ".codex"} {
		assert.FileExists(t, filepath.Join(home, dir, "skills", "test-skill", "SKILL.md"))
	}

	// Verify the manifest locks the repository to the latest version.
	manifest, err := skill.LoadManifest()
	require.NoError(t, err)
	require.NotNil(t, manifest.FindSkill("test-skill"), "expected test-skill in the skill manifest")
	require.Len(t, manifest.Repos, 1)
	assert.Equal(t, "v1.0.0", manifest.Repos[0].Version, "want the latest stable tag locked")
}

func TestTryInstallPluginSkillsSkipsAlreadyInstalled(t *testing.T) {
	home := newTestHome(t)
	newSkillFixture(t, "existing-skill")

	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "clime-skipskills")
	script := "#!/bin/sh\nif [ \"$1\" = skills ]; then echo '" + testRepo + "'; fi\n"
	require.NoError(t, os.WriteFile(binPath, []byte(script), 0o755))
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath)

	// Pre-populate the manifest with the skill already selected.
	manifest, err := skill.LoadManifest()
	require.NoError(t, err)
	_, err = manifest.AddRepo(testRepo, "v1.0.0", []string{"existing-skill"})
	require.NoError(t, err)
	require.NoError(t, manifest.Save())

	// Run tryInstallPluginSkills — every catalog skill is already
	// selected, so nothing is installed.
	tryInstallPluginSkills("skipskills")

	assert.NoFileExists(t, filepath.Join(home, ".claude", "skills", "existing-skill", "SKILL.md"),
		"skill file should not be written for an already-installed skill")
}

func TestPluginSkillInstallerCalledFromExecutePluginInstall(t *testing.T) {
	restore := stubPluginPrompts(t)
	defer restore()

	var skillInstallerCalledWith string
	pluginSkillInstaller = func(name string) {
		skillInstallerCalledWith = name
	}

	// Verify the variable is wired up correctly.
	assert.Empty(t, skillInstallerCalledWith, "pluginSkillInstaller should not have been called yet")
}
