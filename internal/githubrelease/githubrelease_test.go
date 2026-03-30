package githubrelease

import "testing"

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
