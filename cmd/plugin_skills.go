package cmd

import (
	"fmt"
	"os/exec"
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

// tryInstallPluginSkills invokes `clime-<name> skills` to discover a skill
// source from the plugin. If the plugin provides skills, they are installed
// at the repository's latest version. Errors are silently ignored so plugin
// installation is never blocked.
func tryInstallPluginSkills(name string) {
	source := getPluginSkillSource(name)
	if source == "" {
		return
	}

	id, err := skill.ParseRepo(source)
	if err != nil {
		return
	}
	manifest, err := skill.LoadManifest()
	if err != nil {
		return
	}

	version := ""
	if r := manifest.FindRepo(id); r != nil {
		version = r.Version
	} else if version, err = skill.ResolveVersion(id, ""); err != nil {
		return
	}
	snapDir, err := skill.EnsureSnapshot(id, version)
	if err != nil {
		return
	}
	catalog, err := skill.ReadCatalog(snapDir)
	if err != nil {
		return
	}

	var names []string
	for _, entry := range catalog.Skills {
		if manifest.FindSkill(entry.Name) == nil {
			names = append(names, entry.Name)
		}
	}
	if len(names) == 0 {
		return
	}

	key := id.DisplayKey()
	if r := manifest.FindRepo(id); r != nil {
		key = r.Key
	}
	if err := applySelection(manifest, id, key, version, names); err != nil {
		return
	}
	terminal.Successf("Installed plugin skill(s) %s from %s", strings.Join(names, ", "), key)
}

// getPluginSkillSource runs `clime-<name> skills` and returns the trimmed
// output. Returns an empty string if the plugin is not found, the subcommand
// fails, or the output is empty.
func getPluginSkillSource(name string) string {
	binPath, found := plugin.Find(name)
	if !found {
		return ""
	}
	out, err := exec.Command(binPath, "skills").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// pluginSkillSource pairs a plugin name with its skill source repository.
type pluginSkillSource struct {
	pluginName string
	source     string
}

// discoverPluginSkillSources iterates all discovered plugins and returns
// those that support the `skills` subcommand with a valid source.
func discoverPluginSkillSources() []pluginSkillSource {
	plugins := plugin.Discover()
	var sources []pluginSkillSource
	for _, p := range plugins {
		source := getPluginSkillSource(p.Name)
		if source != "" {
			sources = append(sources, pluginSkillSource{
				pluginName: p.Name,
				source:     source,
			})
		}
	}
	return sources
}

// installFromPluginSkills handles the "Plugin Skills" interactive flow.
// It scans all plugins for skill sources, presents available skills, and
// installs the user's selections at each repository's resolved version.
func installFromPluginSkills(manifest *skill.Manifest) error {
	spinner := uicli.NewSpinner().
		WithStyle(uicli.SpinnerDots).
		WithColor(uicli.CyanColor).
		WithMessage("Scanning plugins for skills...").
		Start()

	sources := discoverPluginSkillSources()
	if len(sources) == 0 {
		spinner.Error("No plugins with skills found")
		terminal.Info("None of the installed plugins support the \"skills\" subcommand.")
		return nil
	}

	// Collect installable skills from all plugin sources.
	type pluginRepo struct {
		id      skill.RepoID
		version string
	}
	type skillCandidate struct {
		name  string
		repo  pluginRepo
		label string
	}
	var candidates []skillCandidate

	for _, ps := range sources {
		id, err := skill.ParseRepo(ps.source)
		if err != nil {
			continue
		}
		version := ""
		if r := manifest.FindRepo(id); r != nil {
			version = r.Version
		} else if version, err = skill.ResolveVersion(id, ""); err != nil {
			continue
		}
		snapDir, err := skill.EnsureSnapshot(id, version)
		if err != nil {
			continue
		}
		catalog, err := skill.ReadCatalog(snapDir)
		if err != nil {
			continue
		}
		for _, entry := range catalog.Skills {
			if manifest.FindSkill(entry.Name) != nil {
				continue
			}
			label := fmt.Sprintf("%s — %s", entry.Name, ps.pluginName)
			if entry.Description != "" {
				label = fmt.Sprintf("%s — %s (%s)", entry.Name, uicli.TruncateString(entry.Description, 50), ps.pluginName)
			}
			candidates = append(candidates, skillCandidate{
				name:  entry.Name,
				repo:  pluginRepo{id: id, version: version},
				label: label,
			})
		}
	}

	if len(candidates) == 0 {
		spinner.Success(fmt.Sprintf("Scanned %d plugin(s)", len(sources)))
		terminal.Info("All skills from plugins are already installed.")
		return nil
	}

	spinner.Success(fmt.Sprintf("Found %d skill(s) from %d plugin(s)", len(candidates), len(sources)))

	options := make([]string, len(candidates))
	for i, c := range candidates {
		options[i] = c.label
	}

	fmt.Println()
	selectedIdxs, err := multiSelectPrompt(prompt.SelectConfig{
		Label:   "Select skills to install (space to toggle, enter to confirm)",
		Options: options,
	})
	if err != nil {
		return err
	}

	if len(selectedIdxs) == 0 {
		terminal.Info("No skills selected.")
		return nil
	}

	// Group selected skills by repository so each is applied once.
	type repoSelection struct {
		repo  pluginRepo
		names []string
	}
	selections := make(map[string]*repoSelection)
	var order []string
	for _, idx := range selectedIdxs {
		c := candidates[idx]
		canonical := c.repo.id.Canonical()
		sel, ok := selections[canonical]
		if !ok {
			sel = &repoSelection{repo: c.repo}
			selections[canonical] = sel
			order = append(order, canonical)
		}
		sel.names = append(sel.names, c.name)
	}

	fmt.Println()
	for _, canonical := range order {
		sel := selections[canonical]
		key := sel.repo.id.DisplayKey()
		if r := manifest.FindRepo(sel.repo.id); r != nil {
			key = r.Key
		}
		if err := applySelection(manifest, sel.repo.id, key, sel.repo.version, sel.names); err != nil {
			terminal.Errorf("Failed to install from %s: %v", key, err)
		}
	}
	return nil
}
