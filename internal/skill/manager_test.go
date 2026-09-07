package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	gitIn(t, dir, "init")
	gitIn(t, dir, "config", "user.email", "test@test.com")
	gitIn(t, dir, "config", "user.name", "Test")
	return r
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
		writeFile(r.t, filepath.Join(r.dir, "skills", name, "SKILL.md"), body)
		catalog += "  - name: " + name + "\n    path: skills/" + name + "\n"
	}
	writeFile(r.t, filepath.Join(r.dir, "skills.yaml"), catalog)
	gitIn(r.t, r.dir, "add", "-A")
	gitIn(r.t, r.dir, "commit", "-m", tag)
	gitIn(r.t, r.dir, "tag", tag)
}

// newTestManager builds a Manager over a temp home with ~/.claude present.
func newTestManager(t *testing.T, manifest *Manifest) (*Manager, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	targets, err := DetectTargets()
	if err != nil {
		t.Fatal(err)
	}
	return &Manager{
		Manifest: manifest,
		Store:    &Store{Root: filepath.Join(home, ".clime", "sources")},
		Targets:  targets,
	}, home
}

func readInstalledSkill(t *testing.T, home, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".claude", "skills", name, "SKILL.md"))
	if err != nil {
		t.Fatalf("reading installed %s: %v", name, err)
	}
	return string(data)
}

func TestManagerInstallEndToEnd(t *testing.T) {
	repoDir := t.TempDir()
	writeFile(t, filepath.Join(repoDir, "skills.yaml"), `skills:
  - name: alpha
    description: Alpha skill
    path: skills/alpha
`)
	writeFile(t, filepath.Join(repoDir, "skills", "alpha", "SKILL.md"), "# Alpha")
	writeFile(t, filepath.Join(repoDir, "skills", "alpha", "extra.txt"), "extra")

	mgr, home := newTestManager(t, &Manifest{})
	src, err := ParseSource(repoDir)
	if err != nil {
		t.Fatalf("ParseSource() error = %v", err)
	}

	snap, catalog, err := mgr.Fetch(src)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(catalog.Skills) != 1 {
		t.Fatalf("catalog = %+v, want 1 skill", catalog.Skills)
	}

	n, err := mgr.Install(snap, catalog.Skills)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if n != 1 {
		t.Fatalf("Install() = %d, want 1", n)
	}

	if got := readInstalledSkill(t, home, "alpha"); got != "# Alpha" {
		t.Fatalf("SKILL.md = %q", got)
	}
	extra, err := os.ReadFile(filepath.Join(home, ".claude", "skills", "alpha", "extra.txt"))
	if err != nil || string(extra) != "extra" {
		t.Fatalf("extra.txt = %q, %v", extra, err)
	}

	installed, ok := mgr.Manifest.GetSkill("alpha")
	if !ok || installed.Source != repoDir || installed.Path != "skills/alpha" {
		t.Fatalf("manifest entry = %+v", installed)
	}
	if record, ok := mgr.Manifest.GetSource(src); ok && record.Version != "" {
		t.Fatalf("local source must not record a version, got %q", record.Version)
	}

	removed, err := mgr.Uninstall("alpha")
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if len(removed) != 1 || removed[0] != "claude" {
		t.Fatalf("Uninstall() = %v, want [claude]", removed)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "alpha")); !os.IsNotExist(err) {
		t.Fatal("skill directory still exists after Uninstall")
	}
	if _, ok := mgr.Manifest.GetSkill("alpha"); ok {
		t.Fatal("manifest still lists the skill after Uninstall")
	}

	if _, err := mgr.Uninstall("alpha"); err == nil {
		t.Fatal("Uninstall() of a missing skill should fail")
	}
}

func TestManagerInstallRequiresSkillMd(t *testing.T) {
	repoDir := t.TempDir()
	writeFile(t, filepath.Join(repoDir, "skills", "broken", "README.md"), "# not a skill")

	mgr, _ := newTestManager(t, &Manifest{})
	snap := &Snapshot{Source: Source{Repo: repoDir}, Dir: repoDir}

	n, err := mgr.Install(snap, []Entry{{Name: "broken", Path: "skills/broken"}})
	if err == nil {
		t.Fatal("Install() should fail for a skill without SKILL.md")
	}
	if n != 0 {
		t.Fatalf("Install() = %d, want 0", n)
	}
	if _, ok := mgr.Manifest.GetSkill("broken"); ok {
		t.Fatal("failed install must not be recorded in the manifest")
	}
}

