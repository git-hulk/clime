package skill

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureSnapshotCachesAndServesOffline(t *testing.T) {
	setTempHome(t)
	dir := initSkillFixture(t, "a-skill")
	gitCmd(t, dir, "tag", "v1.0.0")
	routeRepos(t, map[string]string{fixtureRepo: dir})
	id, _ := ParseRepo(fixtureRepo)

	first, err := EnsureSnapshot(id, "v1.0.0")
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(first, "skills", "a-skill", "SKILL.md"))
	require.True(t, snapshotReady(first), "snapshot metadata missing")

	// A committed entry is immutable and reused without network access.
	disableGit(t)
	second, err := EnsureSnapshot(id, "v1.0.0")
	require.NoError(t, err, "cached snapshot required the network")
	assert.Equal(t, first, second, "cache dir changed between calls")
}

func TestEnsureSnapshotSupportsMultipleVersions(t *testing.T) {
	setTempHome(t)
	dir := initSkillFixture(t, "a-skill")
	gitCmd(t, dir, "tag", "v1.0.0")
	writeSkillFixtureContent(t, dir, "v2", "a-skill")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-q", "-m", "second")
	gitCmd(t, dir, "tag", "v2.0.0")
	routeRepos(t, map[string]string{fixtureRepo: dir})
	id, _ := ParseRepo(fixtureRepo)

	v1, err := EnsureSnapshot(id, "v1.0.0")
	require.NoError(t, err)
	v2, err := EnsureSnapshot(id, "v2.0.0")
	require.NoError(t, err)
	require.NotEqual(t, v1, v2, "versions share one cache entry")
	assert.FileExists(t, filepath.Join(v1, "skills", "a-skill", "v1.txt"), "v1 snapshot lost its content")
	assert.FileExists(t, filepath.Join(v2, "skills", "a-skill", "v2.txt"), "v2 snapshot missing new content")
}

func TestSafeVersionDistinctForUnsafeNames(t *testing.T) {
	assert.NotEqual(t, safeVersion("release/v1"), safeVersion("release_v1"),
		"distinct versions collide in cache addressing")
	assert.Equal(t, "v1.4.2", safeVersion("v1.4.2"), "plain tag should be kept verbatim")
}
