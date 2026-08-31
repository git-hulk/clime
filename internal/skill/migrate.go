package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// legacySkill and legacyManifest describe the pre-versioning manifest format
// with top-level skills and sources fields.
type legacySkill struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description,omitempty"`
	Source      string    `yaml:"source"`
	Path        string    `yaml:"path"`
	InstalledAt time.Time `yaml:"installed_at,omitempty"`
}

type legacyManifest struct {
	Skills  []legacySkill `yaml:"skills"`
	Sources []string      `yaml:"sources,omitempty"`
}

// legacySourceDir reproduces the old ~/.clime/sources/<sanitized-repo> layout
// so migration can locate the mutable checkout a legacy skill came from.
func legacySourceDir(source string) (string, error) {
	dir, err := climeDir()
	if err != nil {
		return "", err
	}
	name := source
	name = strings.TrimPrefix(name, "https://")
	name = strings.TrimPrefix(name, "http://")
	name = strings.TrimPrefix(name, "git@")
	name = strings.TrimSuffix(name, ".git")
	name = strings.ReplaceAll(name, ":", "/")
	return filepath.Join(dir, "sources", name), nil
}

// migrateLegacy converts a legacy manifest to the repository-keyed format.
// It groups skills by canonical repository, locks each repository to the HEAD
// commit of its existing local checkout (never the remote), snapshots that
// checkout into the immutable cache, and validates the migrated manifest
// before replacing the file. The original is saved as skills.yaml.bak, and
// any failure leaves the legacy manifest and agent targets unchanged. A local
// directory source, a missing checkout, or a post-migration conflict aborts
// the whole migration.
func migrateLegacy(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var legacy legacyManifest
	if err := yaml.Unmarshal(data, &legacy); err != nil {
		return fmt.Errorf("failed to parse legacy manifest: %w", err)
	}

	type group struct {
		id     RepoID
		source string
		skills []string
	}
	groups := make(map[string]*group)
	var order []string
	for _, s := range legacy.Skills {
		id, err := ParseRepo(s.Source)
		if err != nil {
			return fmt.Errorf("skill %q source %q: %w", s.Name, s.Source, err)
		}
		canonical := id.Canonical()
		g, ok := groups[canonical]
		if !ok {
			g = &group{id: id, source: s.Source}
			groups[canonical] = g
			order = append(order, canonical)
		}
		g.skills = append(g.skills, s.Name)
	}
	sort.Strings(order)

	m := newEmptyManifest(path)
	for _, canonical := range order {
		g := groups[canonical]
		srcDir, err := legacySourceDir(g.source)
		if err != nil {
			return err
		}
		if info, statErr := os.Stat(srcDir); statErr != nil || !info.IsDir() {
			return fmt.Errorf("repository %s has no local checkout at %s; run the previous clime version or reinstall the skills after migration", canonical, srcDir)
		}
		out, err := runGit(srcDir, "rev-parse", "HEAD")
		if err != nil {
			return fmt.Errorf("failed to resolve HEAD of %s: %w", canonical, err)
		}
		commit := strings.TrimSpace(out)
		if !IsFullCommit(commit) {
			return fmt.Errorf("repository %s checkout at %s has no usable HEAD commit", canonical, srcDir)
		}
		if _, err := commitSnapshotFromDir(g.id, commit, srcDir); err != nil {
			return fmt.Errorf("failed to snapshot %s: %w", canonical, err)
		}
		if _, err := m.AddRepo(g.id.DisplayKey(), commit, g.skills); err != nil {
			return err
		}
	}

	// Validate the migrated manifest, cached content, catalogs, and
	// conflicts before touching the legacy file. Snapshots exist already,
	// so this never contacts a remote.
	if _, err := preflight(m); err != nil {
		return fmt.Errorf("migrated manifest is invalid, keeping the legacy manifest: %w", err)
	}

	if err := os.WriteFile(path+".bak", data, 0o644); err != nil {
		return fmt.Errorf("failed to save legacy manifest backup: %w", err)
	}
	if err := m.Save(); err != nil {
		return err
	}
	return nil
}