func TestManagerInstallWithoutTargets(t *testing.T) {
	repoDir := t.TempDir()
	writeFile(t, filepath.Join(repoDir, "skills", "alpha", "SKILL.md"), "# Alpha")

	mgr, _ := newTestManager(t, &Manifest{})
	mgr.Targets = nil
	events := &recordingEvents{}
	mgr.Events = events

	snap := &Snapshot{Source: Source{Repo: repoDir}, Dir: repoDir}
	if _, err := mgr.Install(snap, []Entry{{Name: "alpha", Path: "skills/alpha"}}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !events.noTargets {
		t.Fatal("expected NoTargets event")
	}
	if _, ok := mgr.Manifest.GetSkill("alpha"); ok {
		t.Fatal("a skill installed nowhere must not be recorded")
	}
}

type recordingEvents struct {
	NopEvents
	noTargets bool
	upToDate  string
}

func (e *recordingEvents) NoTargets()                        { e.noTargets = true }
func (e *recordingEvents) SourceUpToDate(_ Source, v string) { e.upToDate = v }

func TestSyncKeepsLockedVersionWhileUpdateFollowsLatest(t *testing.T) {
	remote := newSkillRemote(t)
	remote.release("v1.0.0", map[string]string{"test-skill": "# v1"})

	manifest := &Manifest{
		Skills:  []InstalledSkill{{Name: "test-skill", Source: remote.URL, Path: "skills/test-skill"}},
		Sources: []SourceRecord{{Repo: remote.URL, Version: "v1.0.0"}},
	}
	mgr, home := newTestManager(t, manifest)
	src := Source{Repo: remote.URL}
	sourceVersion := func() string {
		record, _ := manifest.GetSource(src)
		return record.Version
	}

	if _, err := mgr.Sync(src); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if got := readInstalledSkill(t, home, "test-skill"); got != "# v1" {
		t.Fatalf("after sync, SKILL.md = %q, want the locked v1.0.0 content", got)
	}

	remote.release("v2.0.0", map[string]string{"test-skill": "# v2"})

	if _, err := mgr.Sync(src); err != nil {
		t.Fatalf("Sync() after upstream release error = %v", err)
	}
	if got := readInstalledSkill(t, home, "test-skill"); got != "# v1" {
		t.Fatalf("sync must not pick up a newer release, SKILL.md = %q", got)
	}
	if got := sourceVersion(); got != "v1.0.0" {
		t.Fatalf("sync changed the locked version to %q", got)
	}

	if _, err := mgr.Update(src); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got := readInstalledSkill(t, home, "test-skill"); got != "# v2" {
		t.Fatalf("after update, SKILL.md = %q, want the v2.0.0 content", got)
	}
	if got := sourceVersion(); got != "v2.0.0" {
		t.Fatalf("after update, locked version = %q, want v2.0.0", got)
	}

	events := &recordingEvents{}
	mgr.Events = events
	if n, err := mgr.Update(src); err != nil || n != 0 {
		t.Fatalf("Update() when already at latest = (%d, %v), want (0, nil)", n, err)
	}
	if events.upToDate != "v2.0.0" {
		t.Fatalf("expected SourceUpToDate(v2.0.0), got %q", events.upToDate)
	}
	mgr.Events = nil

	if _, err := mgr.Update(src.WithQuery("v1.0.0")); err != nil {
		t.Fatalf("Update() to an explicit older version error = %v", err)
	}
	if got := readInstalledSkill(t, home, "test-skill"); got != "# v1" {
		t.Fatalf("after pinning v1.0.0, SKILL.md = %q", got)
	}
	if got := sourceVersion(); got != "v1.0.0" {
		t.Fatalf("after pinning v1.0.0, locked version = %q", got)
	}
}

func TestUpdateRefusesWhenCatalogDropsInstalledSkill(t *testing.T) {
	remote := newSkillRemote(t)
	remote.release("v1.0.0", map[string]string{"alpha": "# alpha v1", "beta": "# beta v1"})

	manifest := &Manifest{
		Skills: []InstalledSkill{
			{Name: "alpha", Source: remote.URL, Path: "skills/alpha"},
			{Name: "beta", Source: remote.URL, Path: "skills/beta"},
		},
		Sources: []SourceRecord{{Repo: remote.URL, Version: "v1.0.0"}},
	}
	mgr, home := newTestManager(t, manifest)
	src := Source{Repo: remote.URL}

	if _, err := mgr.Sync(src); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	remote.release("v2.0.0", map[string]string{"alpha": "# alpha v2"})

	_, err := mgr.Update(src)
	if err == nil {
		t.Fatal("Update() should fail when the new catalog drops an installed skill")
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
	if record, _ := manifest.GetSource(src); record.Version != "v1.0.0" {
		t.Fatalf("a refused update must keep the locked version, got %q", record.Version)
	}
	if _, ok := manifest.GetSkill("beta"); !ok {
		t.Fatal("a refused update must keep beta in the manifest")
	}
}

func TestSyncBackfillsMissingSourceVersion(t *testing.T) {
	remote := newSkillRemote(t)
	remote.release("v1.0.0", map[string]string{"test-skill": "# v1"})

	// A manifest from before versions were tracked: no source record.
	manifest := &Manifest{
		Skills: []InstalledSkill{{Name: "test-skill", Source: remote.URL, Path: "skills/test-skill"}},
	}
	mgr, home := newTestManager(t, manifest)
	src := Source{Repo: remote.URL}

	if _, err := mgr.Sync(src); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if got := readInstalledSkill(t, home, "test-skill"); got != "# v1" {
		t.Fatalf("SKILL.md = %q", got)
	}
	if record, ok := manifest.GetSource(src); !ok || record.Version != "v1.0.0" {
		t.Fatalf("source record = (%+v, %v), want the resolved version recorded", record, ok)
	}
}
