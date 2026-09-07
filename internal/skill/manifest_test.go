package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

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
	require.Len(t, got, 2)
	require.Equal(t, "alpha", got[0].Name)
	require.Equal(t, "beta", got[1].Name)
}

func TestSourcesAreCaseInsensitive(t *testing.T) {
	t.Parallel()
	m := &Manifest{}

	m.AddSource(Source{Repo: "AfterShip/Skills"})
	m.AddSource(Source{Repo: "aftership/skills"})
	require.Len(t, m.Sources, 1)
	require.Equal(t, "AfterShip/Skills", m.Sources[0].Repo)

	require.True(t, m.RemoveSource(Source{Repo: "AFTERSHIP/SKILLS"}), "RemoveSource should match case-insensitively")
	require.Empty(t, m.Sources)
}

func TestSetSourceVersion(t *testing.T) {
	t.Parallel()
	m := &Manifest{}

	m.SetSourceVersion(Source{Repo: "owner/repo"}, "v1.0.0")
	m.SetSourceVersion(Source{Repo: "Owner/Repo"}, "v2.0.0")

	require.Len(t, m.Sources, 1)
	record, ok := m.GetSource(Source{Repo: "OWNER/REPO"})
	require.True(t, ok)
	require.Equal(t, "owner/repo", record.Repo)
	require.Equal(t, "v2.0.0", record.Version)
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
	require.Len(t, installed, 1)
	require.Equal(t, "owner/repo", installed[0].Repo)

	known := m.KnownSources()
	require.Len(t, known, 2)
	require.Equal(t, "owner/repo", known[0].Repo)
	require.Equal(t, "tracked/only", known[1].Repo)
}

func writeManifestFile(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".clime")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "skills.yaml"), []byte(content), 0o644))
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

	m, err := LoadManifest("")
	require.NoError(t, err)

	require.Len(t, m.Sources, 1)
	require.Equal(t, "owner/repo", m.Sources[0].Repo)
	require.Equal(t, "v1.2.3", m.Sources[0].Version)

	for _, name := range []string{"pinned-skill", "legacy-skill"} {
		s, _ := m.GetSkill(name)
		require.Equal(t, "owner/repo", s.Source)
		require.Empty(t, s.LegacyVersion)
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

	m, err := LoadManifest("")
	require.NoError(t, err)
	require.Len(t, m.Sources, 1)

	record, ok := m.GetSource(Source{Repo: "owner/repo"})
	require.True(t, ok)
	require.Empty(t, record.Version)
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

	_, err := LoadManifest("")
	require.NoError(t, err)

	backup := filepath.Join(home, ".clime", "skills.yaml.bak")
	got, err := os.ReadFile(backup)
	require.NoError(t, err)
	require.Equal(t, legacy, string(got))

	// A later migrating load keeps the original backup rather than
	// replacing it with already-migrated content.
	writeManifestFile(t, home, "skills:\n  - name: other\n    source: Owner/Repo@v2\n    path: skills/other\n")

	_, err = LoadManifest("")
	require.NoError(t, err)

	got, err = os.ReadFile(backup)
	require.NoError(t, err)
	require.Equal(t, legacy, string(got))
}

func TestLoadManifestParseErrorNamesFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeManifestFile(t, home, "skills: [oops\n")

	_, err := LoadManifest("")
	require.ErrorContains(t, err, filepath.Join(home, ".clime", "skills.yaml"), "expected a parse error")
}

func TestCustomManifestPersistsToSelectedPath(t *testing.T) {
	for _, initial := range []string{"", "skills: []\n", "sources:\n  - owner/repo@v1.2.3\n"} {
		t.Run(initial, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Chdir(t.TempDir())
			defaultPath := filepath.Join(home, ".clime", "skills.yaml")
			defaultContent := "skills: []\nsources: []\n"
			writeFile(t, defaultPath, defaultContent)
			path := filepath.Join("config", "skills.yaml")
			if initial != "" {
				writeFile(t, path, initial)
			}

			manifest, err := LoadManifest(path)
			require.NoError(t, err)
			if initial == "sources:\n  - owner/repo@v1.2.3\n" {
				backup, err := os.ReadFile(path + ".bak")
				require.NoError(t, err)
				require.Equal(t, initial, string(backup))
				migrated, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Contains(t, string(migrated), "version: v1.2.3")
			}
			manifest.AddSkill(InstalledSkill{Name: "custom", Source: "owner/repo", Path: "skills/custom"})
			require.NoError(t, manifest.Save())
			reloaded, err := LoadManifest(path)
			require.NoError(t, err)
			installed, found := reloaded.GetSkill("custom")
			require.True(t, found)
			require.Equal(t, "skills/custom", installed.Path)
			unchanged, err := os.ReadFile(defaultPath)
			require.NoError(t, err)
			require.Equal(t, defaultContent, string(unchanged))
		})
	}
}
