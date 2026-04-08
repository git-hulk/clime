package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// httpError represents an HTTP error with a status code.
type httpError struct {
	StatusCode int
	Path       string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("HTTP %d for %s", e.StatusCode, e.Path)
}

// errAPIUnavailable indicates the GitHub API returned a non-404 error
// (e.g. rate limiting, authentication failure) and a clone fallback should be tried.
var errAPIUnavailable = errors.New("GitHub API unavailable")

// fileFetcher fetches a single file from a repository given an owner/repo string and a file path.
type fileFetcher func(repo, path string) ([]byte, error)

// SkillEntry describes a skill in a repo's skills.yaml manifest.
type SkillEntry struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Path        string   `yaml:"path"`
	Tags        []string `yaml:"tags,omitempty"`
}

// RepoManifest is the top-level structure of a repo's skills.yaml.
type RepoManifest struct {
	Skills []SkillEntry `yaml:"skills"`
}

// LocalRepoDir resolves repo to a local directory when it already exists on disk.
// It returns the absolute path, whether the repo was resolved locally, and any error.
func LocalRepoDir(repo string) (string, bool, error) {
	if repo == "" {
		return "", false, nil
	}
	if strings.HasPrefix(repo, "https://") || strings.HasPrefix(repo, "git@") {
		return "", false, nil
	}

	info, err := os.Stat(repo)
	if err == nil {
		if !info.IsDir() {
			return "", false, fmt.Errorf("local repo path %q is not a directory", repo)
		}
		dir, err := filepath.Abs(repo)
		if err != nil {
			return "", false, fmt.Errorf("failed to resolve local repo path %q: %w", repo, err)
		}
		return dir, true, nil
	}
	if !os.IsNotExist(err) {
		return "", false, fmt.Errorf("failed to inspect local repo path %q: %w", repo, err)
	}

	if filepath.IsAbs(repo) || repo == "." || repo == ".." || strings.HasPrefix(repo, "./") || strings.HasPrefix(repo, "../") {
		return "", false, fmt.Errorf("local repo path %q does not exist", repo)
	}

	return "", false, nil
}

// repoToCloneURL converts an "owner/repo" shorthand to a git clone URL.
// Full URLs (https://, git@) and local paths (absolute or relative) are returned as-is.
func repoToCloneURL(repo string) string {
	if strings.HasPrefix(repo, "https://") || strings.HasPrefix(repo, "git@") {
		return repo
	}
	if strings.HasPrefix(repo, "/") || strings.HasPrefix(repo, "./") || strings.HasPrefix(repo, "../") {
		return repo
	}
	return fmt.Sprintf("https://github.com/%s.git", repo)
}

// isGitHubShorthand checks if repo is in "owner/repo" format (not a URL or local path).
func isGitHubShorthand(repo string) bool {
	if strings.HasPrefix(repo, "https://") || strings.HasPrefix(repo, "git@") {
		return false
	}
	if strings.HasPrefix(repo, "/") || strings.HasPrefix(repo, "./") || strings.HasPrefix(repo, "../") {
		return false
	}
	return strings.Contains(repo, "/")
}

// cloneRepo performs a shallow clone (depth 1) into a temp directory and
// returns the path. The caller is responsible for removing it.
func cloneRepo(repo string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "clime-skill-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	url := repoToCloneURL(repo)
	cmd := exec.Command("git", "clone", "--depth", "1", url, tmpDir)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("git clone failed: %w\n%s", err, out)
	}
	return tmpDir, nil
}

// fetchGitHubFile fetches a single file from a GitHub repository via the API.
// repo must be in "owner/repo" format. path is relative to the repo root.
func fetchGitHubFile(repo, path string) ([]byte, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", repo, path)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.raw+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &httpError{StatusCode: resp.StatusCode, Path: path}
	}
	return io.ReadAll(resp.Body)
}

// fetchGitHubFileWithGH fetches a single file from a GitHub repository using the gh CLI.
// repo must be in "owner/repo" format. path is relative to the repo root.
func fetchGitHubFileWithGH(repo, path string) ([]byte, error) {
	cmd := exec.Command("gh", "api",
		fmt.Sprintf("repos/%s/contents/%s", repo, path),
		"-H", "Accept: application/vnd.github.raw+json",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh api failed for %s: %w", path, err)
	}
	return out, nil
}

