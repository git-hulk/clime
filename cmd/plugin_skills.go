package cmd

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"

	uicli "github.com/alperdrsnn/clime"
	"github.com/git-hulk/clime/internal/plugin"
	"github.com/git-hulk/clime/internal/prompt"
	"github.com/git-hulk/clime/internal/skill"
)

const pluginSkillsOption = "Plugin Skills"

// pluginSkillInstaller is the function called after plugin installation to
// auto-install any skills the plugin provides. It's a variable for testing.
var pluginSkillInstaller = tryInstallPluginSkills

// pluginSkillEvents reports only successful installs; every failure stays
// silent so plugin installation is never blocked.
type pluginSkillEvents struct {
	skill.NopEvents
}

// pluginSkillSource pairs a plugin name with its skill source path/repo.
type pluginSkillSource struct {
	pluginName string
	source     string
}

func (pluginSkillEvents) SkillInstalled(_ skill.Verb, name string, targets []string) {
	terminal.Successf("Installed plugin skill %q to %s", name, strings.Join(targets, ", "))
}

// tryInstallPluginSkills invokes `clime-<name> skills` to discover a skill
// source from the plugin. If the plugin provides skills, they are automatically
// installed. Errors are silently ignored so plugin installation is never blocked.
func tryInstallPluginSkills(name string) {
	source := getPluginSkillSource(name)
	if source == "" {
		return
	}
	skillSource, err := skill.ParseSource(source)
	if err != nil {
		return
	}

	manager, err := skill.Open(pluginSkillEvents{})
	if err != nil {
		return
	}

	snapshot, catalog, err := manager.Fetch(skillSource)
	if err != nil {
		return
	}

	var entries []skill.Entry
	for _, entry := range catalog.Skills {
		if _, installed := manager.Manifest.GetSkill(entry.Name); !installed {
			entries = append(entries, entry)
		}
	}
	_, _ = manager.Install(snapshot, entries)
}

// getPluginSkillSource runs `clime-<name> skills` and returns the trimmed
// output. Returns an empty string if the plugin is not found, the subcommand
// fails, or the output is empty.
func getPluginSkillSource(name string) string {
	binPath, found := plugin.Find(name)
	if !found {
		return ""
	}
	output, err := exec.Command(binPath, "skills").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// discoverPluginSkillSources iterates all discovered plugins and returns
// those that support the `skills` subcommand with a valid source.
func discoverPluginSkillSources() []pluginSkillSource {
	plugins := plugin.Discover()
	var sources []pluginSkillSource
	for _, discoveredPlugin := range plugins {
		source := getPluginSkillSource(discoveredPlugin.Name)
		if source != "" {
			sources = append(sources, pluginSkillSource{
				pluginName: discoveredPlugin.Name,
				source:     source,
			})
		}
	}
	return sources
}

// installFromPluginSkills handles the "Plugin Skills" interactive flow.
// It scans all plugins for skill sources, presents available skills, and
// installs the user's selections.
func installFromPluginSkills(manifest *skill.Manifest) error {
	spinner := startSpinner("Scanning plugins for skills...")

	sources := discoverPluginSkillSources()
	if len(sources) == 0 {
		spinner.Error("No plugins with skills found")
		terminal.Info("None of the installed plugins support the \"skills\" subcommand.")
		return nil
	}

	manager, err := newSkillsManager(manifest)
	if err != nil {
		spinner.Error("Failed to prepare skill manager")
		return err
	}

	manager.Store.Progress = fetchProgress(spinner)
	// Collect skills from all plugin sources.
	type skillCandidate struct {
		entry       skill.Entry
		skillSource skill.Source
		label       string
	}
	var candidates []skillCandidate

	for _, pluginSource := range sources {
		skillSource, err := skill.ParseSource(pluginSource.source)
		if err != nil {
			continue
		}
		snapshot, catalog, err := manager.Fetch(skillSource)
		if err != nil {
			continue
		}
		for _, entry := range catalog.Skills {
			if _, installed := manifest.GetSkill(entry.Name); installed {
				continue
			}
			label := fmt.Sprintf("%s — %s", entry.Name, pluginSource.pluginName)
			if entry.Description != "" {
				label = fmt.Sprintf("%s — %s (%s)", entry.Name, uicli.TruncateString(entry.Description, 50), pluginSource.pluginName)
			}
			candidates = append(candidates, skillCandidate{
				entry:       entry,
				skillSource: skillSource.WithQuery(snapshot.Version),
				label:       label,
			})
		}
	}

	if len(candidates) == 0 {
		spinner.Success(fmt.Sprintf("Scanned %d plugin(s)", len(sources)))
		terminal.Info("All skills from plugins are already installed.")
		return nil
	}

	spinner.Success(fmt.Sprintf("Found %d skill(s) from %d plugin(s)", len(candidates), len(sources)))

	slices.SortStableFunc(candidates, func(left, right skillCandidate) int {
		return strings.Compare(left.entry.Name, right.entry.Name)
	})
	options := make([]string, len(candidates))
	for i, candidate := range candidates {
		options[i] = candidate.label
	}

	fmt.Println()
	selectedIndices, err := multiSelectPrompt(prompt.SelectConfig{
		Label:    "Select skills to install (space to toggle, enter to confirm)",
		Options:  options,
		PageSize: skillsPageSize,
	})
	if err != nil {
		return err
	}

	if len(selectedIndices) == 0 {
		terminal.Info("No skills selected.")
		return nil
	}

	// Group selected skills by source so each repository resolves once.
	type sourceSkills struct {
		skillSource skill.Source
		entries     []skill.Entry
	}
	sourceMap := make(map[string]*sourceSkills)
	for _, selectedIndex := range selectedIndices {
		candidate := candidates[selectedIndex]
		selectedSource, ok := sourceMap[candidate.skillSource.Repo]
		if !ok {
			selectedSource = &sourceSkills{skillSource: candidate.skillSource}
			sourceMap[candidate.skillSource.Repo] = selectedSource
		}
		selectedSource.entries = append(selectedSource.entries, candidate.entry)
	}

	fmt.Println()
	for _, selectedSource := range sourceMap {
		snapshot, err := manager.Store.Snapshot(selectedSource.skillSource)
		if err != nil {
			terminal.Errorf("Failed to prepare %q: %v", selectedSource.skillSource, err)
			continue
		}

		manifest.AddSource(selectedSource.skillSource)
		manifest.Save()

		// Per-skill failures are already reported through the progress events.
		_, _ = manager.Install(snapshot, selectedSource.entries)
	}

	return nil
}
