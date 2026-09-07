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

func TestManifestGroupsSkillsBySource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skills.yaml")
	content := `AfterShip/Skills:
    skills:
        - rest-api-design
        - test-abc
    version: f8c4c0e02021b0debef257750d4d020e9dad38aa
other/repo:
    skills:
        - other-skill
    version: v1.2.3
tracked/only:
    skills: []
`
	writeFile(t, path, content)
	manifest, err := LoadManifest(path)
	require.NoError(t, err)
	require.Len(t, manifest.Sources, 3)
	require.Equal(t, []InstalledSkill{
		{Name: "rest-api-design", Source: "AfterShip/Skills"},
		{Name: "test-abc", Source: "AfterShip/Skills"},
		{Name: "other-skill", Source: "other/repo"},
	}, manifest.Skills)
	record, found := manifest.GetSource(Source{Repo: "aftership/skills"})
	require.True(t, found)
	require.Equal(t, "f8c4c0e02021b0debef257750d4d020e9dad38aa", record.Version)
	require.NoError(t, manifest.Save())
	written, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, content, string(written))

	manifest.RemoveSkill("test-abc")
	manifest.AddSkill(InstalledSkill{Name: "new-skill", Source: "AfterShip/Skills"})
	manifest.SetSourceVersion(Source{Repo: "aftership/skills"}, "v2.0.0")
	require.NoError(t, manifest.Save())
	reloaded, err := LoadManifest(path)
	require.NoError(t, err)
	require.Equal(t, manifest.SkillsFrom(Source{Repo: "AfterShip/Skills"}), reloaded.SkillsFrom(Source{Repo: "AfterShip/Skills"}))
	record, found = reloaded.GetSource(Source{Repo: "AfterShip/Skills"})
	require.True(t, found)
	require.Equal(t, "v2.0.0", record.Version)
}

func TestManifestRejectsInvalidSourceGroups(t *testing.T) {
	for _, content := range []string{
		"skills: []\nsources: []\n",
		"owner/repo: {skills: [alpha]}\nowner/repo: {skills: [beta]}\n",
		"owner/repo: {skills: []}\nOwner/Repo: {skills: []}\n",
		"owner/repo: {skills: [alpha, alpha]}\n",
		"owner/repo: {skills: [alpha]}\nother/repo: {skills: [alpha]}\n",
		"owner/repo: {skills: [../alpha]}\n",
	} {
		t.Run(content, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "skills.yaml")
			writeFile(t, path, content)
			_, err := LoadManifest(path)
			require.ErrorContains(t, err, "failed to parse "+path)
			unchanged, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, content, string(unchanged))
		})
	}
}

func TestLoadManifestParseErrorNamesFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeManifestFile(t, home, "skills: [oops\n")

	_, err := LoadManifest("")
	require.ErrorContains(t, err, filepath.Join(home, ".clime", "skills.yaml"), "expected a parse error")
}

func TestCustomManifestPersistsToSelectedPath(t *testing.T) {
	for _, initial := range []string{"", "{}\n", "owner/repo:\n  skills: []\n  version: v1.2.3\n"} {
		t.Run(initial, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Chdir(t.TempDir())
			defaultPath := filepath.Join(home, ".clime", "skills.yaml")
			defaultContent := "{}\n"
			writeFile(t, defaultPath, defaultContent)
			path := filepath.Join("config", "skills.yaml")
			if initial != "" {
				writeFile(t, path, initial)
			}

			manifest, err := LoadManifest(path)
			require.NoError(t, err)
			manifest.AddSkill(InstalledSkill{Name: "custom", Source: "owner/repo"})
			require.NoError(t, manifest.Save())
			reloaded, err := LoadManifest(path)
			require.NoError(t, err)
			installed, found := reloaded.GetSkill("custom")
			require.True(t, found)
			require.Equal(t, "owner/repo", installed.Source)
			unchanged, err := os.ReadFile(defaultPath)
			require.NoError(t, err)
			require.Equal(t, defaultContent, string(unchanged))
		})
	}
}
