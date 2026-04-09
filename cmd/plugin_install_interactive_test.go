package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/git-hulk/clime/internal/plugin"
	"github.com/git-hulk/clime/internal/prompt"
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
				if inputIdx >= len(tt.inputs) {
					t.Fatalf("unexpected input prompt %d: %s", inputIdx, label)
				}
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

			if err := runInteractivePluginInstall(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if gotName != tt.wantName {
				t.Errorf("name = %q, want %q", gotName, tt.wantName)
			}
			if gotPlugin.Script != tt.wantPlugin.Script {
				t.Errorf("Script = %q, want %q", gotPlugin.Script, tt.wantPlugin.Script)
			}
			if gotPlugin.BinaryPath != tt.wantPlugin.BinaryPath {
				t.Errorf("BinaryPath = %q, want %q", gotPlugin.BinaryPath, tt.wantPlugin.BinaryPath)
			}
			if gotPlugin.Npm != tt.wantPlugin.Npm {
				t.Errorf("Npm = %q, want %q", gotPlugin.Npm, tt.wantPlugin.Npm)
			}
			if gotPlugin.Brew != tt.wantPlugin.Brew {
				t.Errorf("Brew = %q, want %q", gotPlugin.Brew, tt.wantPlugin.Brew)
			}
			if gotPlugin.Repo != tt.wantPlugin.Repo {
				t.Errorf("Repo = %q, want %q", gotPlugin.Repo, tt.wantPlugin.Repo)
			}
			if gotPlugin.Description != tt.wantPlugin.Description {
				t.Errorf("Description = %q, want %q", gotPlugin.Description, tt.wantPlugin.Description)
			}
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

	if err := runInteractivePluginInstall(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatal("expected pluginInstallRunner not to be called")
	}
}

func TestRunInteractivePluginInstallEmptyName(t *testing.T) {
	restore := stubPluginPrompts(t)
	defer restore()

	inputPrompt = func(label string) (string, error) {
		return "", nil
	}

	err := runInteractivePluginInstall()
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("error = %q, want empty name message", err.Error())
	}
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
	if !errors.Is(err, prompt.ErrInterrupted) {
		t.Fatalf("error = %v, want ErrInterrupted", err)
	}
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
				if inputIdx >= len(tt.inputs) {
					t.Fatalf("unexpected input prompt %d: %s", inputIdx, label)
				}
				val := tt.inputs[inputIdx]
				inputIdx++
				return val, nil
			}

			selectPrompt = func(config prompt.SelectConfig) (int, error) {
				return tt.typeIdx, nil
			}

			err := runInteractivePluginInstall()
			if err == nil {
				t.Fatal("expected error for empty source")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}
