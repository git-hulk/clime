package githubrelease

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthenticatedGitHubReleases(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "release.tar.gz")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "clime", Mode: 0o755, Size: 6}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("binary")); err != nil {
		t.Fatal(err)
	}
	for _, close := range []func() error{tw.Close, gz.Close, file.Close} {
		if err := close(); err != nil {
			t.Fatal(err)
		}
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
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	release, err := FetchLatest("owner/repo")
	if err != nil || release.TagName != "v1.2.3" || len(release.Assets) != 1 {
		t.Fatalf("release = %+v, %v", release, err)
	}
	downloadURL := release.Assets[0].BrowserDownloadURL
	if body, err := DownloadTarGzBinary(downloadURL, "clime"); err != nil || string(body) != "binary" {
		t.Fatalf("binary = %q, %v", body, err)
	}
	t.Setenv("CLIME_TEST_GH_FAIL", "1")
	if _, err := FetchLatest("owner/repo"); err == nil || !strings.Contains(err.Error(), "gh release view") {
		t.Fatalf("authenticated lookup must report gh failure: %v", err)
	}
	if _, err := DownloadTarGzBinary(downloadURL, "clime"); err == nil || !strings.Contains(err.Error(), "gh release download") {
		t.Fatalf("authenticated download must report gh failure: %v", err)
	}
}

func TestReleaseVersion(t *testing.T) {
	t.Parallel()

	release := &Release{TagName: "v1.2.3"}
	if got := release.Version(); got != "1.2.3" {
		t.Fatalf("Version() = %q, want %q", got, "1.2.3")
	}
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
	if err != nil {
		t.Fatalf("FindTarGzAsset() error = %v", err)
	}
	if asset.BrowserDownloadURL != "https://example.com/darwin-arm64" {
		t.Fatalf("FindTarGzAsset() picked %q", asset.BrowserDownloadURL)
	}
}

func TestFindTarGzAssetNotFound(t *testing.T) {
	t.Parallel()

	release := &Release{
		Assets: []Asset{
			{Name: "clime_1.2.3_linux_amd64.tar.gz"},
		},
	}

	if _, err := release.FindTarGzAsset("clime_", "darwin", "arm64"); err == nil {
		t.Fatal("FindTarGzAsset() expected an error for missing asset")
	}
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
			if ok != tt.wantOK {
				t.Fatalf("parseGitHubDownloadURL(%q) ok = %v, want %v", tt.url, ok, tt.wantOK)
			}
			if ok {
				if repo != tt.wantRepo {
					t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
				}
				if asset != tt.wantAsset {
					t.Errorf("asset = %q, want %q", asset, tt.wantAsset)
				}
			}
		})
	}
}
