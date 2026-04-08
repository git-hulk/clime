package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	uicli "github.com/alperdrsnn/clime"
	"github.com/git-hulk/clime/internal/prompt"
	"github.com/git-hulk/clime/internal/skill"
	"github.com/spf13/cobra"
)

func init() {
	skillsCmd.AddCommand(skillsListCmd)
	skillsCmd.AddCommand(skillsInstallCmd)
	skillsCmd.AddCommand(skillsUninstallCmd)
	rootCmd.AddCommand(skillsCmd)
}

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Manage AI agent skills from GitHub repositories or local paths",
	Long: "Install skills from GitHub repositories or local paths into ~/.claude/skills and ~/.codex/skills " +
		"for use with Claude Code and Codex.",
	RunE: skillsListCmd.RunE,
}

var skillsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed skills and their sources",
	RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := skill.LoadManifest()
		if err != nil {
			return fmt.Errorf("failed to load skills manifest: %w", err)
		}

		if len(manifest.Skills) == 0 {
			terminal.Warning("No skills installed.")
			terminal.Info("Install skills with: clime skills install")
			return nil
		}

		fmt.Println()
		fmt.Printf("  %s %s\n\n",
			uicli.BoldColor.Sprint("Installed Skills"),
			uicli.DimColor.Sprintf("(%d total)", len(manifest.Skills)),
		)

		headers := []string{"NAME", "DESCRIPTION", "SOURCE"}
		const descWidth = 50
		var rows [][]string
		for _, s := range manifest.Skills {
			desc := s.Description
			if desc == "" {
				desc = "—"
			}
			desc = uicli.TruncateString(desc, descWidth)
			rows = append(rows, []string{s.Name, desc, s.Source})
		}

		colWidths := make([]int, len(headers))
		for i, h := range headers {
			colWidths[i] = len(h)
		}
		for _, row := range rows {
			for i, cell := range row {
				if len(cell) > colWidths[i] {
					colWidths[i] = len(cell)
				}
			}
		}

		const gap = 2
		const indent = "  "

		fmt.Print(indent)
		for i, h := range headers {
			if i > 0 {
				fmt.Print(strings.Repeat(" ", gap))
			}
			fmt.Print(uicli.BoldColor.Sprintf("%-*s", colWidths[i], h))
		}
		fmt.Println()

		fmt.Print(indent)
		for i, w := range colWidths {
			if i > 0 {
				fmt.Print(strings.Repeat(" ", gap))
			}
			fmt.Print(strings.Repeat("-", w))
		}
		fmt.Println()

		for _, row := range rows {
			fmt.Print(indent)
			for i, cell := range row {
				if i > 0 {
					fmt.Print(strings.Repeat(" ", gap))
				}
				fmt.Printf("%-*s", colWidths[i], cell)
			}
			fmt.Println()
		}

		return nil
	},
}

type sourceAction int

const (
	actionBrowseInstall sourceAction = iota
	actionRemoveSource
	actionUpdate
)

const newRepoOption = "Enter a new repository..."

var (
	selectPrompt       = prompt.Select
	multiSelectPrompt  = prompt.MultiSelect
	inputPrompt        = prompt.Input
	skillsActionRunner = runSkillsSourceAction
)

var skillsInstallCmd = &cobra.Command{
	Use:   "install [owner/repo|path]",
	Short: "Install skills from a GitHub repository or local path",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := skill.LoadManifest()
		if err != nil {
			return fmt.Errorf("failed to load skills manifest: %w", err)
		}

		if len(args) > 0 {
			return runSkillsSourceAction(manifest, args[0], actionBrowseInstall)
		}

		return runInteractiveSkillsInstall(manifest)
	},
}

func runInteractiveSkillsInstall(manifest *skill.Manifest) error {
	sources := uniqueSkillSources(manifest)
	if len(sources) == 0 {
		fmt.Println()
		repo, err := inputPrompt("Enter repository (owner/repo)")
		if err != nil {
			return err
		}
		return skillsActionRunner(manifest, repo, actionBrowseInstall)
	}

	options := append(sources, newRepoOption)
	for {
		fmt.Println()
		idx, err := selectPrompt(prompt.SelectConfig{
			Label:   "Select a skill source",
			Options: options,
		})
		if err != nil {
			if errors.Is(err, prompt.ErrBack) {
				continue
			}
			return err
		}

		if options[idx] == newRepoOption {
			repo, err := inputPrompt("Enter repository (owner/repo)")
			if err != nil {
				return err
			}
			return skillsActionRunner(manifest, repo, actionBrowseInstall)
		}

		repo := options[idx]
		for {
			action, err := pickSourceAction(repo)
			if err != nil {
				if errors.Is(err, prompt.ErrBack) {
					break
				}
				return err
			}

			err = skillsActionRunner(manifest, repo, action)
			if errors.Is(err, prompt.ErrBack) {
				continue
			}
			return err
		}
	}
}

