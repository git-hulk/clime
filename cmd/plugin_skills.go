package cmd

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

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
// source from the plugin. If the plugin provides skills, they are automatically
// installed. Errors are silently ignored so plugin installation is never blocked.
func tryInstallPluginSkills(name string) {
	source := getPluginSkillSource(name)
	if source == "" {
		return
	}

	manifest, err := skill.LoadLegacyManifest()
	if err != nil {
		return
	}

	repoManifest, err := skill.FetchRepoManifest(source)
	if err != nil {
		return
	}

	dir, cleanup, err := skill.PrepareRepoDir(source)
	if err != nil {
		return
	}
	defer cleanup()

	for _, entry := range repoManifest.Skills {
		if _, installed := manifest.GetSkill(entry.Name); installed {
			continue
		}
		targets, err := skill.InstallFromDir(entry.Name, dir, entry.Path)
		if err != nil || len(targets) == 0 {
			continue
		}
		manifest.AddSkill(skill.InstalledSkill{
			Name:        entry.Name,
			Description: entry.Description,
			Source:      source,
			Path:        entry.Path,
			InstalledAt: time.Now(),
		})
		manifest.Save()
		terminal.Successf("Installed plugin skill %q to %s", entry.Name, strings.Join(targets, ", "))
	}
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

// pluginSkillSource pairs a plugin name with its skill source path/repo.
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
// installs the user's selections.
func installFromPluginSkills(manifest *skill.LegacyManifest) error {
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

	// Collect skills from all plugin sources.
	type skillCandidate struct {
		entry  skill.SkillEntry
		source string
		label  string
	}
	var candidates []skillCandidate
	var dirs []struct {
		dir     string
		cleanup func()
	}

	for _, ps := range sources {
		repoManifest, err := skill.FetchRepoManifest(ps.source)
		if err != nil {
			continue
		}
		for _, entry := range repoManifest.Skills {
			if _, installed := manifest.GetSkill(entry.Name); installed {
				continue
			}
			label := fmt.Sprintf("%s — %s", entry.Name, ps.pluginName)
			if entry.Description != "" {
				label = fmt.Sprintf("%s — %s (%s)", entry.Name, uicli.TruncateString(entry.Description, 50), ps.pluginName)
			}
			candidates = append(candidates, skillCandidate{
				entry:  entry,
				source: ps.source,
				label:  label,
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

	// Group selected skills by source to prepare repos efficiently.
	type sourceSkills struct {
		source  string
		entries []skill.SkillEntry
	}
	sourceMap := make(map[string]*sourceSkills)
	for _, idx := range selectedIdxs {
		c := candidates[idx]
		ss, ok := sourceMap[c.source]
		if !ok {
			ss = &sourceSkills{source: c.source}
			sourceMap[c.source] = ss
		}
		ss.entries = append(ss.entries, c.entry)
	}

	fmt.Println()
	for _, ss := range sourceMap {
		dir, cleanup, err := skill.PrepareRepoDir(ss.source)
		if err != nil {
			terminal.Errorf("Failed to prepare %q: %v", ss.source, err)
			continue
		}
		dirs = append(dirs, struct {
			dir     string
			cleanup func()
		}{dir, cleanup})

		manifest.AddSource(ss.source)
		manifest.Save()

		for _, entry := range ss.entries {
			if err := installSkillEntry(manifest, &entry, ss.source, dir); err != nil {
				terminal.Errorf("Failed to install %q: %v", entry.Name, err)
			}
		}
	}

	for _, d := range dirs {
		d.cleanup()
	}

	return nil
}
