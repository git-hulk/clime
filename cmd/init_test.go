package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/git-hulk/clime/internal/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitRemembersLocalSource(t *testing.T) {
	for _, absolute := range []bool{false, true} {
		name := "relative path"
		if absolute {
			name = "absolute path"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Chdir(t.TempDir())
			sourcePath, err := filepath.Abs("plugins.yaml")
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(sourcePath, []byte("plugins:\n  - name: remembered\n"), 0644))

			manifest := &plugin.Manifest{InitURL: "https://example.com/previous.yaml"}
			manifest.Add("remembered", "1.0.0", plugin.SourceTypeGitHub, "example/clime-remembered", "")
			require.NoError(t, manifest.Save())

			source := "plugins.yaml"
			if absolute {
				source = sourcePath
			}
			require.NoError(t, initCmd.RunE(initCmd, []string{source}))
			manifest, err = plugin.LoadManifest()
			require.NoError(t, err)
			require.Equal(t, sourcePath, manifest.InitURL)

			t.Chdir(t.TempDir())
			output := captureStdout(t, func() {
				require.NoError(t, initCmd.RunE(initCmd, nil))
			})
			require.Contains(t, output, "Skipping 1 already installed plugin(s): remembered")
		})
	}
}

func TestFilterPluginsByTags(t *testing.T) {
	plugins := []plugin.Plugin{
		{Name: "core"}, // untagged
		{Name: "devops-tool", Tags: []string{"devops"}},    // single tag
		{Name: "email", Tags: []string{"devops", "email"}}, // multi tag
		{Name: "infra", Tags: []string{"infra"}},           // single tag
	}

	tests := []struct {
		name     string
		tags     []string
		wantName []string
	}{
		{
			name:     "nil tags installs only untagged",
			tags:     nil,
			wantName: []string{"core"},
		},
		{
			name:     "empty tags installs only untagged",
			tags:     []string{},
			wantName: []string{"core"},
		},
		{
			name:     "single matching tag",
			tags:     []string{"devops"},
			wantName: []string{"core", "devops-tool", "email"},
		},
		{
			name:     "multiple tags OR matching",
			tags:     []string{"devops", "infra"},
			wantName: []string{"core", "devops-tool", "email", "infra"},
		},
		{
			name:     "no matching tag returns only untagged",
			tags:     []string{"nonexistent"},
			wantName: []string{"core"},
		},
		{
			name:     "whitespace in tags is trimmed",
			tags:     []string{" devops "},
			wantName: []string{"core", "devops-tool", "email"},
		},
		{
			name:     "empty string tag is ignored",
			tags:     []string{""},
			wantName: []string{"core"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := plugin.FilterByTags(plugins, tt.tags)
			require.Len(t, got, len(tt.wantName))
			for i, p := range got {
				assert.Equal(t, tt.wantName[i], p.Name)
			}
		})
	}
}
