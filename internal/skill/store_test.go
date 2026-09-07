package skill

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return &Store{Root: t.TempDir()}
}

func TestStoreRepoDir(t *testing.T) {
	t.Parallel()

	st := &Store{Root: "/root"}
	tests := []struct {
		repo string
		want string
	}{
		{"owner/repo", filepath.Join("/root", "owner", "repo")},
		{"https://github.com/owner/repo.git", filepath.Join("/root", "github.com", "owner", "repo")},
		{"git@github.com:owner/repo.git", filepath.Join("/root", "github.com", "owner", "repo")},
		{"http://example.com/foo/bar.git", filepath.Join("/root", "example.com", "foo", "bar")},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, st.repoDir(Source{Repo: tt.repo}))
	}
}

func TestSnapshotLocalSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	st := newTestStore(t)

	snap, err := st.Snapshot(Source{Repo: dir})
	require.NoError(t, err)
	want, _ := filepath.Abs(dir)
	require.Equal(t, want, snap.Dir)
	require.Empty(t, snap.Version)
}

func TestSnapshotReportsFetchProgressAndKeepsCacheHitsSilent(t *testing.T) {
	remote := createTestGitRepo(t, "skills/test-skill", map[string]string{"SKILL.md": "# Skill"})
	gitIn(t, remote, "tag", "v1.0.0")
	sha := gitIn(t, remote, "rev-parse", "HEAD")
	for _, version := range []string{"v1.0.0", sha} {
		t.Run(version, func(t *testing.T) {
			st := newTestStore(t)
			var messages []string
			st.Progress = func(message string) { messages = append(messages, message) }
			src := Source{Repo: "file://" + remote, Query: version}

			_, err := st.Snapshot(src)
			require.NoError(t, err)

			require.Contains(t, strings.Join(messages, "\n"), "Counting objects: 100%")
			messages = nil

			_, err = st.Snapshot(src)
			require.NoError(t, err)

			require.Empty(t, messages)
			_, err = st.Snapshot(src.WithQuery(strings.Repeat("f", 40)))
			require.ErrorContains(t, err, "fatal:")
		})
	}
}

func TestGitProgressStreamsPartialLines(t *testing.T) {
	var messages []string
	p := &gitProgress{report: func(message string) { messages = append(messages, message) }}
	p.Write([]byte("Receiving obj"))
	require.Empty(t, messages, "partial line must wait for a delimiter")
	p.Write([]byte("ects: 50%\r\nReceiving objects: 100%\r"))
	require.Equal(t, "Receiving objects: 50%\nReceiving objects: 100%", strings.Join(messages, "\n"))
	p.Write([]byte("last message"))
	p.flush()
	require.Len(t, messages, 3)
	require.Equal(t, "last message", messages[2])
	require.Equal(t, "Receiving objects: 50%\r\nReceiving objects: 100%\rlast message", p.output.String())
}

func TestSnapshotRejectsVersionForLocalPath(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)

	_, err := st.Snapshot(Source{Repo: t.TempDir(), Query: "v1.0.0"})
	require.Error(t, err, "Snapshot() should reject a version query on a local path")
}

