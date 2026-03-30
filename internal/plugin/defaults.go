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

// FilterByTags returns plugins that should be installed based on the given
// tags. Untagged plugins are always included. Tagged plugins are only included
// if tags are provided and they share at least one tag with the provided list.
// If no tags are specified, tagged plugins are skipped.
func FilterByTags(plugins []Plugin, tags []string) []Plugin {
	tagSet := make(map[string]bool, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t != "" {
			tagSet[t] = true
		}
	}

	filtered := make([]Plugin, 0, len(plugins))
	for _, p := range plugins {
		if len(p.Tags) == 0 {
			filtered = append(filtered, p)
			continue
		}
		for _, pt := range p.Tags {
			if tagSet[strings.TrimSpace(pt)] {
				filtered = append(filtered, p)
				break
			}
		}
	}
	return filtered
}

// FetchPlugins downloads and parses a default plugins YAML file from a URL.
func FetchPlugins(url string) (*PluginList, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch default plugins: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch default plugins: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var defaults PluginList
	if err := yaml.Unmarshal(data, &defaults); err != nil {
		return nil, fmt.Errorf("failed to parse default plugins YAML: %w", err)
	}

	return &defaults, nil
}
