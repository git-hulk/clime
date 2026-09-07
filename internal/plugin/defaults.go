package plugin

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Plugin describes a plugin entry in the remote defaults file.
type Plugin struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Repo        string   `yaml:"repo,omitempty"`
	Script      string   `yaml:"script,omitempty"`
	BinaryPath  string   `yaml:"binary_path,omitempty"`
	Npm         string   `yaml:"npm,omitempty"`
	Brew        string   `yaml:"brew,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
}

// Plugins is the top-level structure of the remote defaults YAML.
type PluginList struct {
	Plugins []Plugin `yaml:"plugins"`
}

// LoadPluginsFromFile reads and parses a default plugins YAML file from a local path.
func LoadPluginsFromFile(path string) (*PluginList, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read local plugins file: %w", err)
	}

	var defaults PluginList
	if err := yaml.Unmarshal(data, &defaults); err != nil {
		return nil, fmt.Errorf("failed to parse default plugins YAML: %w", err)
	}

	return &defaults, nil
}

// CategorizeForInit splits the resolved plugin list into:
//   - toInstall:   not present in the manifest.
//   - toReinstall: present with type=script but the local source URL differs
//     from the remote-declared script URL. Drift for brew/npm/github is
//     intentionally not detected here — those install methods touch
//     system-wide state and need explicit user action to switch source.
//   - skipped:     already installed and matching, or non-script type.
func CategorizeForInit(plugins []Plugin, manifest *Manifest) (toInstall, toReinstall []Plugin, skipped []string) {
	for _, pluginConfig := range plugins {
		entry, exists := manifest.Get(pluginConfig.Name)
		if !exists {
			toInstall = append(toInstall, pluginConfig)
			continue
		}
		if pluginConfig.Script != "" && entry.Type == SourceTypeScript && entry.Source != pluginConfig.Script {
			toReinstall = append(toReinstall, pluginConfig)
			continue
		}
		skipped = append(skipped, pluginConfig.Name)
	}
	return
}

// FilterByTags returns plugins that should be installed based on the given
// tags. Untagged plugins are always included. Tagged plugins are only included
// if tags are provided and they share at least one tag with the provided list.
// If no tags are specified, tagged plugins are skipped.
func FilterByTags(plugins []Plugin, tags []string) []Plugin {
	tagSet := make(map[string]bool, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tagSet[tag] = true
		}
	}

	filtered := make([]Plugin, 0, len(plugins))
	for _, pluginConfig := range plugins {
		if len(pluginConfig.Tags) == 0 {
			filtered = append(filtered, pluginConfig)
			continue
		}
		for _, pluginTag := range pluginConfig.Tags {
			if tagSet[strings.TrimSpace(pluginTag)] {
				filtered = append(filtered, pluginConfig)
				break
			}
		}
	}
	return filtered
}

// FetchPlugins downloads and parses a default plugins YAML file from a URL.
func FetchPlugins(url string) (*PluginList, error) {
	response, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch default plugins: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch default plugins: HTTP %d", response.StatusCode)
	}

	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var defaults PluginList
	if err := yaml.Unmarshal(data, &defaults); err != nil {
		return nil, fmt.Errorf("failed to parse default plugins YAML: %w", err)
	}

	return &defaults, nil
}
