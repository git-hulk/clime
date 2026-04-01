package cmd

import (
	"testing"

	"github.com/git-hulk/clime/internal/plugin"
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

	if gotName != name {
		t.Fatalf("name = %q, want %q", gotName, name)
	}
	if gotVersion != "0.5.0" {
		t.Fatalf("version = %q, want %q", gotVersion, "0.5.0")
	}
	if gotSource != plugin.SourceTypeBrew {
		t.Fatalf("source = %q, want %q", gotSource, plugin.SourceTypeBrew)
	}
	if gotPath != "~/.clime/plugins/clime-clickhouse-sql-parser" {
		t.Fatalf("path = %q, want %q", gotPath, "~/.clime/plugins/clime-clickhouse-sql-parser")
	}
	if gotDesc == description {
		t.Fatalf("description should be truncated when longer than width; got %q", gotDesc)
	}
}

func TestPluginListColumnsFallbacks(t *testing.T) {
	t.Parallel()

	gotName, gotDesc, gotVersion, gotSource, gotPath := pluginListColumns(
		plugin.DiscoveredPlugin{Name: "foo", Description: "", Path: "/tmp/clime-foo"},
		&plugin.Manifest{},
		"",
		60,
	)

	if gotName != "foo" {
		t.Fatalf("name = %q, want %q", gotName, "foo")
	}
	if gotDesc != "—" {
		t.Fatalf("description = %q, want %q", gotDesc, "—")
	}
	if gotVersion != "—" {
		t.Fatalf("version = %q, want %q", gotVersion, "—")
	}
	if gotSource != "—" {
		t.Fatalf("source = %q, want %q", gotSource, "—")
	}
	if gotPath != "/tmp/clime-foo" {
		t.Fatalf("path = %q, want %q", gotPath, "/tmp/clime-foo")
	}
}