func uniqueSkillSources(manifest *skill.Manifest) []string {
	// Collect unique sources from installed skills, preserving order.
	seen := make(map[string]bool)
	var sources []string
	for _, s := range manifest.Skills {
		if s.Source != "" && !seen[s.Source] {
			seen[s.Source] = true
			sources = append(sources, s.Source)
		}
	}

	return sources
}

func runSkillsSourceAction(manifest *skill.Manifest, repo string, action sourceAction) error {
	if err := validateSkillRepoSource(repo); err != nil {
		return err
	}

	switch action {
	case actionRemoveSource:
		return removeSource(manifest, repo)
	case actionUpdate:
		return updateSource(manifest, repo)
	default:
		return installFromRepo(manifest, repo)
	}
}

func validateSkillRepoSource(repo string) error {
	if repo == "" {
		return fmt.Errorf("invalid repo format: expected owner/repo or local path, got %q", repo)
	}
	if _, ok, err := skill.LocalRepoDir(repo); err != nil {
		return err
	} else if ok {
		return nil
	}
	if !strings.Contains(repo, "/") {
		return fmt.Errorf("invalid repo format: expected owner/repo or local path, got %q", repo)
	}
	return nil
}

func pickSourceAction(repo string) (sourceAction, error) {
	options := []string{
		"Browse and install skills",
		"Update installed skills",
		"Remove source and its installed skills",
	}

	fmt.Println()
	idx, err := selectPrompt(prompt.SelectConfig{
		Label:   fmt.Sprintf("Action for %s", repo),
		Options: options,
	})
	if err != nil {
		return 0, err
	}

	switch idx {
	case 1:
		return actionUpdate, nil
	case 2:
		return actionRemoveSource, nil
	default:
		return actionBrowseInstall, nil
	}
}

// removeSource uninstalls all skills from the given source and removes them from the manifest.
func removeSource(manifest *skill.Manifest, repo string) error {
	var names []string
	for _, s := range manifest.Skills {
		if s.Source == repo {
			names = append(names, s.Name)
		}
	}

	if len(names) == 0 {
		terminal.Warningf("No skills installed from %s.", repo)
		return nil
	}

	fmt.Println()
	for _, name := range names {
		if err := uninstallByName(manifest, name); err != nil {
			terminal.Errorf("Failed to uninstall %q: %v", name, err)
		}
	}
	return nil
}

// updateSource re-installs all skills from the given source with the latest files.
func updateSource(manifest *skill.Manifest, repo string) error {
	var installed []skill.InstalledSkill
	for _, s := range manifest.Skills {
		if s.Source == repo {
			installed = append(installed, s)
		}
	}

	if len(installed) == 0 {
		terminal.Warningf("No skills installed from %s.", repo)
		return nil
	}

	// Resolve the repo once and reuse it for all skill updates.
	dir, cleanup, err := skill.PrepareRepoDir(repo)
	if err != nil {
		return err
	}
	defer cleanup()

	fmt.Println()
	for _, s := range installed {
		entry := &skill.SkillEntry{
			Name:        s.Name,
			Description: s.Description,
			Path:        s.Path,
		}
		if err := installSkillEntry(manifest, entry, repo, dir); err != nil {
			terminal.Errorf("Failed to update %q: %v", s.Name, err)
		}
	}
	return nil
}

func installSkillEntry(manifest *skill.Manifest, entry *skill.SkillEntry, repo string, localDir string) error {
	spinner := uicli.NewSpinner().
		WithStyle(uicli.SpinnerDots).
		WithColor(uicli.CyanColor).
		WithMessage(fmt.Sprintf("Installing skill %q from %s...", entry.Name, repo)).
		Start()

	var targets []string
	var err error
	if localDir != "" {
		targets, err = skill.InstallFromDir(entry.Name, localDir, entry.Path)
	} else {
		targets, err = skill.Install(entry.Name, repo, entry.Path)
	}
	if err != nil {
		spinner.Error(fmt.Sprintf("Failed to install skill %q", entry.Name))
		return fmt.Errorf("failed to install skill %q: %w", entry.Name, err)
	}

	if len(targets) == 0 {
		spinner.Stop()
		terminal.Warning("No skill directories were installed. Neither ~/.claude nor ~/.codex was found.")
		return nil
	}

	manifest.AddSkill(skill.InstalledSkill{
		Name:        entry.Name,
		Description: entry.Description,
		Source:      repo,
		Path:        entry.Path,
		InstalledAt: time.Now(),
	})
	if err := manifest.Save(); err != nil {
		return fmt.Errorf("skill installed but failed to update manifest: %w", err)
	}

	spinner.Success(fmt.Sprintf("Installed skill %q to %s", entry.Name, strings.Join(targets, ", ")))
	return nil
}

