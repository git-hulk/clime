package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/git-hulk/clime/internal/plugin"
	"github.com/stretchr/testify/require"
)

func TestBrewInstallerInstall(t *testing.T) {
	t.Parallel()

	brewBin := t.TempDir()
	pluginDir := t.TempDir()
	installedBin := filepath.Join(brewBin, "clime-deploy")
	require.NoError(t, os.WriteFile(installedBin, []byte("#!/bin/sh\n"), 0755))

	var ranInstall bool
	b := &BrewInstaller{
		Formula: "acme/tap/clime-deploy",
		runBrewInstall: func(formula string) error {
			ranInstall = true
			require.Equal(t, "acme/tap/clime-deploy", formula)
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
	require.NoError(t, err)
	require.True(t, ranInstall, "brew install should run")
	require.Equal(t, "1.2.3", version)

	linkPath := filepath.Join(pluginDir, "clime-deploy")
	target, err := os.Readlink(linkPath)
	require.NoError(t, err)
	require.Equal(t, installedBin, target)
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
	require.ErrorContains(t, err, "homebrew is not installed", "Install() should fail when brew is not on PATH")
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
	require.ErrorContains(t, err, "installing formula", "Install() should fail when brew install fails")
}

func TestBrewInstallerInstallUsesExistingBinaryWhenBrewInstallFails(t *testing.T) {
	t.Parallel()

	pluginDir := t.TempDir()
	brewBin := t.TempDir()
	installedBin := filepath.Join(brewBin, "copilot")
	require.NoError(t, os.WriteFile(installedBin, []byte("#!/bin/sh\n"), 0755))

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
	require.NoError(t, err)
	require.Equal(t, "1.0.0", version)

	linkPath := filepath.Join(pluginDir, "clime-copilot-cli")
	target, err := os.Readlink(linkPath)
	require.NoError(t, err)
	require.Equal(t, installedBin, target)
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
	require.ErrorContains(t, err, "not found after brew install", "Install() should fail when binary is not found")
}

func TestBrewInstallerUpdate(t *testing.T) {
	t.Parallel()

	var ranUpdate bool
	pluginDir := t.TempDir()
	brewBin := t.TempDir()
	// Create binary so resolveInstalledBinary succeeds.
	require.NoError(t, os.WriteFile(filepath.Join(brewBin, "clime-deploy"), []byte("#!/bin/sh\n"), 0755))

	b := &BrewInstaller{
		Formula: "acme/tap/clime-deploy",
		runBrewUpdate: func(formula string) error {
			ranUpdate = true
			require.Equal(t, "acme/tap/clime-deploy", formula)
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
	require.NoError(t, err)
	require.True(t, ranUpdate, "brew update should run for brew source")
	require.True(t, result.Updated, "Update() should mark updated for brew source")
	require.Equal(t, plugin.VersionLatest, result.LatestVersion)
}

func TestBrewInstallerUpdateUpToDate(t *testing.T) {
	t.Parallel()

	pluginDir := t.TempDir()
	brewBin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(brewBin, "clime-deploy"), []byte("#!/bin/sh\n"), 0755))

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
	require.NoError(t, err)
	require.False(t, result.Updated, "Update() should not mark updated when semver version is unchanged")
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
	require.ErrorContains(t, err, "homebrew is not installed", "Update() should fail when brew is not on PATH")
}

func TestBrewInstallerUpdateResolvesSymlink(t *testing.T) {
	t.Parallel()

	pluginDir := t.TempDir()
	brewBin := t.TempDir()
	binPath := filepath.Join(brewBin, "clime-deploy")
	require.NoError(t, os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0755))

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
	require.NoError(t, err)

	linkPath := filepath.Join(pluginDir, "clime-deploy")
	target, err := os.Readlink(linkPath)
	require.NoError(t, err)
	require.Equal(t, binPath, target)
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
	require.NoError(t, os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0755))
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
	require.NoError(t, b.Uninstall("rmtest", entry))
	require.True(t, ranUninstall, "brew uninstall should have run")

	_, err = os.Stat(fakeBin)
	require.ErrorIs(t, err, os.ErrNotExist, "plugin binary should have been removed")
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
			require.Equal(t, tt.want, b.DetectVersion("foo"))
		})
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
			require.Equal(t, []string{"brew", tt.action, tt.formula}, cmd.Args)

			require.Contains(t, cmd.Env, "HOMEBREW_NO_INSTALL_CLEANUP=1")
		})
	}
}

