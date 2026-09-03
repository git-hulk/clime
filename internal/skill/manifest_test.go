package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddSkill(t *testing.T) {
	t.Parallel()
	m := &Manifest{}

	m.AddSkill(InstalledSkill{
		Name:   "my-skill",
		Source: "owner/repo",
		Path:   "my-skill",
	})
	if len(m.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(m.Skills))
	}

	// Update existing skill.
	m.AddSkill(InstalledSkill{
		Name:   "my-skill",
		Source: "owner/repo",
		Path:   "my-skill/updated",
	})
	if len(m.Skills) != 1 {
		t.Fatalf("expected 1 skill after update, got %d", len(m.Skills))
	}
	if m.Skills[0].Path != "my-skill/updated" {
		t.Fatalf("expected path 'my-skill/updated', got %q", m.Skills[0].Path)
	}
}

func TestRemoveSkill(t *testing.T) {
	t.Parallel()
	m := &Manifest{
		Skills: []InstalledSkill{
			{Name: "skill-a"},
			{Name: "skill-b"},
		},
	}

	if !m.RemoveSkill("skill-a") {
		t.Fatal("expected RemoveSkill to return true")
	}
	if len(m.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(m.Skills))
	}

	if m.RemoveSkill("missing") {
		t.Fatal("expected RemoveSkill to return false for missing skill")
	}
}

func TestGetSkill(t *testing.T) {
	t.Parallel()
	m := &Manifest{
		Skills: []InstalledSkill{{Name: "my-skill", Source: "owner/repo"}},
	}

	s, ok := m.GetSkill("my-skill")
	if !ok {
		t.Fatal("expected to find skill")
	}
	if s.Source != "owner/repo" {
		t.Fatalf("expected source owner/repo, got %s", s.Source)
	}

	_, ok = m.GetSkill("missing")
	if ok {
		t.Fatal("expected not to find missing skill")
	}
}

func TestAddSourceStripsVersionQuery(t *testing.T) {
	t.Parallel()
	m := &Manifest{}

	m.AddSource("owner/repo")
	m.AddSource("owner/repo@latest")
	m.AddSource("owner/repo@v1.2.3")
	m.AddSource("other/repo@v2")

	want := []string{"owner/repo", "other/repo"}
	if len(m.Sources) != len(want) {
		t.Fatalf("sources = %v, want %v", m.Sources, want)
	}
	for i, s := range want {
		if m.Sources[i].Repo != s {
			t.Fatalf("sources = %v, want %v", m.Sources, want)
		}
	}
}

func TestLoadManifestNormalizesVersionedSources(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeManifestFile(t, home, `skills:
  - name: pinned-skill
    source: owner/repo@v1.2.3
    path: skills/pinned-skill
  - name: legacy-skill
    source: owner/repo@latest
    version: v1.4.0
    path: skills/legacy-skill
sources:
  - owner/repo
  - owner/repo@latest
  - owner/repo@v1.2.3
`)

	// The source gets its version from the legacy fields, so nothing resolves.
	stubResolveLatest(t, func(repo string) (string, error) {
		t.Fatal("resolveLatestVersion should not run when a version was migrated")
		return "", nil
	})

	m, err := LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}

	if len(m.Sources) != 1 || m.Sources[0].Repo != "owner/repo" {
		t.Fatalf("sources = %v, want [owner/repo]", m.Sources)
	}
	if m.Sources[0].Version != "v1.2.3" {
		t.Fatalf("source version = %q, want the migrated pin %q", m.Sources[0].Version, "v1.2.3")
	}

	for _, name := range []string{"pinned-skill", "legacy-skill"} {
		s, _ := m.GetSkill(name)
		if s.Source != "owner/repo" || s.LegacyVersion != "" {
			t.Fatalf("%s = {source: %q, legacy version: %q}, want stripped source and no per-skill version", name, s.Source, s.LegacyVersion)
		}
	}
}

func writeManifestFile(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".clime")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func stubResolveLatest(t *testing.T, fn func(repo string) (string, error)) {
	t.Helper()
	orig := resolveLatestVersion
	resolveLatestVersion = fn
	t.Cleanup(func() { resolveLatestVersion = orig })
}

func TestLoadManifestStampsMissingVersions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeManifestFile(t, home, `skills:
  - name: old-skill
    source: owner/repo
    path: skills/old-skill
  - name: sibling-skill
    source: Owner/Repo
    path: skills/sibling-skill
`)

	var calls int
	stubResolveLatest(t, func(repo string) (string, error) {
		calls++
		if repo != "owner/repo" {
			t.Fatalf("resolveLatestVersion(%q), want %q", repo, "owner/repo")
		}
		return "v2.0.0", nil
	})

	m, err := LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("resolveLatestVersion called %d times, want once per source", calls)
	}
	if len(m.Sources) != 1 {
		t.Fatalf("sources = %v, want the skills' shared source listed once", m.Sources)
	}
	if src, ok := m.GetSource("owner/repo"); !ok || src.Version != "v2.0.0" {
		t.Fatalf("source = %+v, want version stamped %q", src, "v2.0.0")
	}

	// The stamp is persisted: a reload must not resolve again.
	stubResolveLatest(t, func(repo string) (string, error) {
		t.Fatal("resolveLatestVersion should not run after versions are persisted")
		return "", nil
	})
	if _, err := LoadManifest(); err != nil {
		t.Fatalf("LoadManifest() reload error = %v", err)
	}
}

