package skill

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakeGitHubCLI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLIME_TEST_GH_LOG", filepath.Join(dir, "calls"))
	t.Setenv("CLIME_TEST_GH_TAGS", filepath.Join(dir, "tags"))
	t.Setenv("CLIME_TEST_GH_BRANCHES", filepath.Join(dir, "branches"))
	t.Setenv("CLIME_TEST_GH_ARCHIVE", filepath.Join(dir, "archive.tar.gz"))
	t.Setenv("CLIME_TEST_GH_HEAD", strings.Repeat("b", 40))
	writeFile(t, filepath.Join(dir, "tags"), "v1.0.0\t"+strings.Repeat("a", 40)+"\nv2.0.0\t"+strings.Repeat("b", 40)+"\n")
	writeFile(t, filepath.Join(dir, "branches"), "main\t"+strings.Repeat("b", 40)+"\n")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$CLIME_TEST_GH_LOG"
if [ "$1" = auth ]; then exit "${CLIME_TEST_GH_AUTH_EXIT:-0}"; fi
if [ "$CLIME_TEST_GH_FAIL" = 1 ]; then echo 'access denied' >&2; exit 1; fi
case "$4" in
  */tags?*) /bin/cat "$CLIME_TEST_GH_TAGS";;
  */branches?*) /bin/cat "$CLIME_TEST_GH_BRANCHES";;
  */commits/HEAD) printf '%s\n' "$CLIME_TEST_GH_HEAD";;
  */tarball/*) /bin/cat "$CLIME_TEST_GH_ARCHIVE";;
  *) echo "unexpected gh call" >&2; exit 1;;
esac
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "git"), []byte("#!/bin/sh\necho 'unexpected git call' >> \"$CLIME_TEST_GH_LOG\"\nexit 99\n"), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func githubTestArchive(t *testing.T, path string, headers []tar.Header) {
	t.Helper()
	file, err := os.Create(path)
	require.NoError(t, err)
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	for _, h := range headers {
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len("# Skill"))
		}
		require.NoError(t, tw.WriteHeader(&h))
		if h.Typeflag == tar.TypeReg {
			_, err := tw.Write([]byte("# Skill"))
			require.NoError(t, err)
		}
	}
	for _, close := range []func() error{tw.Close, gz.Close, file.Close} {
		require.NoError(t, close())
	}
}

func TestGitHubSnapshotUsesAuthenticatedGHAndReusesCache(t *testing.T) {
	dir := fakeGitHubCLI(t)
	githubTestArchive(t, filepath.Join(dir, "archive.tar.gz"), []tar.Header{
		{Name: "owner-repo-sha/skills/alpha/SKILL.md", Typeflag: tar.TypeReg, Mode: 0o644},
		{Name: "owner-repo-sha/skills/alpha/run.sh", Typeflag: tar.TypeReg, Mode: 0o755},
		{Name: "owner-repo-sha/skills/alpha/link", Typeflag: tar.TypeSymlink, Linkname: "SKILL.md"},
	})
	st := newTestStore(t)
	var progress []string
	st.Progress = func(message string) { progress = append(progress, message) }
	src := Source{Repo: "owner/repo"}
	snap, err := st.Snapshot(src)
	require.NoError(t, err)
	require.Equal(t, "latest", snap.Version)
	require.Equal(t, "v2.0.0", snap.revision)
	catalog, err := snap.Catalog()
	require.NoError(t, err)
	require.Len(t, catalog.Skills, 1)
	require.Equal(t, "alpha", catalog.Skills[0].Name)
	require.NotEmpty(t, progress)
	require.Contains(t, strings.Join(progress, "\n"), "KiB")

	info, err := os.Stat(filepath.Join(snap.Dir, "skills/alpha/run.sh"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o755), info.Mode().Perm())

	body, err := os.ReadFile(filepath.Join(snap.Dir, "skills/alpha/link"))
	require.NoError(t, err)
	require.Equal(t, "# Skill", string(body))

	_, err = st.Snapshot(src.WithQuery("v1"))
	require.NoError(t, err)

	calls, err := os.ReadFile(filepath.Join(dir, "calls"))
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(string(calls), "auth status --hostname github.com"))
	require.NotContains(t, string(calls), "unexpected git")
	require.Contains(t, string(calls), "--paginate", "tag lookup must paginate")
	t.Setenv("CLIME_TEST_GH_FAIL", "1")
	progress = nil

	_, err = st.Snapshot(src.WithQuery("v2.0.0"))
	require.NoError(t, err)
	require.Empty(t, progress)

	after, _ := os.ReadFile(filepath.Join(dir, "calls"))
	require.Equal(t, string(calls), string(after), "cache hit must not invoke gh or git")

	_, err = st.Snapshot(src.WithQuery("latest"))
	require.Error(t, err, "authenticated gh failures must be reported")

	failedVersion := strings.Repeat("c", 40)

	_, err = st.Snapshot(src.WithQuery(failedVersion))
	require.ErrorContains(t, err, "access denied")

	require.False(t, dirExists(versionDir(st.repoDir(src), failedVersion)), "failed download must not be cached")
	after, _ = os.ReadFile(filepath.Join(dir, "calls"))
	require.NotContains(t, string(after), "unexpected git", "authenticated gh failure must not fall back to git")
}

func TestGitHubVersionQueries(t *testing.T) {
	dir := fakeGitHubCLI(t)
	for _, tt := range []struct{ query, want string }{
		{"latest", "v2.0.0"}, {"v1", "v1.0.0"}, {"v1.0", "v1.0.0"},
		{"v1.0.0", "v1.0.0"}, {"main", "main"},
		{strings.Repeat("c", 40), strings.Repeat("c", 40)}, {"bbbbbbb", strings.Repeat("b", 40)},
	} {
		got, err := resolveVersion(&githubSource{Source: Source{Repo: "owner/repo"}}, tt.query)
		require.NoError(t, err)
		wantVersion := tt.want
		if tt.query == "latest" {
			wantVersion = "latest"
		}
		require.Equal(t, wantVersion, got.version)
		if tt.query == "main" {
			require.Equal(t, strings.Repeat("b", 40), got.revision)
		} else {
			require.Equal(t, tt.want, got.revision)
		}
	}
	writeFile(t, filepath.Join(dir, "tags"), "")
	got, err := resolveVersion(&githubSource{Source: Source{Repo: "owner/repo"}}, "latest")
	require.NoError(t, err)
	require.Equal(t, "latest", got.version)
	require.Equal(t, strings.Repeat("b", 40), got.revision)
	for _, query := range []string{"missing", "v9", "ddddddd"} {
		_, err := resolveVersion(&githubSource{Source: Source{Repo: "owner/repo"}}, query)
		require.Error(t, err)
	}
	writeFile(t, filepath.Join(dir, "branches"), "one\tabcdef012"+strings.Repeat("a", 31)+"\ntwo\tabcdef012"+strings.Repeat("b", 31)+"\n")

	_, err = resolveVersion(&githubSource{Source: Source{Repo: "owner/repo"}}, "abcdef0")
	require.ErrorContains(t, err, "ambiguous")
}

func TestGitHubSourceForms(t *testing.T) {
	for _, repo := range []string{"owner/repo", "https://github.com/owner/repo.git", "git@github.com:owner/repo.git", "ssh://git@github.com/owner/repo.git"} {
		assert.Equal(t, "owner/repo", (Source{Repo: repo}).githubRepo())
	}
	for _, repo := range []string{t.TempDir(), "file:///tmp/repo", "https://gitlab.com/owner/repo", "https://github.com.evil.test/owner/repo"} {
		assert.Empty(t, (Source{Repo: repo}).githubRepo())
	}
}

func TestGitHubLoggedOutUsesGit(t *testing.T) {
	remote := createTestGitRepo(t, "skills/alpha", map[string]string{"SKILL.md": "# Skill"})
	gitIn(t, remote, "tag", "v1.0.0")
	git, err := exec.LookPath("git")
	require.NoError(t, err)
	dir := fakeGitHubCLI(t)
	t.Setenv("CLIME_TEST_GH_AUTH_EXIT", "1")
	require.NoError(t, os.Remove(filepath.Join(dir, "git")))
	require.NoError(t, os.Symlink(git, filepath.Join(dir, "git")))
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "url.file://"+remote+".insteadOf")
	t.Setenv("GIT_CONFIG_VALUE_0", "https://github.com/owner/repo.git")
	snap, err := newTestStore(t).Snapshot(Source{Repo: "owner/repo"})
	require.NoError(t, err)
	require.Equal(t, "latest", snap.Version)
	require.Equal(t, "v1.0.0", snap.revision)
	calls, _ := os.ReadFile(filepath.Join(dir, "calls"))
	require.NotContains(t, string(calls), "api ")
}

func TestGitHubFailedArchiveDoesNotPublishSnapshot(t *testing.T) {
	dir := fakeGitHubCLI(t)
	for _, name := range []string{"owner-repo-sha/../../escape", "owner-repo-sha/skills/link", "owner-repo-sha/truncated"} {
		header := tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644}
		if strings.HasSuffix(name, "/link") {
			header.Typeflag, header.Linkname = tar.TypeSymlink, "../../../escape"
		}
		githubTestArchive(t, filepath.Join(dir, "archive.tar.gz"), []tar.Header{header})
		if strings.HasSuffix(name, "/truncated") {
			archive := filepath.Join(dir, "archive.tar.gz")
			info, err := os.Stat(archive)
			require.NoError(t, err)
			require.NoError(t, os.Truncate(archive, info.Size()-8))
		}
		st := newTestStore(t)
		src := Source{Repo: "owner/repo", Query: strings.Repeat("a", 40)}

		_, err := st.Snapshot(src)
		require.Error(t, err, "unsafe archive must fail")

		require.False(t, dirExists(versionDir(st.repoDir(src), src.Query)), "failed archive must not be cached")
	}
}
