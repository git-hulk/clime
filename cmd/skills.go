package cmd

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	uicli "github.com/alperdrsnn/clime"
	"github.com/git-hulk/clime/internal/prompt"
	"github.com/git-hulk/clime/internal/skill"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type sourceAction int

const (
	actionBrowseInstall sourceAction = iota
	actionRemoveSource
	actionUpdate
)

const newRepoOption = "Enter a new repository..."

const skillsPageSize = 10

var (
	selectPrompt       = prompt.Select
	multiSelectPrompt  = prompt.MultiSelect
	inputPrompt        = prompt.Input
	skillsActionRunner = runSkillsSourceAction

	skillsInstallForce bool
	skillsManifestPath string
)

// verbUI maps a skill verb to the wording the progress output uses.
var verbUI = map[skill.Verb]struct{ resolving, present, past, preposition string }{
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
	skillsCmd.PersistentFlags().StringVar(&skillsManifestPath, "manifest", "",
		"path to the installed-skills manifest (default ~/.clime/skills.yaml)")
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
	Long: "Install skills from GitHub repositories or local paths into ~/.agents/skills. " +
		"When ~/.claude exists, Claude Code gets symlinks in ~/.claude/skills.",
	RunE: skillsListCmd.RunE,
}

var skillsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed skills and their sources",
	Long: "List installed skills and their sources, 10 per page in an interactive " +
		"terminal. Piped output includes every skill.",
	RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := skill.LoadManifest(skillsManifestPath)
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
		for _, installedSkill := range manifest.Skills {
			record, _ := manifest.GetSource(skill.Source{Repo: installedSkill.Source})
			rows = append(rows, []string{installedSkill.Name, installedSkill.Source, skill.DisplayVersion(record.Version)})
		}
		slices.SortStableFunc(rows, func(left, right []string) int {
			return strings.Compare(left[0], right[0])
		})
		if !term.IsTerminal(int(os.Stdout.Fd())) || !term.IsTerminal(int(os.Stdin.Fd())) {
			printTable(headers, rows)
			return nil
		}
		return printSkillPages(headers, rows)
	},
}

func printSkillPages(headers []string, rows [][]string) error {
	for start := 0; ; {
		end := min(start+skillsPageSize, len(rows))
		printTable(headers, rows[start:end])
		if len(rows) <= skillsPageSize {
			return nil
		}
		options := []string{}
		if end < len(rows) {
			options = append(options, "Next page")
		}
		if start > 0 {
			options = append(options, "Previous page")
		}
		options = append(options, "Done")
		selectedIndex, err := selectPrompt(prompt.SelectConfig{
			Label:       fmt.Sprintf("Page %d/%d", start/skillsPageSize+1, (len(rows)-1)/skillsPageSize+1),
			Options:     options,
			LeftOption:  "Previous page",
			RightOption: "Next page",
		})
		if errors.Is(err, prompt.ErrBack) {
			return nil
		}
		if err != nil {
			return err
		}
		switch options[selectedIndex] {
		case "Next page":
			start = end
		case "Previous page":
			start -= skillsPageSize
		default:
			return nil
		}
		fmt.Println()
	}
}

func printTable(headers []string, rows [][]string) {
	columnWidths := make([]int, len(headers))
	for i, header := range headers {
		columnWidths[i] = len(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > columnWidths[i] {
				columnWidths[i] = len(cell)
			}
		}
	}

	const gap = 2
	const indent = "  "

	fmt.Print(indent)
	for i, header := range headers {
		if i > 0 {
			fmt.Print(strings.Repeat(" ", gap))
		}
		fmt.Print(uicli.BoldColor.Sprintf("%-*s", columnWidths[i], header))
	}
	fmt.Println()

	fmt.Print(indent)
	for i, width := range columnWidths {
		if i > 0 {
			fmt.Print(strings.Repeat(" ", gap))
		}
		fmt.Print(strings.Repeat("-", width))
	}
	fmt.Println()

	for _, row := range rows {
		fmt.Print(indent)
		for i, cell := range row {
			if i > 0 {
				fmt.Print(strings.Repeat(" ", gap))
			}
			fmt.Printf("%-*s", columnWidths[i], cell)
		}
		fmt.Println()
	}
}

