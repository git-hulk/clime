package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SourceType identifies how a plugin was installed.
const (
	SourceTypeGitHub = "github"
	SourceTypeNpm    = "npm"
	SourceTypeScript = "script"
)

// ManifestEntry represents a plugin installed via `clime plugin install`.
type ManifestEntry struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description,omitempty"`
	Version     string    `yaml:"version"`
	Type        string    `yaml:"type"`
	Source      string    `yaml:"source"`
	Repo        string    `yaml:"repo,omitempty"` // deprecated: migrated to Type+Source on load
	BinaryPath  string    `yaml:"binary_path,omitempty"`
	InstalledAt time.Time `yaml:"installed_at"`
}

// Manifest holds the list of managed plugins.
type Manifest struct {
	Plugins []ManifestEntry `yaml:"plugins"`
}

func manifestPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".clime", "plugins.yaml"), nil
}

// LoadManifest reads the plugin manifest from ~/.clime/plugins.yaml.
// Returns an empty manifest if the file does not exist.
func LoadManifest() (*Manifest, error) {
	path, err := manifestPath()
	if err != nil {
		return &Manifest{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Manifest{}, nil
		}
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.migrateRepo() {
		_ = m.Save()
	}
	return &m, nil
}

// migrateRepo converts legacy "repo" field entries to Type+Source.
// Returns true if any entries were migrated.
func (m *Manifest) migrateRepo() bool {
	migrated := false
	for i, p := range m.Plugins {
		if p.Type != "" || p.Repo == "" {
			continue
		}
		repo := p.Repo
		switch {
		case strings.HasPrefix(repo, "npm:"):
			m.Plugins[i].Type = SourceTypeNpm
			m.Plugins[i].Source = strings.TrimPrefix(repo, "npm:")
		case strings.HasPrefix(repo, "https://") || strings.HasPrefix(repo, "http://"):
			m.Plugins[i].Type = SourceTypeScript
			m.Plugins[i].Source = repo
		default:
			m.Plugins[i].Type = SourceTypeGitHub
			m.Plugins[i].Source = repo
		}
		m.Plugins[i].Repo = ""
		migrated = true
	}
	return migrated
}

// Save writes the manifest back to disk.
func (m *Manifest) Save() error {
	path, err := manifestPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Add adds or updates a plugin entry in the manifest.
func (m *Manifest) Add(name, version, sourceType, source, binaryPath string) {
	for i, p := range m.Plugins {
		if p.Name == name {
			m.Plugins[i].Version = version
			m.Plugins[i].Type = sourceType
			m.Plugins[i].Source = source
			m.Plugins[i].BinaryPath = binaryPath
			m.Plugins[i].InstalledAt = time.Now()
			return
		}
	}
	m.Plugins = append(m.Plugins, ManifestEntry{
		Name:        name,
		Version:     version,
		Type:        sourceType,
		Source:      source,
		BinaryPath:  binaryPath,
		InstalledAt: time.Now(),
	})
}

// SetDescription sets the description for a plugin entry in the manifest.
func (m *Manifest) SetDescription(name, description string) {
	for i, p := range m.Plugins {
		if p.Name == name {
			m.Plugins[i].Description = description
			return
		}
	}
}

// Remove removes a plugin entry from the manifest.
func (m *Manifest) Remove(name string) bool {
	for i, p := range m.Plugins {
		if p.Name == name {
			m.Plugins = append(m.Plugins[:i], m.Plugins[i+1:]...)
			return true
		}
	}
	return false
}

// Get returns a manifest entry by name.
func (m *Manifest) Get(name string) (ManifestEntry, bool) {
	for _, p := range m.Plugins {
		if p.Name == name {
			return p, true
		}
	}
	return ManifestEntry{}, false
}
