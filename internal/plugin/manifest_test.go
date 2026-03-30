package plugin

import "testing"

func TestMigrateRepo(t *testing.T) {
	t.Parallel()

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
			name:       "already migrated is not changed",
			entry:      ManifestEntry{Name: "bar", Type: SourceTypeGitHub, Source: "acme/clime-bar"},
			wantType:   SourceTypeGitHub,
			wantSource: "acme/clime-bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manifest{Plugins: []ManifestEntry{tt.entry}}
			m.migrateRepo()

			got := m.Plugins[0]
			if got.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", got.Type, tt.wantType)
			}
			if got.Source != tt.wantSource {
				t.Errorf("Source = %q, want %q", got.Source, tt.wantSource)
			}
			if got.Repo != "" {
				t.Errorf("Repo should be cleared after migration, got %q", got.Repo)
			}
		})
	}
}
