package plugin

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/git-hulk/clime/internal/githubrelease"
)

func TestUpdaterSkipsWhenAlreadyLatest(t *testing.T) {
	t.Parallel()

	u := &Updater{
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
		loadManifest: func() (*Manifest, error) {
			return &Manifest{
				Plugins: []ManifestEntry{
					{Name: "foo", Version: "1.2.3", Type: SourceTypeGitHub, Source: "acme/clime-foo"},
				},
			}, nil
		},
		saveManifest: func(*Manifest) error {
			t.Fatal("saveManifest should not be called when plugin is already latest")
			return nil
		},
		writeBinary: func(destPath string, binaryContent []byte) error {
			t.Fatal("writeBinary should not be called when plugin is already latest")
			return nil
		},
	}

	result, err := u.Update(UpdateOptions{Name: "foo"})
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

func TestUpdaterAppliesUpdateAndSavesManifest(t *testing.T) {
	t.Parallel()

	const (
		repo        = "acme/clime-foo"
		downloadURL = "https://example.com/clime-foo.tar.gz"
		installDir  = "/tmp/clime-plugin-test"
	)

	var (
		gotDestPath string
		gotContent  []byte
		saved       bool
	)
	assetName := fmt.Sprintf("clime-foo_1.1.0_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)

	u := &Updater{
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
		loadManifest: func() (*Manifest, error) {
			return &Manifest{
				Plugins: []ManifestEntry{
					{Name: "foo", Version: "1.0.0", Type: SourceTypeGitHub, Source: repo},
				},
			}, nil
		},
		saveManifest: func(m *Manifest) error {
			saved = true
			entry, ok := m.Get("foo")
			if !ok {
				t.Fatal("manifest entry for foo not found")
			}
			if entry.Version != "1.1.0" {
				t.Fatalf("saved version = %q, want %q", entry.Version, "1.1.0")
			}
			if entry.Source != repo {
				t.Fatalf("saved source = %q, want %q", entry.Source, repo)
			}
			return nil
		},
		writeBinary: func(destPath string, binaryContent []byte) error {
			gotDestPath = destPath
			gotContent = binaryContent
			return nil
		},
	}

	result, err := u.Update(UpdateOptions{Name: "foo"})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !result.Updated {
		t.Fatal("Update() should mark updated")
	}
	if !saved {
		t.Fatal("manifest should be saved after update")
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

func TestUpdaterUpdatesScriptBasedSource(t *testing.T) {
	t.Parallel()

	var (
		ranScript bool
		saved     bool
	)
	u := &Updater{
		fetchLatest: func(repo string) (*githubrelease.Release, error) {
			t.Fatal("fetchLatest should not be called for script source")
			return nil, nil
		},
		downloadBinary: func(url, binaryName string) ([]byte, error) {
			t.Fatal("downloadBinary should not be called for script source")
			return nil, nil
		},
		pluginBinDir: func() (string, error) {
			return "/tmp/clime-plugin-test", nil
		},
		loadManifest: func() (*Manifest, error) {
			return &Manifest{
				Plugins: []ManifestEntry{
					{Name: "account", Version: "latest", Type: SourceTypeScript, Source: "https://example.com/install.sh"},
				},
			}, nil
		},
		saveManifest: func(m *Manifest) error {
			saved = true
			entry, ok := m.Get("account")
			if !ok {
				t.Fatal("manifest entry for account not found")
			}
			if entry.Version != "latest" {
				t.Fatalf("saved version = %q, want %q", entry.Version, "latest")
			}
			if entry.Source != "https://example.com/install.sh" {
				t.Fatalf("saved source = %q, want %q", entry.Source, "https://example.com/install.sh")
			}
			return nil
		},
		writeBinary: func(destPath string, binaryContent []byte) error {
			t.Fatal("writeBinary should not be called for script source")
			return nil
		},
		runScript: func(scriptURL string) error {
			ranScript = true
			if scriptURL != "https://example.com/install.sh" {
				t.Fatalf("scriptURL = %q, want %q", scriptURL, "https://example.com/install.sh")
			}
			return nil
		},
	}

	result, err := u.Update(UpdateOptions{Name: "account"})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !ranScript {
		t.Fatal("script install should run for script source")
	}
	if !saved {
		t.Fatal("manifest should be saved for script source update")
	}
	if !result.Updated {
		t.Fatal("Update() should mark updated for script source")
	}
	if result.LatestVersion != "latest" {
		t.Fatalf("LatestVersion = %q, want %q", result.LatestVersion, "latest")
	}
}

func TestUpdaterUpdatesNpmBasedSource(t *testing.T) {
	t.Parallel()

	var (
		ranNpmUpdate bool
		saved        bool
	)
	u := &Updater{
		fetchLatest: func(repo string) (*githubrelease.Release, error) {
			t.Fatal("fetchLatest should not be called for npm source")
			return nil, nil
		},
		downloadBinary: func(url, binaryName string) ([]byte, error) {
			t.Fatal("downloadBinary should not be called for npm source")
			return nil, nil
		},
		pluginBinDir: func() (string, error) {
			return "/tmp/clime-plugin-test", nil
		},
		loadManifest: func() (*Manifest, error) {
			return &Manifest{
				Plugins: []ManifestEntry{
					{Name: "deploy", Version: "latest", Type: SourceTypeNpm, Source: "@myorg/clime-deploy"},
				},
			}, nil
		},
		saveManifest: func(m *Manifest) error {
			saved = true
			entry, ok := m.Get("deploy")
			if !ok {
				t.Fatal("manifest entry for deploy not found")
			}
			if entry.Version != "latest" {
				t.Fatalf("saved version = %q, want %q", entry.Version, "latest")
			}
			if entry.Source != "@myorg/clime-deploy" {
				t.Fatalf("saved source = %q, want %q", entry.Source, "@myorg/clime-deploy")
			}
			return nil
		},
		writeBinary: func(destPath string, binaryContent []byte) error {
			t.Fatal("writeBinary should not be called for npm source")
			return nil
		},
		runScript: func(scriptURL string) error {
			t.Fatal("runScript should not be called for npm source")
			return nil
		},
		runNpmUpdate: func(pkg string) error {
			ranNpmUpdate = true
			if pkg != "@myorg/clime-deploy" {
				t.Fatalf("pkg = %q, want %q", pkg, "@myorg/clime-deploy")
			}
			return nil
		},
	}

	result, err := u.Update(UpdateOptions{Name: "deploy"})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !ranNpmUpdate {
		t.Fatal("npm update should run for npm source")
	}
	if !saved {
		t.Fatal("manifest should be saved for npm source update")
	}
	if !result.Updated {
		t.Fatal("Update() should mark updated for npm source")
	}
	if result.LatestVersion != "latest" {
		t.Fatalf("LatestVersion = %q, want %q", result.LatestVersion, "latest")
	}
	wantPath := filepath.Join("/tmp/clime-plugin-test", "clime-deploy")
	if result.Path != wantPath {
		t.Fatalf("Path = %q, want %q", result.Path, wantPath)
	}
}

func TestUpdaterUsesDefaultRepoWhenManifestIsEmpty(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("fetch failure")
	u := &Updater{
		fetchLatest: func(repo string) (*githubrelease.Release, error) {
			if repo != "git-hulk/clime-foo" {
				t.Fatalf("default repo = %q, want %q", repo, "git-hulk/clime-foo")
			}
			return nil, sentinel
		},
		downloadBinary: func(url, binaryName string) ([]byte, error) {
			t.Fatal("downloadBinary should not be called on fetch error")
			return nil, nil
		},
		pluginBinDir: func() (string, error) {
			t.Fatal("pluginBinDir should not be called on fetch error")
			return "", nil
		},
		loadManifest: func() (*Manifest, error) {
			return &Manifest{}, nil
		},
		saveManifest: func(*Manifest) error {
			t.Fatal("saveManifest should not be called on fetch error")
			return nil
		},
		writeBinary: func(destPath string, binaryContent []byte) error {
			t.Fatal("writeBinary should not be called on fetch error")
			return nil
		},
	}

	_, err := u.Update(UpdateOptions{Name: "foo"})
	if err == nil {
		t.Fatal("Update() expected fetch error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("Update() error = %v, want wrapped sentinel", err)
	}
}
