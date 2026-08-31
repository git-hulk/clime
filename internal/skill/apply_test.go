package skill

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// snapshotTree reads every file under root into a map for byte-for-byte
// comparison of target state.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[rel] = string(data)
		return nil
	})
	if !os.IsNotExist(err) {
		require.NoError(t, err)
	}
	return files
}

// setupInstalled builds a fixture repo with tags v1.0.0/v2.0.0, installs
// v1.0.0 of the given skills into both agent targets, and returns the home
// directory, fixture directory, and loaded manifest.
func setupInstalled(t *testing.T, skills ...string) (home, fixture string, m *Manifest) {
	t.Helper()
	home = setTempHome(t, ".claude", ".codex")
	fixture = initSkillFixture(t, skills...)
	gitCmd(t, fixture, "tag", "v1.0.0")
	writeSkillFixtureContent(t, fixture, "v2", skills...)
	gitCmd(t, fixture, "add", "-A")
	gitCmd(t, fixture, "commit", "-q", "-m", "v2")
	gitCmd(t, fixture, "tag", "v2.0.0")
	routeRepos(t, map[string]string{fixtureRepo: fixture})

	m, err := LoadManifest()
	require.NoError(t, err)
	_, err = m.AddRepo(fixtureRepo, "v1.0.0", skills)
	require.NoError(t, err)
	targets, err := Reconcile(m, nil, true)
	require.NoError(t, err)
	require.Len(t, targets, 2, "want claude and codex targets")
	return home, fixture, m
}

func TestReconcileInstallsIntoBothTargets(t *testing.T) {
	home, _, _ := setupInstalled(t, "a-skill", "b-skill")
	for _, base := range []string{".claude", ".codex"} {
		for _, name := range []string{"a-skill", "b-skill"} {
			assert.FileExists(t, filepath.Join(home, base, "skills", name, "SKILL.md"))
		}
		// No staging or backup leftovers after commit.
		for _, dir := range []string{stagingDirName, backupDirName} {
			assert.NoDirExists(t, filepath.Join(home, base, "skills", dir), "leftover directory")
		}
	}

	// The manifest was saved.
	m, err := LoadManifest()
	require.NoError(t, err)
	require.Len(t, m.Repos, 1)
	assert.Equal(t, "v1.0.0", m.Repos[0].Version)
}

func TestUpdateReplacesDirectoryRemovingStaleFiles(t *testing.T) {
	home, _, m := setupInstalled(t, "a-skill")
	old := filepath.Join(home, ".claude", "skills", "a-skill", "v1.txt")
	require.FileExists(t, old, "v1 marker missing before update")

	m.SetVersion(m.Repos[0], "v2.0.0")
	_, err := Reconcile(m, nil, true)
	require.NoError(t, err)

	assert.NoFileExists(t, old, "file deleted upstream survived the update")
	for _, base := range []string{".claude", ".codex"} {
		assert.FileExists(t, filepath.Join(home, base, "skills", "a-skill", "v2.txt"), "new file missing in %s", base)
	}
}

func TestReconcileSyncWorksOfflineFromCache(t *testing.T) {
	home, _, m := setupInstalled(t, "a-skill")
	// Wipe the installed targets, disable Git, and sync from cache only.
	for _, base := range []string{".claude", ".codex"} {
		require.NoError(t, os.RemoveAll(filepath.Join(home, base, "skills", "a-skill")))
	}
	disableGit(t)

	_, err := Reconcile(m, nil, false)
	require.NoError(t, err, "offline sync failed")
	for _, base := range []string{".claude", ".codex"} {
		assert.FileExists(t, filepath.Join(home, base, "skills", "a-skill", "SKILL.md"),
			"offline sync did not restore %s", base)
	}
}

func TestUpdateFailsWhenCatalogDropsSelectedSkill(t *testing.T) {
	home, fixture, m := setupInstalled(t, "a-skill", "b-skill")
	// v3 removes b-skill from the catalog.
	writeSkillFixtureContent(t, fixture, "v3", "a-skill")
	gitCmd(t, fixture, "rm", "-q", "-r", "skills/b-skill")
	gitCmd(t, fixture, "add", "-A")
	gitCmd(t, fixture, "commit", "-q", "-m", "drop b-skill")
	gitCmd(t, fixture, "tag", "v3.0.0")

	manifestFile := filepath.Join(home, ".clime", "skills.yaml")
	beforeManifest, err := os.ReadFile(manifestFile)
	require.NoError(t, err)
	beforeTargets := snapshotTree(t, filepath.Join(home, ".claude", "skills"))

	m.SetVersion(m.Repos[0], "v3.0.0")
	_, err = Reconcile(m, nil, true)
	require.ErrorContains(t, err, `skill "b-skill" does not exist`)

	afterManifest, err := os.ReadFile(manifestFile)
	require.NoError(t, err)
	assert.Equal(t, string(beforeManifest), string(afterManifest), "manifest changed after failed update")
	assert.Equal(t, beforeTargets, snapshotTree(t, filepath.Join(home, ".claude", "skills")),
		"targets changed after failed update")
}

// failNthRename wraps renameOp to fail on the nth matching call.
func failNthRename(t *testing.T, n int, match func(src, dst string) bool) {
	t.Helper()
	orig := renameOp
	count := 0
	renameOp = func(src, dst string) error {
		if match(src, dst) {
			count++
			if count == n {
				return errors.New("injected rename failure")
			}
		}
		return orig(src, dst)
	}
	t.Cleanup(func() { renameOp = orig })
}

