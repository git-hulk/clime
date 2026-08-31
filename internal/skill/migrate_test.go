package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupLegacyState installs a legacy skills.yaml and a legacy mutable source
// checkout under ~/.clime/sources/<repo>, returning the manifest path.
func setupLegacyState(t *testing.T, legacyYAML string, sources map[string][]string) string {
	t.Helper()
	home := setTempHome(t, ".claude")
	climeDir := filepath.Join(home, ".clime")
	require.NoError(t, os.MkdirAll(climeDir, 0o755))
	for repo, skills := range sources {
		srcDir := filepath.Join(climeDir, "sources", repo)
		require.NoError(t, os.MkdirAll(srcDir, 0o755))
		gitCmd(t, srcDir, "init", "-q", "-b", "main")
		writeSkillFixtureContent(t, srcDir, "v1", skills...)
		gitCmd(t, srcDir, "add", "-A")
		gitCmd(t, srcDir, "commit", "-q", "-m", "legacy content")
	}
	path := filepath.Join(climeDir, "skills.yaml")
	require.NoError(t, os.WriteFile(path, []byte(legacyYAML), 0o644))
	return path
}

func TestMigrateLegacyLocksCachedHEAD(t *testing.T) {
	legacy := `skills:
  - name: a-skill
    description: A
    source: acme/skills
    path: skills/a-skill
  - name: b-skill
    description: B
    source: acme/skills
    path: skills/b-skill
sources:
  - acme/skills
`
	path := setupLegacyState(t, legacy, map[string][]string{"acme/skills": {"a-skill", "b-skill"}})
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	head := gitCmd(t, filepath.Join(home, ".clime", "sources", "acme", "skills"), "rev-parse", "HEAD")

	// Migration must not contact any remote.
	disableGit(t)
	runGitOrig := runGit
	runGit = func(dir string, args ...string) (string, error) {
		if dir != "" && args[0] == "rev-parse" {
			return head + "\n", nil
		}
		return runGitOrig(dir, args...)
	}
	t.Cleanup(func() { runGit = runGitOrig })

	m, err := LoadManifest()
	require.NoError(t, err)
	require.Len(t, m.Repos, 1)
	r := m.Repos[0]
	assert.Equal(t, "acme/skills", r.Key)
	assert.Equal(t, head, r.Version, "migration should lock to the checkout HEAD")
	assert.Equal(t, []string{"a-skill", "b-skill"}, r.Skills)

	// The original file is preserved as skills.yaml.bak.
	bak, err := os.ReadFile(path + ".bak")
	require.NoError(t, err)
	assert.Equal(t, legacy, string(bak), "backup does not match the original legacy manifest")

	// A cache entry exists for the detected commit.
	id, err := ParseRepo("acme/skills")
	require.NoError(t, err)
	dir, err := SnapshotDir(id, head)
	require.NoError(t, err)
	require.True(t, snapshotReady(dir), "migration did not create an immutable cache entry")

	// The migrated state syncs offline.
	_, err = Reconcile(m, nil, false)
	require.NoError(t, err, "post-migration offline sync failed")
}

func TestMigrateLegacyAbortsOnLocalDirectorySource(t *testing.T) {
	legacy := `skills:
  - name: local-skill
    source: /tmp/local-skills
    path: skills/local-skill
`
	path := setupLegacyState(t, legacy, nil)

	_, err := LoadManifest()
	require.ErrorContains(t, err, "local directories are not supported",
		"migration should abort for a local directory source")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, legacy, string(data), "legacy manifest was modified by a failed migration")
	assert.NoFileExists(t, path+".bak", "failed migration left a .bak file")
}

func TestMigrateLegacyAbortsWhenCheckoutMissing(t *testing.T) {
	legacy := `skills:
  - name: a-skill
    source: acme/skills
    path: skills/a-skill
`
	path := setupLegacyState(t, legacy, nil)

	_, err := LoadManifest()
	require.ErrorContains(t, err, "no local checkout")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, legacy, string(data), "legacy manifest was modified by a failed migration")
}

func TestMigrateLegacyAbortsOnNameConflict(t *testing.T) {
	legacy := `skills:
  - name: shared-skill
    source: acme/skills
    path: skills/shared-skill
  - name: shared-skill
    source: other/skills
    path: skills/shared-skill
`
	path := setupLegacyState(t, legacy, map[string][]string{
		"acme/skills":  {"shared-skill"},
		"other/skills": {"shared-skill"},
	})

	_, err := LoadManifest()
	require.ErrorContains(t, err, "selected by both")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, legacy, string(data), "legacy manifest was modified by a failed migration")
}
