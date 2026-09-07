package githubrelease

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/git-hulk/clime/internal/githubcli"
)

const apiBase = "https://api.github.com"

type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func (release *Release) Version() string {
	return strings.TrimPrefix(release.TagName, "v")
}

func (release *Release) FindTarGzAsset(prefix, goos, goarch string) (*Asset, error) {
	suffix := fmt.Sprintf("_%s_%s.tar.gz", goos, goarch)
	for _, asset := range release.Assets {
		if strings.HasPrefix(asset.Name, prefix) && strings.HasSuffix(asset.Name, suffix) {
			return &asset, nil
		}
	}
	return nil, fmt.Errorf("no release asset found for %s/%s (looked for %s*%s)", goos, goarch, prefix, suffix)
}

// githubCLIRelease is the JSON shape returned by `gh release view --json tagName,assets`.
type githubCLIRelease struct {
	TagName string           `json:"tagName"`
	Assets  []githubCLIAsset `json:"assets"`
}

type githubCLIAsset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func fetchLatestViaGitHubCLI(repo string) (*Release, error) {
	output, err := exec.Command("gh", "release", "view",
		"--repo", repo,
		"--json", "tagName,assets",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("gh release view: %w", err)
	}
	var cliRelease githubCLIRelease
	if err := json.Unmarshal(output, &cliRelease); err != nil {
		return nil, fmt.Errorf("parse gh release output: %w", err)
	}
	release := &Release{TagName: cliRelease.TagName}
	for _, asset := range cliRelease.Assets {
		release.Assets = append(release.Assets, Asset{
			Name:               asset.Name,
			BrowserDownloadURL: asset.URL,
		})
	}
	return release, nil
}

func FetchLatest(repo string) (*Release, error) {
	if githubcli.Authenticated() {
		return fetchLatestViaGitHubCLI(repo)
	}

	url := fmt.Sprintf("%s/repos/%s/releases/latest", apiBase, repo)
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("GitHub API returned %d: %s", response.StatusCode, string(body))
	}

	var release Release
	if err := json.NewDecoder(response.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

// parseGitHubDownloadURL extracts repo ("owner/name") and asset filename from
// a GitHub release download URL like:
//
//	https://github.com/owner/repo/releases/download/v1.0/asset.tar.gz
func parseGitHubDownloadURL(downloadURL string) (repo, assetName string, ok bool) {
	// Expected path: /owner/repo/releases/download/tag/assetName
	const prefix = "https://github.com/"
	if !strings.HasPrefix(downloadURL, prefix) {
		return "", "", false
	}
	path := strings.TrimPrefix(downloadURL, prefix)
	parts := strings.SplitN(path, "/", 6) // owner, repo, "releases", "download", tag, asset
	if len(parts) != 6 || parts[2] != "releases" || parts[3] != "download" {
		return "", "", false
	}
	return parts[0] + "/" + parts[1], parts[5], true
}

func DownloadTarGzBinary(downloadURL, binaryName string) ([]byte, error) {
	var archiveData []byte

	if repo, assetName, ok := parseGitHubDownloadURL(downloadURL); ok && githubcli.Authenticated() {
		output, err := exec.Command("gh", "release", "download",
			"--repo", repo,
			"--pattern", assetName,
			"--output", "-",
		).Output()
		if err != nil {
			return nil, fmt.Errorf("gh release download: %w", err)
		}
		archiveData = output
	}

	if archiveData == nil {
		request, err := http.NewRequest("GET", downloadURL, nil)
		if err != nil {
			return nil, err
		}
		if token := os.Getenv("GITHUB_TOKEN"); token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()

		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(response.Body)
			return nil, fmt.Errorf("download failed with status %d: %s", response.StatusCode, string(body))
		}

		archiveData, err = io.ReadAll(response.Body)
		if err != nil {
			return nil, fmt.Errorf("read response body: %w", err)
		}
	}

	return extractBinaryFromTarGz(archiveData, binaryName)
}

func extractBinaryFromTarGz(data []byte, binaryName string) ([]byte, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decompress archive: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != binaryName {
			continue
		}
		content, err := io.ReadAll(tarReader)
		if err != nil {
			return nil, fmt.Errorf("read binary from archive: %w", err)
		}
		return content, nil
	}

	return nil, fmt.Errorf("binary %q not found in release archive", binaryName)
}
