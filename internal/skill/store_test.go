package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		if got := st.repoDir(Source{Repo: tt.repo}); got != tt.want {
			t.Errorf("repoDir(%q) = %q, want %q", tt.repo, got, tt.want)
		}
	}
}

func TestSnapshotLocalSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	st := newTestStore(t)

	snap, err := st.Snapshot(Source{Repo: dir})
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	want, _ := filepath.Abs(dir)
	if snap.Dir != want {
		t.Fatalf("Snapshot().Dir = %q, want %q", snap.Dir, want)
	}
	if snap.Version != "" {
		t.Fatalf("Snapshot().Version = %q, want empty for a local path", snap.Version)
	}
}

func TestSnapshotRejectsVersionForLocalPath(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	if _, err := st.Snapshot(Source{Repo: t.TempDir(), Query: "v1.0.0"}); err == nil {
		t.Fatal("Snapshot() should reject a version query on a local path")
	}
}

func TestSnapshotResolvesLatestAndCaches(t *testing.T) {
	remote := createTestGitRepo(t, "skills/test-skill", map[string]string{
		"SKILL.md": "---\nname: test-skill\n---\n# v1",
	})
	gitIn(t, remote, "tag", "v1.0.0")

	if err := os.WriteFile(filepath.Join(remote, "later.txt"), []byte("later"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, remote, "add", "-A")
	gitIn(t, remote, "commit", "-m", "untagged head")

	st := newTestStore(t)
	src := Source{Repo: "file://" + remote}

	snap, err := st.Snapshot(src)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if snap.Version != "v1.0.0" {
		t.Fatalf("resolved version = %q, want %q", snap.Version, "v1.0.0")
	}
	if !strings.HasSuffix(snap.Dir, "@v1.0.0") {
		t.Fatalf("cache dir = %q, want it keyed by the resolved version v1.0.0", snap.Dir)
	}
	if got := checkoutVersion(t, snap.Dir); got != "v1.0.0" {
		t.Fatalf("checkout version = %q, want %q", got, "v1.0.0")
	}
	if _, err := os.Stat(filepath.Join(snap.Dir, "later.txt")); err == nil {
		t.Fatal("checkout should be the latest tag, not the untagged HEAD")
	}

	// A second call reuses the resolved snapshot.
	snap2, err := st.Snapshot(src.WithQuery("latest"))
	if err != nil {
		t.Fatalf("Snapshot(latest) error = %v", err)
	}
	if snap2.Dir != snap.Dir || snap2.Version != snap.Version {
		t.Fatalf("second call = (%q, %q), want cached (%q, %q)", snap2.Dir, snap2.Version, snap.Dir, snap.Version)
	}

	// A concrete cached version needs no network: point the source at a
	// repo that no longer exists and ask for the pinned version.
	gone := Source{Repo: src.Repo + "-gone"}
	cached := versionDir(st.repoDir(gone), "v1.0.0")
	if err := os.MkdirAll(cached, 0o755); err != nil {
		t.Fatal(err)
	}
	snap3, err := st.Snapshot(gone.WithQuery("v1.0.0"))
	if err != nil {
		t.Fatalf("Snapshot(cached pin) error = %v", err)
	}
	if snap3.Dir != cached {
		t.Fatalf("Snapshot(cached pin).Dir = %q, want %q", snap3.Dir, cached)
	}
}

func TestCloneAtVersion(t *testing.T) {
	remote := createTestGitRepo(t, "skills/test-skill", map[string]string{
		"SKILL.md": "---\nname: test-skill\n---\n# v1",
	})
	src := Source{Repo: remote}

	gitIn(t, remote, "tag", "v1.0.0")
	taggedSHA := gitIn(t, remote, "rev-parse", "HEAD")

	// A later commit so the tag no longer points at the default branch HEAD.
	if err := os.WriteFile(filepath.Join(remote, "later.txt"), []byte("later"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, remote, "add", "-A")
	gitIn(t, remote, "commit", "-m", "second")
	gitIn(t, remote, "config", "uploadpack.allowAnySHA1InWant", "true")

	t.Run("tag", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "clone")
		if err := cloneAtVersion(src, "v1.0.0", dir); err != nil {
			t.Fatalf("cloneAtVersion() error = %v", err)
		}
		if got := checkoutVersion(t, dir); got != "v1.0.0" {
			t.Fatalf("checkout version = %q, want %q", got, "v1.0.0")
		}
		if _, err := os.Stat(filepath.Join(dir, "later.txt")); err == nil {
			t.Fatal("file from a later commit should not exist in the tagged checkout")
		}
	})

	t.Run("commit SHA", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "clone")
		if err := cloneAtVersion(src, taggedSHA, dir); err != nil {
			t.Fatalf("cloneAtVersion() error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "later.txt")); err == nil {
			t.Fatal("file from a later commit should not exist in the pinned checkout")
		}
	})

	t.Run("unknown version", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "clone")
		if err := cloneAtVersion(src, "v9.9.9", dir); err == nil {
			t.Fatal("cloneAtVersion() should fail for an unknown version")
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("failed clone should not leave a cache directory behind")
		}
	})
}

func TestStoreRemove(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	src := Source{Repo: "test-owner/test-repo"}
	dir := versionDir(st.repoDir(src), "v1.0.0")
	if err := os.MkdirAll(st.repoDir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := st.Remove(src); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := os.Stat(st.repoDir(src)); !os.IsNotExist(err) {
		t.Fatal("source cache still exists after Remove")
	}
}

func TestSnapshotSkillFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "my-skill", "SKILL.md"), "# Skill")
	writeFile(t, filepath.Join(dir, "my-skill", "sub", "nested.txt"), "nested")

	snap := &Snapshot{Source: Source{Repo: dir}, Dir: dir}
	files, err := snap.SkillFiles("my-skill")
	if err != nil {
		t.Fatalf("SkillFiles() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if string(files["SKILL.md"]) != "# Skill" {
		t.Fatalf("unexpected SKILL.md: %q", files["SKILL.md"])
	}
	if string(files[filepath.Join("sub", "nested.txt")]) != "nested" {
		t.Fatalf("unexpected nested.txt: %q", files[filepath.Join("sub", "nested.txt")])
	}

	if _, err := snap.SkillFiles("does-not-exist"); err == nil {
		t.Fatal("SkillFiles() should fail for a missing path")
	}
}
