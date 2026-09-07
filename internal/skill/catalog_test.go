package skill

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadCatalogFromSkillsYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "skills.yaml"), `skills:
  - name: my-skill
    description: A test skill
    path: skills/my-skill
    tags: [devops]
`)
	writeFile(t, filepath.Join(dir, "skills", "my-skill", "SKILL.md"), "# My Skill")

	catalog, err := ReadCatalog(dir)
	require.NoError(t, err)
	require.Equal(t, []Entry{{
		Name: "my-skill", Description: "A test skill", Path: "skills/my-skill", Tags: []string{"devops"},
	}}, catalog.Skills)
}

func TestReadCatalogFromMarketplaceJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude-plugin", "marketplace.json"), `{
  "plugins": [
    {
      "name": "Test Plugin",
      "description": "A test plugin",
      "skills": ["./skills/skill-a", "./skills/skill-b"]
    }
  ]
}`)
	for _, name := range []string{"skill-a", "skill-b"} {
		content := "---\nname: " + name + "\ndescription: " + name + " desc\n---\n# " + name
		writeFile(t, filepath.Join(dir, "skills", name, "SKILL.md"), content)
	}

	catalog, err := ReadCatalog(dir)
	require.NoError(t, err)
	require.Len(t, catalog.Skills, 2)
	require.Equal(t, "skill-a", catalog.Skills[0].Name)
	require.Equal(t, "skills/skill-a", catalog.Skills[0].Path)
	require.Equal(t, "skill-b", catalog.Skills[1].Name)
}

func TestReadCatalogFromPluginJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude-plugin", "plugin.json"), `{"name": "test-plugin", "skills": "./.claude/skills"}`)
	for _, name := range []string{"skill-x", "skill-y"} {
		content := "---\nname: " + name + "\ndescription: " + name + " desc\n---\n# " + name
		writeFile(t, filepath.Join(dir, ".claude", "skills", name, "SKILL.md"), content)
	}

	catalog, err := ReadCatalog(dir)
	require.NoError(t, err)
	require.Len(t, catalog.Skills, 2)
	names := map[string]bool{}
	for _, s := range catalog.Skills {
		names[s.Name] = true
	}
	require.True(t, names["skill-x"])
	require.True(t, names["skill-y"])
}

func TestReadCatalogPluginJSONFallbackFromEmptyMarketplace(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// marketplace.json with plugins that have no skills arrays.
	writeFile(t, filepath.Join(dir, ".claude-plugin", "marketplace.json"),
		`{"plugins": [{"name": "test", "description": "test plugin", "source": "./"}]}`)
	writeFile(t, filepath.Join(dir, ".claude-plugin", "plugin.json"),
		`{"name": "test", "skills": "./.claude/skills"}`)
	writeFile(t, filepath.Join(dir, ".claude", "skills", "my-skill", "SKILL.md"),
		"---\nname: my-skill\ndescription: A skill\n---\n# My Skill")

	catalog, err := ReadCatalog(dir)
	require.NoError(t, err)
	require.Len(t, catalog.Skills, 1)
	require.Equal(t, "my-skill", catalog.Skills[0].Name)
}

func TestReadCatalogNoManifest(t *testing.T) {
	t.Parallel()

	_, err := ReadCatalog(t.TempDir())
	require.Error(t, err, "ReadCatalog() should fail when no catalog file exists")
}

func TestReadCatalogFromSkillsDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "skills", "alpha", "SKILL.md"),
		"---\nname: custom-alpha\ndescription: Alpha description\n---\n# Alpha")
	writeFile(t, filepath.Join(dir, "skills", "beta", "SKILL.md"), "# Beta")
	writeFile(t, filepath.Join(dir, "skills", "gamma", "SKILL.md"),
		"---\ndescription: Gamma description\n---\n# Gamma")
	writeFile(t, filepath.Join(dir, "skills", "assets", "example.txt"), "not a skill")
	writeFile(t, filepath.Join(dir, "skills", "README.md"), "not a skill")
	writeFile(t, filepath.Join(dir, "skills", "nested", "child", "SKILL.md"), "# Nested")
	writeFile(t, filepath.Join(dir, "skills", "invalid", "SKILL.md", "file.txt"), "not a regular SKILL.md")

	catalog, err := ReadCatalog(dir)
	require.NoError(t, err)
	want := []Entry{
		{Name: "custom-alpha", Description: "Alpha description", Path: "skills/alpha"},
		{Name: "beta", Path: "skills/beta"},
		{Name: "gamma", Description: "Gamma description", Path: "skills/gamma"},
	}
	require.Equal(t, want, catalog.Skills)
}

func TestReadCatalogPrefersManifestOverSkillsDirectory(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ path, content string }{
		{"skills.yaml", "skills:\n  - name: chosen\n    path: skills/chosen\n"},
		{"skills.yml", "skills:\n  - name: chosen\n    path: skills/chosen\n"},
		{".claude-plugin/marketplace.json", `{"plugins": [{"skills": ["skills/chosen"]}]}`},
		{".claude-plugin/plugin.json", `{"skills": "custom"}`},
	} {
		t.Run(tt.path, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, tt.path), tt.content)
			writeFile(t, filepath.Join(dir, "skills", "chosen", "SKILL.md"), "# Chosen")
			writeFile(t, filepath.Join(dir, "custom", "chosen", "SKILL.md"), "# Chosen")
			writeFile(t, filepath.Join(dir, "skills", "unlisted", "SKILL.md"), "# Unlisted")
			catalog, err := ReadCatalog(dir)
			require.NoError(t, err)
			require.Len(t, catalog.Skills, 1)
			require.Equal(t, "chosen", catalog.Skills[0].Name)
		})
	}
}

func TestReadCatalogSkillsDirectoryRequiresSkillMd(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "skills", "assets", "example.txt"), "not a skill")

	_, err := ReadCatalog(dir)
	require.Error(t, err, "ReadCatalog() should fail when the skills directory contains no skills")
}
