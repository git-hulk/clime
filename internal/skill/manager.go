package skill

import (
	"fmt"
	"strings"
)

// Verb names a skill operation, for progress reporting and error text.
type Verb int

const (
	VerbInstall Verb = iota
	VerbUpdate
	VerbSync
)

// Events receives progress notifications from Manager operations so the
// caller can render spinners or logs. Implementations run synchronously;
// a nil Manager.Events is valid and reports nothing.
type Events interface {
	// SourceResolving fires before a source is resolved and materialized.
	SourceResolving(verb Verb, src Source)
	// SourceFailed fires when preparing a source fails, including an
	// update refused because the new catalog drops an installed skill.
	SourceFailed(verb Verb, src Source, err error)
	// SourceUpToDate fires when an update finds the source already at
	// the requested version.
	SourceUpToDate(src Source, version string)
	// SourceReady fires once a source is materialized and its skills are
	// about to be installed. version is empty for local sources.
	SourceReady(verb Verb, src Source, version string)
	// SkillInstalling and SkillInstalled/SkillFailed bracket one skill.
	SkillInstalling(verb Verb, name string, src Source)
	SkillInstalled(verb Verb, name string, targets []string)
	SkillFailed(verb Verb, name string, err error)
	// NoTargets fires when no agent directory exists to install into.
	NoTargets()
}

// NopEvents implements Events with no-ops; embed it to implement only
// the notifications you care about.
type NopEvents struct{}

// Manager composes the manifest, the source store, and the detected
// targets into the skill verbs: install, update, sync, and uninstall.
type Manager struct {
	Manifest *Manifest
	Store    *Store
	Targets  []Target
	Events   Events
}

func (v Verb) String() string {
	switch v {
	case VerbUpdate:
		return "update"
	case VerbSync:
		return "sync"
	default:
		return "install"
	}
}

func (NopEvents) SourceResolving(Verb, Source)          {}
func (NopEvents) SourceFailed(Verb, Source, error)      {}
func (NopEvents) SourceUpToDate(Source, string)         {}
func (NopEvents) SourceReady(Verb, Source, string)      {}
func (NopEvents) SkillInstalling(Verb, string, Source)  {}
func (NopEvents) SkillInstalled(Verb, string, []string) {}
func (NopEvents) SkillFailed(Verb, string, error)       {}
func (NopEvents) NoTargets()                            {}

// Open loads the manifest, opens the source store, and detects targets.
func Open(events Events) (*Manager, error) {
	manifest, err := LoadManifest()
	if err != nil {
		return nil, fmt.Errorf("failed to load skills manifest: %w", err)
	}
	store, err := OpenStore()
	if err != nil {
		return nil, err
	}
	targets, err := DetectTargets()
	if err != nil {
		return nil, err
	}
	return &Manager{Manifest: manifest, Store: store, Targets: targets, Events: events}, nil
}

func (m *Manager) events() Events {
	if m.Events == nil {
		return NopEvents{}
	}
	return m.Events
}

// Fetch materializes a source and reads its catalog, for browsing before
// an install. It reports no events; wrap it with caller-side progress.
func (m *Manager) Fetch(src Source) (*Snapshot, *Catalog, error) {
	snap, err := m.Store.Snapshot(src)
	if err != nil {
		return nil, nil, err
	}
	catalog, err := snap.Catalog()
	if err != nil {
		return nil, nil, err
	}
	return snap, catalog, nil
}

// Install installs the given catalog entries from a snapshot into every
// target and records them in the manifest. It continues past per-skill
// failures and returns how many skills succeeded.
func (m *Manager) Install(snap *Snapshot, entries []Entry) (int, error) {
	return m.install(VerbInstall, snap, entries)
}

// Update moves one source to the version its query resolves to (latest
// when it carries none), re-installing its installed skills from the new
// catalog. The update is refused when the new catalog no longer lists an
// installed skill, so a skill is never removed implicitly. Returns how
// many skills changed; zero with a nil error means already up to date.
func (m *Manager) Update(src Source) (int, error) {
	ev := m.events()
	installed := m.Manifest.SkillsFrom(src)
	if len(installed) == 0 {
		return 0, fmt.Errorf("no skills installed from %s", src.Repo)
	}

	ev.SourceResolving(VerbUpdate, src)
	snap, err := m.Store.Snapshot(src)
	if err != nil {
		ev.SourceFailed(VerbUpdate, src, err)
		return 0, err
	}

	if current, _ := m.Manifest.GetSource(src); snap.Version != "" && snap.Version == current.Version {
		ev.SourceUpToDate(src, snap.Version)
		return 0, nil
	}

	catalog, err := snap.Catalog()
	if err != nil {
		ev.SourceFailed(VerbUpdate, src, err)
		return 0, err
	}
	entries := make([]Entry, 0, len(installed))
	var missing []string
	for _, s := range installed {
		entry, ok := catalog.Find(s.Name)
		if !ok {
			missing = append(missing, s.Name)
			continue
		}
		entries = append(entries, entry)
	}
	if len(missing) > 0 {
		err := fmt.Errorf("%s at %s no longer provides %s; uninstall them first to update",
			src.Repo, DisplayVersion(snap.Version), strings.Join(missing, ", "))
		ev.SourceFailed(VerbUpdate, src, err)
		return 0, err
	}

	ev.SourceReady(VerbUpdate, src, snap.Version)
	return m.install(VerbUpdate, snap, entries)
}

