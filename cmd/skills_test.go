package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/git-hulk/clime/internal/prompt"
	"github.com/git-hulk/clime/internal/skill"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRepo = "test/skills"

func gitFixtureCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed:\n%s", args, out)
	return strings.TrimSpace(string(out))
}

// newSkillFixture creates a Git skill repository with a skills.yaml catalog,
// tags it v1.0.0, and routes the canonical URL of testRepo to it.
func newSkillFixture(t *testing.T, skills ...string) string {
	t.Helper()
	dir := t.TempDir()
	gitFixtureCmd(t, dir, "init", "-q", "-b", "main")
	gitFixtureCmd(t, dir, "config", "uploadpack.allowAnySHA1InWant", "true")

	var catalog strings.Builder
	catalog.WriteString("skills:\n")
	for _, name := range skills {
		fmt.Fprintf(&catalog, "  - name: %s\n    description: Test skill %s\n    path: skills/%s\n", name, name, name)
		skillDir := filepath.Join(dir, "skills", name)
		require.NoError(t, os.MkdirAll(skillDir, 0o755))
		md := fmt.Sprintf("---\nname: %s\ndescription: Test skill %s\n---\nContent\n", name, name)
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(md), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "skills.yaml"), []byte(catalog.String()), 0o644))
	gitFixtureCmd(t, dir, "add", "-A")
	gitFixtureCmd(t, dir, "commit", "-q", "-m", "initial")
	gitFixtureCmd(t, dir, "tag", "v1.0.0")

	id, err := skill.ParseRepo(testRepo)
	require.NoError(t, err)
	cfg := fmt.Sprintf("[url %q]\n\tinsteadOf = %s\n", dir, id.CloneURL())
	cfgPath := filepath.Join(t.TempDir(), "gitconfig")
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0o644))
	t.Setenv("GIT_CONFIG_GLOBAL", cfgPath)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	return dir
}

func newTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, dir := range []string{".claude", ".codex"} {
		require.NoError(t, os.MkdirAll(filepath.Join(home, dir), 0o755))
	}
	return home
}

func stubSkillPrompts(t *testing.T) func() {
	t.Helper()
	origSelect := selectPrompt
	origMultiSelect := multiSelectPrompt
	origInput := inputPrompt
	return func() {
		selectPrompt = origSelect
		multiSelectPrompt = origMultiSelect
		inputPrompt = origInput
	}
}

func TestInstallFromRepoArgInstallsSelectedSkills(t *testing.T) {
	home := newTestHome(t)
	newSkillFixture(t, "a-skill", "b-skill")

	restore := stubSkillPrompts(t)
	defer restore()
	multiSelectPrompt = func(config prompt.SelectConfig) ([]int, error) {
		return []int{0, 1}, nil
	}

	manifest, err := skill.LoadManifest()
	require.NoError(t, err)
	require.NoError(t, installFromRepoArg(manifest, testRepo, false))

	for _, base := range []string{".claude", ".codex"} {
		for _, name := range []string{"a-skill", "b-skill"} {
			assert.FileExists(t, filepath.Join(home, base, "skills", name, "SKILL.md"))
		}
	}

	reloaded, err := skill.LoadManifest()
	require.NoError(t, err)
	require.Len(t, reloaded.Repos, 1)
	r := reloaded.Repos[0]
	assert.Equal(t, testRepo, r.Key)
	assert.Equal(t, "v1.0.0", r.Version, "want the latest stable tag locked")
}

func TestInstallExplicitVersionSuffix(t *testing.T) {
	newTestHome(t)
	dir := newSkillFixture(t, "a-skill")
	gitFixtureCmd(t, dir, "commit", "-q", "--allow-empty", "-m", "post-tag")
	gitFixtureCmd(t, dir, "tag", "v2.0.0")

	restore := stubSkillPrompts(t)
	defer restore()
	multiSelectPrompt = func(config prompt.SelectConfig) ([]int, error) {
		return []int{0}, nil
	}

	manifest, err := skill.LoadManifest()
	require.NoError(t, err)
	require.NoError(t, installFromRepoArg(manifest, testRepo+"@v1.0.0", false))

	reloaded, err := skill.LoadManifest()
	require.NoError(t, err)
	require.Len(t, reloaded.Repos, 1)
	assert.Equal(t, "v1.0.0", reloaded.Repos[0].Version, "want the explicitly requested version")
}

