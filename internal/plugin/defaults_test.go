package plugin

import (
	"reflect"
	"testing"
)

func TestCategorizeForInit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		plugins       []Plugin
		manifest      *Manifest
		wantInstall   []string
		wantReinstall []string
		wantSkipped   []string
	}{
		{
			name: "new plugin gets installed",
			plugins: []Plugin{
				{Name: "auth", Script: "https://example.com/install.sh"},
			},
			manifest:    &Manifest{},
			wantInstall: []string{"auth"},
		},
		{
			name: "matching script URL is skipped",
			plugins: []Plugin{
				{Name: "auth", Script: "https://example.com/install.sh"},
			},
			manifest: &Manifest{Plugins: []ManifestEntry{
				{Name: "auth", Type: SourceTypeScript, Source: "https://example.com/install.sh"},
			}},
			wantSkipped: []string{"auth"},
		},
		{
			name: "drifted script URL triggers reinstall",
			plugins: []Plugin{
				{Name: "auth", Script: "https://NEW.example.com/install.sh"},
			},
			manifest: &Manifest{Plugins: []ManifestEntry{
				{Name: "auth", Type: SourceTypeScript, Source: "https://OLD.example.com/install.sh"},
			}},
			wantReinstall: []string{"auth"},
		},
		{
			name: "brew-installed plugin with brew remote is skipped",
			plugins: []Plugin{
				{Name: "lint", Brew: "golangci-lint"},
			},
			manifest: &Manifest{Plugins: []ManifestEntry{
				{Name: "lint", Type: SourceTypeBrew, Source: "golangci-lint"},
			}},
			wantSkipped: []string{"lint"},
		},
		{
			name: "type mismatch (local brew, remote script) is skipped",
			plugins: []Plugin{
				{Name: "lint", Script: "https://example.com/lint.sh"},
			},
			manifest: &Manifest{Plugins: []ManifestEntry{
				{Name: "lint", Type: SourceTypeBrew, Source: "golangci-lint"},
			}},
			wantSkipped: []string{"lint"},
		},
		{
			name: "mixed list: install + reinstall + skip",
			plugins: []Plugin{
				{Name: "auth", Script: "https://NEW/install.sh"},
				{Name: "employee", Script: "https://example.com/employee.sh"},
				{Name: "newbie", Script: "https://example.com/newbie.sh"},
			},
			manifest: &Manifest{Plugins: []ManifestEntry{
				{Name: "auth", Type: SourceTypeScript, Source: "https://OLD/install.sh"},
				{Name: "employee", Type: SourceTypeScript, Source: "https://example.com/employee.sh"},
			}},
			wantInstall:   []string{"newbie"},
			wantReinstall: []string{"auth"},
			wantSkipped:   []string{"employee"},
		},
	}

	names := func(ps []Plugin) []string {
		if ps == nil {
			return nil
		}
		out := make([]string, len(ps))
		for i, p := range ps {
			out[i] = p.Name
		}
		return out
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			install, reinstall, skipped := CategorizeForInit(tt.plugins, tt.manifest)
			if got := names(install); !reflect.DeepEqual(got, tt.wantInstall) {
				t.Errorf("install: got %v, want %v", got, tt.wantInstall)
			}
			if got := names(reinstall); !reflect.DeepEqual(got, tt.wantReinstall) {
				t.Errorf("reinstall: got %v, want %v", got, tt.wantReinstall)
			}
			if !reflect.DeepEqual(skipped, tt.wantSkipped) {
				t.Errorf("skipped: got %v, want %v", skipped, tt.wantSkipped)
			}
		})
	}
}
