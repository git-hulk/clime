package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// InstalledSkill tracks a skill that has been installed locally.
type InstalledSkill struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description,omitempty"`
	Source      string    `yaml:"source"`
	Path        string    `yaml:"path"`
	InstalledAt time.Time `yaml:"installed_at"`
}

// LegacyManifest holds installed skills and known sources.
type LegacyManifest struct {
	Skills  []InstalledSkill `yaml:"skills"`
	Sources []string         `yaml:"sources,omitempty"`
}

func legacyManifestPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".clime", "skills.yaml"), nil
}

// LoadLegacyManifest reads the skills manifest from ~/.clime/skills.yaml.
// Creates the directory and an empty manifest file if they do not exist.
func LoadLegacyManifest() (*LegacyManifest, error) {
	path, err := legacyManifestPath()
	if err != nil {
		return nil, fmt.Errorf("failed to determine manifest path: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			m := &LegacyManifest{}
			if err := m.Save(); err != nil {
				return nil, fmt.Errorf("failed to create skills manifest: %w", err)
			}
			return m, nil
		}
		return nil, err
	}
	var m LegacyManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Save writes the manifest to disk.
func (m *LegacyManifest) Save() error {
	path, err := legacyManifestPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// AddSkill adds or updates an installed skill entry.
func (m *LegacyManifest) AddSkill(s InstalledSkill) {
	for i, existing := range m.Skills {
		if existing.Name == s.Name {
			m.Skills[i] = s
			return
		}
	}
	m.Skills = append(m.Skills, s)
}

// RemoveSkill removes an installed skill entry.
func (m *LegacyManifest) RemoveSkill(name string) bool {
	for i, s := range m.Skills {
		if s.Name == name {
			m.Skills = append(m.Skills[:i], m.Skills[i+1:]...)
			return true
		}
	}
	return false
}

// GetSkill returns an installed skill by name.
func (m *LegacyManifest) GetSkill(name string) (InstalledSkill, bool) {
	for _, s := range m.Skills {
		if s.Name == name {
			return s, true
		}
	}
	return InstalledSkill{}, false
}

// AddSource adds a source to the known sources list if not already present.
func (m *LegacyManifest) AddSource(source string) {
	for _, s := range m.Sources {
		if s == source {
			return
		}
	}
	m.Sources = append(m.Sources, source)
}

// RemoveSource removes a source from the known sources list.
func (m *LegacyManifest) RemoveSource(source string) bool {
	for i, s := range m.Sources {
		if s == source {
			m.Sources = append(m.Sources[:i], m.Sources[i+1:]...)
			return true
		}
	}
	return false
}
