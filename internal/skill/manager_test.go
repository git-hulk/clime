package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
	require.NoError(r.t, os.RemoveAll(filepath.Join(r.dir, "skills")))
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
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude"), 0o755))
	targets, err := DetectTargets()
	require.NoError(t, err)
	return &Manager{
		Manifest: manifest,
		Store:    &Store{Root: filepath.Join(home, ".clime", "sources")},
		Targets:  targets,
	}, home
}

func readInstalledSkill(t *testing.T, home, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".claude", "skills", name, "SKILL.md"))
	require.NoError(t, err)
	return string(data)
}

func TestManagerInstallEndToEnd(t *testing.T) {
	repoDir := t.TempDir()
	writeFile(t, filepath.Join(repoDir, "skills.yaml"), `skills:
  - name: alpha
    description: Alpha skill
    path: old/alpha
`)
	writeFile(t, filepath.Join(repoDir, "skills", "alpha", "SKILL.md"), "# Alpha")
	writeFile(t, filepath.Join(repoDir, "skills", "alpha", "extra.txt"), "extra")

	mgr, home := newTestManager(t, &Manifest{})
	src, err := ParseSource(repoDir)
	require.NoError(t, err)

	snap, catalog, err := mgr.Fetch(src)
	require.NoError(t, err)
	require.Len(t, catalog.Skills, 1)

	n, err := mgr.Install(snap, catalog.Skills)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	require.Equal(t, "# Alpha", readInstalledSkill(t, home, "alpha"))
	shared := filepath.Join(home, ".agents", "skills", "alpha")

	got, err := os.Readlink(filepath.Join(home, ".claude", "skills", "alpha"))
	require.NoError(t, err)
	require.Equal(t, shared, got)

	extra, err := os.ReadFile(filepath.Join(home, ".claude", "skills", "alpha", "extra.txt"))
	require.NoError(t, err)
	require.Equal(t, "extra", string(extra))

	installed, ok := mgr.Manifest.GetSkill("alpha")
	require.True(t, ok)
	require.Equal(t, repoDir, installed.Source)

	record, ok := mgr.Manifest.GetSource(src)
	if ok {
		require.Empty(t, record.Version, "local source must not record a version")
	}

	removed, err := mgr.Uninstall("alpha")
	require.NoError(t, err)
	require.Equal(t, []string{"agents", "claude"}, removed)
	for _, dir := range []string{shared, filepath.Join(home, ".claude", "skills", "alpha")} {
		_, err := os.Lstat(dir)
		require.ErrorIs(t, err, os.ErrNotExist)
	}

	_, ok = mgr.Manifest.GetSkill("alpha")
	require.False(t, ok, "manifest still lists the skill after Uninstall")

	_, err = mgr.Uninstall("alpha")
	require.Error(t, err, "Uninstall() of a missing skill should fail")
}

