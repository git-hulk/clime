package installer

import (
	"path/filepath"
	"testing"

	"github.com/git-hulk/clime/internal/plugin"
)

func TestNpmInstallerUpdate(t *testing.T) {
	t.Parallel()

	var ranNpmUpdate bool
	n := &NpmInstaller{
		Package: "@myorg/clime-deploy",
		runNpmUpdate: func(pkg string) error {
			ranNpmUpdate = true
			if pkg != "@myorg/clime-deploy" {
				t.Fatalf("pkg = %q, want %q", pkg, "@myorg/clime-deploy")
			}
			return nil
		},
		pluginBinDir: func() (string, error) {
			return "/tmp/clime-plugin-test", nil
		},
		getVersion: func(pkg string) (string, error) {
			return plugin.VersionLatest, nil
		},
	}

	entry := plugin.ManifestEntry{
		Name:    "deploy",
		Version: plugin.VersionLatest,
		Type:    plugin.SourceTypeNpm,
		Source:  "@myorg/clime-deploy",
	}
	result, err := n.Update("deploy", entry)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !ranNpmUpdate {
		t.Fatal("npm update should run for npm source")
	}
	if !result.Updated {
		t.Fatal("Update() should mark updated for npm source")
	}
	if result.LatestVersion != plugin.VersionLatest {
		t.Fatalf("LatestVersion = %q, want %q", result.LatestVersion, plugin.VersionLatest)
	}
	wantPath := filepath.Join("/tmp/clime-plugin-test", "clime-deploy")
	if result.Path != wantPath {
		t.Fatalf("Path = %q, want %q", result.Path, wantPath)
	}
}

func TestNpmInstallerUpdateUpToDate(t *testing.T) {
	t.Parallel()

	n := &NpmInstaller{
		Package: "@myorg/clime-deploy",
		runNpmUpdate: func(pkg string) error {
			return nil
		},
		pluginBinDir: func() (string, error) {
			return "/tmp/clime-plugin-test", nil
		},
		getVersion: func(pkg string) (string, error) {
			return "1.2.3", nil
		},
	}

	entry := plugin.ManifestEntry{
		Name:    "deploy",
		Version: "1.2.3",
		Type:    plugin.SourceTypeNpm,
		Source:  "@myorg/clime-deploy",
	}
	result, err := n.Update("deploy", entry)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if result.Updated {
		t.Fatal("Update() should not mark updated when semver version is unchanged")
	}
}

func TestNpmInstallerPluginType(t *testing.T) {
	t.Parallel()
	n := NewNpmInstaller("@myorg/clime-deploy")
	if n.PluginType() != plugin.SourceTypeNpm {
		t.Fatalf("PluginType() = %q, want %q", n.PluginType(), plugin.SourceTypeNpm)
	}
	if n.Source() != "@myorg/clime-deploy" {
		t.Fatalf("Source() = %q, want %q", n.Source(), "@myorg/clime-deploy")
	}
}
