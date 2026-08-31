package skill

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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

// FetchRepoManifest fetches the skill catalog from a repo.
// The repo is always cloned (or updated) into ~/.clime/sources/ so
// the source is cached locally for subsequent operations.
// Existing local paths are read directly without cloning.
func FetchRepoManifest(repo string) (*Catalog, error) {
	dir, cleanup, err := PrepareRepoDir(repo)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	catalog, err := ReadCatalog(dir)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", repo, err)
	}
	return catalog, nil
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
