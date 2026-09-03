package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// InstalledSkill tracks a skill that has been installed locally.
type InstalledSkill struct {
	Name   string `yaml:"name"`
	Source string `yaml:"source"`
	// LegacyVersion carries the per-skill version that older manifests
	// recorded. The version belongs to the source repository; normalize
	// moves this value there and clears it, so it is never written back.
	LegacyVersion string `yaml:"version,omitempty"`
	Path          string `yaml:"path"`
}

// Source tracks a known skill source repository and the version its
// installed skills come from.
type Source struct {
	Repo    string `yaml:"repo"`
	Version string `yaml:"version,omitempty"`
}

// UnmarshalYAML accepts both the mapping form and the legacy plain-string
// form ("owner/repo") of a sources entry.
func (s *Source) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		return value.Decode(&s.Repo)
	}
	type plain Source
	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}
	*s = Source(p)
	return nil
}

// Manifest holds installed skills and known sources.
type Manifest struct {
	Skills  []InstalledSkill `yaml:"skills"`
	Sources []Source         `yaml:"sources,omitempty"`
}

func manifestPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".clime", "skills.yaml"), nil
}

// LoadManifest reads the skills manifest from ~/.clime/skills.yaml.
// Creates the directory and an empty manifest file if they do not exist.
func LoadManifest() (*Manifest, error) {
	path, err := manifestPath()
	if err != nil {
		return nil, fmt.Errorf("failed to determine manifest path: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			m := &Manifest{}
			if err := m.Save(); err != nil {
				return nil, fmt.Errorf("failed to create skills manifest: %w", err)
			}
			return m, nil
		}
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	changed := m.normalize()
	if m.stampMissingVersions() {
		changed = true
	}
	if changed {
		// Persist so migration and resolution are a one-time cost; the
		// manifest is still usable when saving fails, so the error is
		// not fatal here.
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

// resolveLatestVersion resolves a repository's latest version; swappable in tests.
var resolveLatestVersion = func(repo string) (string, error) {
	return ResolveVersion(repo, "latest")
}

// stampMissingVersions backfills the version of sources recorded before
// versions were tracked by resolving them to latest. Local paths have no
// version identity, and a source that fails to resolve (e.g. offline) is
// left empty to retry on a later load. Returns whether anything was stamped.
func (m *Manifest) stampMissingVersions() bool {
	changed := false
	for i, s := range m.Sources {
		if s.Version != "" || s.Repo == "" {
			continue
		}
		if _, isLocal, err := LocalRepoDir(s.Repo); err != nil || isLocal {
			continue
		}
		if v, _ := resolveLatestVersion(s.Repo); v != "" {
			m.Sources[i].Version = v
			changed = true
		}
	}
	return changed
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
		repo, query := ParseSourceVersion(s.Source)
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
	for _, s := range sources {
		repo, query := ParseSourceVersion(s.Repo)
		noteCandidate(repo, query)
		if i := m.sourceIndex(repo); i >= 0 {
			if m.Sources[i].Version == "" {
				m.Sources[i].Version = s.Version
			}
			changed = true
			continue
		}
		if repo != s.Repo {
			changed = true
		}
		m.Sources = append(m.Sources, Source{Repo: repo, Version: s.Version})
	}

	for _, s := range m.Skills {
		if s.Source != "" && m.sourceIndex(s.Source) < 0 {
			m.Sources = append(m.Sources, Source{Repo: s.Source})
			changed = true
		}
	}

	for i, s := range m.Sources {
		if s.Version == "" {
			if v := candidates[strings.ToLower(s.Repo)]; v != "" {
				m.Sources[i].Version = v
				changed = true
			}
		}
	}

	return changed
}

// Save writes the manifest to disk.
func (m *Manifest) Save() error {
	path, err := manifestPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// AddSkill adds or updates an installed skill entry.
func (m *Manifest) AddSkill(s InstalledSkill) {
	for i, existing := range m.Skills {
		if existing.Name == s.Name {
			m.Skills[i] = s
			return
		}
	}
	m.Skills = append(m.Skills, s)
}

// RemoveSkill removes an installed skill entry.
func (m *Manifest) RemoveSkill(name string) bool {
	for i, s := range m.Skills {
		if s.Name == name {
			m.Skills = append(m.Skills[:i], m.Skills[i+1:]...)
			return true
		}
	}
	return false
}

// GetSkill returns an installed skill by name.
func (m *Manifest) GetSkill(name string) (InstalledSkill, bool) {
	for _, s := range m.Skills {
		if s.Name == name {
			return s, true
		}
	}
	return InstalledSkill{}, false
}

// SameSource reports whether two source identifiers refer to the same
// repository. Repository names are case-insensitive on GitHub and other
// major hosts, so comparison ignores case; version queries are stripped
// because versions are per-install state, not part of a source's identity.
func SameSource(a, b string) bool {
	repoA, _ := ParseSourceVersion(a)
	repoB, _ := ParseSourceVersion(b)
	return strings.EqualFold(repoA, repoB)
}

// sourceIndex returns the index of the source entry for a repository, or -1.
func (m *Manifest) sourceIndex(source string) int {
	return slices.IndexFunc(m.Sources, func(s Source) bool { return SameSource(s.Repo, source) })
}

// GetSource returns the recorded entry for a source repository.
func (m *Manifest) GetSource(source string) (Source, bool) {
	if i := m.sourceIndex(source); i >= 0 {
		return m.Sources[i], true
	}
	return Source{}, false
}

// AddSource adds a source's repository to the known sources list if not
// already present, keeping the spelling of an existing entry.
func (m *Manifest) AddSource(source string) {
	repo, _ := ParseSourceVersion(source)
	if m.sourceIndex(repo) >= 0 {
		return
	}
	m.Sources = append(m.Sources, Source{Repo: repo})
}

// SetSourceVersion records the version a source's skills are installed
// from, adding the source when it is not yet listed.
func (m *Manifest) SetSourceVersion(source, version string) {
	repo, _ := ParseSourceVersion(source)
	if i := m.sourceIndex(repo); i >= 0 {
		m.Sources[i].Version = version
		return
	}
	m.Sources = append(m.Sources, Source{Repo: repo, Version: version})
}

// RemoveSource removes a source from the known sources list.
func (m *Manifest) RemoveSource(source string) bool {
	if i := m.sourceIndex(source); i >= 0 {
		m.Sources = append(m.Sources[:i], m.Sources[i+1:]...)
		return true
	}
	return false
}
