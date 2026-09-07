package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/git-hulk/clime/internal/githubrelease"
	"github.com/git-hulk/clime/internal/plugin"
	"github.com/stretchr/testify/require"
)

func TestGitHubInstallerUpdateSkipsWhenAlreadyLatest(t *testing.T) {
	t.Parallel()

	g := &GitHubInstaller{
		Repo: "acme/clime-foo",
		fetchLatest: func(repo string) (*githubrelease.Release, error) {
			require.Equal(t, "acme/clime-foo", repo)
			return &githubrelease.Release{TagName: "v1.2.3"}, nil
		},
		downloadBinary: func(url, binaryName string) ([]byte, error) {
			require.FailNow(t, "downloadBinary should not be called when plugin is already latest")
			return nil, nil
		},
		pluginBinDir: func() (string, error) {
			require.FailNow(t, "pluginBinDir should not be called when plugin is already latest")
			return "", nil
		},
		writeBinary: func(destPath string, binaryContent []byte) error {
			require.FailNow(t, "writeBinary should not be called when plugin is already latest")
			return nil
		},
	}

	entry := plugin.ManifestEntry{Name: "foo", Version: "1.2.3", Type: plugin.SourceTypeGitHub, Source: "acme/clime-foo"}
	result, err := g.Update("foo", entry)
	require.NoError(t, err)
	require.False(t, result.Updated, "Update() should not mark updated when versions match")
	require.Equal(t, "1.2.3", result.LatestVersion)
}

func TestGitHubInstallerUpdateApplies(t *testing.T) {
	t.Parallel()

	const (
		repo        = "acme/clime-foo"
		downloadURL = "https://example.com/clime-foo.tar.gz"
	)
	installDir := t.TempDir()
	assetName := fmt.Sprintf("clime-foo_1.1.0_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)

	g := &GitHubInstaller{
		Repo: repo,
		fetchLatest: func(gotRepo string) (*githubrelease.Release, error) {
			require.Equal(t, repo, gotRepo)
			return &githubrelease.Release{
				TagName: "v1.1.0",
				Assets: []githubrelease.Asset{
					{Name: assetName, BrowserDownloadURL: downloadURL},
				},
			}, nil
		},
		downloadBinary: func(url, binaryName string) ([]byte, error) {
			require.Equal(t, downloadURL, url)
			require.Equal(t, "clime-foo", binaryName)
			return []byte("new-binary"), nil
		},
		pluginBinDir: func() (string, error) {
			return installDir, nil
		},
		writeBinary: writePluginBinary,
	}

	entry := plugin.ManifestEntry{Name: "foo", Version: "1.0.0", Type: plugin.SourceTypeGitHub, Source: repo}
	result, err := g.Update("foo", entry)
	require.NoError(t, err)
	require.True(t, result.Updated, "Update() should mark updated")
	wantPath := filepath.Join(installDir, "clime-foo")
	gotContent, err := os.ReadFile(wantPath)
	require.NoError(t, err)
	require.Equal(t, wantPath, result.Path)
	require.Equal(t, "new-binary", string(gotContent))
	require.Equal(t, "1.0.0", result.CurrentVersion)
	require.Equal(t, "1.1.0", result.LatestVersion)
}

func TestGitHubInstallerReportsReleaseFetchFailure(t *testing.T) {
	t.Parallel()

	g := &GitHubInstaller{
		Repo: "acme/clime-foo",
		fetchLatest: func(repo string) (*githubrelease.Release, error) {
			return nil, fmt.Errorf("not found")
		},
	}

	_, err := g.Install("foo")
	require.ErrorContains(t, err, "not found", "Install() expected error")
}