func TestInstallLocalPathReturnsUnsupportedError(t *testing.T) {
	newTestHome(t)
	manifest, err := skill.LoadManifest()
	require.NoError(t, err)

	err = installFromRepoArg(manifest, t.TempDir(), false)
	assert.ErrorIs(t, err, skill.ErrLocalPathUnsupported)

	reloaded, err := skill.LoadManifest()
	require.NoError(t, err)
	assert.Empty(t, reloaded.Repos, "manifest gained an entry from a rejected local path")
}

func TestUpdateCommandMovesToLatestPreservingSelection(t *testing.T) {
	home := newTestHome(t)
	dir := newSkillFixture(t, "a-skill", "b-skill")

	restore := stubSkillPrompts(t)
	defer restore()
	multiSelectPrompt = func(config prompt.SelectConfig) ([]int, error) {
		return []int{0}, nil // only a-skill
	}
	manifest, err := skill.LoadManifest()
	require.NoError(t, err)
	require.NoError(t, installFromRepoArg(manifest, testRepo, false))

	// Publish v1.1.0 with changed content.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "skills", "a-skill", "extra.txt"), []byte("x"), 0o644))
	gitFixtureCmd(t, dir, "add", "-A")
	gitFixtureCmd(t, dir, "commit", "-q", "-m", "v1.1.0")
	gitFixtureCmd(t, dir, "tag", "v1.1.0")

	require.NoError(t, skillsUpdateCmd.RunE(skillsUpdateCmd, nil))

	reloaded, err := skill.LoadManifest()
	require.NoError(t, err)
	require.Len(t, reloaded.Repos, 1)
	r := reloaded.Repos[0]
	assert.Equal(t, "v1.1.0", r.Version)
	assert.Equal(t, []string{"a-skill"}, r.Skills, "selection changed")
	assert.FileExists(t, filepath.Join(home, ".claude", "skills", "a-skill", "extra.txt"), "updated content missing")
	assert.NoDirExists(t, filepath.Join(home, ".claude", "skills", "b-skill"),
		"update installed a skill that was never selected")
}

func TestUninstallLastSkillRemovesRepositoryEntry(t *testing.T) {
	home := newTestHome(t)
	newSkillFixture(t, "a-skill")

	restore := stubSkillPrompts(t)
	defer restore()
	multiSelectPrompt = func(config prompt.SelectConfig) ([]int, error) {
		return []int{0}, nil
	}
	manifest, err := skill.LoadManifest()
	require.NoError(t, err)
	require.NoError(t, installFromRepoArg(manifest, testRepo, false))

	manifest, err = skill.LoadManifest()
	require.NoError(t, err)
	require.NoError(t, uninstallSkills(manifest, []string{"a-skill"}))

	for _, base := range []string{".claude", ".codex"} {
		assert.NoDirExists(t, filepath.Join(home, base, "skills", "a-skill"), "a-skill still present in %s", base)
	}
	reloaded, err := skill.LoadManifest()
	require.NoError(t, err)
	assert.Empty(t, reloaded.Repos, "repository entry should be removed with its final skill")
}

func TestUninstallUnknownSkillFails(t *testing.T) {
	newTestHome(t)
	manifest, err := skill.LoadManifest()
	require.NoError(t, err)
	assert.Error(t, uninstallSkills(manifest, []string{"missing"}))
}

func TestSelectInstallCandidates(t *testing.T) {
	newTestHome(t)
	manifest, err := skill.LoadManifest()
	require.NoError(t, err)
	id, err := skill.ParseRepo(testRepo)
	require.NoError(t, err)
	otherID, err := skill.ParseRepo("other/repo")
	require.NoError(t, err)
	_, err = manifest.AddRepo(testRepo, "v1.0.0", []string{"installed-skill"})
	require.NoError(t, err)
	_, err = manifest.AddRepo("other/repo", "v1.0.0", []string{"foreign-skill"})
	require.NoError(t, err)

	catalog := &skill.Catalog{Skills: []skill.SkillEntry{
		{Name: "installed-skill", Path: "skills/installed-skill"},
		{Name: "new-skill", Path: "skills/new-skill"},
		{Name: "foreign-skill", Path: "skills/foreign-skill"},
	}}

	got := selectInstallCandidates(catalog, manifest, id, false)
	require.Len(t, got, 1, "want only new-skill offered")
	assert.Equal(t, "new-skill", got[0].name)

	forced := selectInstallCandidates(catalog, manifest, id, true)
	require.Len(t, forced, 2, "want installed-skill and new-skill offered")
	assert.Equal(t, "installed-skill", forced[0].name)
	assert.Contains(t, forced[0].label, "(reinstall)")

	// A skill owned by another repository is never offered.
	fromOther := selectInstallCandidates(catalog, manifest, otherID, true)
	for _, c := range fromOther {
		assert.NotEqual(t, "installed-skill", c.name, "skill owned by another repository was offered")
	}
}