// PrepareRepoDir returns a directory that can be read for skill files.
// Existing local repos are reused directly; remote repos are cloned into a temp dir.
// The returned cleanup function must always be called by the caller.
func PrepareRepoDir(repo string) (string, func(), error) {
	if dir, ok, err := LocalRepoDir(repo); err != nil {
		return "", nil, err
	} else if ok {
		return dir, func() {}, nil
	}

	dir, err := cloneRepo(repo)
	if err != nil {
		return "", nil, fmt.Errorf("failed to clone %s: %w", repo, err)
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

// FetchRepoManifest fetches skills from a repo's manifest.
// For GitHub repos (owner/repo format), it tries the GitHub API first.
// If the API is unavailable (e.g. rate limited), it falls back to git clone.
// Existing local paths are read directly without cloning.
func FetchRepoManifest(repo string) (*RepoManifest, error) {
	if dir, ok, err := LocalRepoDir(repo); err != nil {
		return nil, err
	} else if ok {
		return readRepoManifestFromDir(dir, repo)
	}

	if isGitHubShorthand(repo) {
		// Try gh CLI first for authenticated access.
		if manifest, err := fetchRepoManifestWithGH(repo); err == nil {
			return manifest, nil
		}

		// Fall back to GitHub API.
		manifest, err := fetchRepoManifestFromAPI(repo)
		if err == nil {
			return manifest, nil
		}
		// Fall back to git clone when the API is unavailable (rate limited, auth error, etc.).
		if errors.Is(err, errAPIUnavailable) {
			return fetchRepoManifestFromClone(repo)
		}
		return nil, err
	}
	return fetchRepoManifestFromClone(repo)
}

// fetchRepoManifestWithGH fetches the manifest using the gh CLI.
// Returns an error if gh is unavailable or the manifest cannot be found.
func fetchRepoManifestWithGH(repo string) (*RepoManifest, error) {
	for _, name := range []string{"skills.yaml", "skills.yml"} {
		data, err := fetchGitHubFileWithGH(repo, name)
		if err == nil {
			var manifest RepoManifest
			if err := yaml.Unmarshal(data, &manifest); err != nil {
				return nil, fmt.Errorf("failed to parse %s: %w", name, err)
			}
			return &manifest, nil
		}
	}

	// Try marketplace.json.
	if data, err := fetchGitHubFileWithGH(repo, ".claude-plugin/marketplace.json"); err == nil {
		if manifest, err := parseMarketplaceData(repo, data, fetchGitHubFileWithGH); err == nil && len(manifest.Skills) > 0 {
			return manifest, nil
		}
	}

	// Fall back to plugin.json.
	if data, err := fetchGitHubFileWithGH(repo, ".claude-plugin/plugin.json"); err == nil {
		if manifest, err := parsePluginData(repo, data, fetchGitHubFileWithGH); err == nil && len(manifest.Skills) > 0 {
			return manifest, nil
		}
	}

	return nil, fmt.Errorf("no skills manifest found in %s via gh CLI", repo)
}

// fetchRepoManifestFromAPI fetches the manifest using the GitHub Contents API.
// Returns errAPIUnavailable when the API returns non-404 errors (e.g. rate limiting)
// so the caller can fall back to git clone.
func fetchRepoManifestFromAPI(repo string) (*RepoManifest, error) {
	var hasAPIError bool

	// Try skills.yaml / skills.yml first.
	for _, name := range []string{"skills.yaml", "skills.yml"} {
		data, err := fetchGitHubFile(repo, name)
		if err == nil {
			var manifest RepoManifest
			if err := yaml.Unmarshal(data, &manifest); err != nil {
				return nil, fmt.Errorf("failed to parse %s: %w", name, err)
			}
			return &manifest, nil
		}
		if isNon404HTTPError(err) {
			hasAPIError = true
		}
	}

	// Fall back to .claude-plugin/marketplace.json.
	if data, err := fetchGitHubFile(repo, ".claude-plugin/marketplace.json"); err == nil {
		if manifest, err := parseMarketplaceData(repo, data, fetchGitHubFile); err == nil && len(manifest.Skills) > 0 {
			return manifest, nil
		}
	} else if isNon404HTTPError(err) {
		hasAPIError = true
	}

	// Fall back to .claude-plugin/plugin.json.
	if data, err := fetchGitHubFile(repo, ".claude-plugin/plugin.json"); err == nil {
		if manifest, err := parsePluginData(repo, data, fetchGitHubFile); err == nil && len(manifest.Skills) > 0 {
			return manifest, nil
		}
	} else if isNon404HTTPError(err) {
		hasAPIError = true
	}

	if hasAPIError {
		return nil, fmt.Errorf("%w for %s: consider setting GITHUB_TOKEN", errAPIUnavailable, repo)
	}
	return nil, fmt.Errorf("no skills manifest found in %s: tried skills.yaml, skills.yml, .claude-plugin/marketplace.json, and .claude-plugin/plugin.json", repo)
}

// isNon404HTTPError returns true if the error is an HTTP error with a status code
// other than 404 (e.g. 403 rate limit, 401 unauthorized).
func isNon404HTTPError(err error) bool {
	var he *httpError
	return errors.As(err, &he) && he.StatusCode != http.StatusNotFound
}

// fetchRepoManifestFromClone resolves the repo to a readable directory and parses skills from its root.
func fetchRepoManifestFromClone(repo string) (*RepoManifest, error) {
	dir, cleanup, err := PrepareRepoDir(repo)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	return readRepoManifestFromDir(dir, repo)
}

func readRepoManifestFromDir(dir, repo string) (*RepoManifest, error) {
	// Try skills.yaml / skills.yml first.
	var data []byte
	var err error
	for _, name := range []string{"skills.yaml", "skills.yml"} {
		data, err = os.ReadFile(filepath.Join(dir, name))
		if err == nil {
			break
		}
	}
	if err == nil {
		var manifest RepoManifest
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("failed to parse skills.yaml: %w", err)
		}
		return &manifest, nil
	}

	// Fall back to .claude-plugin/marketplace.json.
	if manifest, err := parseMarketplaceManifest(dir); err == nil && len(manifest.Skills) > 0 {
		return manifest, nil
	}

	// Fall back to .claude-plugin/plugin.json.
	if manifest, err := parsePluginManifest(dir); err == nil && len(manifest.Skills) > 0 {
		return manifest, nil
	}

	return nil, fmt.Errorf("no skills manifest found in %s: tried skills.yaml, skills.yml, .claude-plugin/marketplace.json, and .claude-plugin/plugin.json", repo)
}

// marketplaceFile represents the .claude-plugin/marketplace.json format.
type marketplaceFile struct {
	Plugins []marketplacePlugin `json:"plugins"`
}

type marketplacePlugin struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Source      string   `json:"source"`
	Skills      []string `json:"skills"`
}

