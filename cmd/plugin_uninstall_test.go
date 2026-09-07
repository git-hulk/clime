package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUniquePluginNames(t *testing.T) {
	t.Parallel()

	got := uniquePluginNames([]string{
		"account",
		"account",
		"opencli",
		"opencli",
		"cmdb",
	})

	want := []string{"account", "opencli", "cmdb"}
	require.Equal(t, want, got)
}

func TestPluginUninstallArgsAllowsMultiple(t *testing.T) {
	t.Parallel()

	require.NoError(t, pluginUninstallCmd.Args(pluginUninstallCmd, []string{"foo"}))
	require.NoError(t, pluginUninstallCmd.Args(pluginUninstallCmd, []string{"foo", "bar"}))
	require.Error(t, pluginUninstallCmd.Args(pluginUninstallCmd, nil), "zero args should fail")
}

func TestPluginUninstallWarnsWhenPluginDoesNotExist(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	output := captureStdout(t, func() {
		require.NoError(t, pluginUninstallCmd.RunE(pluginUninstallCmd, []string{"missing-plugin"}))
	})

	require.Contains(t, output, `Plugin "missing-plugin" is not installed; skipping.`)
	require.NotContains(t, output, `Removed plugin "missing-plugin"`)
}
