package skill

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"

	"gopkg.in/yaml.v3"
)

// RepoSpec is one repository entry of the desired-state manifest: a locked
// version plus the skills selected from that repository.
type RepoSpec struct {
	// Key is the manifest spelling of the repository; it is preserved on
	// mutation and never rewritten to add or remove the github.com prefix.
	Key     string
	ID      RepoID
	Version string
	Skills  []string

	valueNode *yaml.Node
}

// Manifest is the desired-state manifest at ~/.clime/skills.yaml, keyed by
// repository. Mutations edit YAML nodes in place so user comments and
// untouched entries survive round trips.
type Manifest struct {
	path  string
	doc   *yaml.Node
	root  *yaml.Node
	Repos []*RepoSpec
}

func manifestPath() (string, error) {
	dir, err := climeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "skills.yaml"), nil
}

// LoadManifest reads ~/.clime/skills.yaml, migrating a legacy manifest on
// first read.
func LoadManifest() (*Manifest, error) {
	path, err := manifestPath()
	if err != nil {
		return nil, err
	}
	m, err := LoadManifestFrom(path)
	if errors.Is(err, errLegacyManifest) {
		if err := migrateLegacy(path); err != nil {
			return nil, fmt.Errorf("failed to migrate legacy skills manifest: %w", err)
		}
		return LoadManifestFrom(path)
	}
	return m, err
}

// errLegacyManifest signals the pre-versioning manifest format.
var errLegacyManifest = errors.New("legacy skills manifest")

func newEmptyManifest(path string) *Manifest {
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	return &Manifest{
		path: path,
		doc:  &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}},
		root: root,
	}
}

// LoadManifestFrom parses a repository-keyed manifest from path. A missing or
// empty file yields an empty manifest. It returns errLegacyManifest when the
// file still uses the legacy top-level skills/sources fields.
func LoadManifestFrom(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newEmptyManifest(path), nil
		}
		return nil, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return newEmptyManifest(path), nil
	}
	root := doc.Content[0]
	if root.Kind == yaml.ScalarNode && root.Tag == "!!null" {
		return newEmptyManifest(path), nil
	}
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("invalid skills manifest %s: top level must be a mapping of repositories", path)
	}

	m := &Manifest{path: path, doc: &doc, root: root}
	for i := 0; i+1 < len(root.Content); i += 2 {
		keyNode, valueNode := root.Content[i], root.Content[i+1]
		if keyNode.Value == "skills" || keyNode.Value == "sources" {
			return nil, errLegacyManifest
		}
		spec, err := decodeRepoSpec(keyNode, valueNode)
		if err != nil {
			return nil, fmt.Errorf("invalid skills manifest %s: %w", path, err)
		}
		m.Repos = append(m.Repos, spec)
	}
	return m, nil
}

func decodeRepoSpec(keyNode, valueNode *yaml.Node) (*RepoSpec, error) {
	id, err := ParseRepo(keyNode.Value)
	if err != nil {
		return nil, fmt.Errorf("repository key %q: %w", keyNode.Value, err)
	}
	if valueNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("repository %q: entry must be a mapping with version and skills", keyNode.Value)
	}
	spec := &RepoSpec{Key: keyNode.Value, ID: id, valueNode: valueNode}
	for i := 0; i+1 < len(valueNode.Content); i += 2 {
		k, v := valueNode.Content[i], valueNode.Content[i+1]
		switch k.Value {
		case "version":
			spec.Version = v.Value
		case "skills":
			if v.Kind != yaml.SequenceNode {
				return nil, fmt.Errorf("repository %q: skills must be a list", keyNode.Value)
			}
			for _, item := range v.Content {
				spec.Skills = append(spec.Skills, item.Value)
			}
		}
	}
	return spec, nil
}

var skillNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Validate statically checks the complete manifest: parseable unique
// repository keys, non-empty versions and skill lists, no duplicate skill
// within a repository, and no skill name claimed by two repositories. All
// problems are reported together.
func (m *Manifest) Validate() error {
	var errs []error
	seenRepo := make(map[string]string)
	seenSkill := make(map[string]string)
	for _, r := range m.Repos {
		canonical := r.ID.Canonical()
		if prev, ok := seenRepo[canonical]; ok {
			errs = append(errs, fmt.Errorf("repositories %q and %q identify the same repository %s", prev, r.Key, canonical))
		} else {
			seenRepo[canonical] = r.Key
		}
		if r.Version == "" {
			errs = append(errs, fmt.Errorf("repository %q has no version", r.Key))
		}
		if len(r.Skills) == 0 {
			errs = append(errs, fmt.Errorf("repository %q selects no skills", r.Key))
		}
		seenInRepo := make(map[string]bool)
		for _, name := range r.Skills {
			if !skillNameRe.MatchString(name) {
				errs = append(errs, fmt.Errorf("repository %q selects invalid skill name %q", r.Key, name))
				continue
			}
			if seenInRepo[name] {
				errs = append(errs, fmt.Errorf("repository %q lists skill %q twice", r.Key, name))
				continue
			}
			seenInRepo[name] = true
			if prev, ok := seenSkill[name]; ok {
				errs = append(errs, fmt.Errorf("skill %q is selected by both %q and %q; both targets install into <skills-root>/%s", name, prev, r.Key, name))
				continue
			}
			seenSkill[name] = r.Key
		}
	}
	return errors.Join(errs...)
}

