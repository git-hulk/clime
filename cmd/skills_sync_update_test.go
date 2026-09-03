package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/git-hulk/clime/internal/skill"
)

// skillRemote is a git repository used as a versioned skill source. Its
// file:// URL makes clime treat it as a remote rather than a local path, so
// versions resolve and cache exactly as they do for a hosted repository.
type skillRemote struct {
	t   *testing.T
	dir string
	URL string
}

func newSkillRemote(t *testing.T) *skillRemote {
	t.Helper()
	dir := t.TempDir()
	r := &skillRemote{t: t, dir: dir, URL: "file://" + dir}
	r.git("init")
	r.git("config", "user.email", "test@test.com")
	r.git("config", "user.name", "Test")
	return r
}

func (r *skillRemote) git(args ...string) {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// release replaces the catalog with the given skills, writing each skill's
// SKILL.md body, and tags the resulting commit.
func (r *skillRemote) release(tag string, skills map[string]string) {
	r.t.Helper()
	if err := os.RemoveAll(filepath.Join(r.dir, "skills")); err != nil {
		r.t.Fatal(err)
	}
	catalog := "skills:\n"
	for name, body := range skills {
		skillDir := filepath.Join(r.dir, "skills", name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			r.t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
			r.t.Fatal(err)
		}
		catalog += "  - name: " + name + "\n    path: skills/" + name + "\n"
	}
	if err := os.WriteFile(filepath.Join(r.dir, "skills.yaml"), []byte(catalog), 0o644); err != nil {
		r.t.Fatal(err)
	}
	r.git("add", "-A")
	r.git("commit", "-m", tag)
	r.git("tag", tag)
}

func setupSkillsHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home
}

func readInstalledSkill(t *testing.T, home, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".claude", "skills", name, "SKILL.md"))
	if err != nil {
		t.Fatalf("reading installed %s: %v", name, err)
	}
	return string(data)
}

func TestSyncKeepsLockedVersionWhileUpdateFollowsLatest(t *testing.T) {
	home := setupSkillsHome(t)
	remote := newSkillRemote(t)
	remote.release("v1.0.0", map[string]string{"test-skill": "# v1"})

	manifest := &skill.Manifest{
		Skills:  []skill.InstalledSkill{{Name: "test-skill", Source: remote.URL, Path: "skills/test-skill"}},
		Sources: []skill.Source{{Repo: remote.URL, Version: "v1.0.0"}},
	}
	sourceVersion := func() string {
		src, _ := manifest.GetSource(remote.URL)
		return src.Version
	}

	if _, err := syncSource(manifest, remote.URL); err != nil {
		t.Fatalf("syncSource() error = %v", err)
	}
	if got := readInstalledSkill(t, home, "test-skill"); got != "# v1" {
		t.Fatalf("after sync, SKILL.md = %q, want the locked v1.0.0 content", got)
	}

	remote.release("v2.0.0", map[string]string{"test-skill": "# v2"})

	if _, err := syncSource(manifest, remote.URL); err != nil {
		t.Fatalf("syncSource() after upstream release error = %v", err)
	}
	if got := readInstalledSkill(t, home, "test-skill"); got != "# v1" {
		t.Fatalf("sync must not pick up a newer release, SKILL.md = %q", got)
	}
	if got := sourceVersion(); got != "v1.0.0" {
		t.Fatalf("sync changed the locked version to %q", got)
	}

	if _, err := updateSource(manifest, remote.URL); err != nil {
		t.Fatalf("updateSource() error = %v", err)
	}
	if got := readInstalledSkill(t, home, "test-skill"); got != "# v2" {
		t.Fatalf("after update, SKILL.md = %q, want the v2.0.0 content", got)
	}
	if got := sourceVersion(); got != "v2.0.0" {
		t.Fatalf("after update, locked version = %q, want v2.0.0", got)
	}

	if _, err := updateSource(manifest, remote.URL); err != nil {
		t.Fatalf("updateSource() when already at latest error = %v", err)
	}

	if _, err := updateSource(manifest, remote.URL+"@v1.0.0"); err != nil {
		t.Fatalf("updateSource() to an explicit older version error = %v", err)
	}
	if got := readInstalledSkill(t, home, "test-skill"); got != "# v1" {
		t.Fatalf("after pinning v1.0.0, SKILL.md = %q", got)
	}
	if got := sourceVersion(); got != "v1.0.0" {
		t.Fatalf("after pinning v1.0.0, locked version = %q", got)
	}
}

func TestUpdateRefusesWhenCatalogDropsInstalledSkill(t *testing.T) {
	home := setupSkillsHome(t)
	remote := newSkillRemote(t)
	remote.release("v1.0.0", map[string]string{"alpha": "# alpha v1", "beta": "# beta v1"})

	manifest := &skill.Manifest{
		Skills: []skill.InstalledSkill{
			{Name: "alpha", Source: remote.URL, Path: "skills/alpha"},
			{Name: "beta", Source: remote.URL, Path: "skills/beta"},
		},
		Sources: []skill.Source{{Repo: remote.URL, Version: "v1.0.0"}},
	}
	if _, err := syncSource(manifest, remote.URL); err != nil {
		t.Fatalf("syncSource() error = %v", err)
	}

	remote.release("v2.0.0", map[string]string{"alpha": "# alpha v2"})

	_, err := updateSource(manifest, remote.URL)
	if err == nil {
		t.Fatal("updateSource() should fail when the new catalog drops an installed skill")
	}
	if !strings.Contains(err.Error(), "beta") {
		t.Fatalf("error = %v, want it to name the missing skill", err)
	}
	if got := readInstalledSkill(t, home, "alpha"); got != "# alpha v1" {
		t.Fatalf("a refused update must leave targets unchanged, alpha = %q", got)
	}
	if got := readInstalledSkill(t, home, "beta"); got != "# beta v1" {
		t.Fatalf("a refused update must not remove beta, got %q", got)
	}
	if src, _ := manifest.GetSource(remote.URL); src.Version != "v1.0.0" {
		t.Fatalf("a refused update must keep the locked version, got %q", src.Version)
	}
	if _, ok := manifest.GetSkill("beta"); !ok {
		t.Fatal("a refused update must keep beta in the manifest")
	}
}

func TestLockedSourceLeavesLocalPathsUnversioned(t *testing.T) {
	local := t.TempDir()
	manifest := &skill.Manifest{Sources: []skill.Source{
		{Repo: local, Version: "deadbeef"},
		{Repo: "owner/repo", Version: "v1.2.3"},
		{Repo: "owner/unversioned"},
	}}

	if got := lockedSource(manifest, local); got != local {
		t.Fatalf("lockedSource(local) = %q, want the path unchanged", got)
	}
	if got := lockedSource(manifest, "owner/repo"); got != "owner/repo@v1.2.3" {
		t.Fatalf("lockedSource(owner/repo) = %q, want pinned to v1.2.3", got)
	}
	if got := lockedSource(manifest, "owner/unversioned"); got != "owner/unversioned" {
		t.Fatalf("lockedSource(unversioned) = %q, want the repo unchanged", got)
	}
}
