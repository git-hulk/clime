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
func (record *SourceRecord) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		return value.Decode(&record.Repo)
	}
	type sourceRecordYAML SourceRecord
	var decodedRecord sourceRecordYAML
	if err := value.Decode(&decodedRecord); err != nil {
		return err
	}
	*record = SourceRecord(decodedRecord)
	return nil
}

// parseManifest unmarshals manifest data and migrates legacy layouts,
// persisting the migrated form so the migration is a one-time cost.
func parseManifest(path string, data []byte) (*Manifest, error) {
	manifest := Manifest{path: path}
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	if manifest.normalize() {
		// The manifest is still usable when saving fails, so the error
		// is not fatal here.
		backupManifest(path, data)
		_ = manifest.Save()
	}
	return &manifest, nil
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
func (manifest *Manifest) normalize() bool {
	changed := false

	// Version candidates for sources that have none, keyed by lowercased repo.
	candidates := make(map[string]string)
	noteCandidate := func(repo, version string) {
		key := strings.ToLower(repo)
		if version != "" && version != "latest" && candidates[key] == "" {
			candidates[key] = version
		}
	}

	for i, installedSkill := range manifest.Skills {
		repo, query := splitSource(installedSkill.Source)
		if repo != installedSkill.Source {
			manifest.Skills[i].Source = repo
			changed = true
		}
		noteCandidate(repo, query)
		if installedSkill.LegacyVersion != "" {
			noteCandidate(repo, installedSkill.LegacyVersion)
			manifest.Skills[i].LegacyVersion = ""
			changed = true
		}
	}

	sources := manifest.Sources
	manifest.Sources = nil
	for _, record := range sources {
		repo, query := splitSource(record.Repo)
		noteCandidate(repo, query)
		if i := manifest.sourceIndex(repo); i >= 0 {
			if manifest.Sources[i].Version == "" {
				manifest.Sources[i].Version = record.Version
			}
			changed = true
			continue
		}
		if repo != record.Repo {
			changed = true
		}
		manifest.Sources = append(manifest.Sources, SourceRecord{Repo: repo, Version: record.Version})
	}

	for _, installedSkill := range manifest.Skills {
		if installedSkill.Source != "" && manifest.sourceIndex(installedSkill.Source) < 0 {
			manifest.Sources = append(manifest.Sources, SourceRecord{Repo: installedSkill.Source})
			changed = true
		}
	}

	for i, record := range manifest.Sources {
		if record.Version == "" {
			if candidateVersion := candidates[strings.ToLower(record.Repo)]; candidateVersion != "" {
				manifest.Sources[i].Version = candidateVersion
				changed = true
			}
		}
	}

	return changed
}
