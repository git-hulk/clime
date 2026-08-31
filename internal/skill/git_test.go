package skill

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fixtureRepo = "test/skills"

func TestResolveVersionLatestSelectsHighestStableSemver(t *testing.T) {
	dir := initSkillFixture(t, "a-skill")
	gitCmd(t, dir, "tag", "v1.0.0")
	gitCmd(t, dir, "tag", "v1.2.0")
	gitCmd(t, dir, "tag", "v1.10.0")
	gitCmd(t, dir, "tag", "v2.0.0-rc.1")
	gitCmd(t, dir, "tag", "banana")
	routeRepos(t, map[string]string{fixtureRepo: dir})

	id, _ := ParseRepo(fixtureRepo)
	for _, spec := range []string{"", "latest"} {
		version, err := ResolveVersion(id, spec)
		require.NoError(t, err, "ResolveVersion(%q)", spec)
		assert.Equal(t, "v1.10.0", version, "ResolveVersion(%q)", spec)
	}
}

func TestResolveVersionLatestWithoutSemverTagsUsesHeadSHA(t *testing.T) {
	dir := initSkillFixture(t, "a-skill")
	gitCmd(t, dir, "tag", "release-1")
	routeRepos(t, map[string]string{fixtureRepo: dir})
	head := gitCmd(t, dir, "rev-parse", "HEAD")

	id, _ := ParseRepo(fixtureRepo)
	version, err := ResolveVersion(id, "latest")
	require.NoError(t, err)
	assert.Equal(t, head, version, "latest should lock to the default branch HEAD")
	assert.True(t, IsFullCommit(version), "version %q is not a full commit SHA", version)
}

func TestResolveVersionBranchLocksToFullSHA(t *testing.T) {
	dir := initSkillFixture(t, "a-skill")
	gitCmd(t, dir, "checkout", "-q", "-b", "dev")
	writeSkillFixtureContent(t, dir, "v2", "a-skill")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-q", "-m", "dev change")
	devHead := gitCmd(t, dir, "rev-parse", "HEAD")
	gitCmd(t, dir, "checkout", "-q", "main")
	routeRepos(t, map[string]string{fixtureRepo: dir})

	id, _ := ParseRepo(fixtureRepo)
	version, err := ResolveVersion(id, "dev")
	require.NoError(t, err)
	assert.Equal(t, devHead, version)
}

func TestResolveVersionShortCommitExpandsToFullSHA(t *testing.T) {
	dir := initSkillFixture(t, "a-skill")
	routeRepos(t, map[string]string{fixtureRepo: dir})
	head := gitCmd(t, dir, "rev-parse", "HEAD")

	id, _ := ParseRepo(fixtureRepo)
	version, err := ResolveVersion(id, head[:8])
	require.NoError(t, err)
	assert.Equal(t, head, version)
}

func TestResolveVersionExplicitTagKeptVerbatim(t *testing.T) {
	dir := initSkillFixture(t, "a-skill")
	gitCmd(t, dir, "tag", "v2.0.0-rc.1")
	routeRepos(t, map[string]string{fixtureRepo: dir})

	id, _ := ParseRepo(fixtureRepo)
	// A prerelease is selected only when requested explicitly.
	version, err := ResolveVersion(id, "v2.0.0-rc.1")
	require.NoError(t, err)
	assert.Equal(t, "v2.0.0-rc.1", version)
}

func TestResolveVersionFullSHAPassesThroughWithoutNetwork(t *testing.T) {
	disableGit(t)
	id, _ := ParseRepo(fixtureRepo)
	sha := strings.Repeat("ab12", 10)
	version, err := ResolveVersion(id, sha)
	require.NoError(t, err)
	assert.Equal(t, sha, version)
}

func TestResolveVersionUnknownSpecFails(t *testing.T) {
	dir := initSkillFixture(t, "a-skill")
	routeRepos(t, map[string]string{fixtureRepo: dir})

	id, _ := ParseRepo(fixtureRepo)
	_, err := ResolveVersion(id, "no-such-thing")
	assert.Error(t, err, "unknown version spec should fail")
}

func TestFetchSnapshotByTagAndBySHA(t *testing.T) {
	dir := initSkillFixture(t, "a-skill")
	gitCmd(t, dir, "tag", "v1.0.0")
	routeRepos(t, map[string]string{fixtureRepo: dir})
	head := gitCmd(t, dir, "rev-parse", "HEAD")
	id, _ := ParseRepo(fixtureRepo)

	for _, version := range []string{"v1.0.0", head} {
		tmp := t.TempDir()
		commit, err := fetchSnapshot(id, version, tmp)
		require.NoError(t, err, "fetchSnapshot(%q)", version)
		assert.Equal(t, head, commit, "fetchSnapshot(%q) commit", version)
		assert.FileExists(t, filepath.Join(tmp, "skills", "a-skill", "SKILL.md"))
		assert.NoDirExists(t, filepath.Join(tmp, ".git"), "snapshot still contains .git metadata")
	}
}

func TestGitErrorsAreCredentialFree(t *testing.T) {
	orig := runGit
	t.Cleanup(func() { runGit = orig })

	// Simulate a git failure whose stderr leaks HTTPS userinfo, as the
	// real runGit sanitizes stderr before wrapping it.
	_, err := orig("", "ls-remote", "https://alice:hunter2@invalid.invalid/acme/skills.git")
	if err == nil {
		t.Skip("expected ls-remote against invalid host to fail")
	}
	assert.NotContains(t, err.Error(), "hunter2", "git error leaks credentials")
	assert.NotContains(t, err.Error(), "alice:", "git error leaks credentials")
}
