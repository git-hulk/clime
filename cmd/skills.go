package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	uicli "github.com/alperdrsnn/clime"
	"github.com/git-hulk/clime/internal/prompt"
	"github.com/git-hulk/clime/internal/skill"
	"github.com/spf13/cobra"
)

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

	skillsInstallForce bool
)

// verbUI maps a skill verb to the wording the progress output uses.
var verbUI = map[skill.Verb]struct{ resolving, present, past, prep string }{
	skill.VerbInstall: {"Preparing", "Installing", "Installed", "at"},
	skill.VerbUpdate:  {"Resolving", "Updating", "Updated", "to"},
	skill.VerbSync:    {"Preparing", "Syncing", "Synced", "at"},
}

// skillsUI renders Manager progress events as terminal spinners.
type skillsUI struct {
	spinner *uicli.Spinner
}

type installCandidate struct {
	entry skill.Entry
	label string
}

func init() {
	skillsInstallCmd.Flags().BoolVarP(&skillsInstallForce, "force", "f", false,
		"when installing from a repo, also (re)install skills that are already installed and overwrite them")
	skillsCmd.AddCommand(skillsListCmd)
	skillsCmd.AddCommand(skillsInstallCmd)
	skillsCmd.AddCommand(skillsUpdateCmd)
	skillsCmd.AddCommand(skillsSyncCmd)
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

		headers := []string{"NAME", "SOURCE", "VERSION"}
		var rows [][]string
		for _, s := range manifest.Skills {
			record, _ := manifest.GetSource(skill.Source{Repo: s.Source})
			rows = append(rows, []string{s.Name, s.Source, skill.DisplayVersion(record.Version)})
		}
		printTable(headers, rows)

		return nil
	},
}

func printTable(headers []string, rows [][]string) {
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
}

var skillsInstallCmd = &cobra.Command{
	Use:   "install [owner/repo[@version]|path]",
	Short: "Install skills from a GitHub repository or local path",
	Long: "Install skills from a GitHub repository or local path. A repository source " +
		"may carry a Go-style version suffix resolved like `go get`: owner/repo@latest " +
		"(the default when no version is given) picks the highest stable semver tag, " +
		"owner/repo@v1 the highest v1.x.y tag, and an exact tag, branch, or commit SHA " +
		"pins that revision.",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := skill.LoadManifest()
		if err != nil {
			return fmt.Errorf("failed to load skills manifest: %w", err)
		}

		if len(args) > 0 {
			if skillsInstallForce {
				return installFromRepo(manifest, args[0], true)
			}
			return runSkillsSourceAction(manifest, args[0], actionBrowseInstall)
		}

		return runInteractiveSkillsInstall(manifest)
	},
}

var skillsUpdateCmd = &cobra.Command{
	Use:   "update [owner/repo[@version]]",
	Short: "Update installed skills to the latest version",
	Long: "Update the skills installed from a source repository. With no argument every " +
		"source is updated to its latest version. With a repository, only that source is " +
		"updated: to latest, or to the version given by a Go-style suffix such as " +
		"owner/repo@v1.2.3 or owner/repo@v1. The set of installed skills is preserved.",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := skill.LoadManifest()
		if err != nil {
			return fmt.Errorf("failed to load skills manifest: %w", err)
		}

		if len(args) > 0 {
			return runSkillsSourceAction(manifest, args[0], actionUpdate)
		}
		mgr, err := newSkillsManager(manifest)
		if err != nil {
			return err
		}
		return forEachInstalledSource(mgr, skill.VerbUpdate, mgr.Update)
	},
}

var skillsSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Reinstall skills at the versions locked in the manifest",
	Long: "Reinstall every skill recorded in ~/.clime/skills.yaml from its source at the " +
		"locked version, without looking for a newer one. Versions already cached under " +
		"~/.clime/sources are applied without network access. Use `clime skills update` " +
		"to move a source to a newer version.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := skill.LoadManifest()
		if err != nil {
			return fmt.Errorf("failed to load skills manifest: %w", err)
		}
		mgr, err := newSkillsManager(manifest)
		if err != nil {
			return err
		}
		return forEachInstalledSource(mgr, skill.VerbSync, mgr.Sync)
	},
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

