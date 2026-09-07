package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/git-hulk/clime/internal/plugin"
	"github.com/stretchr/testify/require"
)

func TestScriptInstallerUpdate(t *testing.T) {
	t.Parallel()

	var ranScript bool
	s := &ScriptInstaller{
		ScriptURL:  "https://example.com/install.sh",
		BinaryPath: "/usr/local/bin/account",
		runScript: func(scriptURL string) error {
			ranScript = true
			require.Equal(t, "https://example.com/install.sh", scriptURL)
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
	require.NoError(t, err)
	require.True(t, ranScript, "script install should run for script source")
	require.True(t, result.Updated, "Update() should mark updated for script source")
	require.Equal(t, "2.1.0", result.LatestVersion)
}

func TestScriptInstallerUpdateUpToDate(t *testing.T) {
	t.Parallel()

	s := &ScriptInstaller{
		ScriptURL:  "https://example.com/install.sh",
		BinaryPath: "/usr/local/bin/account",
		runScript: func(scriptURL string) error {
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
		Version: "2.1.0",
		Type:    plugin.SourceTypeScript,
		Source:  "https://example.com/install.sh",
	}
	result, err := s.Update("account", entry)
	require.NoError(t, err)
	require.False(t, result.Updated, "Update() should not mark updated when semver version is unchanged")
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
	require.NoError(t, err)
	require.Equal(t, plugin.VersionLatest, result.LatestVersion)
}

func TestScriptInstallerInstallAutoDetectBinary(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	s := &ScriptInstaller{
		ScriptURL: "https://bun.sh/install",
		runScript: func(scriptURL string) error {
			return nil
		},
		pluginBinDir: func() (string, error) {
			return tmpDir, nil
		},
		findPlugin: func(name string) (string, bool) {
			return "/usr/local/bin/bun", true
		},
		runVersion: func(binPath string) (string, error) {
			return "1.2.0", nil
		},
		lookPath: func(name string) (string, error) {
			require.Equal(t, "bun", name)
			return "/usr/local/bin/bun", nil
		},
	}

	version, err := s.Install("bun")
	require.NoError(t, err)
	require.Equal(t, "1.2.0", version)

	// Verify symlink was created
	linkPath := filepath.Join(tmpDir, "clime-bun")
	target, err := os.Readlink(linkPath)
	require.NoError(t, err)
	require.Equal(t, "/usr/local/bin/bun", target)
}

func TestScriptInstallerInstallAutoDetectNotFound(t *testing.T) {
	t.Parallel()

	s := &ScriptInstaller{
		ScriptURL: "https://example.com/install.sh",
		runScript: func(scriptURL string) error {
			return nil
		},
		lookPath: func(name string) (string, error) {
			return "", fmt.Errorf("not found")
		},
	}

	_, err := s.Install("missing")
	require.ErrorContains(t, err, "not found on PATH", "Install() should fail when binary not found on PATH")
}

func TestScriptInstallerDetectsVersionUsingSupportedFlags(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ name, script, want string }{
		{"fallback to -V", `case "$1" in
   -V) echo "3.2.1" ;;
   *) exit 1 ;;
  esac`, "3.2.1"},
		{"first supported flag wins", `case "$1" in
   -v) echo "1.0.0" ;;
   version) echo "2.0.0" ;;
   -V) echo "3.0.0" ;;
   *) exit 1 ;;
  esac`, "1.0.0"},
		{"unsupported version commands", "exit 1", plugin.VersionLatest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fakecli")
			require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+tt.script), 0o755))
			installer := NewScriptInstaller("", "")
			installer.findPlugin = func(string) (string, bool) { return path, true }
			require.Equal(t, tt.want, installer.DetectVersion("fakecli"))
		})
	}
}

func TestScriptInstallerUninstallRemovesSymlinkTarget(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	pluginsDir, err := plugin.PluginBinDir()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(pluginsDir, 0o755))
	for _, kind := range []string{"absolute", "relative"} {
		t.Run(kind, func(t *testing.T) {
			targetBin := filepath.Join(t.TempDir(), "tool")
			require.NoError(t, os.WriteFile(targetBin, []byte("#!/bin/sh\n"), 0o755))
			target := targetBin
			if kind == "relative" {
				target, err = filepath.Rel(pluginsDir, targetBin)
				require.NoError(t, err)
			}
			linkPath := filepath.Join(pluginsDir, "clime-tool")
			require.NoError(t, os.Symlink(target, linkPath))
			installer := NewScriptInstaller("https://example.com/install.sh", "")
			require.NoError(t, installer.Uninstall("tool", plugin.ManifestEntry{Name: "tool"}))
			_, err := os.Lstat(linkPath)
			require.ErrorIs(t, err, os.ErrNotExist, "plugin symlink must be removed")
			_, err = os.Stat(targetBin)
			require.ErrorIs(t, err, os.ErrNotExist, "resolved target must be removed")
		})
	}
}
