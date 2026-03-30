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

func (r *Release) Version() string {
	return strings.TrimPrefix(r.TagName, "v")
}

func (r *Release) FindTarGzAsset(prefix, goos, goarch string) (*Asset, error) {
	suffix := fmt.Sprintf("_%s_%s.tar.gz", goos, goarch)
	for _, asset := range r.Assets {
		if strings.HasPrefix(asset.Name, prefix) && strings.HasSuffix(asset.Name, suffix) {
			return &asset, nil
		}
	}
	return nil, fmt.Errorf("no release asset found for %s/%s (looked for %s*%s)", goos, goarch, prefix, suffix)
}

// ghCLIAuthed returns true if the gh CLI is installed and authenticated.
func ghCLIAuthed() bool {
	return exec.Command("gh", "auth", "status").Run() == nil
}

// ghRelease is the JSON shape returned by `gh release view --json tagName,assets`.
type ghRelease struct {
	TagName string    `json:"tagName"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func fetchLatestViaGH(repo string) (*Release, error) {
	out, err := exec.Command("gh", "release", "view",
		"--repo", repo,
		"--json", "tagName,assets",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("gh release view: %w", err)
	}
	var gr ghRelease
	if err := json.Unmarshal(out, &gr); err != nil {
		return nil, fmt.Errorf("parse gh release output: %w", err)
	}
	release := &Release{TagName: gr.TagName}
	for _, a := range gr.Assets {
		release.Assets = append(release.Assets, Asset{
			Name:               a.Name,
			BrowserDownloadURL: a.URL,
		})
	}
	return release, nil
}

func FetchLatest(repo string) (*Release, error) {
	if ghCLIAuthed() {
		if r, err := fetchLatestViaGH(repo); err == nil {
			return r, nil
		}
	}

	url := fmt.Sprintf("%s/repos/%s/releases/latest", apiBase, repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
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

	if repo, assetName, ok := parseGitHubDownloadURL(downloadURL); ok && ghCLIAuthed() {
		out, err := exec.Command("gh", "release", "download",
			"--repo", repo,
			"--pattern", assetName,
			"--output", "-",
		).Output()
		if err == nil {
			archiveData = out
		}
	}

	if archiveData == nil {
		req, err := http.NewRequest("GET", downloadURL, nil)
		if err != nil {
			return nil, err
		}
		if token := os.Getenv("GITHUB_TOKEN"); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(body))
		}

		archiveData, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read response body: %w", err)
		}
	}

	return extractBinaryFromTarGz(archiveData, binaryName)
}

func extractBinaryFromTarGz(data []byte, binaryName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decompress archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != binaryName {
			continue
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read binary from archive: %w", err)
		}
		return content, nil
	}

	return nil, fmt.Errorf("binary %q not found in release archive", binaryName)
}