// Sync re-installs a source's skills at the version locked in the
// manifest, using the stored skill paths, and returns how many skills it
// re-installed. A source without a locked version is installed at latest
// and the resolved version is recorded.
func (m *Manager) Sync(src Source) (int, error) {
	ev := m.events()
	installed := m.Manifest.SkillsFrom(src)
	if len(installed) == 0 {
		return 0, fmt.Errorf("no skills installed from %s", src.Repo)
	}

	locked := src
	if record, ok := m.Manifest.GetSource(src); ok && record.Version != "" && !src.IsLocal() {
		locked = src.WithQuery(record.Version)
	}

	ev.SourceResolving(VerbSync, locked)
	snap, err := m.Store.Snapshot(locked)
	if err != nil {
		ev.SourceFailed(VerbSync, locked, err)
		return 0, err
	}
	ev.SourceReady(VerbSync, src, snap.Version)

	entries := make([]Entry, 0, len(installed))
	for _, s := range installed {
		entries = append(entries, Entry{Name: s.Name, Path: s.Path})
	}
	return m.install(VerbSync, snap, entries)
}

// Uninstall removes one skill from every target and from the manifest,
// returning the names of the targets it was removed from.
func (m *Manager) Uninstall(name string) ([]string, error) {
	if _, ok := m.Manifest.GetSkill(name); !ok {
		return nil, fmt.Errorf("skill %q is not installed", name)
	}

	var removed []string
	for _, t := range m.Targets {
		ok, err := t.Remove(name)
		if err != nil {
			return removed, err
		}
		if ok {
			removed = append(removed, t.Name)
		}
	}

	m.Manifest.RemoveSkill(name)
	if err := m.Manifest.Save(); err != nil {
		return removed, fmt.Errorf("skill removed but failed to update manifest: %w", err)
	}
	return removed, nil
}

// install installs each entry in turn, reporting failures as they happen,
// and returns how many succeeded.
func (m *Manager) install(verb Verb, snap *Snapshot, entries []Entry) (int, error) {
	failed := 0
	for _, entry := range entries {
		if err := m.installEntry(verb, snap, entry); err != nil {
			m.events().SkillFailed(verb, entry.Name, err)
			failed++
		}
	}
	if failed > 0 {
		return len(entries) - failed, fmt.Errorf("%d skill(s) failed", failed)
	}
	return len(entries), nil
}

// installEntry writes one skill from the snapshot into every target and
// records it in the manifest. The resolved version is recorded on the
// source, so a floating query is never persisted.
func (m *Manager) installEntry(verb Verb, snap *Snapshot, entry Entry) error {
	ev := m.events()
	ev.SkillInstalling(verb, entry.Name, snap.Source)

	files, err := snap.SkillFiles(entry.Path)
	if err != nil {
		return fmt.Errorf("failed to %s skill %q: %w", verb, entry.Name, err)
	}
	if _, ok := files["SKILL.md"]; !ok {
		return fmt.Errorf("skill %q is missing required SKILL.md file", entry.Name)
	}

	if len(m.Targets) == 0 {
		ev.NoTargets()
		return nil
	}
	var targets []string
	for _, t := range m.Targets {
		if err := t.Install(entry.Name, files); err != nil {
			return fmt.Errorf("failed to %s skill %q: %w", verb, entry.Name, err)
		}
		targets = append(targets, t.Name)
	}

	if snap.Version != "" {
		m.Manifest.SetSourceVersion(snap.Source, snap.Version)
	}
	m.Manifest.AddSkill(InstalledSkill{
		Name:   entry.Name,
		Source: snap.Source.Repo,
		Path:   entry.Path,
	})
	if err := m.Manifest.Save(); err != nil {
		return fmt.Errorf("skill installed but failed to update manifest: %w", err)
	}

	ev.SkillInstalled(verb, entry.Name, targets)
	return nil
}
