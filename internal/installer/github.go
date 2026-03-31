package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/git-hulk/clime/internal/githubrelease"
	"github.com/git-hulk/clime/internal/plugin"
)

// GitHubInstaller installs plugins from GitHub releases.
type GitHubInstaller struct {
	Repo           string
	fetchLatest    func(repo string) (*githubrelease.Release, error)
	downloadBinary func(url, binaryName string) ([]byte, error)
	pluginBinDir   func() (string, error)
	writeBinary    func(destPath string, content []byte) error
}

// NewGitHubInstaller returns a GitHubInstaller for the given repository.
func NewGitHubInstaller(repo string) *GitHubInstaller {
	return &GitHubInstaller{
		Repo:           repo,
		fetchLatest:    githubrelease.FetchLatest,
		downloadBinary: githubrelease.DownloadTarGzBinary,
		pluginBinDir:   plugin.PluginBinDir,
		writeBinary:    writePluginBinary,
	}
}

func (g *GitHubInstaller) Install(name string) (string, error) {
	release, err := g.fetchLatest(g.Repo)
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest release for %s: %w", g.Repo, err)
	}

	repoName := g.repoBaseName()
	asset, err := g.findAsset(release, repoName)
	if err != nil {
		return "", err
	}

	installDir, err := g.pluginBinDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create plugin directory: %w", err)
	}

	destPath := filepath.Join(installDir, plugin.BinPrefix+name)
	binaryContent, err := g.downloadBinary(asset.BrowserDownloadURL, repoName)
	if err != nil {
		return "", fmt.Errorf("failed to install plugin: %w", err)
	}
	if err := g.writeBinary(destPath, binaryContent); err != nil {
		return "", fmt.Errorf("failed to install plugin: %w", err)
	}

	return release.Version(), nil
}

func (g *GitHubInstaller) Update(name string, current plugin.ManifestEntry) (*UpdateResult, error) {
	release, err := g.fetchLatest(g.Repo)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest release for %s: %w", g.Repo, err)
	}

	latest := release.Version()
	result := &UpdateResult{
		Name:           name,
		Source:         g.Repo,
		CurrentVersion: current.Version,
		LatestVersion:  latest,
	}

	if current.Version != "" && normalizeVersion(current.Version) == normalizeVersion(latest) {
		return result, nil
	}

	repoName := g.repoBaseName()
	asset, err := g.findAsset(release, repoName)
	if err != nil {
		return nil, err
	}

	binaryContent, err := g.downloadBinary(asset.BrowserDownloadURL, repoName)
	if err != nil {
		return nil, fmt.Errorf("failed to update plugin: %w", err)
	}

	installDir, err := g.pluginBinDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create plugin directory: %w", err)
	}

	destPath := filepath.Join(installDir, plugin.BinPrefix+name)
	if err := g.writeBinary(destPath, binaryContent); err != nil {
		return nil, fmt.Errorf("failed to update plugin: %w", err)
	}

	result.Path = destPath
	result.Updated = true
	return result, nil
}

func (g *GitHubInstaller) Uninstall(name string, entry plugin.ManifestEntry) error {
	return removePluginBinary(name)
}

func (g *GitHubInstaller) DetectVersion(name string) string {
	release, err := g.fetchLatest(g.Repo)
	if err != nil {
		return plugin.VersionLatest
	}
	return release.Version()
}

func (g *GitHubInstaller) PluginType() string { return plugin.SourceTypeGitHub }
func (g *GitHubInstaller) Source() string     { return g.Repo }

func (g *GitHubInstaller) repoBaseName() string {
	if i := strings.LastIndex(g.Repo, "/"); i >= 0 {
		return g.Repo[i+1:]
	}
	return g.Repo
}

func (g *GitHubInstaller) findAsset(release *githubrelease.Release, repoName string) (*githubrelease.Asset, error) {
	pattern := fmt.Sprintf("%s_", repoName)
	return release.FindTarGzAsset(pattern, runtime.GOOS, runtime.GOARCH)
}
