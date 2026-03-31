package installer

import (
	"testing"

	"github.com/git-hulk/clime/internal/plugin"
)

func TestScriptInstallerUpdate(t *testing.T) {
	t.Parallel()

	var ranScript bool
	s := &ScriptInstaller{
		ScriptURL:  "https://example.com/install.sh",
		BinaryPath: "/usr/local/bin/account",
		runScript: func(scriptURL string) error {
			ranScript = true
			if scriptURL != "https://example.com/install.sh" {
				t.Fatalf("scriptURL = %q, want %q", scriptURL, "https://example.com/install.sh")
			}
			return nil
		},
		pluginBinDir: func() (string, error) {
			return "/tmp/clime-plugin-test", nil
		},
		findPlugin: func(name string) (string, bool) {
			return "/usr/local/bin/account", true
		},
		runVersion: func(binPath string) (string, error) {
			return "2.1.0", nil
		},
	}

	entry := plugin.ManifestEntry{
		Name:    "account",
		Version: plugin.VersionLatest,
		Type:    plugin.SourceTypeScript,
		Source:  "https://example.com/install.sh",
	}
	result, err := s.Update("account", entry)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !ranScript {
		t.Fatal("script install should run for script source")
	}
	if !result.Updated {
		t.Fatal("Update() should mark updated for script source")
	}
	if result.LatestVersion != "2.1.0" {
		t.Fatalf("LatestVersion = %q, want %q", result.LatestVersion, "2.1.0")
	}
}

func TestScriptInstallerUpdateFallsBackToLatest(t *testing.T) {
	t.Parallel()

	s := &ScriptInstaller{
		ScriptURL: "https://example.com/install.sh",
		runScript: func(scriptURL string) error {
			return nil
		},
		pluginBinDir: func() (string, error) {
			return "/tmp/clime-plugin-test", nil
		},
		findPlugin: func(name string) (string, bool) {
			return "", false
		},
		runVersion: func(binPath string) (string, error) {
			return "", nil
		},
	}

	entry := plugin.ManifestEntry{
		Name:    "tool",
		Version: "1.0.0",
		Type:    plugin.SourceTypeScript,
		Source:  "https://example.com/install.sh",
	}
	result, err := s.Update("tool", entry)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if result.LatestVersion != plugin.VersionLatest {
		t.Fatalf("LatestVersion = %q, want %q", result.LatestVersion, plugin.VersionLatest)
	}
}

func TestScriptInstallerPluginType(t *testing.T) {
	t.Parallel()
	s := NewScriptInstaller("https://example.com/install.sh", "/usr/local/bin/foo")
	if s.PluginType() != plugin.SourceTypeScript {
		t.Fatalf("PluginType() = %q, want %q", s.PluginType(), plugin.SourceTypeScript)
	}
	if s.Source() != "https://example.com/install.sh" {
		t.Fatalf("Source() = %q, want %q", s.Source(), "https://example.com/install.sh")
	}
}