// installFromRepo fetches skills from a repo and lets the user pick which to install.
func installFromRepo(manifest *skill.Manifest, repo string) error {
	spinner := uicli.NewSpinner().
		WithStyle(uicli.SpinnerDots).
		WithColor(uicli.CyanColor).
		WithMessage(fmt.Sprintf("Fetching skills from %q...", repo)).
		Start()

	repoManifest, err := skill.FetchRepoManifest(repo)
	if err != nil {
		spinner.Error(fmt.Sprintf("Failed to fetch %q", repo))
		return fmt.Errorf("failed to fetch skills: %w", err)
	}

	if len(repoManifest.Skills) == 0 {
		spinner.Error(fmt.Sprintf("No skills found in %q", repo))
		return fmt.Errorf("repository %q has no skills defined", repo)
	}

	spinner.Success(fmt.Sprintf("Found %d skill(s) in %q", len(repoManifest.Skills), repo))

	// Filter out already-installed skills.
	type candidate struct {
		entry skill.SkillEntry
		label string
	}
	var candidates []candidate
	for _, s := range repoManifest.Skills {
		if _, installed := manifest.GetSkill(s.Name); installed {
			continue
		}
		label := s.Name
		if s.Description != "" {
			label = fmt.Sprintf("%s — %s", s.Name, uicli.TruncateString(s.Description, 60))
		}
		candidates = append(candidates, candidate{entry: s, label: label})
	}

	if len(candidates) == 0 {
		terminal.Info("All skills from this repository are already installed.")
		return nil
	}

	// Multi-select skills to install.
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

	// Resolve the repo once and reuse it for all skill installations.
	dir, cleanup, err := skill.PrepareRepoDir(repo)
	if err != nil {
		return err
	}
	defer cleanup()

	// Install each selected skill.
	fmt.Println()
	for _, idx := range selectedIdxs {
		entry := candidates[idx].entry
		if err := installSkillEntry(manifest, &entry, repo, dir); err != nil {
			terminal.Errorf("Failed to install %q: %v", entry.Name, err)
		}
	}

	return nil
}

var skillsUninstallCmd = &cobra.Command{
	Use:   "uninstall [skill-name]",
	Short: "Uninstall a previously installed skill",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := skill.LoadManifest()
		if err != nil {
			return fmt.Errorf("failed to load skills manifest: %w", err)
		}

		if len(args) == 0 {
			return interactiveUninstall(manifest)
		}

		return uninstallByName(manifest, args[0])
	},
}

func uninstallByName(manifest *skill.Manifest, name string) error {
	if _, exists := manifest.GetSkill(name); !exists {
		return fmt.Errorf("skill %q is not installed", name)
	}

	spinner := uicli.NewSpinner().
		WithStyle(uicli.SpinnerDots).
		WithColor(uicli.CyanColor).
		WithMessage(fmt.Sprintf("Removing skill %q...", name)).
		Start()

	targets, err := skill.Uninstall(name)
	if err != nil {
		spinner.Error(fmt.Sprintf("Failed to remove skill %q", name))
		return fmt.Errorf("failed to remove skill %q: %w", name, err)
	}

	manifest.RemoveSkill(name)
	if err := manifest.Save(); err != nil {
		return fmt.Errorf("skill removed but failed to update manifest: %w", err)
	}

	spinner.Success(fmt.Sprintf("Removed skill %q from %s", name, strings.Join(targets, ", ")))
	return nil
}

func interactiveUninstall(manifest *skill.Manifest) error {
	if len(manifest.Skills) == 0 {
		terminal.Warning("No skills installed.")
		return nil
	}

	options := make([]string, len(manifest.Skills))
	for i, s := range manifest.Skills {
		label := s.Name
		if s.Description != "" {
			label = fmt.Sprintf("%s — %s", s.Name, uicli.TruncateString(s.Description, 60))
		}
		options[i] = label
	}

	for {
		fmt.Println()
		selectedIdxs, err := multiSelectPrompt(prompt.SelectConfig{
			Label:   "Select skills to uninstall (space to toggle, enter to confirm)",
			Options: options,
		})
		if err != nil {
			if errors.Is(err, prompt.ErrBack) {
				continue
			}
			return err
		}

		if len(selectedIdxs) == 0 {
			terminal.Info("No skills selected.")
			return nil
		}

		// Collect names before uninstalling, since uninstallByName modifies manifest.Skills.
		names := make([]string, len(selectedIdxs))
		for i, idx := range selectedIdxs {
			names[i] = manifest.Skills[idx].Name
		}

		fmt.Println()
		for _, name := range names {
			if err := uninstallByName(manifest, name); err != nil {
				terminal.Errorf("Failed to uninstall %q: %v", name, err)
			}
		}

		return nil
	}
}
