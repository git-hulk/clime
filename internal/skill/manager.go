package skill

import (
	"fmt"
	"path/filepath"
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
	SourceResolving(verb Verb, source Source)
	// SourceFailed fires when preparing a source fails, including an
	// update refused because the new catalog drops an installed skill.
	SourceFailed(verb Verb, source Source, err error)
	// SourceUpToDate fires when an update finds the source already at
	// the requested version.
	SourceUpToDate(source Source, version string)
	// SourceReady fires once a source is materialized and its skills are
	// about to be installed. version is empty for local sources.
	SourceReady(verb Verb, source Source, version string)
	// SkillInstalling and SkillInstalled/SkillFailed bracket one skill.
	SkillInstalling(verb Verb, name string, source Source)
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

func (verb Verb) String() string {
	switch verb {
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
	manifest, err := LoadManifest("")
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

func (manager *Manager) events() Events {
	if manager.Events == nil {
		return NopEvents{}
	}
	return manager.Events
}

// Fetch materializes a source and reads its catalog, for browsing before
// an install. With no query, it uses the manifest's locked version when
// available, otherwise latest. It reports no events; wrap it with
// caller-side progress.
func (manager *Manager) Fetch(source Source) (*Snapshot, *Catalog, error) {
	if source.Query == "" && !source.IsLocal() {
		if record, ok := manager.Manifest.GetSource(source); ok && record.Version != "" {
			source = source.WithQuery(record.Version)
		}
	}
	snapshot, err := manager.Store.Snapshot(source)
	if err != nil {
		return nil, nil, err
	}
	catalog, err := snapshot.Catalog()
	if err != nil {
		return nil, nil, err
	}
	return snapshot, catalog, nil
}

// Install installs the given catalog entries from a snapshot into every
// target and records them in the manifest. It continues past per-skill
// failures and returns how many skills succeeded. After successful
// installation, other cached versions of the source are removed.
func (manager *Manager) Install(snapshot *Snapshot, entries []Entry) (int, error) {
	return manager.install(VerbInstall, snapshot, entries)
}

// Update moves one source to the version its query resolves to (latest
// when it carries none, unless the saved version names a branch),
// re-installing its installed skills from the new catalog. The update is
// refused when the new catalog no longer lists an installed skill, so a skill
// is never removed implicitly. After successful installation, other cached
// versions of the source are removed. Returns how
// many skills changed; zero with a nil error means already up to date.
func (manager *Manager) Update(source Source) (int, error) {
	events := manager.events()
	installed := manager.Manifest.SkillsFrom(source)
	if len(installed) == 0 {
		return 0, fmt.Errorf("no skills installed from %s", source.Repo)
	}

	events.SourceResolving(VerbUpdate, source)
	current, _ := manager.Manifest.GetSource(source)
	var snapshot *Snapshot
	var err error
	if source.Query == "" && current.Version != "" && !fullSHAPattern.MatchString(current.Version) {
		snapshot, err = manager.Store.Snapshot(source.WithQuery(current.Version))
		if err != nil {
			events.SourceFailed(VerbUpdate, source, err)
			return 0, err
		}
	}
	// Keep following a saved branch or latest. Tags and commits update to latest.
	if snapshot == nil || snapshot.Version == snapshot.revision {
		snapshot, err = manager.Store.Snapshot(source)
	}
	if err != nil {
		events.SourceFailed(VerbUpdate, source, err)
		return 0, err
	}

	installedRevision, err := manager.Store.installedRevision(source)
	if err != nil {
		events.SourceFailed(VerbUpdate, source, err)
		return 0, err
	}
	if snapshot.revision != "" && snapshot.revision == installedRevision {
		if snapshot.Version != current.Version {
			manager.Manifest.SetSourceVersion(source, snapshot.Version)
			if err := manager.Manifest.Save(); err != nil {
				return 0, fmt.Errorf("failed to save source version: %w", err)
			}
		}
		events.SourceUpToDate(source, snapshot.Version)
		return 0, nil
	}

	catalog, err := snapshot.Catalog()
	if err != nil {
		events.SourceFailed(VerbUpdate, source, err)
		return 0, err
	}
	entries := make([]Entry, 0, len(installed))
	var missing []string
	for _, installedSkill := range installed {
		entry, ok := catalog.Find(installedSkill.Name)
		if !ok {
			missing = append(missing, installedSkill.Name)
			continue
		}
		entries = append(entries, entry)
	}
	if len(missing) > 0 {
		err := fmt.Errorf("%s at %s no longer provides %s; uninstall them first to update",
			source.Repo, DisplayVersion(snapshot.Version), strings.Join(missing, ", "))
		events.SourceFailed(VerbUpdate, source, err)
		return 0, err
	}

	events.SourceReady(VerbUpdate, source, snapshot.Version)
	return manager.install(VerbUpdate, snapshot, entries)
}

// Sync re-installs a source's skills at the saved version, following branch
// names to their current head and using catalog entry paths. It returns how
// many skills it re-installed. A source without a saved version uses latest.
// Branch names and latest are preserved and resolved remotely on each sync. After successful installation,
// other cached versions of the source are removed.
func (manager *Manager) Sync(source Source) (int, error) {
	events := manager.events()
	installed := manager.Manifest.SkillsFrom(source)
	if len(installed) == 0 {
		return 0, fmt.Errorf("no skills installed from %s", source.Repo)
	}

	locked := source
	if record, ok := manager.Manifest.GetSource(source); ok && record.Version != "" && !source.IsLocal() {
		locked = source.WithQuery(record.Version)
	}

	events.SourceResolving(VerbSync, locked)
	snapshot, err := manager.Store.Snapshot(locked)
	if err != nil {
		events.SourceFailed(VerbSync, locked, err)
		return 0, err
	}

	catalog, err := snapshot.Catalog()
	if err != nil {
		events.SourceFailed(VerbSync, locked, err)
		return 0, err
	}
	entries := make([]Entry, 0, len(installed))
	for _, installedSkill := range installed {
		entry, ok := catalog.Find(installedSkill.Name)
		if !ok {
			err := fmt.Errorf("%s no longer provides skill %q", source.Repo, installedSkill.Name)
			events.SourceFailed(VerbSync, locked, err)
			return 0, err
		}
		entries = append(entries, entry)
	}
	events.SourceReady(VerbSync, source, snapshot.Version)
	return manager.install(VerbSync, snapshot, entries)
}

// Uninstall removes one skill from every target and from the manifest,
// returning the names of the targets it was removed from.
func (manager *Manager) Uninstall(name string) ([]string, error) {
	if _, ok := manager.Manifest.GetSkill(name); !ok {
		return nil, fmt.Errorf("skill %q is not installed", name)
	}

	var removed []string
	for _, target := range manager.Targets {
		ok, err := target.Remove(name)
		if err != nil {
			return removed, err
		}
		if ok {
			removed = append(removed, target.Name)
		}
	}

	manager.Manifest.RemoveSkill(name)
	if err := manager.Manifest.Save(); err != nil {
		return removed, fmt.Errorf("skill removed but failed to update manifest: %w", err)
	}
	return removed, nil
}

// install installs each entry in turn, reporting failures as they happen,
// and returns how many succeeded. It prunes other snapshots only after
// every entry has been installed and saved to the manifest.
func (manager *Manager) install(verb Verb, snapshot *Snapshot, entries []Entry) (int, error) {
	failed := 0
	for _, entry := range entries {
		if err := manager.installEntry(verb, snapshot, entry); err != nil {
			manager.events().SkillFailed(verb, entry.Name, err)
			failed++
		}
	}
	if failed > 0 {
		return len(entries) - failed, fmt.Errorf("%d skill(s) failed", failed)
	}
	if len(entries) > 0 && snapshot.Version != "" && len(manager.Targets) > 0 {
		if err := manager.Store.recordInstalledRevision(snapshot.Source, snapshot.revision); err != nil {
			return len(entries), fmt.Errorf("skills installed but failed to record installed revision: %w", err)
		}
		if err := manager.Store.prune(snapshot.Source, snapshot.revision); err != nil {
			return len(entries), fmt.Errorf("skills installed but failed to remove old snapshots: %w", err)
		}
	}
	return len(entries), nil
}

// installEntry writes one skill from the snapshot into every target and
// records it in the manifest. The resolved version is recorded on the
// source; branch names and latest are preserved independently of the cache revision.
func (manager *Manager) installEntry(verb Verb, snapshot *Snapshot, entry Entry) error {
	events := manager.events()
	events.SkillInstalling(verb, entry.Name, snapshot.Source)

	if err := validateSkillName(entry.Name); err != nil {
		return err
	}
	path := entry.Path
	files, err := snapshot.SkillFiles(path)
	if err != nil && path != filepath.Join("skills", entry.Name) {
		path = filepath.Join("skills", entry.Name)
		files, err = snapshot.SkillFiles(path)
	}
	if err != nil {
		return fmt.Errorf("failed to %s skill %q: %w", verb, entry.Name, err)
	}
	if _, ok := files["SKILL.md"]; !ok {
		return fmt.Errorf("skill %q is missing required SKILL.md file", entry.Name)
	}

	if len(manager.Targets) == 0 {
		events.NoTargets()
		return nil
	}
	var targets []string
	for _, target := range manager.Targets {
		if err := target.Install(entry.Name, files); err != nil {
			return fmt.Errorf("failed to %s skill %q: %w", verb, entry.Name, err)
		}
		targets = append(targets, target.Name)
	}

	if snapshot.Version != "" {
		manager.Manifest.SetSourceVersion(snapshot.Source, snapshot.Version)
	}
	manager.Manifest.AddSkill(InstalledSkill{
		Name:   entry.Name,
		Source: snapshot.Source.Repo,
	})
	if err := manager.Manifest.Save(); err != nil {
		return fmt.Errorf("skill installed but failed to update manifest: %w", err)
	}

	events.SkillInstalled(verb, entry.Name, targets)
	return nil
}