// FindRepo returns the entry identifying the same repository, if any.
func (m *Manifest) FindRepo(id RepoID) *RepoSpec {
	for _, r := range m.Repos {
		if r.ID.Canonical() == id.Canonical() {
			return r
		}
	}
	return nil
}

// FindSkill returns the repository entry that selects the named skill.
func (m *Manifest) FindSkill(name string) *RepoSpec {
	for _, r := range m.Repos {
		if slices.Contains(r.Skills, name) {
			return r
		}
	}
	return nil
}

// SkillNames returns every selected skill name across all repositories.
func (m *Manifest) SkillNames() []string {
	var names []string
	for _, r := range m.Repos {
		names = append(names, r.Skills...)
	}
	sort.Strings(names)
	return names
}

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func (r *RepoSpec) findField(name string) *yaml.Node {
	for i := 0; i+1 < len(r.valueNode.Content); i += 2 {
		if r.valueNode.Content[i].Value == name {
			return r.valueNode.Content[i+1]
		}
	}
	return nil
}

// SetVersion updates the locked version, mutating only the version scalar so
// every comment and sibling node is preserved.
func (m *Manifest) SetVersion(r *RepoSpec, version string) {
	r.Version = version
	if node := r.findField("version"); node != nil {
		node.SetString(version)
		return
	}
	// Insert version before skills for readability.
	r.valueNode.Content = append([]*yaml.Node{scalarNode("version"), scalarNode(version)}, r.valueNode.Content...)
}

// AddSkills inserts skill names in sorted position within the existing list,
// leaving already-selected names and their nodes untouched.
func (m *Manifest) AddSkills(r *RepoSpec, names []string) {
	seq := r.findField("skills")
	if seq == nil {
		seq = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		r.valueNode.Content = append(r.valueNode.Content, scalarNode("skills"), seq)
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	for _, name := range sorted {
		if slices.Contains(r.Skills, name) {
			continue
		}
		idx := len(seq.Content)
		for i, item := range seq.Content {
			if item.Value > name {
				idx = i
				break
			}
		}
		seq.Content = append(seq.Content, nil)
		copy(seq.Content[idx+1:], seq.Content[idx:])
		seq.Content[idx] = scalarNode(name)
		r.Skills = append(r.Skills, "")
		copy(r.Skills[idx+1:], r.Skills[idx:])
		r.Skills[idx] = name
	}
}

// AddRepo appends a new repository entry with the given key spelling, locked
// version, and sorted skills.
func (m *Manifest) AddRepo(key, version string, skills []string) (*RepoSpec, error) {
	id, err := ParseRepo(key)
	if err != nil {
		return nil, err
	}
	sorted := append([]string(nil), skills...)
	sort.Strings(sorted)

	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, name := range sorted {
		seq.Content = append(seq.Content, scalarNode(name))
	}
	valueNode := &yaml.Node{
		Kind:    yaml.MappingNode,
		Tag:     "!!map",
		Content: []*yaml.Node{scalarNode("version"), scalarNode(version), scalarNode("skills"), seq},
	}
	m.root.Content = append(m.root.Content, scalarNode(key), valueNode)

	spec := &RepoSpec{Key: key, ID: id, Version: version, Skills: sorted, valueNode: valueNode}
	m.Repos = append(m.Repos, spec)
	return spec, nil
}

// RemoveSkill removes the named skill from whichever repository selects it.
// Removing the final skill also removes the repository entry. It returns the
// affected repository and whether the skill was found.
func (m *Manifest) RemoveSkill(name string) (*RepoSpec, bool) {
	r := m.FindSkill(name)
	if r == nil {
		return nil, false
	}
	for i, s := range r.Skills {
		if s == name {
			r.Skills = append(r.Skills[:i], r.Skills[i+1:]...)
			break
		}
	}
	if seq := r.findField("skills"); seq != nil {
		for i, item := range seq.Content {
			if item.Value == name {
				seq.Content = append(seq.Content[:i], seq.Content[i+1:]...)
				break
			}
		}
	}
	if len(r.Skills) == 0 {
		m.removeRepo(r)
	}
	return r, true
}

func (m *Manifest) removeRepo(r *RepoSpec) {
	for i := 0; i+1 < len(m.root.Content); i += 2 {
		if m.root.Content[i+1] == r.valueNode {
			m.root.Content = append(m.root.Content[:i], m.root.Content[i+2:]...)
			break
		}
	}
	for i, spec := range m.Repos {
		if spec == r {
			m.Repos = append(m.Repos[:i], m.Repos[i+1:]...)
			break
		}
	}
}

// Save writes the manifest through a temporary file and rename so a failed
// write never truncates the previous manifest.
func (m *Manifest) Save() error {
	path := m.path
	if path == "" {
		p, err := manifestPath()
		if err != nil {
			return err
		}
		path = p
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var buf bytes.Buffer
	if len(m.root.Content) > 0 {
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(m.doc); err != nil {
			return fmt.Errorf("failed to encode skills manifest: %w", err)
		}
		if err := enc.Close(); err != nil {
			return err
		}
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".skills-*.yaml")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := renameOp(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}
