package skill

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// snapshotMetaFile marks a committed, immutable cache entry and records how
// it was produced. It is internal metadata and never surfaces in skills.yaml.
const snapshotMetaFile = ".clime-snapshot.json"

type snapshotMeta struct {
	Repository string `json:"repository"`
	Version    string `json:"version"`
	Commit     string `json:"commit"`
}

func climeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".clime"), nil
}

func cacheRoot() (string, error) {
	dir, err := climeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cache"), nil
}

// safeVersion converts a version into a filesystem-safe directory name. When
// characters must be replaced, a short digest of the original version is
// appended so distinct versions cannot collide.
func safeVersion(version string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			return r
		default:
			return '_'
		}
	}, version)
	if safe == version && version != "" && !strings.HasPrefix(version, ".") {
		return safe
	}
	sum := sha256.Sum256([]byte(version))
	return strings.TrimLeft(safe, ".") + "-" + fmt.Sprintf("%x", sum[:4])
}

// SnapshotDir returns the cache directory for a repository at a version.
func SnapshotDir(id RepoID, version string) (string, error) {
	root, err := cacheRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{root, id.Host}, append(strings.Split(id.Path, "/"), safeVersion(version))...)...), nil
}

// snapshotReady reports whether dir holds a committed snapshot.
func snapshotReady(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, snapshotMetaFile))
	return err == nil
}

// EnsureSnapshot returns the immutable cache directory for the repository at
// the locked version, fetching it over Git transport only when it is not yet
// cached. The snapshot is prepared in a temporary directory and committed
// with a rename; a committed entry is never modified in place.
func EnsureSnapshot(id RepoID, version string) (string, error) {
	dir, err := SnapshotDir(id, version)
	if err != nil {
		return "", err
	}
	if snapshotReady(dir) {
		return dir, nil
	}

	root, err := cacheRoot()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(root, ".tmp-fetch-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	commit, err := fetchSnapshot(id, version, tmp)
	if err != nil {
		return "", err
	}
	meta, err := json.MarshalIndent(snapshotMeta{Repository: id.Canonical(), Version: version, Commit: commit}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(tmp, snapshotMetaFile), meta, 0o644); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dir); err != nil {
		// A concurrent fetch may have committed the same snapshot first.
		if snapshotReady(dir) {
			return dir, nil
		}
		return "", fmt.Errorf("failed to commit snapshot cache: %w", err)
	}
	return dir, nil
}

// copyTree copies a directory tree, skipping Git metadata.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			if rel == "." {
				return nil
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, filepath.Join(dst, rel), info.Mode().Perm())
	})
}

func copyFile(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// PurgeCache deletes every committed cache entry that the manifest does not
// reference, plus leftover temporary fetch directories. Referenced snapshots
// are never removed. It returns the deleted entry paths.
func PurgeCache(m *Manifest) ([]string, error) {
	root, err := cacheRoot()
	if err != nil {
		return nil, err
	}
	referenced := make(map[string]bool)
	for _, r := range m.Repos {
		dir, err := SnapshotDir(r.ID, r.Version)
		if err != nil {
			return nil, err
		}
		referenced[dir] = true
	}

	var removed []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !d.IsDir() || path == root {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".tmp-") {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			return filepath.SkipDir
		}
		if !snapshotReady(path) {
			return nil
		}
		if !referenced[path] {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			removed = append(removed, path)
		}
		return filepath.SkipDir
	})
	if err != nil {
		return nil, err
	}
	pruneEmptyDirs(root)
	sort.Strings(removed)
	return removed, nil
}

// pruneEmptyDirs removes empty directories left behind after purge.
func pruneEmptyDirs(root string) {
	var dirs []string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return nil
	})
	// Deepest first so parents empty out as children are removed.
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err == nil && len(entries) == 0 {
			os.Remove(dir)
		}
	}
}
