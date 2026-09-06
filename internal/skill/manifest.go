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
	// recorded. The version belongs to the source repository; the legacy
	// migration moves this value there and clears it, so it is never
	// written back.
	LegacyVersion string `yaml:"version,omitempty"`
	Path          string `yaml:"path"`
}

// SourceRecord tracks a known skill source repository and the version its
// installed skills come from.
type SourceRecord struct {
	Repo    string `yaml:"repo"`
	Version string `yaml:"version,omitempty"`
}

// Manifest is the persistent record at ~/.clime/skills.yaml: which skills
// are installed, from which sources, and the concrete version each source
// is pinned to. Versions live on sources, never on skills.
type Manifest struct {
	Skills  []InstalledSkill `yaml:"skills"`
	Sources []SourceRecord   `yaml:"sources,omitempty"`
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
	m, err := parseManifest(path, data)
	if err != nil {
		return nil, err
	}
	return m, nil
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

// SkillsFrom returns the installed skills recorded from a source.
func (m *Manifest) SkillsFrom(src Source) []InstalledSkill {
	var installed []InstalledSkill
	for _, s := range m.Skills {
		if sameRepo(s.Source, src.Repo) {
			installed = append(installed, s)
		}
	}
	return installed
}

// sourceIndex returns the index of the record for a source repository, or -1.
func (m *Manifest) sourceIndex(repo string) int {
	return slices.IndexFunc(m.Sources, func(r SourceRecord) bool {
		return sameRepo(r.Repo, repo)
	})
}

// GetSource returns the recorded entry for a source.
func (m *Manifest) GetSource(src Source) (SourceRecord, bool) {
	if i := m.sourceIndex(src.Repo); i >= 0 {
		return m.Sources[i], true
	}
	return SourceRecord{}, false
}

// AddSource adds a source to the known sources list if not already
// present, keeping the spelling of an existing entry.
func (m *Manifest) AddSource(src Source) {
	if m.sourceIndex(src.Repo) >= 0 {
		return
	}
	m.Sources = append(m.Sources, SourceRecord{Repo: src.Repo})
}

// SetSourceVersion records the version a source's skills are installed
// from, adding the source when it is not yet listed.
func (m *Manifest) SetSourceVersion(src Source, version string) {
	if i := m.sourceIndex(src.Repo); i >= 0 {
		m.Sources[i].Version = version
		return
	}
	m.Sources = append(m.Sources, SourceRecord{Repo: src.Repo, Version: version})
}

// RemoveSource removes a source from the known sources list.
func (m *Manifest) RemoveSource(src Source) bool {
	if i := m.sourceIndex(src.Repo); i >= 0 {
		m.Sources = append(m.Sources[:i], m.Sources[i+1:]...)
		return true
	}
	return false
}

// InstalledSources lists the sources that have at least one installed
// skill, in first-seen order and spelling.
func (m *Manifest) InstalledSources() []Source {
	repos := make([]string, 0, len(m.Skills))
	for _, s := range m.Skills {
		repos = append(repos, s.Source)
	}
	return dedupeSources(repos)
}

// KnownSources lists the sources of installed skills followed by tracked
// sources, preserving order and first-seen spelling.
func (m *Manifest) KnownSources() []Source {
	repos := make([]string, 0, len(m.Skills)+len(m.Sources))
	for _, s := range m.Skills {
		repos = append(repos, s.Source)
	}
	for _, r := range m.Sources {
		repos = append(repos, r.Repo)
	}
	return dedupeSources(repos)
}

// dedupeSources folds case-insensitive duplicates, keeping first-seen
// order and spelling.
func dedupeSources(repos []string) []Source {
	seen := make(map[string]bool)
	var unique []Source
	for _, repo := range repos {
		key := strings.ToLower(repo)
		if repo != "" && !seen[key] {
			seen[key] = true
			unique = append(unique, Source{Repo: repo})
		}
	}
	return unique
}