// newSkillsManager assembles a Manager over an already-loaded manifest,
// reporting progress through terminal spinners.
func newSkillsManager(manifest *skill.Manifest) (*skill.Manager, error) {
	store, err := skill.OpenStore()
	if err != nil {
		return nil, err
	}
	targets, err := skill.DetectTargets()
	if err != nil {
		return nil, err
	}
	return &skill.Manager{
		Manifest: manifest,
		Store:    store,
		Targets:  targets,
		Events:   &skillsUI{},
	}, nil
}

func startSpinner(msg string) *uicli.Spinner {
	return uicli.NewSpinner().
		WithStyle(uicli.SpinnerDots).
		WithColor(uicli.CyanColor).
		WithMessage(msg).
		Start()
}

// finish ends the active spinner with fn, tolerating events that arrive
// without a preceding start.
func (u *skillsUI) finish(fn func(*uicli.Spinner)) {
	if u.spinner != nil {
		fn(u.spinner)
		u.spinner = nil
	}
}

func (u *skillsUI) SourceResolving(verb skill.Verb, src skill.Source) {
	u.spinner = startSpinner(fmt.Sprintf("%s %s...", verbUI[verb].resolving, src))
}

func (u *skillsUI) SourceFailed(verb skill.Verb, src skill.Source, err error) {
	u.finish(func(s *uicli.Spinner) { s.Error(fmt.Sprintf("Failed to %s %s", verb, src.Repo)) })
}

func (u *skillsUI) SourceUpToDate(src skill.Source, version string) {
	u.finish(func(s *uicli.Spinner) {
		s.Success(fmt.Sprintf("%s is already at %s", src.Repo, skill.DisplayVersion(version)))
	})
}

func (u *skillsUI) SourceReady(verb skill.Verb, src skill.Source, version string) {
	label := fmt.Sprintf("%s %s", verbUI[verb].present, src.Repo)
	if version != "" {
		label = fmt.Sprintf("%s %s %s", label, verbUI[verb].prep, skill.DisplayVersion(version))
	}
	u.finish(func(s *uicli.Spinner) { s.Success(label) })
}

func (u *skillsUI) SkillInstalling(verb skill.Verb, name string, src skill.Source) {
	u.spinner = startSpinner(fmt.Sprintf("%s skill %q from %s...", verbUI[verb].present, name, src.Repo))
}

func (u *skillsUI) SkillInstalled(verb skill.Verb, name string, targets []string) {
	u.finish(func(s *uicli.Spinner) {
		s.Success(fmt.Sprintf("%s skill %q to %s", verbUI[verb].past, name, strings.Join(targets, ", ")))
	})
}

func (u *skillsUI) SkillFailed(verb skill.Verb, name string, err error) {
	u.finish(func(s *uicli.Spinner) {
		if errors.Is(err, os.ErrNotExist) {
			s.Stop()
			return
		}
		s.Error(fmt.Sprintf("Failed to %s skill %q", verb, name))
	})
	terminal.Errorf("Failed to %s %q: %v", verb, name, err)
}

func (u *skillsUI) NoTargets() {
	u.finish(func(s *uicli.Spinner) { s.Stop() })
	terminal.Warning("No skill directories were installed. Neither ~/.claude nor ~/.codex was found.")
}

