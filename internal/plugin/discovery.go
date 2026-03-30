package plugin

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const binPrefix = "clime-"

// Plugin represents a discovered plugin binary.
type Plugin struct {
	Name        string // e.g. "hr" (derived from clime-hr)
	Path        string // absolute path to the binary
	Description string // optional short description for help output
}

// Find looks for a plugin binary matching the given name.
// It searches ~/.clime/plugins/ and $PATH for an executable named clime-<name>.
func Find(name string) (string, bool) {
	binName := binPrefix + name
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}

	// Check ~/.clime/plugins/ first (managed plugins take priority)
	homeDir, err := os.UserHomeDir()
	if err == nil {
		managed := filepath.Join(homeDir, ".clime", "plugins", binName)
		if isExecutable(managed) {
			return managed, true
		}
	}

	// Scan $PATH
	pathEnv := os.Getenv("PATH")
	for _, dir := range strings.Split(pathEnv, string(os.PathListSeparator)) {
		candidate := filepath.Join(dir, binName)
		if isExecutable(candidate) {
			return candidate, true
		}
	}

	return "", false
}

// Discover returns all plugins found in ~/.clime/plugins/ and $PATH.
// Descriptions are populated from the plugin manifest when available.
func Discover() []Plugin {
	seen := make(map[string]bool)
	var plugins []Plugin

	// Check ~/.clime/plugins/ first
	homeDir, err := os.UserHomeDir()
	if err == nil {
		managedDir := filepath.Join(homeDir, ".clime", "plugins")
		plugins = append(plugins, scanDir(managedDir, seen)...)
	}

	// Scan $PATH
	pathEnv := os.Getenv("PATH")
	for _, dir := range strings.Split(pathEnv, string(os.PathListSeparator)) {
		plugins = append(plugins, scanDir(dir, seen)...)
	}

	// Populate descriptions from the manifest
	manifest, err := LoadManifest()
	if err == nil {
		for i, p := range plugins {
			if entry, ok := manifest.Get(p.Name); ok && entry.Description != "" {
				plugins[i].Description = entry.Description
			}
		}
	}

	return plugins
}

func scanDir(dir string, seen map[string]bool) []Plugin {
	var plugins []Plugin
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, binPrefix) {
			continue
		}
		pluginName := strings.TrimPrefix(name, binPrefix)
		if runtime.GOOS == "windows" {
			pluginName = strings.TrimSuffix(pluginName, ".exe")
		}
		if pluginName == "" || seen[pluginName] {
			continue
		}
		fullPath := filepath.Join(dir, name)
		if isExecutable(fullPath) {
			seen[pluginName] = true
			plugins = append(plugins, Plugin{Name: pluginName, Path: fullPath})
		}
	}
	return plugins
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.IsDir() {
		return false
	}
	// On Unix, check execute bit
	if runtime.GOOS != "windows" {
		return info.Mode()&0111 != 0
	}
	return true
}