func TestSnapshotResolvesLatestAndCaches(t *testing.T) {
	remote := createTestGitRepo(t, "skills/test-skill", map[string]string{
		"SKILL.md": "---\nname: test-skill\n---\n# v1",
	})
	gitIn(t, remote, "tag", "v1.0.0")

	require.NoError(t, os.WriteFile(filepath.Join(remote, "later.txt"), []byte("later"), 0o644))
	gitIn(t, remote, "add", "-A")
	gitIn(t, remote, "commit", "-m", "untagged head")

	st := newTestStore(t)
	src := Source{Repo: "file://" + remote}

	snap, err := st.Snapshot(src)
	require.NoError(t, err)
	require.Equal(t, "v1.0.0", snap.Version)
	require.True(t, strings.HasSuffix(snap.Dir, "@v1.0.0"))
	require.Equal(t, "v1.0.0", checkoutVersion(t, snap.Dir))

	_, err = os.Stat(filepath.Join(snap.Dir, "later.txt"))
	require.Error(t, err, "checkout should be the latest tag, not the untagged HEAD")

	// A second call reuses the resolved snapshot.
	snap2, err := st.Snapshot(src.WithQuery("latest"))
	require.NoError(t, err)
	require.Equal(t, snap.Dir, snap2.Dir)
	require.Equal(t, snap.Version, snap2.Version)

	// A concrete cached version needs no network: point the source at a
	// repo that no longer exists and ask for the pinned version.
	gone := Source{Repo: src.Repo + "-gone"}
	cached := versionDir(st.repoDir(gone), "v1.0.0")
	require.NoError(t, os.MkdirAll(cached, 0o755))
	snap3, err := st.Snapshot(gone.WithQuery("v1.0.0"))
	require.NoError(t, err)
	require.Equal(t, cached, snap3.Dir)
}

func TestCloneAtVersion(t *testing.T) {
	remote := createTestGitRepo(t, "skills/test-skill", map[string]string{
		"SKILL.md": "---\nname: test-skill\n---\n# v1",
	})
	src := Source{Repo: remote}

	gitIn(t, remote, "tag", "v1.0.0")
	taggedSHA := gitIn(t, remote, "rev-parse", "HEAD")

	// A later commit so the tag no longer points at the default branch HEAD.
	require.NoError(t, os.WriteFile(filepath.Join(remote, "later.txt"), []byte("later"), 0o644))
	gitIn(t, remote, "add", "-A")
	gitIn(t, remote, "commit", "-m", "second")
	headSHA := gitIn(t, remote, "rev-parse", "HEAD")
	gitIn(t, remote, "config", "uploadpack.allowAnySHA1InWant", "true")

	t.Run("tag", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "clone")
		require.NoError(t, cloneAtVersion(src, "v1.0.0", dir, nil))
		require.Equal(t, "v1.0.0", checkoutVersion(t, dir))

		_, err := os.Stat(filepath.Join(dir, "later.txt"))
		require.Error(t, err, "file from a later commit should not exist in the tagged checkout")
	})

	t.Run("commit SHA", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "clone")
		require.NoError(t, cloneAtVersion(src, taggedSHA, dir, nil))

		_, err := os.Stat(filepath.Join(dir, "later.txt"))
		require.Error(t, err, "file from a later commit should not exist in the pinned checkout")

		require.Equal(t, taggedSHA, gitIn(t, dir, "rev-parse", "HEAD"))
		cmd := exec.Command("git", "cat-file", "-e", headSHA)
		cmd.Dir = dir
		require.Error(t, cmd.Run(), "fetching a pinned commit must not download the unrelated default branch HEAD")
	})

	t.Run("unknown version", func(t *testing.T) {
		for _, version := range []string{"v9.9.9", strings.Repeat("a", 40)} {
			dir := filepath.Join(t.TempDir(), "clone")
			require.Error(t, cloneAtVersion(src, version, dir, nil))

			_, err := os.Stat(dir)
			require.ErrorIs(t, err, os.ErrNotExist)
		}
	})
}

func TestSnapshotSkillFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "my-skill", "SKILL.md"), "# Skill")
	writeFile(t, filepath.Join(dir, "my-skill", "sub", "nested.txt"), "nested")

	snap := &Snapshot{Source: Source{Repo: dir}, Dir: dir}
	files, err := snap.SkillFiles("my-skill")
	require.NoError(t, err)
	require.Len(t, files, 2)
	require.Equal(t, "# Skill", string(files["SKILL.md"]))
	require.Equal(t, "nested", string(files[filepath.Join("sub", "nested.txt")]))

	_, err = snap.SkillFiles("does-not-exist")
	require.Error(t, err, "SkillFiles() should fail for a missing path")
}
