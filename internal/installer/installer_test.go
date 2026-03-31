package installer

import (
	"testing"

	"github.com/git-hulk/clime/internal/plugin"
)

func TestParseVersionOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   string
	}{
		{name: "empty output", output: "", want: plugin.VersionLatest},
		{name: "whitespace only", output: "  \n  ", want: plugin.VersionLatest},
		{name: "semver", output: "1.2.3", want: "1.2.3"},
		{name: "semver with v prefix", output: "v1.2.3", want: "1.2.3"},
		{name: "semver in sentence", output: "clime-foo version 2.0.1", want: "2.0.1"},
		{name: "semver with v in sentence", output: "my-plugin v0.9.0 (built 2024-01-01)", want: "0.9.0"},
		{name: "single word version", output: "dev", want: "dev"},
		{name: "single word with whitespace", output: "  beta  ", want: "beta"},
		{name: "multi-word no semver", output: "unknown version info", want: plugin.VersionLatest},
		{name: "multiple semver picks first", output: "v1.0.0 built with go1.21.0", want: "1.0.0"},
		{name: "semver with trailing newline", output: "3.4.5\n", want: "3.4.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseVersionOutput(tt.output)
			if got != tt.want {
				t.Errorf("parseVersionOutput(%q) = %q, want %q", tt.output, got, tt.want)
			}
		})
	}
}

func TestFromPlugin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		plugin     plugin.Plugin
		wantType   string
		wantSource string
		wantErr    bool
	}{
		{
			name:       "github repo",
			plugin:     plugin.Plugin{Name: "foo", Repo: "acme/clime-foo"},
			wantType:   plugin.SourceTypeGitHub,
			wantSource: "acme/clime-foo",
		},
		{
			name:       "npm package",
			plugin:     plugin.Plugin{Name: "bar", Npm: "@acme/clime-bar"},
			wantType:   plugin.SourceTypeNpm,
			wantSource: "@acme/clime-bar",
		},
		{
			name:       "script",
			plugin:     plugin.Plugin{Name: "baz", Script: "https://example.com/install.sh", BinaryPath: "/usr/local/bin/baz"},
			wantType:   plugin.SourceTypeScript,
			wantSource: "https://example.com/install.sh",
		},
		{
			name:    "no source",
			plugin:  plugin.Plugin{Name: "empty"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			inst, err := FromPlugin(tt.plugin)
			if tt.wantErr {
				if err == nil {
					t.Fatal("FromPlugin() expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("FromPlugin() error = %v", err)
			}
			if inst.PluginType() != tt.wantType {
				t.Errorf("PluginType() = %q, want %q", inst.PluginType(), tt.wantType)
			}
			if inst.Source() != tt.wantSource {
				t.Errorf("Source() = %q, want %q", inst.Source(), tt.wantSource)
			}
		})
	}
}

func TestFromManifest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		entry      plugin.ManifestEntry
		wantType   string
		wantSource string
		wantErr    bool
	}{
		{
			name:       "github",
			entry:      plugin.ManifestEntry{Name: "foo", Type: plugin.SourceTypeGitHub, Source: "acme/clime-foo"},
			wantType:   plugin.SourceTypeGitHub,
			wantSource: "acme/clime-foo",
		},
		{
			name:       "npm",
			entry:      plugin.ManifestEntry{Name: "bar", Type: plugin.SourceTypeNpm, Source: "@acme/clime-bar"},
			wantType:   plugin.SourceTypeNpm,
			wantSource: "@acme/clime-bar",
		},
		{
			name:       "script",
			entry:      plugin.ManifestEntry{Name: "baz", Type: plugin.SourceTypeScript, Source: "https://example.com/install.sh"},
			wantType:   plugin.SourceTypeScript,
			wantSource: "https://example.com/install.sh",
		},
		{
			name:    "unknown type",
			entry:   plugin.ManifestEntry{Name: "x", Type: "unknown"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			inst, err := FromManifest(tt.entry)
			if tt.wantErr {
				if err == nil {
					t.Fatal("FromManifest() expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("FromManifest() error = %v", err)
			}
			if inst.PluginType() != tt.wantType {
				t.Errorf("PluginType() = %q, want %q", inst.PluginType(), tt.wantType)
			}
			if inst.Source() != tt.wantSource {
				t.Errorf("Source() = %q, want %q", inst.Source(), tt.wantSource)
			}
		})
	}
}
