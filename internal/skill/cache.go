package skill

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
