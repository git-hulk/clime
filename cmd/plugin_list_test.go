package cmd

import (
	"testing"

	"github.com/git-hulk/clime/internal/plugin"
	"github.com/stretchr/testify/require"
)

func TestPluginListColumnsKeepsFullNonDescriptionFields(t *testing.T) {
	t.Parallel()

	const (
		name        = "clickhouse-sql-parser"
		description = "A very long description that should be truncated but keep all other fields intact."
		homeDir     = "/home/tester"
		path        = "/home/tester/.clime/plugins/clime-clickhouse-sql-parser"
	)

	manifest := &plugin.Manifest{
		Plugins: []plugin.ManifestEntry{
			{
				Name:    name,
				Version: "0.5.0",
				Type:    plugin.SourceTypeBrew,
			},
		},
	}

	gotName, gotDesc, gotVersion, gotSource, gotPath := pluginListColumns(
		plugin.DiscoveredPlugin{
			Name:        name,
			Description: description,
			Path:        path,
		},
		manifest,
		homeDir,
		24,
	)

	require.Equal(t, name, gotName)
	require.Equal(t, "0.5.0", gotVersion)
	require.Equal(t, plugin.SourceTypeBrew, gotSource)
	require.Equal(t, "~/.clime/plugins/clime-clickhouse-sql-parser", gotPath)
	require.NotEqual(t, description, gotDesc)
}

func TestPluginListColumnsFallbacks(t *testing.T) {
	t.Parallel()

	gotName, gotDesc, gotVersion, gotSource, gotPath := pluginListColumns(
		plugin.DiscoveredPlugin{Name: "foo", Description: "", Path: "/tmp/clime-foo"},
		&plugin.Manifest{},
		"",
		60,
	)

	require.Equal(t, "foo", gotName)
	require.Equal(t, "—", gotDesc)
	require.Equal(t, "—", gotVersion)
	require.Equal(t, "—", gotSource)
	require.Equal(t, "/tmp/clime-foo", gotPath)
}
