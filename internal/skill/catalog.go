package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Entry is one skill as offered by a source: its name, description, and
// path within a snapshot.
type Entry struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Path        string   `yaml:"path"`
	Tags        []string `yaml:"tags,omitempty"`
}

// Catalog lists the skills a source offers at one version.
type Catalog struct {
	Skills []Entry `yaml:"skills"`
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

// pluginFile represents the .claude-plugin/plugin.json format.
type pluginFile struct {
	Name   string `json:"name"`
	Skills string `json:"skills"`
}

// skillFrontmatter holds the YAML frontmatter from a SKILL.md file.
type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// Find returns the entry with the given skill name.
func (catalog *Catalog) Find(name string) (Entry, bool) {
	for _, entry := range catalog.Skills {
		if entry.Name == name {
			return entry, true
		}
	}
	return Entry{}, false
}

// ReadCatalog reads the skill catalog of a checkout directory, trying
// skills.yaml, skills.yml, .claude-plugin/marketplace.json,
// .claude-plugin/plugin.json, and the skills directory in that order.
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

	if catalog, err := readSkillsDir(dir, "skills"); err == nil {
		catalog.Skills = slices.DeleteFunc(catalog.Skills, func(entry Entry) bool {
			info, err := os.Stat(filepath.Join(dir, entry.Path, "SKILL.md"))
			return err != nil || !info.Mode().IsRegular()
		})
		if len(catalog.Skills) > 0 {
			return catalog, nil
		}
	}

	return nil, fmt.Errorf("no skills catalog found: tried skills.yaml, skills.yml, .claude-plugin/marketplace.json, .claude-plugin/plugin.json, and skills/*/SKILL.md")
}

// parseMarketplaceManifest reads .claude-plugin/marketplace.json and builds
// a catalog by reading each skill's SKILL.md frontmatter.
func parseMarketplaceManifest(dir string) (*Catalog, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "marketplace.json"))
	if err != nil {
		return nil, err
	}

	var marketplace marketplaceFile
	if err := json.Unmarshal(data, &marketplace); err != nil {
		return nil, fmt.Errorf("failed to parse marketplace.json: %w", err)
	}

	var catalog Catalog
	seen := make(map[string]bool)
	for _, plugin := range marketplace.Plugins {
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

			catalog.Skills = append(catalog.Skills, entryFromSkillDir(dir, skillPath))
		}
	}
	return &catalog, nil
}

// parsePluginManifest reads .claude-plugin/plugin.json and discovers skills
// by scanning the skills directory it references.
func parsePluginManifest(dir string) (*Catalog, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	if err != nil {
		return nil, err
	}

	var pluginManifest pluginFile
	if err := json.Unmarshal(data, &pluginManifest); err != nil {
		return nil, fmt.Errorf("failed to parse plugin.json: %w", err)
	}
	if pluginManifest.Skills == "" {
		return nil, fmt.Errorf("plugin.json has no skills directory")
	}

	return readSkillsDir(dir, strings.TrimPrefix(pluginManifest.Skills, "./"))
}

// readSkillsDir builds a catalog from the immediate subdirectories of skillsDir.
func readSkillsDir(dir, skillsDir string) (*Catalog, error) {
	entries, err := os.ReadDir(filepath.Join(dir, skillsDir))
	if err != nil {
		return nil, fmt.Errorf("failed to read skills directory: %w", err)
	}

	var catalog Catalog
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		catalog.Skills = append(catalog.Skills, entryFromSkillDir(dir, filepath.Join(skillsDir, entry.Name())))
	}
	return &catalog, nil
}

// entryFromSkillDir builds a catalog entry for a skill directory, taking
// name and description from SKILL.md frontmatter and falling back to the
// directory's basename.
func entryFromSkillDir(dir, skillPath string) Entry {
	entry := Entry{Path: skillPath}
	if frontmatter, err := readSkillFrontmatter(filepath.Join(dir, skillPath, "SKILL.md")); err == nil {
		entry.Name = frontmatter.Name
		entry.Description = frontmatter.Description
	}
	if entry.Name == "" {
		entry.Name = filepath.Base(skillPath)
	}
	return entry
}

// parseSkillFrontmatter parses the YAML frontmatter between --- markers
// from SKILL.md content.
func parseSkillFrontmatter(data []byte) (*skillFrontmatter, error) {
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, fmt.Errorf("no frontmatter found")
	}
	var frontmatterLines []string
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		frontmatterLines = append(frontmatterLines, line)
	}
	var frontmatter skillFrontmatter
	if err := yaml.Unmarshal([]byte(strings.Join(frontmatterLines, "\n")), &frontmatter); err != nil {
		return nil, err
	}
	return &frontmatter, nil
}

// readSkillFrontmatter reads and parses the YAML frontmatter from a
// SKILL.md file on disk.
func readSkillFrontmatter(path string) (*skillFrontmatter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseSkillFrontmatter(data)
}
