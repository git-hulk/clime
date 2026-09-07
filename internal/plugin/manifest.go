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
	SourceTypeBrew   = "brew"

	// VersionLatest is the fallback version used when actual version
	// detection is unavailable or fails.
	VersionLatest = "latest"
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
	InitURL string          `yaml:"init_url,omitempty"`
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
	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	if manifest.migrateRepo() {
		_ = manifest.Save()
	}
	return &manifest, nil
}

// migrateRepo converts legacy "repo" field entries to Type+Source.
// Returns true if any entries were migrated.
func (manifest *Manifest) migrateRepo() bool {
	migrated := false
	for i, entry := range manifest.Plugins {
		if entry.Type != "" || entry.Repo == "" {
			continue
		}
		repo := entry.Repo
		switch {
		case strings.HasPrefix(repo, "npm:"):
			manifest.Plugins[i].Type = SourceTypeNpm
			manifest.Plugins[i].Source = strings.TrimPrefix(repo, "npm:")
		case strings.HasPrefix(repo, "brew:"):
			manifest.Plugins[i].Type = SourceTypeBrew
			manifest.Plugins[i].Source = strings.TrimPrefix(repo, "brew:")
		case strings.HasPrefix(repo, "https://") || strings.HasPrefix(repo, "http://"):
			manifest.Plugins[i].Type = SourceTypeScript
			manifest.Plugins[i].Source = repo
		default:
			manifest.Plugins[i].Type = SourceTypeGitHub
			manifest.Plugins[i].Source = repo
		}
		manifest.Plugins[i].Repo = ""
		migrated = true
	}
	return migrated
}

// Save writes the manifest back to disk.
func (manifest *Manifest) Save() error {
	path, err := manifestPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Add adds or updates a plugin entry in the manifest.
func (manifest *Manifest) Add(name, version, sourceType, source, binaryPath string) {
	for i, entry := range manifest.Plugins {
		if entry.Name == name {
			manifest.Plugins[i].Version = version
			manifest.Plugins[i].Type = sourceType
			manifest.Plugins[i].Source = source
			manifest.Plugins[i].BinaryPath = binaryPath
			manifest.Plugins[i].InstalledAt = time.Now()
			return
		}
	}
	manifest.Plugins = append(manifest.Plugins, ManifestEntry{
		Name:        name,
		Version:     version,
		Type:        sourceType,
		Source:      source,
		BinaryPath:  binaryPath,
		InstalledAt: time.Now(),
	})
}

// SetDescription sets the description for a plugin entry in the manifest.
func (manifest *Manifest) SetDescription(name, description string) {
	for i, entry := range manifest.Plugins {
		if entry.Name == name {
			manifest.Plugins[i].Description = description
			return
		}
	}
}

// Remove removes a plugin entry from the manifest.
func (manifest *Manifest) Remove(name string) bool {
	for i, entry := range manifest.Plugins {
		if entry.Name == name {
			manifest.Plugins = append(manifest.Plugins[:i], manifest.Plugins[i+1:]...)
			return true
		}
	}
	return false
}

// Get returns a manifest entry by name.
func (manifest *Manifest) Get(name string) (ManifestEntry, bool) {
	for _, entry := range manifest.Plugins {
		if entry.Name == name {
			return entry, true
		}
	}
	return ManifestEntry{}, false
}
