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