func TestSyncCommandRestoresFromCacheOffline(t *testing.T) {
	home := newTestHome(t)
	newSkillFixture(t, "a-skill")

	restore := stubSkillPrompts(t)
	defer restore()
	multiSelectPrompt = func(config prompt.SelectConfig) ([]int, error) {
		return []int{0}, nil
	}
	manifest, err := skill.LoadManifest()
	require.NoError(t, err)
	require.NoError(t, installFromRepoArg(manifest, testRepo, false))

	// Remove the installed skill and break Git entirely: sync must
	// reconcile from the immutable cache alone.
	require.NoError(t, os.RemoveAll(filepath.Join(home, ".claude", "skills", "a-skill")))
	t.Setenv("PATH", t.TempDir())

	require.NoError(t, skillsSyncCmd.RunE(skillsSyncCmd, nil))
	assert.FileExists(t, filepath.Join(home, ".claude", "skills", "a-skill", "SKILL.md"),
		"offline sync did not restore the skill")
}

func TestPurgeCommandRemovesUnreferencedEntries(t *testing.T) {
	newTestHome(t)
	dir := newSkillFixture(t, "a-skill")
	gitFixtureCmd(t, dir, "commit", "-q", "--allow-empty", "-m", "second")
	gitFixtureCmd(t, dir, "tag", "v2.0.0")

	restore := stubSkillPrompts(t)
	defer restore()
	multiSelectPrompt = func(config prompt.SelectConfig) ([]int, error) {
		return []int{0}, nil
	}
	manifest, err := skill.LoadManifest()
	require.NoError(t, err)
	require.NoError(t, installFromRepoArg(manifest, testRepo+"@v1.0.0", false))

	// Cache an extra version the manifest does not reference.
	id, err := skill.ParseRepo(testRepo)
	require.NoError(t, err)
	unreferenced, err := skill.EnsureSnapshot(id, "v2.0.0")
	require.NoError(t, err)
	referenced, err := skill.SnapshotDir(id, "v1.0.0")
	require.NoError(t, err)

	require.NoError(t, skillsPurgeCmd.RunE(skillsPurgeCmd, nil))
	assert.NoDirExists(t, unreferenced, "purge kept an unreferenced cache entry")
	assert.DirExists(t, referenced, "purge removed a referenced cache entry")
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()

	defer func() {
		os.Stdout = origStdout
	}()

	fn()

	require.NoError(t, w.Close())
	output := <-done
	require.NoError(t, r.Close())
	return output
}

func TestInteractiveUninstallEscKeepsMenuOpen(t *testing.T) {
	newTestHome(t)
	manifest, err := skill.LoadManifest()
	require.NoError(t, err)
	_, err = manifest.AddRepo(testRepo, "v1.0.0", []string{"alpha"})
	require.NoError(t, err)

	restore := stubSkillPrompts(t)
	defer restore()

	calls := 0
	multiSelectPrompt = func(config prompt.SelectConfig) ([]int, error) {
		calls++
		if calls == 1 {
			return nil, prompt.ErrBack
		}
		return nil, nil
	}

	output := captureStdout(t, func() {
		require.NoError(t, interactiveUninstall(manifest))
	})
	assert.True(t, strings.HasPrefix(output, "\n") && !strings.HasPrefix(output, "\n\n"),
		"stdout = %q, want a single leading spacer line", output)
	assert.Equal(t, 2, calls, "multiSelectPrompt calls")
}

func TestInteractiveUninstallInterruptPropagates(t *testing.T) {
	newTestHome(t)
	manifest, err := skill.LoadManifest()
	require.NoError(t, err)
	_, err = manifest.AddRepo(testRepo, "v1.0.0", []string{"alpha"})
	require.NoError(t, err)

	restore := stubSkillPrompts(t)
	defer restore()

	multiSelectPrompt = func(config prompt.SelectConfig) ([]int, error) {
		return nil, prompt.ErrInterrupted
	}

	assert.ErrorIs(t, interactiveUninstall(manifest), prompt.ErrInterrupted)
}
