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
	shared := filepath.Join(home, ".agents", "skills", "alpha")
	if got, err := os.Readlink(filepath.Join(home, ".claude", "skills", "alpha")); err != nil || got != shared {
		t.Fatalf("Claude link = %q, %v, want %q", got, err, shared)
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
	if len(removed) != 2 || removed[0] != "agents" || removed[1] != "claude" {
		t.Fatalf("Uninstall() = %v, want [agents claude]", removed)
	}
	for _, dir := range []string{shared, filepath.Join(home, ".claude", "skills", "alpha")} {
		if _, err := os.Lstat(dir); !os.IsNotExist(err) {
			t.Fatalf("skill path %s still exists after Uninstall: %v", dir, err)
		}
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

func TestManagerInstallUsesLockedCacheOffline(t *testing.T) {
	for _, version := range []string{"v1.0.0", strings.Repeat("a", 40)} {
		t.Run(version, func(t *testing.T) {
			src := Source{Repo: "file://" + filepath.Join(t.TempDir(), "unavailable")}
			manifest := &Manifest{Sources: []SourceRecord{{Repo: src.Repo, Version: version}}}
			mgr, home := newTestManager(t, manifest)
			dir := versionDir(mgr.Store.repoDir(src), version)
			writeFile(t, filepath.Join(dir, "skills.yaml"), "skills:\n  - name: alpha\n    path: skills/alpha\n")
			writeFile(t, filepath.Join(dir, "skills", "alpha", "SKILL.md"), "# cached alpha")

			snap, catalog, err := mgr.Fetch(src)
			if err != nil {
				t.Fatalf("Fetch() offline error = %v", err)
			}
			if snap.Version != version || snap.Dir != dir {
				t.Fatalf("snapshot = %+v, want cached version %s at %s", snap, version, dir)
			}
			if n, err := mgr.Install(snap, catalog.Skills); err != nil || n != 1 {
				t.Fatalf("Install() = (%d, %v), want (1, nil)", n, err)
			}
			if got := readInstalledSkill(t, home, "alpha"); got != "# cached alpha" {
				t.Fatalf("installed content = %q, want cached content", got)
			}
			saved, err := LoadManifest()
			if err != nil {
				t.Fatal(err)
			}
			if record, _ := saved.GetSource(src); record.Version != version {
				t.Fatalf("install changed the lock to %q, want %q", record.Version, version)
			}
		})
	}
}

func TestManagerFetchHonorsQueriesAndFetchesMissingVersions(t *testing.T) {
	remote := newSkillRemote(t)
	remote.release("v1.0.0", map[string]string{"alpha": "# v1"})
	remote.release("v2.0.0", map[string]string{"alpha": "# v2"})

	for _, tt := range []struct {
		name, locked, query, want string
	}{
		{"uncached lock", "v1.0.0", "", "v1.0.0"},
		{"no lock", "", "", "v2.0.0"},
		{"explicit latest", "v1.0.0", "latest", "v2.0.0"},
		{"explicit tag", "v2.0.0", "v1.0.0", "v1.0.0"},
		{"explicit semver line", "v2.0.0", "v1", "v1.0.0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			src := Source{Repo: remote.URL, Query: tt.query}
			manifest := &Manifest{}
			if tt.locked != "" {
				manifest.SetSourceVersion(src, tt.locked)
			}
			mgr, _ := newTestManager(t, manifest)
			snap, catalog, err := mgr.Fetch(src)
			if err != nil {
				t.Fatalf("Fetch() error = %v", err)
			}
			if snap.Version != tt.want {
				t.Fatalf("version = %q, want %q", snap.Version, tt.want)
			}
			if _, ok := catalog.Find("alpha"); !ok {
				t.Fatal("fetched catalog does not contain alpha")
			}
			if record, _ := manifest.GetSource(src); record.Version != tt.locked {
				t.Fatalf("browsing changed the lock to %q, want %q", record.Version, tt.locked)
			}
		})
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
	if dirExists(versionDir(mgr.Store.repoDir(src), "v1.0.0")) {
		t.Fatal("successful update must remove the old snapshot")
	}
	if !dirExists(versionDir(mgr.Store.repoDir(src), "v2.0.0")) {
		t.Fatal("successful update must keep the installed snapshot")
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
	if dirExists(versionDir(mgr.Store.repoDir(src), "v2.0.0")) {
		t.Fatal("successful downgrade must remove the superseded snapshot")
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
	if !dirExists(versionDir(mgr.Store.repoDir(src), "v1.0.0")) {
		t.Fatal("a refused update must keep the old snapshot")
	}
}

func TestSkillOperationsPruneSnapshotsOnlyAfterInstallationSucceeds(t *testing.T) {
	for _, verb := range []Verb{VerbInstall, VerbUpdate, VerbSync} {
		t.Run(verb.String(), func(t *testing.T) {
			for _, failure := range []string{"success", "target", "manifest", "no targets"} {
				t.Run(failure, func(t *testing.T) {
					remote := newSkillRemote(t)
					remote.release("v1.0.0", map[string]string{"alpha": "# v1"})
					src := Source{Repo: remote.URL}
					manifest := &Manifest{
						Skills:  []InstalledSkill{{Name: "alpha", Source: src.Repo, Path: "skills/alpha"}},
						Sources: []SourceRecord{{Repo: src.Repo, Version: "v1.0.0"}},
					}
					mgr, home := newTestManager(t, manifest)
					if _, err := mgr.Sync(src); err != nil {
						t.Fatal(err)
					}
					base := mgr.Store.repoDir(src)
					stale := versionDir(base, "v0.1.0")
					other := versionDir(base+"-other", "v1.0.0")
					for _, dir := range []string{stale, other} {
						writeFile(t, filepath.Join(dir, "SKILL.md"), "keep unless superseded")
					}
					remote.release("v2.0.0", map[string]string{"alpha": "# v2"})
					snap, catalog, err := mgr.Fetch(src.WithQuery("v2.0.0"))
					if err != nil {
						t.Fatal(err)
					}
					if verb == VerbSync {
						manifest.SetSourceVersion(src, "v2.0.0")
						if err := manifest.Save(); err != nil {
							t.Fatal(err)
						}
					}
					if verb == VerbInstall {
						if n, err := mgr.Install(snap, nil); err != nil || n != 0 {
							t.Fatalf("Install() with no entries = (%d, %v), want (0, nil)", n, err)
						}
						if !dirExists(stale) {
							t.Fatal("install with no entries must retain old snapshots")
						}
					}
					switch failure {
					case "target":
						// Fail on the second target, after the first has been updated.
						blocked := filepath.Join(home, "blocked")
						writeFile(t, blocked, "not a directory")
						mgr.Targets[1].Dir = blocked
					case "manifest":
						path, err := manifestPath()
						if err != nil {
							t.Fatal(err)
						}
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
						if err := os.Mkdir(path, 0o755); err != nil {
							t.Fatal(err)
						}
					case "no targets":
						mgr.Targets = nil
					}
					switch verb {
					case VerbInstall:
						_, err = mgr.Install(snap, catalog.Skills)
					case VerbUpdate:
						_, err = mgr.Update(src)
					case VerbSync:
						_, err = mgr.Sync(src)
					}
					wantErr := failure == "target" || failure == "manifest"
					if (err != nil) != wantErr {
						t.Fatalf("%s error = %v, want error: %v", verb, err, wantErr)
					}
					for _, dir := range []string{stale, versionDir(base, "v1.0.0")} {
						if got, want := dirExists(dir), failure != "success"; got != want {
							t.Fatalf("snapshot %s exists = %v, want %v", dir, got, want)
						}
					}
					for _, dir := range []string{other, versionDir(base, "v2.0.0")} {
						if !dirExists(dir) {
							t.Fatalf("snapshot %s must be retained", dir)
						}
					}
					if failure == "success" {
						if err := os.Rename(remote.dir, filepath.Join(t.TempDir(), "offline")); err != nil {
							t.Fatal(err)
						}
						if _, err := mgr.Sync(src); err != nil {
							t.Fatalf("Sync() offline after cleanup: %v", err)
						}
						if got := readInstalledSkill(t, home, "alpha"); got != "# v2" {
							t.Fatalf("offline sync installed %q, want # v2", got)
						}
					}
				})
			}
		})
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