var skillsInstallCmd = &cobra.Command{
	Use:   "install [owner/repo[@version]|path]",
	Short: "Install skills from a GitHub repository or local path",
	Long: "Install skills from a GitHub repository or local path. A repository source " +
		"without a version uses its locked version from the selected manifest, reusing " +
		"the local cache when available. Without a lock, it resolves latest. Use " +
		"owner/repo@latest or `clime skills update` to check for a newer version. " +
		"A Go-style version suffix is resolved like `go get`: owner/repo@latest " +
		"picks the highest stable semver tag, " +
		"owner/repo@v1 the highest v1.x.y tag, and an exact tag, branch, or commit SHA " +
		"pins that revision.",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := skill.LoadManifest(skillsManifestPath)
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
		manifest, err := skill.LoadManifest(skillsManifestPath)
		if err != nil {
			return fmt.Errorf("failed to load skills manifest: %w", err)
		}

		if len(args) > 0 {
			return runSkillsSourceAction(manifest, args[0], actionUpdate)
		}
		manager, err := newSkillsManager(manifest)
		if err != nil {
			return err
		}
		return forEachInstalledSource(manager, skill.VerbUpdate, manager.Update)
	},
}

var skillsSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Reinstall skills at the versions locked in the manifest",
	Long: "Reinstall every skill recorded in the selected manifest from its source at the " +
		"locked version, without looking for a newer one. Versions already cached under " +
		"~/.clime/sources are applied without network access. Use `clime skills update` " +
		"to move a source to a newer version.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := skill.LoadManifest(skillsManifestPath)
		if err != nil {
			return fmt.Errorf("failed to load skills manifest: %w", err)
		}
		manager, err := newSkillsManager(manifest)
		if err != nil {
			return err
		}
		return forEachInstalledSource(manager, skill.VerbSync, manager.Sync)
	},
}

var skillsUninstallCmd = &cobra.Command{
	Use:   "uninstall [skill-name]",
	Short: "Uninstall a previously installed skill",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := skill.LoadManifest(skillsManifestPath)
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
	ui := &skillsUI{}
	store.Progress = func(message string) {
		if ui.spinner != nil {
			fetchProgress(ui.spinner)(message)
		}
	}
	return &skill.Manager{
		Manifest: manifest,
		Store:    store,
		Targets:  targets,
		Events:   ui,
	}, nil
}

func startSpinner(message string) *uicli.Spinner {
	return uicli.NewSpinner().
		WithStyle(uicli.SpinnerDots).
		WithColor(uicli.CyanColor).
		WithMessage(message).
		Start()
}

func fetchProgress(spinner *uicli.Spinner) func(string) {
	if term.IsTerminal(int(os.Stdout.Fd())) {
		return spinner.UpdateMessage
	}
	return func(message string) {
		fmt.Fprintln(os.Stderr, message)
	}
}

// finish ends the active spinner with finishSpinner, tolerating events that arrive
// without a preceding start.
func (ui *skillsUI) finish(finishSpinner func(*uicli.Spinner)) {
	if ui.spinner != nil {
		finishSpinner(ui.spinner)
		ui.spinner = nil
	}
}

func (ui *skillsUI) SourceResolving(verb skill.Verb, skillSource skill.Source) {
	ui.spinner = startSpinner(fmt.Sprintf("%s %s...", verbUI[verb].resolving, skillSource))
}

func (ui *skillsUI) SourceFailed(verb skill.Verb, skillSource skill.Source, err error) {
	ui.finish(func(spinner *uicli.Spinner) { spinner.Error(fmt.Sprintf("Failed to %s %s", verb, skillSource.Repo)) })
}