func TestResolveInstalledBinaryFallbacks(t *testing.T) {
	t.Parallel()

	t.Run("bare name in brew bin dir", func(t *testing.T) {
		t.Parallel()
		brewBin := t.TempDir()
		// Only create bare name (no clime- prefix)
		require.NoError(t, os.WriteFile(filepath.Join(brewBin, "deploy"), []byte("#!/bin/sh\n"), 0755))
		b := &BrewInstaller{
			brewBinDir: func() (string, error) { return brewBin, nil },
			lookPath:   func(name string) (string, error) { return "", fmt.Errorf("not found") },
		}
		got, err := b.resolveInstalledBinary("deploy")
		require.NoError(t, err)
		want := filepath.Join(brewBin, "deploy")
		require.Equal(t, want, got)
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
		require.NoError(t, err)
		require.Equal(t, "/usr/local/bin/clime-deploy", got)
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
		require.NoError(t, err)
		require.Equal(t, "/usr/local/bin/deploy", got)
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
		require.NoError(t, err)
		require.Equal(t, "/opt/homebrew/bin/copilot", got)
	})

	t.Run("nothing found", func(t *testing.T) {
		t.Parallel()
		b := &BrewInstaller{
			Formula:    "acme/tap/clime-deploy",
			brewBinDir: func() (string, error) { return "", fmt.Errorf("no brew dir") },
			lookPath:   func(name string) (string, error) { return "", fmt.Errorf("not found") },
		}
		_, err := b.resolveInstalledBinary("deploy")
		require.Error(t, err, "resolveInstalledBinary() should fail when nothing is found")
	})

	t.Run("formula bin dir with single executable", func(t *testing.T) {
		t.Parallel()
		formulaBin := t.TempDir()
		want := filepath.Join(formulaBin, "github-copilot-cli")
		require.NoError(t, os.WriteFile(want, []byte("#!/bin/sh\n"), 0755))

		b := &BrewInstaller{
			Formula:       "copilot-cli",
			brewBinDir:    func() (string, error) { return "", fmt.Errorf("no brew dir") },
			formulaBinDir: func(formula string) (string, error) { return formulaBin, nil },
			lookPath:      func(name string) (string, error) { return "", fmt.Errorf("not found") },
		}

		got, err := b.resolveInstalledBinary("copilot-cli")
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("formula bin dir chooses token-matching executable", func(t *testing.T) {
		t.Parallel()
		formulaBin := t.TempDir()
		unrelated := filepath.Join(formulaBin, "helper-tool")
		want := filepath.Join(formulaBin, "github-copilot-cli")
		require.NoError(t, os.WriteFile(unrelated, []byte("#!/bin/sh\n"), 0755))
		require.NoError(t, os.WriteFile(want, []byte("#!/bin/sh\n"), 0755))

		b := &BrewInstaller{
			Formula:       "copilot-cli",
			brewBinDir:    func() (string, error) { return "", fmt.Errorf("no brew dir") },
			formulaBinDir: func(formula string) (string, error) { return formulaBin, nil },
			lookPath:      func(name string) (string, error) { return "", fmt.Errorf("not found") },
		}

		got, err := b.resolveInstalledBinary("copilot-cli")
		require.NoError(t, err)
		require.Equal(t, want, got)
	})
}
