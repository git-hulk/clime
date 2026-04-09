package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestBrewInstallerInstallBrewNotFound(t *testing.T) {
	t.Parallel()

	b := &BrewInstaller{
		Formula: "acme/tap/clime-deploy",
		lookPath: func(name string) (string, error) {
			return "", fmt.Errorf("not found")
		},
	}

	_, err := b.Install("deploy")
	if err == nil {
		t.Fatal("Install() should fail when brew is not on PATH")
	}
	if got := err.Error(); !strings.Contains(got, "homebrew is not installed") {
		t.Fatalf("error = %q, want it to mention homebrew not installed", got)
	}
}

func TestBrewInstallerInstallBrewInstallFails(t *testing.T) {
	t.Parallel()

	b := &BrewInstaller{
		Formula: "acme/tap/clime-deploy",
		lookPath: func(name string) (string, error) {
			if name == "brew" {
				return "/opt/homebrew/bin/brew", nil
			}
			return "", fmt.Errorf("not found")
		},
		runBrewInstall: func(formula string) error {
			return fmt.Errorf("brew install failed: exit status 1")
		},
	}

	_, err := b.Install("deploy")
	if err == nil {
		t.Fatal("Install() should fail when brew install fails")
	}
	if got := err.Error(); !strings.Contains(got, "installing formula") {
		t.Fatalf("error = %q, want it to mention installing formula", got)
	}
}

func TestBrewInstallerInstallUsesExistingBinaryWhenBrewInstallFails(t *testing.T) {
	t.Parallel()

	pluginDir := t.TempDir()
	brewBin := t.TempDir()
	installedBin := filepath.Join(brewBin, "copilot")
	if err := os.WriteFile(installedBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write installed binary: %v", err)
	}

	b := &BrewInstaller{
		Formula: "copilot-cli",
		lookPath: func(name string) (string, error) {
			switch name {
			case "brew":
				return "/opt/homebrew/bin/brew", nil
			case "copilot":
				return installedBin, nil
			default:
				return "", fmt.Errorf("not found")
			}
		},
		runBrewInstall: func(formula string) error {
			return fmt.Errorf("brew install failed: not writable")
		},
		brewBinDir: func() (string, error) {
			return brewBin, nil
		},
		pluginBinDir: func() (string, error) {
			return pluginDir, nil
		},
		getVersion: func(formula string) (string, error) {
			return "1.0.0", nil
		},
	}

	version, err := b.Install("copilot-cli")
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if version != "1.0.0" {
		t.Fatalf("version = %q, want %q", version, "1.0.0")
	}

	linkPath := filepath.Join(pluginDir, "clime-copilot-cli")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("symlink not created: %v", err)
	}
	if target != installedBin {
		t.Fatalf("symlink target = %q, want %q", target, installedBin)
	}
}

func TestBrewInstallerInstallBinaryNotFound(t *testing.T) {
	t.Parallel()

	b := &BrewInstaller{
		Formula: "acme/tap/clime-deploy",
		lookPath: func(name string) (string, error) {
			if name == "brew" {
				return "/opt/homebrew/bin/brew", nil
			}
			return "", fmt.Errorf("not found")
		},
		runBrewInstall: func(formula string) error {
			return nil
		},
		brewBinDir: func() (string, error) {
			return "/nonexistent", nil
		},
	}

	_, err := b.Install("deploy")
	if err == nil {
		t.Fatal("Install() should fail when binary is not found")
	}
	if got := err.Error(); !strings.Contains(got, "not found after brew install") {
		t.Fatalf("error = %q, want it to mention binary not found", got)
	}
}

