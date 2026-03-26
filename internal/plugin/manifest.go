package plugin

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// ManifestEntry represents a plugin installed via `clime plugin install`.
type ManifestEntry struct {
	Name        string    `yaml:"name"`
	Version     string    `yaml:"version"`
	Repo        string    `yaml:"repo"`
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
	return &m, nil
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
func (m *Manifest) Add(name, version, repo string) {
	for i, p := range m.Plugins {
		if p.Name == name {
			m.Plugins[i].Version = version
			m.Plugins[i].Repo = repo
			m.Plugins[i].InstalledAt = time.Now()
			return
		}
	}
	m.Plugins = append(m.Plugins, ManifestEntry{
		Name:        name,
		Version:     version,
		Repo:        repo,
		InstalledAt: time.Now(),
	})
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