func (ui *skillsUI) SourceUpToDate(skillSource skill.Source, version string) {
	ui.finish(func(spinner *uicli.Spinner) {
		spinner.Success(fmt.Sprintf("Source %s is already at %s", skillSource.Repo, skill.DisplayVersion(version)))
	})
}

func (ui *skillsUI) SourceReady(verb skill.Verb, skillSource skill.Source, version string) {
	label := fmt.Sprintf("%s %s", verbUI[verb].present, skillSource.Repo)
	if version != "" {
		label = fmt.Sprintf("%s %s %s", label, verbUI[verb].preposition, skill.DisplayVersion(version))
	}
	ui.finish(func(spinner *uicli.Spinner) { spinner.Success(label) })
}

func (ui *skillsUI) SkillInstalling(verb skill.Verb, name string, skillSource skill.Source) {
	ui.spinner = startSpinner(fmt.Sprintf("%s skill %q from %s...", verbUI[verb].present, name, skillSource.Repo))
}

func (ui *skillsUI) SkillInstalled(verb skill.Verb, name string, targets []string) {
	ui.finish(func(spinner *uicli.Spinner) {
		spinner.Success(fmt.Sprintf("%s skill %q to %s", verbUI[verb].past, name, strings.Join(targets, ", ")))
	})
}

func (ui *skillsUI) SkillFailed(verb skill.Verb, name string, err error) {
	ui.finish(func(spinner *uicli.Spinner) {
		if errors.Is(err, os.ErrNotExist) {
			spinner.Stop()
			return
		}
		spinner.Error(fmt.Sprintf("Failed to %s skill %q", verb, name))
	})
	terminal.Errorf("Failed to %s %q: %v", verb, name, err)
}