func TestLoadManifestLeavesVersionEmptyWhenResolutionFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeManifestFile(t, home, `skills:
  - name: old-skill
    source: owner/repo
    path: skills/old-skill
`)

	stubResolveLatest(t, func(repo string) (string, error) {
		return "", os.ErrDeadlineExceeded
	})

	m, err := LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if src, _ := m.GetSource("owner/repo"); src.Version != "" {
		t.Fatalf("source version = %q, want empty when resolution fails", src.Version)
	}
}

func TestLoadManifestDoesNotResolveLocalPathSources(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	localSource := t.TempDir()
	writeManifestFile(t, home, `skills:
  - name: local-skill
    source: `+localSource+`
    path: skills/local-skill
`)

	stubResolveLatest(t, func(repo string) (string, error) {
		t.Fatal("resolveLatestVersion should not run for local path sources")
		return "", nil
	})

	m, err := LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if src, _ := m.GetSource(localSource); src.Version != "" {
		t.Fatalf("source version = %q, want empty for a local path source", src.Version)
	}
}

func TestSourcesAreCaseInsensitive(t *testing.T) {
	t.Parallel()
	m := &Manifest{}

	m.AddSource("AfterShip/Skills")
	m.AddSource("aftership/skills")
	m.AddSource("AfterShip/Skills@v1.2.3")
	if len(m.Sources) != 1 || m.Sources[0].Repo != "AfterShip/Skills" {
		t.Fatalf("sources = %v, want first-seen spelling only", m.Sources)
	}

	if !m.RemoveSource("AFTERSHIP/SKILLS") {
		t.Fatal("RemoveSource should match case-insensitively")
	}
	if len(m.Sources) != 0 {
		t.Fatalf("sources = %v, want empty", m.Sources)
	}

	if !SameSource("owner/repo@v1.0.0", "Owner/Repo@latest") {
		t.Fatal("SameSource should ignore case and version queries")
	}
	if SameSource("owner/repo", "owner/other") {
		t.Fatal("SameSource should not match different repos")
	}
}

func TestSetSourceVersion(t *testing.T) {
	t.Parallel()
	m := &Manifest{}

	m.SetSourceVersion("owner/repo@v1.0.0", "v1.0.0")
	m.SetSourceVersion("Owner/Repo", "v2.0.0")

	if len(m.Sources) != 1 {
		t.Fatalf("sources = %v, want one entry across spellings", m.Sources)
	}
	src, ok := m.GetSource("OWNER/REPO")
	if !ok || src.Repo != "owner/repo" || src.Version != "v2.0.0" {
		t.Fatalf("source = %+v, want first-seen spelling with the updated version", src)
	}
}

func TestLoadManifestBacksUpMigratedFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	legacy := `skills:
  - name: old-skill
    source: owner/repo@v1.2.3
    path: skills/old-skill
sources:
  - owner/repo
`
	writeManifestFile(t, home, legacy)
	stubResolveLatest(t, func(repo string) (string, error) {
		t.Fatal("a migrated pin must not need resolution")
		return "", nil
	})

	if _, err := LoadManifest(); err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}

	backup := filepath.Join(home, ".clime", "skills.yaml.bak")
	got, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if string(got) != legacy {
		t.Fatalf("backup =\n%s\nwant the pre-migration manifest\n%s", got, legacy)
	}

	// A later migrating load keeps the original backup rather than
	// replacing it with already-migrated content.
	writeManifestFile(t, home, "skills:\n  - name: other\n    source: Owner/Repo@v2\n    path: skills/other\n")
	if _, err := LoadManifest(); err != nil {
		t.Fatalf("second LoadManifest() error = %v", err)
	}
	got, err = os.ReadFile(backup)
	if err != nil {
		t.Fatalf("reading backup after second load: %v", err)
	}
	if string(got) != legacy {
		t.Fatalf("backup was overwritten:\n%s", got)
	}
}

func TestLoadManifestParseErrorNamesFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeManifestFile(t, home, "skills: [oops\n")

	_, err := LoadManifest()
	if err == nil {
		t.Fatal("LoadManifest() error = nil, want a parse error")
	}
	if !strings.Contains(err.Error(), filepath.Join(home, ".clime", "skills.yaml")) {
		t.Fatalf("error = %v, want it to name the manifest path", err)
	}
}
