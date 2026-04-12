package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

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

// sourceRepoDir returns the persistent local directory for a cached source repository.
// The directory is under ~/.clime/sources/<sanitized-repo>/.
func sourceRepoDir(repo string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	name := repo
	name = strings.TrimPrefix(name, "https://")
	name = strings.TrimPrefix(name, "http://")
	name = strings.TrimPrefix(name, "git@")
	name = strings.TrimSuffix(name, ".git")
	name = strings.ReplaceAll(name, ":", "/")
	return filepath.Join(home, ".clime", "sources", name), nil
}

// RemoveSourceDir removes the persistent local cache for a source repository.
func RemoveSourceDir(repo string) error {
	dir, err := sourceRepoDir(repo)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
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

// cloneViaGH attempts to clone a repo using the GitHub CLI (gh).
func cloneViaGH(repo, dir string) error {
	cmd := exec.Command("gh", "repo", "clone", repo, dir, "--", "--depth", "1")
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("gh repo clone failed: %w\n%s", err, out)
	}
	return nil
}

// cloneViaGit performs a shallow clone (depth 1) using git directly.
func cloneViaGit(repo, dir string) error {
	url := repoToCloneURL(repo)
	cmd := exec.Command("git", "clone", "--depth", "1", url, dir)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("git clone failed: %w\n%s", err, out)
	}
	return nil
}

// cloneRepoTo performs a shallow clone (depth 1) into the specified directory.
// It first attempts to use the GitHub CLI (gh) for cloning, falling back to
// git if gh is unavailable or the clone fails.
func cloneRepoTo(repo, dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	if err := cloneViaGH(repo, dir); err == nil {
		return nil
	}
	return cloneViaGit(repo, dir)
}

// pullRepo updates an existing git repository by pulling the latest changes.
func pullRepo(dir string) error {
	cmd := exec.Command("git", "pull")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git pull failed: %w\n%s", err, out)
	}
	return nil
}

// PrepareRepoDir returns a directory that can be read for skill files.
// Existing local repos are reused directly; remote repos are cached in
// ~/.clime/sources/ and updated with git pull on subsequent calls.
// The returned cleanup function must always be called by the caller.
func PrepareRepoDir(repo string) (string, func(), error) {
	if dir, ok, err := LocalRepoDir(repo); err != nil {
		return "", nil, err
	} else if ok {
		return dir, func() {}, nil
	}

	srcDir, err := sourceRepoDir(repo)
	if err != nil {
		return "", nil, err
	}

	if info, err := os.Stat(srcDir); err == nil && info.IsDir() {
		// Source exists locally, update it with the latest changes.
		if err := pullRepo(srcDir); err != nil {
			return "", nil, fmt.Errorf("failed to update %s: %w", repo, err)
		}
		return srcDir, func() {}, nil
	}

	// Clone to persistent source directory.
	if err := cloneRepoTo(repo, srcDir); err != nil {
		return "", nil, fmt.Errorf("failed to clone %s: %w", repo, err)
	}
	return srcDir, func() {}, nil
}

// FetchRepoManifest fetches skills from a repo's manifest.
// The repo is always cloned (or updated) into ~/.clime/sources/ so
// the source is cached locally for subsequent operations.
// Existing local paths are read directly without cloning.
func FetchRepoManifest(repo string) (*RepoManifest, error) {
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

// pluginFile represents the .claude-plugin/plugin.json format.
type pluginFile struct {
	Name   string `json:"name"`
	Skills string `json:"skills"`
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

// CloneRepo clones (or updates) a repo into ~/.clime/sources/ and returns the path.
func CloneRepo(repo string) (string, error) {
	dir, _, err := PrepareRepoDir(repo)
	return dir, err
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