func (ui *skillsUI) NoTargets() {
	ui.finish(func(spinner *uicli.Spinner) { spinner.Stop() })
	terminal.Warning("No skill directories were installed.")
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
	for _, skillSource := range sources {
		options = append(options, skillSource.Repo)
	}
	options = append(options, pluginSkillsOption, newRepoOption)
	showSourceSpacer := true
	for {
		if showSourceSpacer {
			fmt.Println()
		} else {
			showSourceSpacer = true
		}
		selectedIndex, err := selectPrompt(prompt.SelectConfig{
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

		if options[selectedIndex] == pluginSkillsOption {
			err := installFromPluginSkills(manifest)
			if errors.Is(err, prompt.ErrBack) {
				showSourceSpacer = false
				continue
			}
			return err
		}

		if options[selectedIndex] == newRepoOption {
			repo, err := inputPrompt("Enter repository (owner/repo)")
			if err != nil {
				return err
			}
			return skillsActionRunner(manifest, repo, actionBrowseInstall)
		}

		repo := options[selectedIndex]
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
	skillSource, err := skill.ParseSource(source)
	if err != nil {
		return err
	}

	switch action {
	case actionRemoveSource:
		return removeSource(manifest, skillSource)
	case actionUpdate:
		manager, err := newSkillsManager(manifest)
		if err != nil {
			return err
		}
		_, err = manager.Update(skillSource)
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
	selectedIndex, err := selectPrompt(prompt.SelectConfig{
		Label:   fmt.Sprintf("Action for %s", repo),
		Options: options,
	})
	if err != nil {
		return 0, err
	}

	switch selectedIndex {
	case 1:
		return actionUpdate, nil
	case 2:
		return actionRemoveSource, nil
	default:
		return actionBrowseInstall, nil
	}
}

// removeSource uninstalls all skills from the given source and removes it from the manifest.
func removeSource(manifest *skill.Manifest, skillSource skill.Source) error {
	var names []string
	for _, installedSkill := range manifest.SkillsFrom(skillSource) {
		names = append(names, installedSkill.Name)
	}

	fmt.Println()
	for _, name := range names {
		if err := uninstallByName(manifest, name); err != nil {
			terminal.Errorf("Failed to uninstall %q: %v", name, err)
		}
	}

	manifest.RemoveSource(skillSource)
	if err := manifest.Save(); err != nil {
		return fmt.Errorf("failed to update manifest: %w", err)
	}

	store, err := skill.OpenStore()
	if err == nil {
		err = store.Remove(skillSource)
	}
	if err != nil {
		terminal.Warningf("Failed to remove cached versions of %s: %v", skillSource.Repo, err)
	}

	if len(names) == 0 {
		terminal.Successf("Removed source %s.", skillSource.Repo)
	}
	return nil
}

// forEachInstalledSource applies applySource to every source with installed skills,
// continuing past failures so one unreachable source does not block the
// rest. applySource returns how many skills it changed; the run ends with a summary.
func forEachInstalledSource(manager *skill.Manager, verb skill.Verb, applySource func(skill.Source) (int, error)) error {
	sources := manager.Manifest.InstalledSources()
	if len(sources) == 0 {
		terminal.Warning("No skills installed.")
		return nil
	}

	failed, changed := 0, 0
	for _, skillSource := range sources {
		fmt.Println()
		changedCount, err := applySource(skillSource)
		changed += changedCount
		if err != nil {
			terminal.Errorf("Failed to %s %s: %v", verb, skillSource.Repo, err)
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
	for _, installedSkill := range repoSkills {
		_, installed := manifest.GetSkill(installedSkill.Name)
		if installed && !force {
			continue
		}
		label := installedSkill.Name
		if installedSkill.Description != "" {
			label = fmt.Sprintf("%s — %s", installedSkill.Name, uicli.TruncateString(installedSkill.Description, 60))
		}
		if installed {
			label += " (reinstall)"
		}
		candidates = append(candidates, installCandidate{entry: installedSkill, label: label})
	}
	slices.SortStableFunc(candidates, func(left, right installCandidate) int {
		return strings.Compare(left.entry.Name, right.entry.Name)
	})
	return candidates
}

// installFromRepo fetches skills from a source and lets the user pick which
// to install. When force is true, skills that are already installed are kept
// in the list (instead of being filtered out) so they can be reinstalled and
// overwritten.
func installFromRepo(manifest *skill.Manifest, source string, force bool) error {
	skillSource, err := skill.ParseSource(source)
	if err != nil {
		return err
	}
	manager, err := newSkillsManager(manifest)
	if err != nil {
		return err
	}

	spinner := startSpinner(fmt.Sprintf("Fetching skills from %q...", source))
	manager.Store.Progress = fetchProgress(spinner)
	snapshot, catalog, err := manager.Fetch(skillSource)
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
	manifest.AddSource(skillSource)
	if err := manifest.Save(); err != nil {
		return fmt.Errorf("failed to save skill source: %w", err)
	}

	candidates := selectInstallCandidates(catalog.Skills, manifest, force)
	if len(candidates) == 0 {
		terminal.Info("All skills from this repository are already installed. Use --force to reinstall them.")
		return nil
	}

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

	entries := make([]skill.Entry, 0, len(selectedIndices))
	for _, selectedIndex := range selectedIndices {
		entries = append(entries, candidates[selectedIndex].entry)
	}

	// Per-skill failures are already reported through the progress events.
	fmt.Println()
	_, _ = manager.Install(snapshot, entries)
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
	manager := &skill.Manager{Manifest: manifest, Targets: targets}

	spinner := startSpinner(fmt.Sprintf("Removing skill %q...", name))
	removed, err := manager.Uninstall(name)
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
	for i, installedSkill := range manifest.Skills {
		options[i] = installedSkill.Name
	}
	slices.Sort(options)

	showSpacer := true
	for {
		if showSpacer {
			fmt.Println()
		} else {
			showSpacer = true
		}
		selectedIndices, err := multiSelectPrompt(prompt.SelectConfig{
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

		if len(selectedIndices) == 0 {
			terminal.Info("No skills selected.")
			return nil
		}

		// Collect names before uninstalling, since uninstallByName modifies manifest.Skills.
		names := make([]string, len(selectedIndices))
		for i, selectedIndex := range selectedIndices {
			names[i] = options[selectedIndex]
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
