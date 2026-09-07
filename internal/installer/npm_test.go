package installer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/git-hulk/clime/internal/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNpmInstallerUpdate(t *testing.T) {
	t.Parallel()

	var ranNpmUpdate bool
	n := &NpmInstaller{
		Package: "@myorg/clime-deploy",
		runNpmUpdate: func(pkg string) error {
			ranNpmUpdate = true
			require.Equal(t, "@myorg/clime-deploy", pkg)
			return nil
		},
		pluginBinDir: func() (string, error) {
			return "/tmp/clime-plugin-test", nil
		},
		getVersion: func(pkg string) (string, error) {
			return plugin.VersionLatest, nil
		},
	}

	entry := plugin.ManifestEntry{
		Name:    "deploy",
		Version: plugin.VersionLatest,
		Type:    plugin.SourceTypeNpm,
		Source:  "@myorg/clime-deploy",
	}
	result, err := n.Update("deploy", entry)
	require.NoError(t, err)
	require.True(t, ranNpmUpdate, "npm update should run for npm source")
	require.True(t, result.Updated, "Update() should mark updated for npm source")
	require.Equal(t, plugin.VersionLatest, result.LatestVersion)
	wantPath := filepath.Join("/tmp/clime-plugin-test", "clime-deploy")
	require.Equal(t, wantPath, result.Path)
}

func TestNpmInstallerUpdateUpToDate(t *testing.T) {
	t.Parallel()

	n := &NpmInstaller{
		Package: "@myorg/clime-deploy",
		runNpmUpdate: func(pkg string) error {
			return nil
		},
		pluginBinDir: func() (string, error) {
			return "/tmp/clime-plugin-test", nil
		},
		getVersion: func(pkg string) (string, error) {
			return "1.2.3", nil
		},
	}

	entry := plugin.ManifestEntry{
		Name:    "deploy",
		Version: "1.2.3",
		Type:    plugin.SourceTypeNpm,
		Source:  "@myorg/clime-deploy",
	}
	result, err := n.Update("deploy", entry)
	require.NoError(t, err)
	require.False(t, result.Updated, "Update() should not mark updated when semver version is unchanged")
}

func TestLocateNpmInstalledBinaryPrefersClimePrefix(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustTouch(t, filepath.Join(dir, "clime-deploy"))
	mustTouch(t, filepath.Join(dir, "deploy"))

	path, err := locateNpmInstalledBinary(dir, "@myorg/clime-deploy", "deploy", "clime-deploy", map[string]struct{}{})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "clime-deploy"), path)
}

func TestLocateNpmInstalledBinaryFallsBackToName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustTouch(t, filepath.Join(dir, "codex"))

	path, err := locateNpmInstalledBinary(dir, "@openai/codex", "codex", "clime-codex", map[string]struct{}{})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "codex"), path)
}

func TestLocateNpmInstalledBinaryDiscoversNewBinary(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustTouch(t, filepath.Join(dir, "existing"))
	before := snapshotDirEntries(dir)
	mustTouch(t, filepath.Join(dir, "weirdname"))

	path, err := locateNpmInstalledBinary(dir, "some-package", "tool", "clime-tool", before)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "weirdname"), path)
}

func TestLocateNpmInstalledBinaryErrorsWhenNoBinaryCreated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustTouch(t, filepath.Join(dir, "preexisting"))
	before := snapshotDirEntries(dir)

	_, err := locateNpmInstalledBinary(dir, "openai/codex", "codex", "clime-codex", before)
	require.ErrorContains(t, err, "did not create a binary", "expected error when npm install produced no new binary")
	require.ErrorContains(t, err, "openai/codex")
}

func TestLocateNpmInstalledBinaryErrorsOnAmbiguousNewBinaries(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	before := snapshotDirEntries(dir)
	mustTouch(t, filepath.Join(dir, "tsc"))
	mustTouch(t, filepath.Join(dir, "tsserver"))

	_, err := locateNpmInstalledBinary(dir, "typescript", "ts", "clime-ts", before)
	require.ErrorContains(t, err, "tsc", "expected error when multiple new binaries match nothing")
	require.ErrorContains(t, err, "tsserver")
}

func TestNpmInstallerNormalizesPackageSource(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"openai/codex", "@openai/codex"},
		{"  openai/codex  ", "@openai/codex"},
		{"@openai/codex", "@openai/codex"},
		{"@openai/codex@1.0.0", "@openai/codex@1.0.0"},
		{"lodash", "lodash"},
		{"lodash@4.17.0", "lodash@4.17.0"},
		{"git+https://github.com/openai/codex.git", "git+https://github.com/openai/codex.git"},
		{"github:openai/codex", "github:openai/codex"},
		{"file:./local", "file:./local"},
		{"./local-package", "./local-package"},
		{"/abs/path/pkg", "/abs/path/pkg"},
		{"", ""},
	}
	for _, c := range cases {
		got := NewNpmInstaller(c.in).Source()
		assert.Equal(t, c.want, got)
	}
}

func mustTouch(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}