// parseMarketplaceManifest reads .claude-plugin/marketplace.json from a local directory
// and builds a RepoManifest by reading each skill's SKILL.md frontmatter.
func parseMarketplaceManifest(dir string) (*RepoManifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "marketplace.json"))
	if err != nil {
		return nil, err
	}

	var mf marketplaceFile
	if err := json.Unmarshal(data, &mf); err != nil {
		return nil, fmt.Errorf("failed to parse marketplace.json: %w", err)
	}

	var manifest RepoManifest
	seen := make(map[string]bool)
	for _, plugin := range mf.Plugins {
		sourceDir := strings.TrimPrefix(plugin.Source, "./")
		for _, skillPath := range plugin.Skills {
			skillPath = strings.TrimPrefix(skillPath, "./")
			if sourceDir != "" {
				skillPath = filepath.Join(sourceDir, skillPath)
			}
			if seen[skillPath] {
				continue
			}
			seen[skillPath] = true

			entry := SkillEntry{Path: skillPath}
			if fm, err := readSkillFrontmatter(filepath.Join(dir, skillPath, "SKILL.md")); err == nil {
				entry.Name = fm.Name
				entry.Description = fm.Description
			}
			if entry.Name == "" {
				entry.Name = filepath.Base(skillPath)
			}
			manifest.Skills = append(manifest.Skills, entry)
		}
	}
	return &manifest, nil
}

// parseMarketplaceData parses marketplace.json data and fetches SKILL.md
// frontmatter via the GitHub API for each skill.
func parseMarketplaceData(repo string, data []byte, fetch fileFetcher) (*RepoManifest, error) {
	var mf marketplaceFile
	if err := json.Unmarshal(data, &mf); err != nil {
		return nil, fmt.Errorf("failed to parse marketplace.json: %w", err)
	}

	var manifest RepoManifest
	seen := make(map[string]bool)
	for _, plugin := range mf.Plugins {
		sourceDir := strings.TrimPrefix(plugin.Source, "./")
		for _, skillPath := range plugin.Skills {
			skillPath = strings.TrimPrefix(skillPath, "./")
			if sourceDir != "" {
				skillPath = filepath.Join(sourceDir, skillPath)
			}
			if seen[skillPath] {
				continue
			}
			seen[skillPath] = true

			entry := SkillEntry{Path: skillPath}
			if fmData, err := fetch(repo, skillPath+"/SKILL.md"); err == nil {
				if fm, err := parseSkillFrontmatter(fmData); err == nil {
					entry.Name = fm.Name
					entry.Description = fm.Description
				}
			}
			if entry.Name == "" {
				entry.Name = filepath.Base(skillPath)
			}
			manifest.Skills = append(manifest.Skills, entry)
		}
	}
	return &manifest, nil
}

// pluginFile represents the .claude-plugin/plugin.json format.
type pluginFile struct {
	Name   string `json:"name"`
	Skills string `json:"skills"`
}

