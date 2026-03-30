package plugin

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"gopkg.in/yaml.v3"
)

// DefaultPlugin describes a plugin entry in the remote defaults file.
type DefaultPlugin struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Repo        string   `yaml:"repo,omitempty"`
	Script      string   `yaml:"script,omitempty"`
	BinaryPath  string   `yaml:"binary_path,omitempty"`
	Npm         string   `yaml:"npm,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
}

// DefaultPlugins is the top-level structure of the remote defaults YAML.
type DefaultPlugins struct {
	Plugins []DefaultPlugin `yaml:"plugins"`
}

// LoadDefaultPluginsFromFile reads and parses a default plugins YAML file from a local path.
func LoadDefaultPluginsFromFile(path string) (*DefaultPlugins, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read local plugins file: %w", err)
	}

	var defaults DefaultPlugins
	if err := yaml.Unmarshal(data, &defaults); err != nil {
		return nil, fmt.Errorf("failed to parse default plugins YAML: %w", err)
	}

	return &defaults, nil
}

// FetchDefaultPlugins downloads and parses a default plugins YAML file from a URL.
func FetchDefaultPlugins(url string) (*DefaultPlugins, error) {
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

	var defaults DefaultPlugins
	if err := yaml.Unmarshal(data, &defaults); err != nil {
		return nil, fmt.Errorf("failed to parse default plugins YAML: %w", err)
	}

	return &defaults, nil
}
