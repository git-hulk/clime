package installer

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/git-hulk/clime/internal/githubrelease"
	"github.com/git-hulk/clime/internal/plugin"
)

func TestGitHubInstallerUpdateSkipsWhenAlreadyLatest(t *testing.T) {
	t.Parallel()

	g := &GitHubInstaller{
		Repo: "acme/clime-foo",
		fetchLatest: func(repo string) (*githubrelease.Release, error) {
			if repo != "acme/clime-foo" {
				t.Fatalf("unexpected repo: %s", repo)
			}
			return &githubrelease.Release{TagName: "v1.2.3"}, nil
		},
		downloadBinary: func(url, binaryName string) ([]byte, error) {
			t.Fatal("downloadBinary should not be called when plugin is already latest")
			return nil, nil
		},
		pluginBinDir: func() (string, error) {
			t.Fatal("pluginBinDir should not be called when plugin is already latest")
			return "", nil
		},
		writeBinary: func(destPath string, binaryContent []byte) error {
			t.Fatal("writeBinary should not be called when plugin is already latest")
			return nil
		},
	}

	entry := plugin.ManifestEntry{Name: "foo", Version: "1.2.3", Type: plugin.SourceTypeGitHub, Source: "acme/clime-foo"}
	result, err := g.Update("foo", entry)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if result.Updated {
		t.Fatal("Update() should not mark updated when versions match")
	}
	if result.LatestVersion != "1.2.3" {
		t.Fatalf("LatestVersion = %q, want %q", result.LatestVersion, "1.2.3")
	}
}

func TestGitHubInstallerUpdateApplies(t *testing.T) {
	t.Parallel()

	const (
		repo        = "acme/clime-foo"
		downloadURL = "https://example.com/clime-foo.tar.gz"
		installDir  = "/tmp/clime-plugin-test"
	)

	var (
		gotDestPath string
		gotContent  []byte
	)
	assetName := fmt.Sprintf("clime-foo_1.1.0_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)

	g := &GitHubInstaller{
		Repo: repo,
		fetchLatest: func(gotRepo string) (*githubrelease.Release, error) {
			if gotRepo != repo {
				t.Fatalf("fetchLatest repo = %q, want %q", gotRepo, repo)
			}
			return &githubrelease.Release{
				TagName: "v1.1.0",
				Assets: []githubrelease.Asset{
					{Name: assetName, BrowserDownloadURL: downloadURL},
				},
			}, nil
		},
		downloadBinary: func(url, binaryName string) ([]byte, error) {
			if url != downloadURL {
				t.Fatalf("download url = %q, want %q", url, downloadURL)
			}
			if binaryName != "clime-foo" {
				t.Fatalf("binaryName = %q, want %q", binaryName, "clime-foo")
			}
			return []byte("new-binary"), nil
		},
		pluginBinDir: func() (string, error) {
			return installDir, nil
		},
		writeBinary: func(destPath string, binaryContent []byte) error {
			gotDestPath = destPath
			gotContent = binaryContent
			return nil
		},
	}

	entry := plugin.ManifestEntry{Name: "foo", Version: "1.0.0", Type: plugin.SourceTypeGitHub, Source: repo}
	result, err := g.Update("foo", entry)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !result.Updated {
		t.Fatal("Update() should mark updated")
	}
	wantPath := filepath.Join(installDir, "clime-foo")
	if gotDestPath != wantPath {
		t.Fatalf("destPath = %q, want %q", gotDestPath, wantPath)
	}
	if string(gotContent) != "new-binary" {
		t.Fatalf("binary content = %q, want %q", string(gotContent), "new-binary")
	}
	if result.CurrentVersion != "1.0.0" || result.LatestVersion != "1.1.0" {
		t.Fatalf("result versions = %q -> %q", result.CurrentVersion, result.LatestVersion)
	}
}

func TestGitHubInstallerErrorsWhenNoRepo(t *testing.T) {
	t.Parallel()

	g := &GitHubInstaller{
		Repo: "acme/clime-foo",
		fetchLatest: func(repo string) (*githubrelease.Release, error) {
			return nil, fmt.Errorf("not found")
		},
	}

	_, err := g.Install("foo")
	if err == nil {
		t.Fatal("Install() expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Install() error = %q, want it to contain %q", err.Error(), "not found")
	}
}

func TestGitHubInstallerPluginType(t *testing.T) {
	t.Parallel()
	g := NewGitHubInstaller("acme/foo")
	if g.PluginType() != plugin.SourceTypeGitHub {
		t.Fatalf("PluginType() = %q, want %q", g.PluginType(), plugin.SourceTypeGitHub)
	}
	if g.Source() != "acme/foo" {
		t.Fatalf("Source() = %q, want %q", g.Source(), "acme/foo")
	}
}
