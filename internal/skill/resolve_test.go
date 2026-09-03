package skill

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitIn runs a git command in dir and returns its trimmed output.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func TestResolveVersionLikeGoGet(t *testing.T) {
	remote := createTestGitRepo(t, "skills/test-skill", map[string]string{
		"SKILL.md": "---\nname: test-skill\n---\n# Test",
	})

	firstSHA := gitIn(t, remote, "rev-parse", "HEAD")
	gitIn(t, remote, "tag", "v1.0.0")
	gitIn(t, remote, "tag", "v1.1.0")
	gitIn(t, remote, "tag", "v1.2.0-rc.1")
	gitIn(t, remote, "tag", "2.0.0") // no "v" prefix: not semver, ignored like Go
	gitIn(t, remote, "branch", "feature-x")

	if err := os.WriteFile(filepath.Join(remote, "later.txt"), []byte("later"), 0o644); err != nil {
		t.Fatal(err)
	}
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
		got, err := ResolveVersion(remote, tt.query)
		if err != nil {
			t.Errorf("ResolveVersion(%q) error = %v", tt.query, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ResolveVersion(%q) = %q, want %q", tt.query, got, tt.want)
		}
	}

	for _, query := range []string{"v9", "no-such-ref", "deadbeefdead"} {
		if got, err := ResolveVersion(remote, query); err == nil {
			t.Errorf("ResolveVersion(%q) = %q, want error", query, got)
		}
	}
}

func TestResolveVersionLatestWithoutSemverTags(t *testing.T) {
	remote := createTestGitRepo(t, "skills/test-skill", map[string]string{
		"SKILL.md": "---\nname: test-skill\n---\n# Test",
	})
	gitIn(t, remote, "tag", "release-1") // not semver
	headSHA := gitIn(t, remote, "rev-parse", "HEAD")

	got, err := ResolveVersion(remote, "latest")
	if err != nil {
		t.Fatalf("ResolveVersion(latest) error = %v", err)
	}
	if got != headSHA {
		t.Fatalf("ResolveVersion(latest) = %q, want default branch HEAD %q", got, headSHA)
	}
}

func TestResolveVersionLatestPrefersPrereleaseOverHead(t *testing.T) {
	remote := createTestGitRepo(t, "skills/test-skill", map[string]string{
		"SKILL.md": "---\nname: test-skill\n---\n# Test",
	})
	gitIn(t, remote, "tag", "v0.1.0-alpha")
	gitIn(t, remote, "tag", "v0.2.0-beta.1")

	got, err := ResolveVersion(remote, "latest")
	if err != nil {
		t.Fatalf("ResolveVersion(latest) error = %v", err)
	}
	if got != "v0.2.0-beta.1" {
		t.Fatalf("ResolveVersion(latest) = %q, want highest prerelease %q", got, "v0.2.0-beta.1")
	}
}

func TestResolveVersionFullSHASkipsNetwork(t *testing.T) {
	t.Parallel()

	sha := strings.Repeat("ab12", 10)
	got, err := ResolveVersion("no-such-owner/no-such-repo", sha)
	if err != nil {
		t.Fatalf("ResolveVersion() error = %v", err)
	}
	if got != sha {
		t.Fatalf("ResolveVersion() = %q, want %q", got, sha)
	}
}

func TestPrepareRemoteRepoDirDefaultsToLatest(t *testing.T) {
	remote := createTestGitRepo(t, "skills/test-skill", map[string]string{
		"SKILL.md": "---\nname: test-skill\n---\n# v1",
	})
	gitIn(t, remote, "tag", "v1.0.0")

	if err := os.WriteFile(filepath.Join(remote, "later.txt"), []byte("later"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, remote, "add", "-A")
	gitIn(t, remote, "commit", "-m", "untagged head")

	t.Setenv("HOME", t.TempDir())

	dir, version, cleanup, err := prepareRemoteRepoDir(remote, "")
	if err != nil {
		t.Fatalf("prepareRemoteRepoDir() error = %v", err)
	}
	defer cleanup()

	if version != "v1.0.0" {
		t.Fatalf("resolved version = %q, want %q", version, "v1.0.0")
	}
	if !strings.HasSuffix(dir, "@v1.0.0") {
		t.Fatalf("cache dir = %q, want it keyed by the resolved version v1.0.0", dir)
	}
	if got := RepoVersion(dir); got != "v1.0.0" {
		t.Fatalf("RepoVersion() = %q, want %q", got, "v1.0.0")
	}
	if _, err := os.Stat(filepath.Join(dir, "later.txt")); err == nil {
		t.Fatal("checkout should be the latest tag, not the untagged HEAD")
	}

	// A second call reuses the resolved snapshot.
	dir2, version2, cleanup2, err := prepareRemoteRepoDir(remote, "latest")
	if err != nil {
		t.Fatalf("prepareRemoteRepoDir(latest) error = %v", err)
	}
	defer cleanup2()
	if dir2 != dir || version2 != version {
		t.Fatalf("second call = (%q, %q), want cached (%q, %q)", dir2, version2, dir, version)
	}
}
