package cmd

import (
	"testing"

	"github.com/git-hulk/clime/internal/plugin"
	"github.com/stretchr/testify/require"
)

func TestEnsureInstallNameAvailable(t *testing.T) {
	t.Parallel()

	manifest := &plugin.Manifest{
		Plugins: []plugin.ManifestEntry{
			{Name: "account", Version: "1.0.0"},
		},
	}

	err := ensureInstallNameAvailable(manifest, "account")
	require.ErrorContains(t, err, "already exists", "expected conflict error for existing plugin name")
	require.ErrorContains(t, err, "clime plugin update account")
	require.NoError(t, ensureInstallNameAvailable(manifest, "opencli"))
}