func TestApplyFailureDuringSecondTargetRestoresEverything(t *testing.T) {
	home, _, m := setupInstalled(t, "a-skill")
	claudeBefore := snapshotTree(t, filepath.Join(home, ".claude", "skills"))
	codexBefore := snapshotTree(t, filepath.Join(home, ".codex", "skills"))
	manifestFile := filepath.Join(home, ".clime", "skills.yaml")
	manifestBefore, err := os.ReadFile(manifestFile)
	require.NoError(t, err)

	// Fail the install rename inside the second target (.codex).
	failNthRename(t, 1, func(src, dst string) bool {
		return strings.Contains(dst, filepath.Join(".codex", "skills", "a-skill")) &&
			strings.Contains(src, stagingDirName)
	})

	m.SetVersion(m.Repos[0], "v2.0.0")
	_, err = Reconcile(m, nil, true)
	require.Error(t, err, "expected injected failure")
	var partial *PartialStateError
	require.False(t, errors.As(err, &partial), "restoration should have succeeded, got %v", err)

	assert.Equal(t, claudeBefore, snapshotTree(t, filepath.Join(home, ".claude", "skills")), ".claude after rollback")
	assert.Equal(t, codexBefore, snapshotTree(t, filepath.Join(home, ".codex", "skills")), ".codex after rollback")
	manifestAfter, err := os.ReadFile(manifestFile)
	require.NoError(t, err)
	assert.Equal(t, string(manifestBefore), string(manifestAfter), "manifest changed after rolled-back apply")
}

func TestManifestSaveFailureRestoresTargets(t *testing.T) {
	home, _, m := setupInstalled(t, "a-skill")
	claudeBefore := snapshotTree(t, filepath.Join(home, ".claude", "skills"))
	manifestFile := filepath.Join(home, ".clime", "skills.yaml")
	manifestBefore, err := os.ReadFile(manifestFile)
	require.NoError(t, err)

	// Fail the manifest's temp-file rename.
	failNthRename(t, 1, func(src, dst string) bool {
		return strings.HasSuffix(dst, "skills.yaml")
	})

	m.SetVersion(m.Repos[0], "v2.0.0")
	_, err = Reconcile(m, nil, true)
	require.Error(t, err, "expected manifest save failure")

	assert.Equal(t, claudeBefore, snapshotTree(t, filepath.Join(home, ".claude", "skills")),
		".claude after manifest save failure")
	manifestAfter, err := os.ReadFile(manifestFile)
	require.NoError(t, err)
	assert.Equal(t, string(manifestBefore), string(manifestAfter), "manifest content changed despite failed save")
}

func TestRestoreFailureReportsPartialStateWithBackupPaths(t *testing.T) {
	home, _, m := setupInstalled(t, "a-skill")

	// First failure aborts the apply inside .codex; the rollback rename of
	// .claude's backup then also fails.
	orig := renameOp
	renameOp = func(src, dst string) error {
		if strings.Contains(dst, filepath.Join(".codex", "skills", "a-skill")) && strings.Contains(src, stagingDirName) {
			return errors.New("injected apply failure")
		}
		if strings.Contains(src, backupDirName) {
			return errors.New("injected restore failure")
		}
		return orig(src, dst)
	}
	t.Cleanup(func() { renameOp = orig })

	m.SetVersion(m.Repos[0], "v2.0.0")
	_, err := Reconcile(m, nil, true)
	var partial *PartialStateError
	require.ErrorAs(t, err, &partial)
	require.NotEmpty(t, partial.BackupPaths, "PartialStateError has no backup paths")

	found := false
	for _, backup := range partial.BackupPaths {
		if _, statErr := os.Stat(filepath.Join(backup, "a-skill")); statErr == nil {
			found = true
		}
	}
	assert.True(t, found, "no retained backup contains the replaced skill; paths = %v (home %s)", partial.BackupPaths, home)
}

func TestReconcileRemovesUninstalledSkills(t *testing.T) {
	home, _, m := setupInstalled(t, "a-skill", "b-skill")
	_, ok := m.RemoveSkill("b-skill")
	require.True(t, ok, "RemoveSkill failed")
	_, err := Reconcile(m, []string{"b-skill"}, true)
	require.NoError(t, err)

	for _, base := range []string{".claude", ".codex"} {
		assert.NoDirExists(t, filepath.Join(home, base, "skills", "b-skill"), "b-skill still installed in %s", base)
		assert.FileExists(t, filepath.Join(home, base, "skills", "a-skill", "SKILL.md"), "a-skill was removed from %s", base)
	}
}

func TestValidationFailureNeverInvokesGit(t *testing.T) {
	setTempHome(t, ".claude")
	disableGit(t)

	path := filepath.Join(t.TempDir(), "skills.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`acme/agent-skills:
  version: v1.0.0
  skills:
    - shared-skill
github.com/acme/agent-skills:
  version: v1.0.0
  skills:
    - shared-skill
`), 0o644))
	m, err := LoadManifestFrom(path)
	require.NoError(t, err)

	// disableGit makes any Git invocation fail loudly; a validation error
	// must be reported instead, proving the runner was never reached.
	_, err = Reconcile(m, nil, false)
	require.ErrorContains(t, err, "identify the same repository",
		"want the alias conflict before any git call")
}
