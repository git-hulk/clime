package cmd

import (
	"testing"

	"github.com/git-hulk/clime/internal/plugin"
	"github.com/git-hulk/clime/internal/prompt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stubPluginPrompts(t *testing.T) func() {
	t.Helper()
	origSelect := selectPrompt
	origInput := inputPrompt
	origRunner := pluginInstallRunner
	return func() {
		selectPrompt = origSelect
		inputPrompt = origInput
		pluginInstallRunner = origRunner
	}
}

func TestRunInteractivePluginInstall(t *testing.T) {
	tests := []struct {
		name       string
		typeIdx    int
		inputs     []string
		wantName   string
		wantPlugin plugin.Plugin
	}{
		{
			name:     "script with binary path",
			typeIdx:  0,
			inputs:   []string{"my-tool", "https://example.com/install.sh", "~/.local/bin/my-tool", "My tool description"},
			wantName: "my-tool",
			wantPlugin: plugin.Plugin{
				Name:        "my-tool",
				Script:      "https://example.com/install.sh",
				BinaryPath:  "~/.local/bin/my-tool",
				Description: "My tool description",
			},
		},
		{
			name:     "script without binary path",
			typeIdx:  0,
			inputs:   []string{"my-tool", "https://example.com/install.sh", "", ""},
			wantName: "my-tool",
			wantPlugin: plugin.Plugin{
				Name:   "my-tool",
				Script: "https://example.com/install.sh",
			},
		},
		{
			name:     "npm package",
			typeIdx:  1,
			inputs:   []string{"my-tool", "@scope/my-tool", ""},
			wantName: "my-tool",
			wantPlugin: plugin.Plugin{
				Name: "my-tool",
				Npm:  "@scope/my-tool",
			},
		},
		{
			name:     "homebrew formula",
			typeIdx:  2,
			inputs:   []string{"my-tool", "my-tool-cli", ""},
			wantName: "my-tool",
			wantPlugin: plugin.Plugin{
				Name: "my-tool",
				Brew: "my-tool-cli",
			},
		},
		{
			name:     "github release",
			typeIdx:  3,
			inputs:   []string{"my-tool", "owner/my-tool", "A GitHub plugin"},
			wantName: "my-tool",
			wantPlugin: plugin.Plugin{
				Name:        "my-tool",
				Repo:        "owner/my-tool",
				Description: "A GitHub plugin",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := stubPluginPrompts(t)
			defer restore()

			inputIdx := 0
			inputPrompt = func(label string) (string, error) {
				require.Less(t, inputIdx, len(tt.inputs))
				val := tt.inputs[inputIdx]
				inputIdx++
				return val, nil
			}

			selectPrompt = func(config prompt.SelectConfig) (int, error) {
				return tt.typeIdx, nil
			}

			var gotName string
			var gotPlugin plugin.Plugin
			pluginInstallRunner = func(manifest *plugin.Manifest, name string, p plugin.Plugin) error {
				gotName = name
				gotPlugin = p
				return nil
			}

			require.NoError(t, runInteractivePluginInstall())

			assert.Equal(t, tt.wantName, gotName)
			assert.Equal(t, tt.wantPlugin.Script, gotPlugin.Script)
			assert.Equal(t, tt.wantPlugin.BinaryPath, gotPlugin.BinaryPath)
			assert.Equal(t, tt.wantPlugin.Npm, gotPlugin.Npm)
			assert.Equal(t, tt.wantPlugin.Brew, gotPlugin.Brew)
			assert.Equal(t, tt.wantPlugin.Repo, gotPlugin.Repo)
			assert.Equal(t, tt.wantPlugin.Description, gotPlugin.Description)
		})
	}
}

func TestRunInteractivePluginInstallEscAtTypeReturnsNil(t *testing.T) {
	restore := stubPluginPrompts(t)
	defer restore()

	inputPrompt = func(label string) (string, error) {
		return "my-tool", nil
	}
	selectPrompt = func(config prompt.SelectConfig) (int, error) {
		return 0, prompt.ErrBack
	}

	called := false
	pluginInstallRunner = func(manifest *plugin.Manifest, name string, p plugin.Plugin) error {
		called = true
		return nil
	}

	require.NoError(t, runInteractivePluginInstall())
	require.False(t, called, "expected pluginInstallRunner not to be called")
}

func TestRunInteractivePluginInstallEmptyName(t *testing.T) {
	restore := stubPluginPrompts(t)
	defer restore()

	inputPrompt = func(label string) (string, error) {
		return "", nil
	}

	err := runInteractivePluginInstall()
	require.ErrorContains(t, err, "cannot be empty", "expected error for empty name")
}

func TestRunInteractivePluginInstallInterruptPropagates(t *testing.T) {
	restore := stubPluginPrompts(t)
	defer restore()

	inputPrompt = func(label string) (string, error) {
		return "my-tool", nil
	}
	selectPrompt = func(config prompt.SelectConfig) (int, error) {
		return 0, prompt.ErrInterrupted
	}

	err := runInteractivePluginInstall()
	require.ErrorIs(t, err, prompt.ErrInterrupted)
}

func TestRunInteractivePluginInstallEmptySource(t *testing.T) {
	tests := []struct {
		name    string
		typeIdx int
		inputs  []string
		wantErr string
	}{
		{
			name:    "empty script URL",
			typeIdx: 0,
			inputs:  []string{"my-tool", ""},
			wantErr: "script URL cannot be empty",
		},
		{
			name:    "empty npm package",
			typeIdx: 1,
			inputs:  []string{"my-tool", ""},
			wantErr: "npm package name cannot be empty",
		},
		{
			name:    "empty brew formula",
			typeIdx: 2,
			inputs:  []string{"my-tool", ""},
			wantErr: "Homebrew formula cannot be empty",
		},
		{
			name:    "empty github repo",
			typeIdx: 3,
			inputs:  []string{"my-tool", ""},
			wantErr: "GitHub repository cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := stubPluginPrompts(t)
			defer restore()

			inputIdx := 0
			inputPrompt = func(label string) (string, error) {
				require.Less(t, inputIdx, len(tt.inputs))
				val := tt.inputs[inputIdx]
				inputIdx++
				return val, nil
			}

			selectPrompt = func(config prompt.SelectConfig) (int, error) {
				return tt.typeIdx, nil
			}

			err := runInteractivePluginInstall()
			require.ErrorContains(t, err, tt.wantErr, "expected error for empty source")
		})
	}
}
