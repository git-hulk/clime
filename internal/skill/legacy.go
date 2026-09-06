package skill

// Migration of manifest layouts that predate repository-keyed versions.
// Everything here exists only to read old ~/.clime/skills.yaml files and
// rewrite them into the current shape; steady-state code lives in
// manifest.go.

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// UnmarshalYAML accepts both the mapping form and the legacy plain-string
// form ("owner/repo") of a sources entry.
func (r *SourceRecord) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		return value.Decode(&r.Repo)
	}
	type plain SourceRecord
	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}
	*r = SourceRecord(p)
	return nil
}

// parseManifest unmarshals manifest data and migrates legacy layouts,
// persisting the migrated form so the migration is a one-time cost.
func parseManifest(path string, data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	if m.normalize() {
		// The manifest is still usable when saving fails, so the error
		// is not fatal here.
		backupManifest(path, data)
		_ = m.Save()
	}
	return &m, nil
}

// backupManifest keeps the pre-migration manifest beside the current one so a
// downgraded clime, which cannot parse the repository-keyed layout, has a file
// to restore. An existing backup is kept: it is the older, more original state.
func backupManifest(path string, data []byte) {
	backup := path + ".bak"
	if _, err := os.Stat(backup); err == nil {
		return
	}
	_ = os.WriteFile(backup, data, 0o644)
}

// normalize migrates older manifest layouts to the current one: the version
// lives on the source repository, never on a skill or in a source's name.
// Version queries persisted in a source string are stripped (a concrete pin
// becomes the source's version), legacy per-skill versions move to their
// source, duplicate sources fold into the first-seen spelling, and every
// installed skill's source is listed. Returns whether anything changed.
func (m *Manifest) normalize() bool {
	changed := false

	// Version candidates for sources that have none, keyed by lowercased repo.
	candidates := make(map[string]string)
	noteCandidate := func(repo, version string) {
		key := strings.ToLower(repo)
		if version != "" && version != "latest" && candidates[key] == "" {
			candidates[key] = version
		}
	}

	for i, s := range m.Skills {
		repo, query := splitSource(s.Source)
		if repo != s.Source {
			m.Skills[i].Source = repo
			changed = true
		}
		noteCandidate(repo, query)
		if s.LegacyVersion != "" {
			noteCandidate(repo, s.LegacyVersion)
			m.Skills[i].LegacyVersion = ""
			changed = true
		}
	}

	sources := m.Sources
	m.Sources = nil
	for _, r := range sources {
		repo, query := splitSource(r.Repo)
		noteCandidate(repo, query)
		if i := m.sourceIndex(repo); i >= 0 {
			if m.Sources[i].Version == "" {
				m.Sources[i].Version = r.Version
			}
			changed = true
			continue
		}
		if repo != r.Repo {
			changed = true
		}
		m.Sources = append(m.Sources, SourceRecord{Repo: repo, Version: r.Version})
	}

	for _, s := range m.Skills {
		if s.Source != "" && m.sourceIndex(s.Source) < 0 {
			m.Sources = append(m.Sources, SourceRecord{Repo: s.Source})
			changed = true
		}
	}

	for i, r := range m.Sources {
		if r.Version == "" {
			if v := candidates[strings.ToLower(r.Repo)]; v != "" {
				m.Sources[i].Version = v
				changed = true
			}
		}
	}

	return changed
}
