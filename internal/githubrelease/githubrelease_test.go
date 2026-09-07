package githubrelease

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticatedGitHubReleases(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "release.tar.gz")
	file, err := os.Create(archive)
	require.NoError(t, err)
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "clime", Mode: 0o755, Size: 6}))

	_, err = tw.Write([]byte("binary"))
	require.NoError(t, err)

	for _, close := range []func() error{tw.Close, gz.Close, file.Close} {
		require.NoError(t, close())
	}
	t.Setenv("CLIME_TEST_ARCHIVE", archive)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	script := `#!/bin/sh
if [ "$1" = auth ]; then exit 0; fi
if [ "$CLIME_TEST_GH_FAIL" = 1 ]; then exit 1; fi
case "$2" in
  view) printf '%s\n' '{"tagName":"v1.2.3","assets":[{"name":"clime.tar.gz","url":"https://github.com/owner/repo/releases/download/v1.2.3/clime.tar.gz"}]}' ;;
  download) /bin/cat "$CLIME_TEST_ARCHIVE" ;;
  *) exit 1 ;;
esac
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755))
	release, err := FetchLatest("owner/repo")
	require.NoError(t, err)
	require.Equal(t, "v1.2.3", release.TagName)
	require.Len(t, release.Assets, 1)
	downloadURL := release.Assets[0].BrowserDownloadURL

	body, err := DownloadTarGzBinary(downloadURL, "clime")
	require.NoError(t, err)
	require.Equal(t, "binary", string(body))

	t.Setenv("CLIME_TEST_GH_FAIL", "1")

	_, err = FetchLatest("owner/repo")
	require.ErrorContains(t, err, "gh release view")

	_, err = DownloadTarGzBinary(downloadURL, "clime")
	require.ErrorContains(t, err, "gh release download")
}

func TestFindTarGzAsset(t *testing.T) {
	t.Parallel()

	release := &Release{
		TagName: "v1.2.3",
		Assets: []Asset{
			{Name: "clime_1.2.3_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/linux-amd64"},
			{Name: "clime_1.2.3_darwin_arm64.tar.gz", BrowserDownloadURL: "https://example.com/darwin-arm64"},
		},
	}

	asset, err := release.FindTarGzAsset("clime_", "darwin", "arm64")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/darwin-arm64", asset.BrowserDownloadURL)

	_, err = release.FindTarGzAsset("clime_", "windows", "arm64")
	require.Error(t, err, "missing platform must not select an unrelated asset")
}

func TestParseGitHubDownloadURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		url       string
		wantRepo  string
		wantAsset string
		wantOK    bool
	}{
		{
			name:      "valid URL",
			url:       "https://github.com/git-hulk/clime/releases/download/v0.0.5/clime_0.0.5_darwin_arm64.tar.gz",
			wantRepo:  "git-hulk/clime",
			wantAsset: "clime_0.0.5_darwin_arm64.tar.gz",
			wantOK:    true,
		},
		{
			name:   "non-GitHub URL",
			url:    "https://example.com/downloads/file.tar.gz",
			wantOK: false,
		},
		{
			name:   "GitHub URL but not a release download",
			url:    "https://github.com/git-hulk/clime/archive/refs/tags/v1.0.tar.gz",
			wantOK: false,
		},
		{
			name:   "empty URL",
			url:    "",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo, asset, ok := parseGitHubDownloadURL(tt.url)
			require.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantRepo, repo)
				assert.Equal(t, tt.wantAsset, asset)
			}
		})
	}
}