func TestManagerInstallRequiresSkillMd(t *testing.T) {
	repoDir := t.TempDir()
	writeFile(t, filepath.Join(repoDir, "skills", "broken", "README.md"), "# not a skill")

	mgr, _ := newTestManager(t, &Manifest{})
	snap := &Snapshot{Source: Source{Repo: repoDir}, Dir: repoDir}

	n, err := mgr.Install(snap, []Entry{{Name: "broken", Path: "skills/broken"}})
	require.Error(t, err, "Install() should fail for a skill without SKILL.md")
	require.Equal(t, 0, n)

	_, ok := mgr.Manifest.GetSkill("broken")
	require.False(t, ok, "failed install must not be recorded in the manifest")
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
			require.NoError(t, err)
			require.Equal(t, version, snap.Version)
			require.Equal(t, dir, snap.Dir)

			n, err := mgr.Install(snap, catalog.Skills)
			require.NoError(t, err)
			require.Equal(t, 1, n)

			require.Equal(t, "# cached alpha", readInstalledSkill(t, home, "alpha"))
			saved, err := LoadManifest("")
			require.NoError(t, err)

			record, _ := saved.GetSource(src)
			require.Equal(t, version, record.Version)
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
			require.NoError(t, err)
			wantVersion := tt.want
			if tt.query == "latest" || (tt.query == "" && tt.locked == "") {
				wantVersion = "latest"
			}
			require.Equal(t, wantVersion, snap.Version)
			require.Equal(t, tt.want, snap.revision)

			_, ok := catalog.Find("alpha")
			require.True(t, ok, "fetched catalog does not contain alpha")

			record, _ := manifest.GetSource(src)
			require.Equal(t, tt.locked, record.Version)
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

	_, err := mgr.Install(snap, []Entry{{Name: "alpha", Path: "skills/alpha"}})
	require.NoError(t, err)

	require.True(t, events.noTargets, "expected NoTargets event")

	_, ok := mgr.Manifest.GetSkill("alpha")
	require.False(t, ok, "a skill installed nowhere must not be recorded")
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
		Skills:  []InstalledSkill{{Name: "test-skill", Source: remote.URL}},
		Sources: []SourceRecord{{Repo: remote.URL, Version: "v1.0.0"}},
	}
	mgr, home := newTestManager(t, manifest)
	src := Source{Repo: remote.URL}
	sourceVersion := func() string {
		record, _ := manifest.GetSource(src)
		return record.Version
	}

	_, err := mgr.Sync(src)
	require.NoError(t, err)

	require.Equal(t, "# v1", readInstalledSkill(t, home, "test-skill"))

	remote.release("v2.0.0", map[string]string{"test-skill": "# v2"})

	_, err = mgr.Sync(src)
	require.NoError(t, err)

	require.Equal(t, "# v1", readInstalledSkill(t, home, "test-skill"))
	require.Equal(t, "v1.0.0", sourceVersion())

	_, err = mgr.Update(src)
	require.NoError(t, err)

	require.Equal(t, "# v2", readInstalledSkill(t, home, "test-skill"))
	require.Equal(t, "latest", sourceVersion())
	require.False(t, dirExists(versionDir(mgr.Store.repoDir(src), "v1.0.0")), "successful update must remove the old snapshot")
	require.True(t, dirExists(versionDir(mgr.Store.repoDir(src), "v2.0.0")), "successful update must keep the installed snapshot")

	events := &recordingEvents{}
	mgr.Events = events

	n, err := mgr.Update(src)
	require.NoError(t, err)
	require.Equal(t, 0, n)

	require.Equal(t, "latest", events.upToDate)
	mgr.Events = nil

	_, err = mgr.Update(src.WithQuery("v1.0.0"))
	require.NoError(t, err)

	require.Equal(t, "# v1", readInstalledSkill(t, home, "test-skill"))
	require.Equal(t, "v1.0.0", sourceVersion())
	require.False(t, dirExists(versionDir(mgr.Store.repoDir(src), "v2.0.0")), "successful downgrade must remove the superseded snapshot")
}

func TestUpdateRefusesWhenCatalogDropsInstalledSkill(t *testing.T) {
	remote := newSkillRemote(t)
	remote.release("v1.0.0", map[string]string{"alpha": "# alpha v1", "beta": "# beta v1"})

	manifest := &Manifest{
		Skills: []InstalledSkill{
			{Name: "alpha", Source: remote.URL},
			{Name: "beta", Source: remote.URL},
		},
		Sources: []SourceRecord{{Repo: remote.URL, Version: "v1.0.0"}},
	}
	mgr, home := newTestManager(t, manifest)
	src := Source{Repo: remote.URL}

	_, err := mgr.Sync(src)
	require.NoError(t, err)

	remote.release("v2.0.0", map[string]string{"alpha": "# alpha v2"})

	_, err = mgr.Update(src)
	require.ErrorContains(t, err, "beta", "Update() should fail when the new catalog drops an installed skill")
	require.Equal(t, "# alpha v1", readInstalledSkill(t, home, "alpha"))
	require.Equal(t, "# beta v1", readInstalledSkill(t, home, "beta"))

	record, _ := manifest.GetSource(src)
	require.Equal(t, "v1.0.0", record.Version)

	_, ok := manifest.GetSkill("beta")
	require.True(t, ok, "a refused update must keep beta in the manifest")

	require.True(t, dirExists(versionDir(mgr.Store.repoDir(src), "v1.0.0")), "a refused update must keep the old snapshot")
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
						Skills:  []InstalledSkill{{Name: "alpha", Source: src.Repo}},
						Sources: []SourceRecord{{Repo: src.Repo, Version: "v1.0.0"}},
					}
					mgr, home := newTestManager(t, manifest)

					_, err := mgr.Sync(src)
					require.NoError(t, err)

					base := mgr.Store.repoDir(src)
					stale := versionDir(base, "v0.1.0")
					other := versionDir(base+"-other", "v1.0.0")
					for _, dir := range []string{stale, other} {
						writeFile(t, filepath.Join(dir, "SKILL.md"), "keep unless superseded")
					}
					remote.release("v2.0.0", map[string]string{"alpha": "# v2"})
					snap, catalog, err := mgr.Fetch(src.WithQuery("v2.0.0"))
					require.NoError(t, err)
					if verb == VerbSync {
						manifest.SetSourceVersion(src, "v2.0.0")
						require.NoError(t, manifest.Save())
					}
					if verb == VerbInstall {
						n, err := mgr.Install(snap, nil)
						require.NoError(t, err)
						require.Equal(t, 0, n)

						require.True(t, dirExists(stale), "install with no entries must retain old snapshots")
					}
					switch failure {
					case "target":
						// Fail on the second target, after the first has been updated.
						blocked := filepath.Join(home, "blocked")
						writeFile(t, blocked, "not a directory")
						mgr.Targets[1].Dir = blocked
					case "manifest":
						path, err := manifestPath()
						require.NoError(t, err)
						require.NoError(t, os.Remove(path))
						require.NoError(t, os.Mkdir(path, 0o755))
					case "no targets":
						mgr.Targets = nil
					}
					switch verb {
					case VerbInstall:
						_, err = mgr.Install(snap, catalog.Skills)
					case VerbUpdate:
						_, err = mgr.Update(src.WithQuery("v2.0.0"))
					case VerbSync:
						_, err = mgr.Sync(src)
					}
					wantErr := failure == "target" || failure == "manifest"
					if wantErr {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
					}
					installedRevision, err := mgr.Store.installedRevision(src)
					require.NoError(t, err)
					wantRevision := "v1.0.0"
					if failure == "success" {
						wantRevision = "v2.0.0"
					}
					require.Equal(t, wantRevision, installedRevision)
					for _, dir := range []string{stale, versionDir(base, "v1.0.0")} {
						require.Equal(t, failure != "success", dirExists(dir), "snapshot %s", dir)
					}
					for _, dir := range []string{other, versionDir(base, "v2.0.0")} {
						require.True(t, dirExists(dir), "snapshot %s must be retained", dir)
					}
					if failure == "success" {
						require.NoError(t, os.Rename(remote.dir, filepath.Join(t.TempDir(), "offline")))

						_, err := mgr.Sync(src)
						require.NoError(t, err)

						require.Equal(t, "# v2", readInstalledSkill(t, home, "alpha"))
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
		Skills: []InstalledSkill{{Name: "test-skill", Source: remote.URL}},
	}
	mgr, home := newTestManager(t, manifest)
	src := Source{Repo: remote.URL}

	_, err := mgr.Sync(src)
	require.NoError(t, err)

	require.Equal(t, "# v1", readInstalledSkill(t, home, "test-skill"))

	record, ok := manifest.GetSource(src)
	require.True(t, ok)
	require.Equal(t, "latest", record.Version)
}

