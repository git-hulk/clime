package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveVersionLikeGoGet(t *testing.T) {
	remote := createTestGitRepo(t, "skills/test-skill", map[string]string{
		"SKILL.md": "---\nname: test-skill\n---\n# Test",
	})
	src := Source{Repo: remote}

	firstSHA := gitIn(t, remote, "rev-parse", "HEAD")
	gitIn(t, remote, "tag", "v1.0.0")
	gitIn(t, remote, "tag", "v1.1.0")
	gitIn(t, remote, "tag", "v1.2.0-rc.1")
	gitIn(t, remote, "tag", "2.0.0") // no "v" prefix: not semver, ignored like Go
	gitIn(t, remote, "branch", "feature-x")

	require.NoError(t, os.WriteFile(filepath.Join(remote, "later.txt"), []byte("later"), 0o644))
	gitIn(t, remote, "add", "-A")
	gitIn(t, remote, "commit", "-m", "second")
	headSHA := gitIn(t, remote, "rev-parse", "HEAD")

	tests := []struct {
		query string
		want  string
	}{
		{"latest", "v1.1.0"}, // stable beats the newer prerelease, non-v tag ignored
		{"", "v1.1.0"},
		{"v1", "v1.1.0"},
		{"v1.0", "v1.0.0"},
		{"v1.2.0-rc.1", "v1.2.0-rc.1"}, // exact tag, even a prerelease
		{"2.0.0", "2.0.0"},             // exact tag, even non-semver
		{"feature-x", firstSHA},        // branch resolves to its head commit
		{headSHA, headSHA},             // full SHA resolves to itself
		{headSHA[:12], headSHA},        // advertised short SHA expands
	}

	for _, tt := range tests {
		got, err := resolveVersion(src, tt.query)
		if !assert.NoError(t, err, "resolveVersion(%q)", tt.query) {
			continue
		}
		assert.Equal(t, tt.want, got)
	}

	for _, query := range []string{"v9", "no-such-ref", "deadbeefdead"} {
		_, err := resolveVersion(src, query)
		assert.Error(t, err)
	}
}

func TestResolveVersionLatestWithoutSemverTags(t *testing.T) {
	remote := createTestGitRepo(t, "skills/test-skill", map[string]string{
		"SKILL.md": "---\nname: test-skill\n---\n# Test",
	})
	gitIn(t, remote, "tag", "release-1") // not semver
	headSHA := gitIn(t, remote, "rev-parse", "HEAD")

	got, err := resolveVersion(Source{Repo: remote}, "latest")
	require.NoError(t, err)
	require.Equal(t, headSHA, got)
}

func TestResolveVersionLatestPrefersPrereleaseOverHead(t *testing.T) {
	remote := createTestGitRepo(t, "skills/test-skill", map[string]string{
		"SKILL.md": "---\nname: test-skill\n---\n# Test",
	})
	gitIn(t, remote, "tag", "v0.1.0-alpha")
	gitIn(t, remote, "tag", "v0.2.0-beta.1")

	got, err := resolveVersion(Source{Repo: remote}, "latest")
	require.NoError(t, err)
	require.Equal(t, "v0.2.0-beta.1", got)
}

func TestResolveVersionFullSHASkipsNetwork(t *testing.T) {
	t.Parallel()

	sha := strings.Repeat("ab12", 10)
	got, err := resolveVersion(Source{Repo: "no-such-owner/no-such-repo"}, sha)
	require.NoError(t, err)
	require.Equal(t, sha, got)
}
