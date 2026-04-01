package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/git-hulk/clime/internal/plugin"
)

func TestBrewInstallerInstall(t *testing.T) {
	t.Parallel()

	brewBin := t.TempDir()
	pluginDir := t.TempDir()
	installedBin := filepath.Join(brewBin, "clime-deploy")
	if err := os.WriteFile(installedBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write installed binary: %v", err)
	}

	var ranInstall bool
	b := &BrewInstaller{
		Formula: "acme/tap/clime-deploy",
		runBrewInstall: func(formula string) error {
			ranInstall = true
			if formula != "acme/tap/clime-deploy" {
				t.Fatalf("formula = %q, want %q", formula, "acme/tap/clime-deploy")
			}
			return nil
		},
		brewBinDir: func() (string, error) {
			return brewBin, nil
		},
		pluginBinDir: func() (string, error) {
			return pluginDir, nil
		},
		getVersion: func(formula string) (string, error) {
			return "1.2.3", nil
		},
		lookPath: func(name string) (string, error) {
			if name == "brew" {
				return "/opt/homebrew/bin/brew", nil
			}
			return "", fmt.Errorf("not found")
		},
	}

	version, err := b.Install("deploy")
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !ranInstall {
		t.Fatal("brew install should run")
	}
	if version != "1.2.3" {
		t.Fatalf("version = %q, want %q", version, "1.2.3")
	}

	linkPath := filepath.Join(pluginDir, "clime-deploy")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("symlink not created: %v", err)
	}
	if target != installedBin {
		t.Fatalf("symlink target = %q, want %q", target, installedBin)
	}
}

func TestBrewInstallerUpdate(t *testing.T) {
	t.Parallel()

	var ranUpdate bool
	b := &BrewInstaller{
		Formula: "acme/tap/clime-deploy",
		runBrewUpdate: func(formula string) error {
			ranUpdate = true
			if formula != "acme/tap/clime-deploy" {
				t.Fatalf("formula = %q, want %q", formula, "acme/tap/clime-deploy")
			}
			return nil
		},
		pluginBinDir: func() (string, error) {
			return "/tmp/clime-plugin-test", nil
		},
		getVersion: func(formula string) (string, error) {
			return plugin.VersionLatest, nil
		},
	}

	entry := plugin.ManifestEntry{
		Name:    "deploy",
		Version: plugin.VersionLatest,
		Type:    plugin.SourceTypeBrew,
		Source:  "acme/tap/clime-deploy",
	}
	result, err := b.Update("deploy", entry)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !ranUpdate {
		t.Fatal("brew update should run for brew source")
	}
	if !result.Updated {
		t.Fatal("Update() should mark updated for brew source")
	}
	if result.LatestVersion != plugin.VersionLatest {
		t.Fatalf("LatestVersion = %q, want %q", result.LatestVersion, plugin.VersionLatest)
	}
}

func TestBrewInstallerUpdateUpToDate(t *testing.T) {
	t.Parallel()

	b := &BrewInstaller{
		Formula: "acme/tap/clime-deploy",
		runBrewUpdate: func(formula string) error {
			return nil
		},
		pluginBinDir: func() (string, error) {
			return "/tmp/clime-plugin-test", nil
		},
		getVersion: func(formula string) (string, error) {
			return "1.2.3", nil
		},
	}

	entry := plugin.ManifestEntry{
		Name:    "deploy",
		Version: "1.2.3",
		Type:    plugin.SourceTypeBrew,
		Source:  "acme/tap/clime-deploy",
	}
	result, err := b.Update("deploy", entry)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if result.Updated {
		t.Fatal("Update() should not mark updated when semver version is unchanged")
	}
}

func TestBrewInstallerPluginType(t *testing.T) {
	t.Parallel()
	b := NewBrewInstaller("acme/tap/clime-foo")
	if b.PluginType() != plugin.SourceTypeBrew {
		t.Fatalf("PluginType() = %q, want %q", b.PluginType(), plugin.SourceTypeBrew)
	}
	if b.Source() != "acme/tap/clime-foo" {
		t.Fatalf("Source() = %q, want %q", b.Source(), "acme/tap/clime-foo")
	}
}