func TestSyncUsesGroupedManifestAndConventionalPathsOffline(t *testing.T) {
	manager, home := newTestManager(t, &Manifest{})
	path := filepath.Join(home, ".clime", "skills.yaml")
	writeFile(t, path, `AfterShip/Skills:
  skills:
    - rest-api-design
    - test-abc
  version: f8c4c0e02021b0debef257750d4d020e9dad38aa
`)
	manifest, err := LoadManifest(path)
	require.NoError(t, err)
	manager.Manifest = manifest
	source := Source{Repo: "AfterShip/Skills"}
	cache := versionDir(manager.Store.repoDir(source), "f8c4c0e02021b0debef257750d4d020e9dad38aa")
	for _, name := range []string{"rest-api-design", "test-abc"} {
		writeFile(t, filepath.Join(cache, "skills", name, "SKILL.md"), "# "+name)
	}
	t.Setenv("PATH", t.TempDir())
	count, err := manager.Sync(source)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	for _, name := range []string{"rest-api-design", "test-abc"} {
		require.Equal(t, "# "+name, readInstalledSkill(t, home, name))
	}
	reloaded, err := LoadManifest(path)
	require.NoError(t, err)
	require.Equal(t, manifest.Skills, reloaded.Skills)
	require.Equal(t, manifest.Sources, reloaded.Sources)
}

func TestBranchVersionsStayNamedWhileSyncAndUpdateFollowHead(t *testing.T) {
	remote := newSkillRemote(t)
	remote.release("v1.0.0", map[string]string{"alpha": "# first"})
	gitIn(t, remote.dir, "checkout", "-b", "feature/skills")
	manager, home := newTestManager(t, &Manifest{})
	source := Source{Repo: remote.URL, Query: "feature/skills"}
	snapshot, catalog, err := manager.Fetch(source)
	require.NoError(t, err)
	firstRevision := gitIn(t, remote.dir, "rev-parse", "HEAD")
	require.Equal(t, "feature/skills", snapshot.Version)
	require.Equal(t, versionDir(manager.Store.repoDir(source), firstRevision), snapshot.Dir)
	_, err = manager.Install(snapshot, catalog.Skills)
	require.NoError(t, err)

	for _, operation := range []string{"sync", "update", "explicit update"} {
		writeFile(t, filepath.Join(remote.dir, "skills", "alpha", "SKILL.md"), "# "+operation)
		gitIn(t, remote.dir, "add", "-A")
		gitIn(t, remote.dir, "commit", "-m", operation)
		// Reload to prove the branch identity survives the YAML round trip.
		manager.Manifest, err = LoadManifest("")
		require.NoError(t, err)
		var count int
		if operation == "sync" {
			count, err = manager.Sync(Source{Repo: source.Repo})
		} else if operation == "update" {
			count, err = manager.Update(Source{Repo: source.Repo})
		} else {
			count, err = manager.Update(source)
		}
		require.NoError(t, err)
		require.Equal(t, 1, count)
		require.Equal(t, "# "+operation, readInstalledSkill(t, home, "alpha"))
		saved, err := LoadManifest("")
		require.NoError(t, err)
		record, found := saved.GetSource(source)
		require.True(t, found)
		require.Equal(t, "feature/skills", record.Version)
		currentRevision := gitIn(t, remote.dir, "rev-parse", "HEAD")
		require.True(t, dirExists(versionDir(manager.Store.repoDir(source), currentRevision)))
		require.False(t, dirExists(versionDir(manager.Store.repoDir(source), firstRevision)))
	}
}

