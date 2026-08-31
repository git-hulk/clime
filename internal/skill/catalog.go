package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SkillEntry describes a skill in a repository catalog.
type SkillEntry struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Path        string   `yaml:"path"`
	Tags        []string `yaml:"tags,omitempty"`
}

// Catalog maps skill names to paths within one repository snapshot.
type Catalog struct {
	Skills []SkillEntry `yaml:"skills"`
}

// Find returns the catalog entry with the given skill name.
func (c *Catalog) Find(name string) (SkillEntry, bool) {
	for _, s := range c.Skills {
		if s.Name == name {
			return s, true
		}
	}
	return SkillEntry{}, false
}

// ReadCatalog reads the skill catalog from a repository snapshot, trying
// root skills.yaml / skills.yml first, then .claude-plugin/marketplace.json,
// then .claude-plugin/plugin.json.
func ReadCatalog(dir string) (*Catalog, error) {
	var data []byte
	var err error
	for _, name := range []string{"skills.yaml", "skills.yml"} {
		data, err = os.ReadFile(filepath.Join(dir, name))
		if err == nil {
			break
		}
	}
	if err == nil {
		var catalog Catalog
		if err := yaml.Unmarshal(data, &catalog); err != nil {
			return nil, fmt.Errorf("failed to parse skills.yaml: %w", err)
		}
		return &catalog, nil
	}

	if catalog, err := parseMarketplaceManifest(dir); err == nil && len(catalog.Skills) > 0 {
		return catalog, nil
	}

	if catalog, err := parsePluginManifest(dir); err == nil && len(catalog.Skills) > 0 {
		return catalog, nil
	}

	return nil, fmt.Errorf("no skills catalog found: tried skills.yaml, skills.yml, .claude-plugin/marketplace.json, and .claude-plugin/plugin.json")
}

// skillContentDir validates that entry's path stays inside the snapshot root
// and contains SKILL.md, returning the absolute skill directory.
func skillContentDir(root string, entry SkillEntry) (string, error) {
	rel := filepath.Clean(strings.TrimPrefix(entry.Path, "./"))
	if rel == "." {
		rel = ""
	}
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("skill %q path %q escapes the repository root", entry.Name, entry.Path)
	}
	dir := filepath.Join(root, rel)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("skill %q path %q not found in repository snapshot", entry.Name, entry.Path)
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		return "", fmt.Errorf("skill %q is missing required SKILL.md at %q", entry.Name, entry.Path)
	}
	return dir, nil
}

// marketplaceFile represents the .claude-plugin/marketplace.json format.
type marketplaceFile struct {
	Plugins []marketplacePlugin `json:"plugins"`
}

type marketplacePlugin struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Source      string   `json:"source"`
	Skills      []string `json:"skills"`
}

// parseMarketplaceManifest reads .claude-plugin/marketplace.json from a local
// directory and builds a catalog by reading each skill's SKILL.md frontmatter.
func parseMarketplaceManifest(dir string) (*Catalog, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "marketplace.json"))
	if err != nil {
		return nil, err
	}

	var mf marketplaceFile
	if err := json.Unmarshal(data, &mf); err != nil {
		return nil, fmt.Errorf("failed to parse marketplace.json: %w", err)
	}

	var catalog Catalog
	seen := make(map[string]bool)
	for _, plugin := range mf.Plugins {
		sourceDir := strings.TrimPrefix(plugin.Source, "./")
		for _, skillPath := range plugin.Skills {
			skillPath = strings.TrimPrefix(skillPath, "./")
			if sourceDir != "" {
				skillPath = filepath.Join(sourceDir, skillPath)
			}
			if seen[skillPath] {
				continue
			}
			seen[skillPath] = true

			entry := SkillEntry{Path: skillPath}
			if fm, err := readSkillFrontmatter(filepath.Join(dir, skillPath, "SKILL.md")); err == nil {
				entry.Name = fm.Name
				entry.Description = fm.Description
			}
			if entry.Name == "" {
				entry.Name = filepath.Base(skillPath)
			}
			catalog.Skills = append(catalog.Skills, entry)
		}
	}
	return &catalog, nil
}

// pluginFile represents the .claude-plugin/plugin.json format.
type pluginFile struct {
	Name   string `json:"name"`
	Skills string `json:"skills"`
}

// parsePluginManifest reads .claude-plugin/plugin.json from a local directory
// and discovers skills by scanning the skills directory it references.
func parsePluginManifest(dir string) (*Catalog, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	if err != nil {
		return nil, err
	}

	var pf pluginFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("failed to parse plugin.json: %w", err)
	}
	if pf.Skills == "" {
		return nil, fmt.Errorf("plugin.json has no skills directory")
	}

	skillsDir := strings.TrimPrefix(pf.Skills, "./")
	entries, err := os.ReadDir(filepath.Join(dir, skillsDir))
	if err != nil {
		return nil, fmt.Errorf("failed to read skills directory: %w", err)
	}

	var catalog Catalog
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillPath := filepath.Join(skillsDir, e.Name())
		entry := SkillEntry{Path: skillPath}
		if fm, err := readSkillFrontmatter(filepath.Join(dir, skillPath, "SKILL.md")); err == nil {
			entry.Name = fm.Name
			entry.Description = fm.Description
		}
		if entry.Name == "" {
			entry.Name = e.Name()
		}
		catalog.Skills = append(catalog.Skills, entry)
	}
	return &catalog, nil
}

// skillFrontmatter holds the YAML frontmatter from a SKILL.md file.
type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// parseSkillFrontmatter parses the YAML frontmatter between --- markers from SKILL.md content.
func parseSkillFrontmatter(data []byte) (*skillFrontmatter, error) {
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, fmt.Errorf("no frontmatter found")
	}
	var fmLines []string
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		fmLines = append(fmLines, line)
	}
	var fm skillFrontmatter
	if err := yaml.Unmarshal([]byte(strings.Join(fmLines, "\n")), &fm); err != nil {
		return nil, err
	}
	return &fm, nil
}

// readSkillFrontmatter reads and parses the YAML frontmatter from a SKILL.md file on disk.
func readSkillFrontmatter(path string) (*skillFrontmatter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseSkillFrontmatter(data)
}
