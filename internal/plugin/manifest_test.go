package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestPersistsInitURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	want := "https://example.com/plugins.yaml"
	require.NoError(t, (&Manifest{InitURL: want}).Save())

	got, err := LoadManifest()
	require.NoError(t, err)
	assert.Equal(t, want, got.InitURL)
}

func TestLoadManifestMigratesLegacyRepo(t *testing.T) {
	tests := []struct {
		name       string
		entry      ManifestEntry
		wantType   string
		wantSource string
	}{
		{
			name:       "github repo",
			entry:      ManifestEntry{Name: "foo", Repo: "acme/clime-foo"},
			wantType:   SourceTypeGitHub,
			wantSource: "acme/clime-foo",
		},
		{
			name:       "npm repo",
			entry:      ManifestEntry{Name: "deploy", Repo: "npm:@myorg/clime-deploy"},
			wantType:   SourceTypeNpm,
			wantSource: "@myorg/clime-deploy",
		},
		{
			name:       "script repo",
			entry:      ManifestEntry{Name: "account", Repo: "https://example.com/install.sh"},
			wantType:   SourceTypeScript,
			wantSource: "https://example.com/install.sh",
		},
		{
			name:       "brew repo",
			entry:      ManifestEntry{Name: "lint", Repo: "brew:acme/tap/clime-lint"},
			wantType:   SourceTypeBrew,
			wantSource: "acme/tap/clime-lint",
		},
		{
			name:       "already migrated is not changed",
			entry:      ManifestEntry{Name: "bar", Type: SourceTypeGitHub, Source: "acme/clime-bar"},
			wantType:   SourceTypeGitHub,
			wantSource: "acme/clime-bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			m := &Manifest{Plugins: []ManifestEntry{tt.entry}}
			require.NoError(t, m.Save())
			m, err := LoadManifest()
			require.NoError(t, err)
			require.Len(t, m.Plugins, 1)

			got := m.Plugins[0]
			assert.Equal(t, tt.wantType, got.Type)
			assert.Equal(t, tt.wantSource, got.Source)
			assert.Empty(t, got.Repo)
		})
	}
}