func TestLatestChecksRemoteForInstallUpdateAndSync(t *testing.T) {
	for _, tags := range []bool{true, false} {
		name := "default branch"
		if tags {
			name = "release tags"
		}
		t.Run(name, func(t *testing.T) {
			remote := newSkillRemote(t)
			remote.release("v1.0.0", map[string]string{"alpha": "# first"})
			if !tags {
				gitIn(t, remote.dir, "tag", "-d", "v1.0.0")
			}
			manager, home := newTestManager(t, &Manifest{})
			source := Source{Repo: remote.URL}
			// A cache directory named latest must never bypass remote resolution.
			writeFile(t, filepath.Join(versionDir(manager.Store.repoDir(source), "latest"), "stale"), "stale")
			snapshot, catalog, err := manager.Fetch(source)
			require.NoError(t, err)
			_, err = manager.Install(snapshot, catalog.Skills)
			require.NoError(t, err)

			for index, operation := range []string{"install", "update", "sync"} {
				count, err := manager.Update(source)
				require.NoError(t, err)
				require.Zero(t, count, "unchanged remote revision is already installed")

				tag := fmt.Sprintf("v%d.0.0", index+2)
				remote.release(tag, map[string]string{"alpha": "# " + operation})
				if !tags {
					gitIn(t, remote.dir, "tag", "-d", tag)
				}
				manager.Manifest, err = LoadManifest("")
				require.NoError(t, err)
				// Downloading a revision alone must not mark it as installed.
				snapshot, catalog, err = manager.Fetch(source)
				require.NoError(t, err)
				switch operation {
				case "install":
					count, err = manager.Install(snapshot, catalog.Skills)
				case "update":
					count, err = manager.Update(source)
				case "sync":
					count, err = manager.Sync(source)
				}
				require.NoError(t, err)
				require.Equal(t, 1, count)
				require.Equal(t, "# "+operation, readInstalledSkill(t, home, "alpha"))
				saved, err := LoadManifest("")
				require.NoError(t, err)
				record, found := saved.GetSource(source)
				require.True(t, found)
				require.Equal(t, "latest", record.Version)
				count, err = manager.Update(source)
				require.NoError(t, err)
				require.Zero(t, count)
			}

			// Floating versions still check the remote when content is cached.
			require.NoError(t, os.Rename(remote.dir, filepath.Join(t.TempDir(), "offline")))
			_, _, err = manager.Fetch(source)
			require.Error(t, err)
			_, err = manager.Update(source)
			require.Error(t, err)
			_, err = manager.Sync(source)
			require.Error(t, err)
		})
	}
}

func TestManagerSkillOperationsUseCatalogEntryPath(t *testing.T) {
	for _, skillsDir := range []string{".agents/skills", ".claude/skills"} {
		t.Run(skillsDir, func(t *testing.T) {
			repoDir := t.TempDir()
			skillPath := filepath.Join(repoDir, skillsDir, "folder", "SKILL.md")
			writeFile(t, skillPath, "---\nname: alpha\n---\n# Install")
			manager, home := newTestManager(t, &Manifest{})
			source := Source{Repo: repoDir}
			snapshot, catalog, err := manager.Fetch(source)
			require.NoError(t, err)

			count, err := manager.Install(snapshot, catalog.Skills)
			require.NoError(t, err)
			require.Equal(t, 1, count)
			require.Equal(t, "---\nname: alpha\n---\n# Install", readInstalledSkill(t, home, "alpha"))

			for _, operation := range []string{"sync", "update"} {
				content := "---\nname: alpha\n---\n# " + operation
				writeFile(t, skillPath, content)
				manager.Manifest, err = LoadManifest("")
				require.NoError(t, err)

				if operation == "sync" {
					count, err = manager.Sync(source)
				} else {
					count, err = manager.Update(source)
				}
				require.NoError(t, err)
				require.Equal(t, 1, count)
				require.Equal(t, content, readInstalledSkill(t, home, "alpha"))
			}
		})
	}
}
