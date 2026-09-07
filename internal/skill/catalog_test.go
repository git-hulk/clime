package skill

import (
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseCatalogYAML(t *testing.T) {
	t.Parallel()

	yamlContent := `skills:
  - name: docker-helper
    description: Docker management skill
    path: skills/docker-helper
    tags:
      - devops
  - name: git-wizard
    description: Git workflow automation
    path: skills/git-wizard
`
	var catalog Catalog
	if err := yaml.Unmarshal([]byte(yamlContent), &catalog); err != nil {
		t.Fatalf("failed to parse yaml: %v", err)
	}

	if len(catalog.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(catalog.Skills))
	}
	if catalog.Skills[0].Name != "docker-helper" {
		t.Fatalf("expected first skill name docker-helper, got %s", catalog.Skills[0].Name)
	}
	if catalog.Skills[0].Path != "skills/docker-helper" {
		t.Fatalf("expected path skills/docker-helper, got %s", catalog.Skills[0].Path)
	}
	if len(catalog.Skills[0].Tags) != 1 || catalog.Skills[0].Tags[0] != "devops" {
		t.Fatalf("unexpected tags: %v", catalog.Skills[0].Tags)
	}

	entry, ok := catalog.Find("git-wizard")
	if !ok || entry.Path != "skills/git-wizard" {
		t.Fatalf("Find(git-wizard) = (%+v, %v)", entry, ok)
	}
	if _, ok := catalog.Find("missing"); ok {
		t.Fatal("Find(missing) should report absence")
	}
}

func TestReadCatalogFromSkillsYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "skills.yaml"), `skills:
  - name: my-skill
    description: A test skill
    path: skills/my-skill
`)
	writeFile(t, filepath.Join(dir, "skills", "my-skill", "SKILL.md"), "# My Skill")

	catalog, err := ReadCatalog(dir)
	if err != nil {
		t.Fatalf("ReadCatalog() error = %v", err)
	}
	if len(catalog.Skills) != 1 || catalog.Skills[0].Name != "my-skill" {
		t.Fatalf("catalog = %+v, want the my-skill entry", catalog.Skills)
	}
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
	if err != nil {
		t.Fatalf("ReadCatalog() error = %v", err)
	}
	if len(catalog.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(catalog.Skills))
	}
	if catalog.Skills[0].Name != "skill-a" || catalog.Skills[0].Path != "skills/skill-a" {
		t.Fatalf("first entry = %+v", catalog.Skills[0])
	}
	if catalog.Skills[1].Name != "skill-b" {
		t.Fatalf("second entry = %+v", catalog.Skills[1])
	}
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
	if err != nil {
		t.Fatalf("ReadCatalog() error = %v", err)
	}
	if len(catalog.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(catalog.Skills))
	}
	names := map[string]bool{}
	for _, s := range catalog.Skills {
		names[s.Name] = true
	}
	if !names["skill-x"] || !names["skill-y"] {
		t.Fatalf("expected skill-x and skill-y, got %v", names)
	}
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
	if err != nil {
		t.Fatalf("ReadCatalog() error = %v", err)
	}
	if len(catalog.Skills) != 1 || catalog.Skills[0].Name != "my-skill" {
		t.Fatalf("catalog = %+v, want my-skill via plugin.json fallback", catalog.Skills)
	}
}

func TestReadCatalogEmptyNameFallsBackToDirName(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude-plugin", "marketplace.json"),
		`{"plugins": [{"name": "P", "skills": ["./skills/my-skill"]}]}`)
	// Frontmatter with no name field.
	writeFile(t, filepath.Join(dir, "skills", "my-skill", "SKILL.md"),
		"---\ndescription: has desc but no name\n---\n# Skill")

	catalog, err := ReadCatalog(dir)
	if err != nil {
		t.Fatalf("ReadCatalog() error = %v", err)
	}
	if len(catalog.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(catalog.Skills))
	}
	if catalog.Skills[0].Name != "my-skill" {
		t.Fatalf("expected name my-skill (from directory), got %q", catalog.Skills[0].Name)
	}
	if catalog.Skills[0].Description != "has desc but no name" {
		t.Fatalf("expected description preserved, got %q", catalog.Skills[0].Description)
	}
}

func TestReadCatalogNoManifest(t *testing.T) {
	t.Parallel()

	if _, err := ReadCatalog(t.TempDir()); err == nil {
		t.Fatal("ReadCatalog() should fail when no catalog file exists")
	}
}