// ghDirEntry represents a single entry in a GitHub directory listing.
type ghDirEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// parsePluginManifest reads .claude-plugin/plugin.json from a local directory
// and discovers skills by scanning the skills directory it references.
func parsePluginManifest(dir string) (*RepoManifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	if err != nil {
		return nil, err
	}

	var pf pluginFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("failed to parse plugin.json: %w", err)
	}
	if pf.Skills == "" {
		return nil, fmt.Errorf("plugin.json has no skills directory")
	}

	skillsDir := strings.TrimPrefix(pf.Skills, "./")
	entries, err := os.ReadDir(filepath.Join(dir, skillsDir))
	if err != nil {
		return nil, fmt.Errorf("failed to read skills directory: %w", err)
	}

	var manifest RepoManifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillPath := filepath.Join(skillsDir, e.Name())
		entry := SkillEntry{Path: skillPath}
		if fm, err := readSkillFrontmatter(filepath.Join(dir, skillPath, "SKILL.md")); err == nil {
			entry.Name = fm.Name
			entry.Description = fm.Description
		}
		if entry.Name == "" {
			entry.Name = e.Name()
		}
		manifest.Skills = append(manifest.Skills, entry)
	}
	return &manifest, nil
}

// parsePluginData parses plugin.json data and discovers skills by listing the
// skills directory via the provided fetch function.
func parsePluginData(repo string, data []byte, fetch fileFetcher) (*RepoManifest, error) {
	var pf pluginFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("failed to parse plugin.json: %w", err)
	}
	if pf.Skills == "" {
		return nil, fmt.Errorf("plugin.json has no skills directory")
	}

	skillsDir := strings.TrimPrefix(pf.Skills, "./")

	// List the skills directory contents via the GitHub contents API.
	listData, err := fetch(repo, skillsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list skills directory: %w", err)
	}

	var dirEntries []ghDirEntry
	if err := json.Unmarshal(listData, &dirEntries); err != nil {
		return nil, fmt.Errorf("failed to parse directory listing: %w", err)
	}

	var manifest RepoManifest
	for _, de := range dirEntries {
		if de.Type != "dir" {
			continue
		}
		skillPath := filepath.Join(skillsDir, de.Name)
		entry := SkillEntry{Path: skillPath}
		if fmData, err := fetch(repo, skillPath+"/SKILL.md"); err == nil {
			if fm, err := parseSkillFrontmatter(fmData); err == nil {
				entry.Name = fm.Name
				entry.Description = fm.Description
			}
		}
		if entry.Name == "" {
			entry.Name = de.Name
		}
		manifest.Skills = append(manifest.Skills, entry)
	}
	return &manifest, nil
}

// skillFrontmatter holds the YAML frontmatter from a SKILL.md file.
type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// parseSkillFrontmatter parses the YAML frontmatter between --- markers from SKILL.md content.
func parseSkillFrontmatter(data []byte) (*skillFrontmatter, error) {
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, fmt.Errorf("no frontmatter found")
	}
	var fmLines []string
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		fmLines = append(fmLines, line)
	}
	var fm skillFrontmatter
	if err := yaml.Unmarshal([]byte(strings.Join(fmLines, "\n")), &fm); err != nil {
		return nil, err
	}
	return &fm, nil
}

// readSkillFrontmatter reads and parses the YAML frontmatter from a SKILL.md file on disk.
func readSkillFrontmatter(path string) (*skillFrontmatter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseSkillFrontmatter(data)
}

// CloneRepo performs a shallow clone of a repo into a temp directory and
// returns the path. The caller is responsible for removing it with os.RemoveAll.
func CloneRepo(repo string) (string, error) {
	return cloneRepo(repo)
}

// ReadSkillFilesFromDir reads all files under skillPath from a local directory.
// Returns a map of relative file paths to their contents.
func ReadSkillFilesFromDir(dir, skillPath string) (map[string][]byte, error) {
	root := filepath.Join(dir, skillPath)
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("skill path %q not found: %w", skillPath, err)
	}

	files := make(map[string][]byte)
	if !info.IsDir() {
		// Single file.
		data, err := os.ReadFile(root)
		if err != nil {
			return nil, err
		}
		files[filepath.Base(root)] = data
		return files, nil
	}

	err = filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if fi.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[rel] = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read skill files: %w", err)
	}

	return files, nil
}

// CloneAndReadSkillFiles clones the repo and reads all files under skillPath.
// Returns a map of relative file paths to their contents.
func CloneAndReadSkillFiles(repo, skillPath string) (map[string][]byte, error) {
	dir, cleanup, err := PrepareRepoDir(repo)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	return ReadSkillFilesFromDir(dir, skillPath)
}