func TestBrewInstallerUpdate(t *testing.T) {
	t.Parallel()

	var ranUpdate bool
	pluginDir := t.TempDir()
	brewBin := t.TempDir()
	// Create binary so resolveInstalledBinary succeeds.
	if err := os.WriteFile(filepath.Join(brewBin, "clime-deploy"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

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
			return pluginDir, nil
		},
		getVersion: func(formula string) (string, error) {
			return plugin.VersionLatest, nil
		},
		lookPath: func(name string) (string, error) {
			if name == "brew" {
				return "/opt/homebrew/bin/brew", nil
			}
			return "", fmt.Errorf("not found")
		},
		brewBinDir: func() (string, error) {
			return brewBin, nil
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

	pluginDir := t.TempDir()
	brewBin := t.TempDir()
	if err := os.WriteFile(filepath.Join(brewBin, "clime-deploy"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	b := &BrewInstaller{
		Formula: "acme/tap/clime-deploy",
		runBrewUpdate: func(formula string) error {
			return nil
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
		brewBinDir: func() (string, error) {
			return brewBin, nil
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

func TestBrewInstallerUpdateBrewNotFound(t *testing.T) {
	t.Parallel()

	b := &BrewInstaller{
		Formula: "acme/tap/clime-deploy",
		lookPath: func(name string) (string, error) {
			return "", fmt.Errorf("not found")
		},
	}

	entry := plugin.ManifestEntry{
		Name:    "deploy",
		Version: "1.0.0",
		Type:    plugin.SourceTypeBrew,
		Source:  "acme/tap/clime-deploy",
	}
	_, err := b.Update("deploy", entry)
	if err == nil {
		t.Fatal("Update() should fail when brew is not on PATH")
	}
	if got := err.Error(); !strings.Contains(got, "homebrew is not installed") {
		t.Fatalf("error = %q, want it to mention homebrew not installed", got)
	}
}

func TestBrewInstallerUpdateResolvesSymlink(t *testing.T) {
	t.Parallel()

	pluginDir := t.TempDir()
	brewBin := t.TempDir()
	binPath := filepath.Join(brewBin, "clime-deploy")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	b := &BrewInstaller{
		Formula:       "acme/tap/clime-deploy",
		runBrewUpdate: func(formula string) error { return nil },
		pluginBinDir:  func() (string, error) { return pluginDir, nil },
		getVersion:    func(formula string) (string, error) { return "2.0.0", nil },
		lookPath: func(name string) (string, error) {
			if name == "brew" {
				return "/opt/homebrew/bin/brew", nil
			}
			return "", fmt.Errorf("not found")
		},
		brewBinDir: func() (string, error) { return brewBin, nil },
	}

	entry := plugin.ManifestEntry{
		Name:    "deploy",
		Version: "1.0.0",
		Type:    plugin.SourceTypeBrew,
		Source:  "acme/tap/clime-deploy",
	}
	_, err := b.Update("deploy", entry)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	linkPath := filepath.Join(pluginDir, "clime-deploy")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("symlink not created after update: %v", err)
	}
	if target != binPath {
		t.Fatalf("symlink target = %q, want %q", target, binPath)
	}
}

func TestBrewInstallerUninstall(t *testing.T) {
	t.Parallel()

	// Place the binary where removePluginBinary expects it (~/.clime/plugins/).
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	pluginsDir := filepath.Join(home, ".clime", "plugins")
	_ = os.MkdirAll(pluginsDir, 0755)
	fakeBin := filepath.Join(pluginsDir, "clime-rmtest")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	defer os.Remove(fakeBin)

	var ranUninstall bool
	b := &BrewInstaller{
		Formula: "acme/tap/clime-rmtest",
		lookPath: func(name string) (string, error) {
			if name == "brew" {
				return "/opt/homebrew/bin/brew", nil
			}
			return "", fmt.Errorf("not found")
		},
		runBrewUninstall: func(formula string) error {
			ranUninstall = true
			return nil
		},
	}

	entry := plugin.ManifestEntry{
		Name:   "rmtest",
		Type:   plugin.SourceTypeBrew,
		Source: "acme/tap/clime-rmtest",
	}
	if err := b.Uninstall("rmtest", entry); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if !ranUninstall {
		t.Fatal("brew uninstall should have run")
	}
	if _, err := os.Stat(fakeBin); !os.IsNotExist(err) {
		t.Fatal("plugin binary should have been removed")
	}
}

func TestBrewInstallerDetectVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		err     error
		want    string
	}{
		{
			name:    "returns version",
			version: "3.1.4",
			want:    "3.1.4",
		},
		{
			name: "falls back to latest on error",
			err:  fmt.Errorf("not installed"),
			want: plugin.VersionLatest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := &BrewInstaller{
				Formula: "acme/tap/clime-foo",
				getVersion: func(formula string) (string, error) {
					return tt.version, tt.err
				},
			}
			if got := b.DetectVersion("foo"); got != tt.want {
				t.Fatalf("DetectVersion() = %q, want %q", got, tt.want)
			}
		})
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

func TestBrewInstallOrUpgradeCmd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		action  string
		formula string
	}{
		{name: "install", action: "install", formula: "acme/tap/clime-deploy"},
		{name: "upgrade", action: "upgrade", formula: "acme/tap/clime-deploy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := brewInstallOrUpgradeCmd(tt.action, tt.formula)
			if len(cmd.Args) != 3 || cmd.Args[0] != "brew" || cmd.Args[1] != tt.action || cmd.Args[2] != tt.formula {
				t.Fatalf("cmd args = %v, want [brew %s %s]", cmd.Args, tt.action, tt.formula)
			}

			foundNoCleanup := false
			for _, env := range cmd.Env {
				if env == "HOMEBREW_NO_INSTALL_CLEANUP=1" {
					foundNoCleanup = true
					break
				}
			}
			if !foundNoCleanup {
				t.Fatal("expected HOMEBREW_NO_INSTALL_CLEANUP=1 to be set")
			}
		})
	}
}

func TestResolveInstalledBinaryFallbacks(t *testing.T) {
	t.Parallel()

	t.Run("bare name in brew bin dir", func(t *testing.T) {
		t.Parallel()
		brewBin := t.TempDir()
		// Only create bare name (no clime- prefix)
		if err := os.WriteFile(filepath.Join(brewBin, "deploy"), []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatalf("write binary: %v", err)
		}
		b := &BrewInstaller{
			brewBinDir: func() (string, error) { return brewBin, nil },
			lookPath:   func(name string) (string, error) { return "", fmt.Errorf("not found") },
		}
		got, err := b.resolveInstalledBinary("deploy")
		if err != nil {
			t.Fatalf("resolveInstalledBinary() error = %v", err)
		}
		want := filepath.Join(brewBin, "deploy")
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("lookPath with clime prefix", func(t *testing.T) {
		t.Parallel()
		b := &BrewInstaller{
			brewBinDir: func() (string, error) { return "", fmt.Errorf("no brew dir") },
			lookPath: func(name string) (string, error) {
				if name == "clime-deploy" {
					return "/usr/local/bin/clime-deploy", nil
				}
				return "", fmt.Errorf("not found")
			},
		}
		got, err := b.resolveInstalledBinary("deploy")
		if err != nil {
			t.Fatalf("resolveInstalledBinary() error = %v", err)
		}
		if got != "/usr/local/bin/clime-deploy" {
			t.Fatalf("got %q, want /usr/local/bin/clime-deploy", got)
		}
	})

	t.Run("lookPath with bare name", func(t *testing.T) {
		t.Parallel()
		b := &BrewInstaller{
			brewBinDir: func() (string, error) { return "", fmt.Errorf("no brew dir") },
			lookPath: func(name string) (string, error) {
				if name == "deploy" {
					return "/usr/local/bin/deploy", nil
				}
				return "", fmt.Errorf("not found")
			},
		}
		got, err := b.resolveInstalledBinary("deploy")
		if err != nil {
			t.Fatalf("resolveInstalledBinary() error = %v", err)
		}
		if got != "/usr/local/bin/deploy" {
			t.Fatalf("got %q, want /usr/local/bin/deploy", got)
		}
	})

	t.Run("lookPath with -cli suffix trimmed", func(t *testing.T) {
		t.Parallel()
		b := &BrewInstaller{
			Formula:    "copilot-cli",
			brewBinDir: func() (string, error) { return "", fmt.Errorf("no brew dir") },
			lookPath: func(name string) (string, error) {
				if name == "copilot" {
					return "/opt/homebrew/bin/copilot", nil
				}
				return "", fmt.Errorf("not found")
			},
		}
		got, err := b.resolveInstalledBinary("copilot-cli")
		if err != nil {
			t.Fatalf("resolveInstalledBinary() error = %v", err)
		}
		if got != "/opt/homebrew/bin/copilot" {
			t.Fatalf("got %q, want /opt/homebrew/bin/copilot", got)
		}
	})

	t.Run("nothing found", func(t *testing.T) {
		t.Parallel()
		b := &BrewInstaller{
			Formula:    "acme/tap/clime-deploy",
			brewBinDir: func() (string, error) { return "", fmt.Errorf("no brew dir") },
			lookPath:   func(name string) (string, error) { return "", fmt.Errorf("not found") },
		}
		_, err := b.resolveInstalledBinary("deploy")
		if err == nil {
			t.Fatal("resolveInstalledBinary() should fail when nothing is found")
		}
	})

	t.Run("formula bin dir with single executable", func(t *testing.T) {
		t.Parallel()
		formulaBin := t.TempDir()
		want := filepath.Join(formulaBin, "github-copilot-cli")
		if err := os.WriteFile(want, []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatalf("write binary: %v", err)
		}

		b := &BrewInstaller{
			Formula:       "copilot-cli",
			brewBinDir:    func() (string, error) { return "", fmt.Errorf("no brew dir") },
			formulaBinDir: func(formula string) (string, error) { return formulaBin, nil },
			lookPath:      func(name string) (string, error) { return "", fmt.Errorf("not found") },
		}

		got, err := b.resolveInstalledBinary("copilot-cli")
		if err != nil {
			t.Fatalf("resolveInstalledBinary() error = %v", err)
		}
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("formula bin dir chooses token-matching executable", func(t *testing.T) {
		t.Parallel()
		formulaBin := t.TempDir()
		unrelated := filepath.Join(formulaBin, "helper-tool")
		want := filepath.Join(formulaBin, "github-copilot-cli")
		if err := os.WriteFile(unrelated, []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatalf("write unrelated binary: %v", err)
		}
		if err := os.WriteFile(want, []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatalf("write binary: %v", err)
		}

		b := &BrewInstaller{
			Formula:       "copilot-cli",
			brewBinDir:    func() (string, error) { return "", fmt.Errorf("no brew dir") },
			formulaBinDir: func(formula string) (string, error) { return formulaBin, nil },
			lookPath:      func(name string) (string, error) { return "", fmt.Errorf("not found") },
		}

		got, err := b.resolveInstalledBinary("copilot-cli")
		if err != nil {
			t.Fatalf("resolveInstalledBinary() error = %v", err)
		}
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}
