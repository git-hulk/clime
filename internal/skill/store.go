package skill

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Store owns the version cache under ~/.clime/sources/: one immutable
// directory per (source, resolved version), never mutated after clone.
type Store struct {
	Root string
}

// Snapshot is a Source materialized on disk at one concrete version.
type Snapshot struct {
	Source Source
	Dir    string
	// Version is the concrete tag or full commit SHA the directory holds.
	// Local sources have no version identity, so their version is empty.
	Version string
}

// OpenStore returns the store rooted at ~/.clime/sources.
func OpenStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	return &Store{Root: filepath.Join(home, ".clime", "sources")}, nil
}

// Snapshot materializes src at the version its query resolves to (latest
// when it carries none). A concrete version already in cache is returned
// without network access; floating queries (latest, a semver line, a
// branch) always resolve remotely because only resolved versions are
// cached. Local sources are used in place without cloning.
func (st *Store) Snapshot(src Source) (*Snapshot, error) {
	if src.IsLocal() {
		if src.Query != "" {
			return nil, fmt.Errorf("version %q is not supported for local path %q", src.Query, src.Repo)
		}
		dir, err := src.Dir()
		if err != nil {
			return nil, err
		}
		return &Snapshot{Source: src, Dir: dir}, nil
	}

	query := src.Query
	if query == "" {
		query = "latest"
	}
	base := st.repoDir(src)
	if dir := versionDir(base, query); dirExists(dir) {
		return &Snapshot{Source: src, Dir: dir, Version: query}, nil
	}
	resolved, err := resolveVersion(src, query)
	if err != nil {
		return nil, err
	}
	dir := versionDir(base, resolved)
	if !dirExists(dir) {
		if err := cloneAtVersion(src, resolved, dir); err != nil {
			return nil, fmt.Errorf("failed to clone %s at %s: %w", src.Repo, resolved, err)
		}
	}
	return &Snapshot{Source: src, Dir: dir, Version: resolved}, nil
}

// Remove deletes every cached version of the source.
func (st *Store) Remove(src Source) error {
	return os.RemoveAll(st.repoDir(src))
}

// repoDir returns the base cache path for a source repository; version
// directories live beside it, keyed by versionDir.
func (st *Store) repoDir(src Source) string {
	name := src.Repo
	name = strings.TrimPrefix(name, "https://")
	name = strings.TrimPrefix(name, "http://")
	name = strings.TrimPrefix(name, "git@")
	name = strings.TrimSuffix(name, ".git")
	name = strings.ReplaceAll(name, ":", "/")
	return filepath.Join(st.Root, name)
}

// versionDir returns the immutable cache directory for one version of a
// source repository.
func versionDir(base, version string) string {
	return base + "@" + strings.ReplaceAll(version, "/", "-")
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// Catalog reads the skills this snapshot offers.
func (s *Snapshot) Catalog() (*Catalog, error) {
	catalog, err := ReadCatalog(s.Dir)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.Source.Repo, err)
	}
	return catalog, nil
}

// SkillFiles reads all files under one skill's path, keyed by path
// relative to it. A path naming a single file yields that file alone.
func (s *Snapshot) SkillFiles(path string) (map[string][]byte, error) {
	root := filepath.Join(s.Dir, path)
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("skill path %q not found: %w", path, err)
	}

	files := make(map[string][]byte)
	if !info.IsDir() {
		data, err := os.ReadFile(root)
		if err != nil {
			return nil, err
		}
		files[filepath.Base(root)] = data
		return files, nil
	}

	err = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if fi.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
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

// cloneAtVersion clones a source checked out at the given version (a tag,
// branch, or commit SHA) into dir. Tags and branches are cloned directly;
// a commit SHA falls back to cloning the default branch and fetching the
// commit.
func cloneAtVersion(src Source, version, dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	cmd := exec.Command("git", "clone", "--depth", "1", "--branch", version, src.CloneURL(), dir)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if _, err := cmd.CombinedOutput(); err == nil {
		return nil
	}
	os.RemoveAll(dir)

	if err := cloneDefault(src, dir); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"fetch", "--depth", "1", "origin", version},
		{"checkout", "--detach", "FETCH_HEAD"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			os.RemoveAll(dir)
			return fmt.Errorf("failed to check out version %q: %w\n%s", version, err, out)
		}
	}
	return nil
}

// cloneDefault performs a shallow clone of the default branch, trying the
// GitHub CLI (gh) first and falling back to git.
func cloneDefault(src Source, dir string) error {
	cmd := exec.Command("gh", "repo", "clone", src.Repo, dir, "--", "--depth", "1")
	if _, err := cmd.CombinedOutput(); err == nil {
		return nil
	}
	os.RemoveAll(dir)

	cmd = exec.Command("git", "clone", "--depth", "1", src.CloneURL(), dir)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("git clone failed: %w\n%s", err, out)
	}
	return nil
}
