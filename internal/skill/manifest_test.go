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

func TestSkillsFrom(t *testing.T) {
	t.Parallel()
	m := &Manifest{
		Skills: []InstalledSkill{
			{Name: "alpha", Source: "owner/repo"},
			{Name: "beta", Source: "Owner/Repo"},
			{Name: "gamma", Source: "other/repo"},
		},
	}

	got := m.SkillsFrom(Source{Repo: "OWNER/REPO"})
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "beta" {
		t.Fatalf("SkillsFrom() = %+v, want alpha and beta across spellings", got)
	}
}

func TestSourcesAreCaseInsensitive(t *testing.T) {
	t.Parallel()
	m := &Manifest{}

	m.AddSource(Source{Repo: "AfterShip/Skills"})
	m.AddSource(Source{Repo: "aftership/skills"})
	if len(m.Sources) != 1 || m.Sources[0].Repo != "AfterShip/Skills" {
		t.Fatalf("sources = %v, want first-seen spelling only", m.Sources)
	}

	if !m.RemoveSource(Source{Repo: "AFTERSHIP/SKILLS"}) {
		t.Fatal("RemoveSource should match case-insensitively")
	}
	if len(m.Sources) != 0 {
		t.Fatalf("sources = %v, want empty", m.Sources)
	}
}

func TestSetSourceVersion(t *testing.T) {
	t.Parallel()
	m := &Manifest{}

	m.SetSourceVersion(Source{Repo: "owner/repo"}, "v1.0.0")
	m.SetSourceVersion(Source{Repo: "Owner/Repo"}, "v2.0.0")

	if len(m.Sources) != 1 {
		t.Fatalf("sources = %v, want one entry across spellings", m.Sources)
	}
	record, ok := m.GetSource(Source{Repo: "OWNER/REPO"})
	if !ok || record.Repo != "owner/repo" || record.Version != "v2.0.0" {
		t.Fatalf("source = %+v, want first-seen spelling with the updated version", record)
	}
}

func TestInstalledAndKnownSources(t *testing.T) {
	t.Parallel()
	m := &Manifest{
		Skills: []InstalledSkill{
			{Name: "alpha", Source: "owner/repo"},
			{Name: "beta", Source: "Owner/Repo"},
		},
		Sources: []SourceRecord{
			{Repo: "owner/repo", Version: "v1.0.0"},
			{Repo: "tracked/only"},
		},
	}

	installed := m.InstalledSources()
	if len(installed) != 1 || installed[0].Repo != "owner/repo" {
		t.Fatalf("InstalledSources() = %v, want the skills' shared source once", installed)
	}

	known := m.KnownSources()
	if len(known) != 2 || known[0].Repo != "owner/repo" || known[1].Repo != "tracked/only" {
		t.Fatalf("KnownSources() = %v, want installed sources then tracked ones", known)
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

func TestLoadManifestListsSkillSources(t *testing.T) {
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

	m, err := LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if len(m.Sources) != 1 {
		t.Fatalf("sources = %v, want the skills' shared source listed once", m.Sources)
	}
	if record, ok := m.GetSource(Source{Repo: "owner/repo"}); !ok || record.Version != "" {
		t.Fatalf("source = (%+v, %v), want listed without a version", record, ok)
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
