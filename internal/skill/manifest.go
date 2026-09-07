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
	Name   string
	Source string
}

// SourceRecord tracks a known skill source repository and the version its
// installed skills come from.
type SourceRecord struct {
	Repo    string
	Version string
}

// Manifest is the persistent record of which skills
// are installed, from which sources, and the concrete version each source
// is pinned to. Versions live on sources, never on skills.
type Manifest struct {
	Skills  []InstalledSkill
	Sources []SourceRecord
	path    string
}

// manifestSource is the YAML value under a repository key.
type manifestSource struct {
	Skills  []string `yaml:"skills"`
	Version string   `yaml:"version,omitempty"`
}

func manifestPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".clime", "skills.yaml"), nil
}

// LoadManifest reads the skills manifest from path, defaulting to
// ~/.clime/skills.yaml when path is empty.
// Creates the directory and an empty manifest file if they do not exist.
func LoadManifest(path string) (*Manifest, error) {
	if path == "" {
		var err error
		path, err = manifestPath()
		if err != nil {
			return nil, fmt.Errorf("failed to determine manifest path: %w", err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			manifest := &Manifest{path: path}
			if err := manifest.Save(); err != nil {
				return nil, fmt.Errorf("failed to create skills manifest: %w", err)
			}
			return manifest, nil
		}
		return nil, err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	manifest := &Manifest{path: path}
	if len(document.Content) == 0 {
		return manifest, nil
	}
	var sources map[string]manifestSource
	if err := document.Decode(&sources); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	// Walk the keys in document order, retaining their original spelling.
	for index := 0; index < len(document.Content[0].Content); index += 2 {
		repo := document.Content[0].Content[index].Value
		source := Source{Repo: repo}
		if repo == "" || manifest.sourceIndex(repo) >= 0 {
			return nil, fmt.Errorf("failed to parse %s: empty or duplicate source %q", path, repo)
		}
		record := sources[repo]
		manifest.SetSourceVersion(source, record.Version)
		for _, name := range record.Skills {
			if err := validateSkillName(name); err != nil {
				return nil, fmt.Errorf("failed to parse %s: %w", path, err)
			}
			if _, found := manifest.GetSkill(name); found {
				return nil, fmt.Errorf("failed to parse %s: duplicate skill %q", path, name)
			}
			manifest.AddSkill(InstalledSkill{Name: name, Source: repo})
		}
	}
	return manifest, nil
}

// Save writes the manifest to its loaded path, or ~/.clime/skills.yaml
// for a manifest constructed in memory.
func (manifest *Manifest) Save() error {
	path := manifest.path
	if path == "" {
		var err error
		path, err = manifestPath()
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	document := &yaml.Node{Kind: yaml.MappingNode}
	for _, source := range manifest.KnownSources() {
		record, found := manifest.GetSource(source)
		if found {
			source.Repo = record.Repo
		}
		group := manifestSource{Skills: []string{}, Version: record.Version}
		for _, installedSkill := range manifest.SkillsFrom(source) {
			group.Skills = append(group.Skills, installedSkill.Name)
		}
		var value yaml.Node
		if err := value.Encode(group); err != nil {
			return err
		}
		document.Content = append(document.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: source.Repo}, &value)
	}
	data, err := yaml.Marshal(document)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// AddSkill adds or updates an installed skill entry.
func (manifest *Manifest) AddSkill(installedSkill InstalledSkill) {
	for i, existing := range manifest.Skills {
		if existing.Name == installedSkill.Name {
			manifest.Skills[i] = installedSkill
			return
		}
	}
	manifest.Skills = append(manifest.Skills, installedSkill)
}

// RemoveSkill removes an installed skill entry.
func (manifest *Manifest) RemoveSkill(name string) bool {
	for i, installedSkill := range manifest.Skills {
		if installedSkill.Name == name {
			manifest.Skills = append(manifest.Skills[:i], manifest.Skills[i+1:]...)
			return true
		}
	}
	return false
}

// GetSkill returns an installed skill by name.
func (manifest *Manifest) GetSkill(name string) (InstalledSkill, bool) {
	for _, installedSkill := range manifest.Skills {
		if installedSkill.Name == name {
			return installedSkill, true
		}
	}
	return InstalledSkill{}, false
}

// SkillsFrom returns the installed skills recorded from a source.
func (manifest *Manifest) SkillsFrom(source Source) []InstalledSkill {
	var installed []InstalledSkill
	for _, installedSkill := range manifest.Skills {
		if sameRepo(installedSkill.Source, source.Repo) {
			installed = append(installed, installedSkill)
		}
	}
	return installed
}

// sourceIndex returns the index of the record for a source repository, or -1.
func (manifest *Manifest) sourceIndex(repo string) int {
	return slices.IndexFunc(manifest.Sources, func(record SourceRecord) bool {
		return sameRepo(record.Repo, repo)
	})
}

// GetSource returns the recorded entry for a source.
func (manifest *Manifest) GetSource(source Source) (SourceRecord, bool) {
	if i := manifest.sourceIndex(source.Repo); i >= 0 {
		return manifest.Sources[i], true
	}
	return SourceRecord{}, false
}

// AddSource adds a source to the known sources list if not already
// present, keeping the spelling of an existing entry.
func (manifest *Manifest) AddSource(source Source) {
	if manifest.sourceIndex(source.Repo) >= 0 {
		return
	}
	manifest.Sources = append(manifest.Sources, SourceRecord{Repo: source.Repo})
}

// SetSourceVersion records the version a source's skills are installed
// from, adding the source when it is not yet listed.
func (manifest *Manifest) SetSourceVersion(source Source, version string) {
	if i := manifest.sourceIndex(source.Repo); i >= 0 {
		manifest.Sources[i].Version = version
		return
	}
	manifest.Sources = append(manifest.Sources, SourceRecord{Repo: source.Repo, Version: version})
}

// RemoveSource removes a source from the known sources list.
func (manifest *Manifest) RemoveSource(source Source) bool {
	if i := manifest.sourceIndex(source.Repo); i >= 0 {
		manifest.Sources = append(manifest.Sources[:i], manifest.Sources[i+1:]...)
		return true
	}
	return false
}

// InstalledSources lists the sources that have at least one installed
// skill, in first-seen order and spelling.
func (manifest *Manifest) InstalledSources() []Source {
	repos := make([]string, 0, len(manifest.Skills))
	for _, installedSkill := range manifest.Skills {
		repos = append(repos, installedSkill.Source)
	}
	return dedupeSources(repos)
}

// KnownSources lists the sources of installed skills followed by tracked
// sources, preserving order and first-seen spelling.
func (manifest *Manifest) KnownSources() []Source {
	repos := make([]string, 0, len(manifest.Skills)+len(manifest.Sources))
	for _, installedSkill := range manifest.Skills {
		repos = append(repos, installedSkill.Source)
	}
	for _, record := range manifest.Sources {
		repos = append(repos, record.Repo)
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
