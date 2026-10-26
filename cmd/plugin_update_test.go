package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/git-hulk/clime/internal/plugin"
	"github.com/stretchr/testify/require"
)

func TestPluginUpdateAllSkipsMissingPlugins(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script to simulate npm")
	}
	t.Setenv("HOME", t.TempDir())
	binDir := t.TempDir()
	t.Setenv("PATH", binDir)
	updateLog := filepath.Join(t.TempDir(), "updates")
	t.Setenv("CLIME_TEST_NPM_LOG", updateLog)
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "npm"), []byte(`#!/bin/sh
case "$1" in
  update) printf '%s\n' "$3" >> "$CLIME_TEST_NPM_LOG" ;;
  list) printf '{"dependencies":{"%s":{"version":"2.0.0"}}}\n' "$3" ;;
  *) exit 1 ;;
esac
`), 0755))

	managedDir, err := plugin.PluginBinDir()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(managedDir, 0755))
	managedPath := filepath.Join(managedDir, "clime-managed")
	pathPlugin := filepath.Join(binDir, "clime-path-only")
	for _, path := range []string{managedPath, pathPlugin, filepath.Join(binDir, "clime-unmanaged")} {
		require.NoError(t, os.WriteFile(path, nil, 0755))
	}

	manifest := &plugin.Manifest{}
	for _, name := range []string{"managed", "missing", "path-only"} {
		manifest.Add(name, "1.0.0", plugin.SourceTypeNpm, name, "")
	}
	require.NoError(t, manifest.Save())

	require.NoError(t, pluginUpdateCmd.RunE(pluginUpdateCmd, []string{"all"}))
	updates, err := os.ReadFile(updateLog)
	require.NoError(t, err)
	require.Equal(t, "managed\npath-only\n", string(updates))
	manifest, err = plugin.LoadManifest()
	require.NoError(t, err)
	require.Len(t, manifest.Plugins, 3)
	for _, entry := range manifest.Plugins {
		version := "2.0.0"
		if entry.Name == "missing" {
			version = "1.0.0"
		}
		require.Equal(t, version, entry.Version)
	}

	require.NoError(t, os.Remove(managedPath))
	require.NoError(t, os.Remove(pathPlugin))
	require.NoError(t, os.Remove(updateLog))
	require.NoError(t, pluginUpdateCmd.RunE(pluginUpdateCmd, []string{"all"}))
	require.NoFileExists(t, updateLog)
}
