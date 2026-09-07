package skill

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte("#!/bin/sh\necho 'unexpected git call' >> \"$CLIME_TEST_GH_LOG\"\nexit 99\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func githubTestArchive(t *testing.T, path string, headers []tar.Header) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	for _, h := range headers {
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len("# Skill"))
		}
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte("# Skill")); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, close := range []func() error{tw.Close, gz.Close, file.Close} {
		if err := close(); err != nil {
			t.Fatal(err)
		}
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
	if err != nil {
		t.Fatal(err)
	}
	if snap.Version != "v2.0.0" {
		t.Fatalf("version = %q", snap.Version)
	}
	catalog, err := snap.Catalog()
	if err != nil || len(catalog.Skills) != 1 || catalog.Skills[0].Name != "alpha" {
		t.Fatalf("catalog = %+v, %v", catalog, err)
	}
	if len(progress) == 0 || !strings.Contains(strings.Join(progress, "\n"), "KiB") {
		t.Fatalf("missing download progress: %v", progress)
	}
	if info, err := os.Stat(filepath.Join(snap.Dir, "skills/alpha/run.sh")); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("executable mode not preserved: %v, %v", info, err)
	}
	if body, err := os.ReadFile(filepath.Join(snap.Dir, "skills/alpha/link")); err != nil || string(body) != "# Skill" {
		t.Fatalf("symlink not preserved: %q, %v", body, err)
	}
	if _, err := st.Snapshot(src.WithQuery("v1")); err != nil {
		t.Fatal(err)
	}
	calls, err := os.ReadFile(filepath.Join(dir, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(calls), "auth status --hostname github.com") != 1 || strings.Contains(string(calls), "unexpected git") {
		t.Fatalf("expected one login check and only gh operations: %s", calls)
	}
	if !strings.Contains(string(calls), "--paginate") {
		t.Fatal("tag lookup must paginate")
	}
	t.Setenv("CLIME_TEST_GH_FAIL", "1")
	progress = nil
	if _, err := st.Snapshot(src.WithQuery("v2.0.0")); err != nil || len(progress) != 0 {
		t.Fatalf("offline cache reuse = %v, progress = %v", err, progress)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if string(after) != string(calls) {
		t.Fatal("cache hit must not invoke gh or git")
	}
	if _, err := st.Snapshot(src.WithQuery("latest")); err == nil {
		t.Fatal("authenticated gh failures must be reported")
	}
	failedVersion := strings.Repeat("c", 40)
	if _, err := st.Snapshot(src.WithQuery(failedVersion)); err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("download failure must be reported: %v", err)
	}
	if dirExists(versionDir(st.repoDir(src), failedVersion)) {
		t.Fatal("failed download must not be cached")
	}
	after, _ = os.ReadFile(filepath.Join(dir, "calls"))
	if strings.Contains(string(after), "unexpected git") {
		t.Fatal("authenticated gh failure must not fall back to git")
	}
}

func TestGitHubVersionQueries(t *testing.T) {
	dir := fakeGitHubCLI(t)
	for _, tt := range []struct{ query, want string }{
		{"latest", "v2.0.0"}, {"v1", "v1.0.0"}, {"v1.0", "v1.0.0"},
		{"v1.0.0", "v1.0.0"}, {"main", strings.Repeat("b", 40)},
		{strings.Repeat("c", 40), strings.Repeat("c", 40)}, {"bbbbbbb", strings.Repeat("b", 40)},
	} {
		got, err := resolveVersion(&githubSource{Source: Source{Repo: "owner/repo"}}, tt.query)
		if err != nil || got != tt.want {
			t.Fatalf("%s: got %q, %v; want %q", tt.query, got, err, tt.want)
		}
	}
	writeFile(t, filepath.Join(dir, "tags"), "")
	got, err := resolveVersion(&githubSource{Source: Source{Repo: "owner/repo"}}, "latest")
	if err != nil || got != strings.Repeat("b", 40) {
		t.Fatalf("untagged HEAD = %q, %v", got, err)
	}
	for _, query := range []string{"missing", "v9", "ddddddd"} {
		if _, err := resolveVersion(&githubSource{Source: Source{Repo: "owner/repo"}}, query); err == nil {
			t.Fatalf("unknown query %q must fail", query)
		}
	}
	writeFile(t, filepath.Join(dir, "branches"), "one\tabcdef012"+strings.Repeat("a", 31)+"\ntwo\tabcdef012"+strings.Repeat("b", 31)+"\n")
	if _, err := resolveVersion(&githubSource{Source: Source{Repo: "owner/repo"}}, "abcdef0"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous prefix = %v", err)
	}
}

func TestGitHubSourceForms(t *testing.T) {
	for _, repo := range []string{"owner/repo", "https://github.com/owner/repo.git", "git@github.com:owner/repo.git", "ssh://git@github.com/owner/repo.git"} {
		if got := (Source{Repo: repo}).githubRepo(); got != "owner/repo" {
			t.Errorf("%s => %q", repo, got)
		}
	}
	for _, repo := range []string{t.TempDir(), "file:///tmp/repo", "https://gitlab.com/owner/repo", "https://github.com.evil.test/owner/repo"} {
		if got := (Source{Repo: repo}).githubRepo(); got != "" {
			t.Errorf("%s incorrectly treated as GitHub: %q", repo, got)
		}
	}
}

func TestGitHubLoggedOutUsesGit(t *testing.T) {
	remote := createTestGitRepo(t, "skills/alpha", map[string]string{"SKILL.md": "# Skill"})
	gitIn(t, remote, "tag", "v1.0.0")
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir := fakeGitHubCLI(t)
	t.Setenv("CLIME_TEST_GH_AUTH_EXIT", "1")
	if err := os.Remove(filepath.Join(dir, "git")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(git, filepath.Join(dir, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "url.file://"+remote+".insteadOf")
	t.Setenv("GIT_CONFIG_VALUE_0", "https://github.com/owner/repo.git")
	snap, err := newTestStore(t).Snapshot(Source{Repo: "owner/repo"})
	if err != nil || snap.Version != "v1.0.0" {
		t.Fatalf("logged-out Git fetch = %+v, %v", snap, err)
	}
	calls, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if strings.Contains(string(calls), "api ") {
		t.Fatalf("logged-out gh must not make API calls: %s", calls)
	}
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
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Truncate(archive, info.Size()-8); err != nil {
				t.Fatal(err)
			}
		}
		st := newTestStore(t)
		src := Source{Repo: "owner/repo", Query: strings.Repeat("a", 40)}
		if _, err := st.Snapshot(src); err == nil {
			t.Fatal("unsafe archive must fail")
		}
		if dirExists(versionDir(st.repoDir(src), src.Query)) {
			t.Fatal("failed archive must not be cached")
		}
	}
}