func runInteractiveSkillsInstall(manifest *skill.Manifest) error {
	sources := manifest.KnownSources()
	if len(sources) == 0 {
		fmt.Println()
		repo, err := inputPrompt("Enter repository (owner/repo)")
		if err != nil {
			return err
		}
		return skillsActionRunner(manifest, repo, actionBrowseInstall)
	}

	options := make([]string, 0, len(sources)+2)
	for _, src := range sources {
		options = append(options, src.Repo)
	}
	options = append(options, pluginSkillsOption, newRepoOption)
	showSourceSpacer := true
	for {
		if showSourceSpacer {
			fmt.Println()
		} else {
			showSourceSpacer = true
		}
		idx, err := selectPrompt(prompt.SelectConfig{
			Label:   "Select a skill source",
			Options: options,
		})
		if err != nil {
			if errors.Is(err, prompt.ErrBack) {
				showSourceSpacer = false
				continue
			}
			return err
		}

		if options[idx] == pluginSkillsOption {
			err := installFromPluginSkills(manifest)
			if errors.Is(err, prompt.ErrBack) {
				showSourceSpacer = false
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
		showActionSpacer := true
		for {
			action, err := pickSourceAction(repo, showActionSpacer)
			if err != nil {
				if errors.Is(err, prompt.ErrBack) {
					showSourceSpacer = false
					break
				}
				return err
			}

			err = skillsActionRunner(manifest, repo, action)
			if errors.Is(err, prompt.ErrBack) {
				showActionSpacer = false
				continue
			}
			return err
		}
	}
}

func runSkillsSourceAction(manifest *skill.Manifest, source string, action sourceAction) error {
	src, err := skill.ParseSource(source)
	if err != nil {
		return err
	}

	switch action {
	case actionRemoveSource:
		return removeSource(manifest, src)
	case actionUpdate:
		mgr, err := newSkillsManager(manifest)
		if err != nil {
			return err
		}
		_, err = mgr.Update(src)
		return err
	default:
		return installFromRepo(manifest, source, false)
	}
}

func pickSourceAction(repo string, showSpacer bool) (sourceAction, error) {
	options := []string{
		"Browse and install skills",
		"Update installed skills",
		"Remove source and its installed skills",
	}

	if showSpacer {
		fmt.Println()
	}
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

// removeSource uninstalls all skills from the given source and removes it from the manifest.
func removeSource(manifest *skill.Manifest, src skill.Source) error {
	var names []string
	for _, s := range manifest.SkillsFrom(src) {
		names = append(names, s.Name)
	}

	fmt.Println()
	for _, name := range names {
		if err := uninstallByName(manifest, name); err != nil {
			terminal.Errorf("Failed to uninstall %q: %v", name, err)
		}
	}

	manifest.RemoveSource(src)
	if err := manifest.Save(); err != nil {
		return fmt.Errorf("failed to update manifest: %w", err)
	}

	store, err := skill.OpenStore()
	if err == nil {
		err = store.Remove(src)
	}
	if err != nil {
		terminal.Warningf("Failed to remove cached versions of %s: %v", src.Repo, err)
	}

	if len(names) == 0 {
		terminal.Successf("Removed source %s.", src.Repo)
	}
	return nil
}

// forEachInstalledSource applies fn to every source with installed skills,
// continuing past failures so one unreachable source does not block the
// rest. fn returns how many skills it changed; the run ends with a summary.
func forEachInstalledSource(mgr *skill.Manager, verb skill.Verb, fn func(skill.Source) (int, error)) error {
	sources := mgr.Manifest.InstalledSources()
	if len(sources) == 0 {
		terminal.Warning("No skills installed.")
		return nil
	}

	failed, changed := 0, 0
	for _, src := range sources {
		fmt.Println()
		n, err := fn(src)
		changed += n
		if err != nil {
			terminal.Errorf("Failed to %s %s: %v", verb, src.Repo, err)
			failed++
		}
	}

	fmt.Println()
	if failed > 0 {
		return fmt.Errorf("failed to %s %d of %d source(s)", verb, failed, len(sources))
	}
	if changed == 0 {
		terminal.Success("All skills are up to date.")
		return nil
	}
	terminal.Successf("%s %d skill(s) from %d source(s).", verbUI[verb].past, changed, len(sources))
	return nil
}

// selectInstallCandidates returns the repo skills that should be offered for
// installation. Already-installed skills are skipped unless force is set, in
// which case they are included and their label is marked "(reinstall)".
func selectInstallCandidates(repoSkills []skill.Entry, manifest *skill.Manifest, force bool) []installCandidate {
	var candidates []installCandidate
	for _, s := range repoSkills {
		_, installed := manifest.GetSkill(s.Name)
		if installed && !force {
			continue
		}
		label := s.Name
		if s.Description != "" {
			label = fmt.Sprintf("%s — %s", s.Name, uicli.TruncateString(s.Description, 60))
		}
		if installed {
			label += " (reinstall)"
		}
		candidates = append(candidates, installCandidate{entry: s, label: label})
	}
	return candidates
}

// installFromRepo fetches skills from a source and lets the user pick which
// to install. When force is true, skills that are already installed are kept
// in the list (instead of being filtered out) so they can be reinstalled and
// overwritten.
func installFromRepo(manifest *skill.Manifest, source string, force bool) error {
	src, err := skill.ParseSource(source)
	if err != nil {
		return err
	}
	mgr, err := newSkillsManager(manifest)
	if err != nil {
		return err
	}

	spinner := startSpinner(fmt.Sprintf("Fetching skills from %q...", source))
	snap, catalog, err := mgr.Fetch(src)
	if err != nil {
		spinner.Error(fmt.Sprintf("Failed to fetch %q", source))
		return fmt.Errorf("failed to fetch skills: %w", err)
	}
	if len(catalog.Skills) == 0 {
		spinner.Error(fmt.Sprintf("No skills found in %q", source))
		return fmt.Errorf("repository %q has no skills defined", source)
	}
	spinner.Success(fmt.Sprintf("Found %d skill(s) in %q", len(catalog.Skills), source))

	// Record the source so it appears in future interactive menus.
	manifest.AddSource(src)
	if err := manifest.Save(); err != nil {
		return fmt.Errorf("failed to save skill source: %w", err)
	}

	candidates := selectInstallCandidates(catalog.Skills, manifest, force)
	if len(candidates) == 0 {
		terminal.Info("All skills from this repository are already installed. Use --force to reinstall them.")
		return nil
	}

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

	entries := make([]skill.Entry, 0, len(selectedIdxs))
	for _, idx := range selectedIdxs {
		entries = append(entries, candidates[idx].entry)
	}

	// Per-skill failures are already reported through the progress events.
	fmt.Println()
	_, _ = mgr.Install(snap, entries)
	return nil
}

func uninstallByName(manifest *skill.Manifest, name string) error {
	if _, exists := manifest.GetSkill(name); !exists {
		return fmt.Errorf("skill %q is not installed", name)
	}
	targets, err := skill.DetectTargets()
	if err != nil {
		return err
	}
	mgr := &skill.Manager{Manifest: manifest, Targets: targets}

	spinner := startSpinner(fmt.Sprintf("Removing skill %q...", name))
	removed, err := mgr.Uninstall(name)
	if err != nil {
		spinner.Error(fmt.Sprintf("Failed to remove skill %q", name))
		return fmt.Errorf("failed to remove skill %q: %w", name, err)
	}

	spinner.Success(fmt.Sprintf("Removed skill %q from %s", name, strings.Join(removed, ", ")))
	return nil
}

func interactiveUninstall(manifest *skill.Manifest) error {
	if len(manifest.Skills) == 0 {
		terminal.Warning("No skills installed.")
		return nil
	}

	options := make([]string, len(manifest.Skills))
	for i, s := range manifest.Skills {
		options[i] = s.Name
	}

	showSpacer := true
	for {
		if showSpacer {
			fmt.Println()
		} else {
			showSpacer = true
		}
		selectedIdxs, err := multiSelectPrompt(prompt.SelectConfig{
			Label:   "Select skills to uninstall (space to toggle, enter to confirm)",
			Options: options,
		})
		if err != nil {
			if errors.Is(err, prompt.ErrBack) {
				showSpacer = false
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
