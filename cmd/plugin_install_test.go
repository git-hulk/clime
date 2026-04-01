package cmd

import (
	"strings"
	"testing"

	"github.com/git-hulk/clime/internal/plugin"
)

func TestEnsureInstallNameAvailable(t *testing.T) {
	t.Parallel()

	manifest := &plugin.Manifest{
		Plugins: []plugin.ManifestEntry{
			{Name: "account", Version: "1.0.0"},
		},
	}

	err := ensureInstallNameAvailable(manifest, "account")
	if err == nil {
		t.Fatal("expected conflict error for existing plugin name")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %q, want conflict message", err.Error())
	}
	if !strings.Contains(err.Error(), "clime plugin update account") {
		t.Fatalf("error = %q, want update guidance", err.Error())
	}
}

func TestEnsureInstallNameAvailableNoConflict(t *testing.T) {
	t.Parallel()

	manifest := &plugin.Manifest{
		Plugins: []plugin.ManifestEntry{
			{Name: "account", Version: "1.0.0"},
		},
	}

	if err := ensureInstallNameAvailable(manifest, "opencli"); err != nil {
		t.Fatalf("unexpected error for new plugin name: %v", err)
	}
}
