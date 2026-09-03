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
			source, _ := manifest.GetSource(s.Source)
			rows = append(rows, []string{s.Name, s.Source, displaySkillVersion(source.Version)})
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

// displaySkillVersion shortens a full commit SHA for table display; tags are
// shown as-is and an unknown version renders as a dash.
func displaySkillVersion(version string) string {
	if version == "" {
		return "—"
	}
	if len(version) == 40 && isHexString(version) {
		return version[:12]
	}
	return version
}

func isHexString(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
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

	skillsInstallForce bool
)

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
				if err := validateSkillRepoSource(args[0]); err != nil {
					return err
				}
				return installFromRepo(manifest, args[0], true)
			}
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

	options := append(sources, pluginSkillsOption, newRepoOption)
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

// uniqueSkillSources lists the sources of installed skills followed by
// tracked sources, preserving order and first-seen spelling; repository
// names are case-insensitive.
func uniqueSkillSources(manifest *skill.Manifest) []string {
	sources := make([]string, 0, len(manifest.Skills)+len(manifest.Sources))
	for _, s := range manifest.Skills {
		sources = append(sources, s.Source)
	}
	for _, s := range manifest.Sources {
		sources = append(sources, s.Repo)
	}
	return dedupeSources(sources)
}

// installedSkillSources lists the sources that have at least one installed
// skill, in first-seen order and spelling.
func installedSkillSources(manifest *skill.Manifest) []string {
	sources := make([]string, 0, len(manifest.Skills))
	for _, s := range manifest.Skills {
		sources = append(sources, s.Source)
	}
	return dedupeSources(sources)
}

func dedupeSources(sources []string) []string {
	seen := make(map[string]bool)
	var unique []string
	for _, source := range sources {
		key := strings.ToLower(source)
		if source != "" && !seen[key] {
			seen[key] = true
			unique = append(unique, source)
		}
	}
	return unique
}

// skillsFromSource returns the installed skills recorded from a source repository.
func skillsFromSource(manifest *skill.Manifest, repo string) []skill.InstalledSkill {
	var installed []skill.InstalledSkill
	for _, s := range manifest.Skills {
		if skill.SameSource(s.Source, repo) {
			installed = append(installed, s)
		}
	}
	return installed
}

func runSkillsSourceAction(manifest *skill.Manifest, repo string, action sourceAction) error {
	if err := validateSkillRepoSource(repo); err != nil {
		return err
	}

	switch action {
	case actionRemoveSource:
		return removeSource(manifest, repo)
	case actionUpdate:
		_, err := updateSource(manifest, repo)
		return err
	default:
		return installFromRepo(manifest, repo, false)
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
func removeSource(manifest *skill.Manifest, repo string) error {
	var names []string
	for _, s := range skillsFromSource(manifest, repo) {
		names = append(names, s.Name)
	}

	fmt.Println()
	for _, name := range names {
		if err := uninstallByName(manifest, name); err != nil {
			terminal.Errorf("Failed to uninstall %q: %v", name, err)
		}
	}

	manifest.RemoveSource(repo)
	if err := manifest.Save(); err != nil {
		return fmt.Errorf("failed to update manifest: %w", err)
	}

	if len(names) == 0 {
		terminal.Successf("Removed source %s.", repo)
	}
	return nil
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
		return forEachInstalledSource(manifest, verbUpdate, updateSource)
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
		return forEachInstalledSource(manifest, verbSync, syncSource)
	},
}

// skillVerb names an operation on skills in the forms progress output needs.
type skillVerb struct {
	base    string // "sync"
	present string // "Syncing"
	past    string // "Synced"
	prep    string // joins the source to its version: "Syncing repo at v1"
}

var (
	verbInstall = skillVerb{"install", "Installing", "Installed", "at"}
	verbUpdate  = skillVerb{"update", "Updating", "Updated", "to"}
	verbSync    = skillVerb{"sync", "Syncing", "Synced", "at"}
)

// forEachInstalledSource applies fn to every source with installed skills,
// continuing past failures so one unreachable source does not block the
// rest. fn returns how many skills it changed; the run ends with a summary.
func forEachInstalledSource(manifest *skill.Manifest, verb skillVerb, fn func(*skill.Manifest, string) (int, error)) error {
	repos := installedSkillSources(manifest)
	if len(repos) == 0 {
		terminal.Warning("No skills installed.")
		return nil
	}

	failed, changed := 0, 0
	for _, repo := range repos {
		fmt.Println()
		n, err := fn(manifest, repo)
		changed += n
		if err != nil {
			terminal.Errorf("Failed to %s %s: %v", verb.base, repo, err)
			failed++
		}
	}

	fmt.Println()
	if failed > 0 {
		return fmt.Errorf("failed to %s %d of %d source(s)", verb.base, failed, len(repos))
	}
	if changed == 0 {
		terminal.Success("All skills are up to date.")
		return nil
	}
	terminal.Successf("%s %d skill(s) from %d source(s).", verb.past, changed, len(repos))
	return nil
}

// lockedSource pins a repository to the version recorded in the manifest.
// Local paths carry no version identity and are returned unchanged.
func lockedSource(manifest *skill.Manifest, repo string) string {
	src, ok := manifest.GetSource(repo)
	if !ok || src.Version == "" {
		return repo
	}
	if _, isLocal, _ := skill.LocalRepoDir(repo); isLocal {
		return repo
	}
	return repo + "@" + src.Version
}

// syncSource re-installs every skill from a source at the version locked in
// the manifest, using the stored skill paths, and returns how many skills it
// re-installed. A source without a locked version is installed at latest and
// the resolved version is recorded.
func syncSource(manifest *skill.Manifest, repo string) (int, error) {
	installed := skillsFromSource(manifest, repo)
	if len(installed) == 0 {
		return 0, fmt.Errorf("no skills installed from %s", repo)
	}

	source := lockedSource(manifest, repo)
	spinner := uicli.NewSpinner().
		WithStyle(uicli.SpinnerDots).
		WithColor(uicli.CyanColor).
		WithMessage(fmt.Sprintf("Preparing %s...", source)).
		Start()

	dir, version, cleanup, err := skill.PrepareRepoDir(source)
	if err != nil {
		spinner.Error(fmt.Sprintf("Failed to prepare %s", source))
		return 0, err
	}
	defer cleanup()
	spinner.Success(sourceVersionLabel(verbSync, repo, version))

	return installSkillEntries(manifest, installedEntries(installed), repo, dir, version, verbSync)
}

// sourceVersionLabel renders "Syncing owner/repo at v1.2.3" or "Updating
// owner/repo to v1.2.3", omitting the version for sources without one.
func sourceVersionLabel(verb skillVerb, repo, version string) string {
	if version == "" {
		return fmt.Sprintf("%s %s", verb.present, repo)
	}
	return fmt.Sprintf("%s %s %s %s", verb.present, repo, verb.prep, displaySkillVersion(version))
}

func installedEntries(installed []skill.InstalledSkill) []skill.SkillEntry {
	entries := make([]skill.SkillEntry, 0, len(installed))
	for _, s := range installed {
		entries = append(entries, skill.SkillEntry{Name: s.Name, Path: s.Path})
	}
	return entries
}

// installSkillEntries installs each entry in turn, reporting failures as
// they happen, and returns how many succeeded.
func installSkillEntries(manifest *skill.Manifest, entries []skill.SkillEntry, repo, dir, version string, verb skillVerb) (int, error) {
	failed := 0
	for i := range entries {
		if err := installSkillEntry(manifest, &entries[i], repo, dir, version, verb); err != nil {
			terminal.Errorf("Failed to %s %q: %v", verb.base, entries[i].Name, err)
			failed++
		}
	}
	if failed > 0 {
		return len(entries) - failed, fmt.Errorf("%d skill(s) failed", failed)
	}
	return len(entries), nil
}

// updateSource re-installs every skill from a source at the version its
// query resolves to (latest when the source carries none), taking each
// skill's path from the catalog at that version, and returns
// how many skills it re-installed. The update is refused when the new
// catalog no longer lists an installed skill, so a skill is never removed
// implicitly.
func updateSource(manifest *skill.Manifest, source string) (int, error) {
	repo, _ := skill.ParseSourceVersion(source)
	installed := skillsFromSource(manifest, repo)
	if len(installed) == 0 {
		return 0, fmt.Errorf("no skills installed from %s", repo)
	}

	spinner := uicli.NewSpinner().
		WithStyle(uicli.SpinnerDots).
		WithColor(uicli.CyanColor).
		WithMessage(fmt.Sprintf("Resolving %s...", source)).
		Start()

	dir, version, cleanup, err := skill.PrepareRepoDir(source)
	if err != nil {
		spinner.Error(fmt.Sprintf("Failed to resolve %s", source))
		return 0, err
	}
	defer cleanup()

	if current, _ := manifest.GetSource(repo); version != "" && version == current.Version {
		spinner.Success(fmt.Sprintf("%s is already at %s", repo, displaySkillVersion(version)))
		return 0, nil
	}

	catalog, err := skill.ReadRepoManifestFromDir(dir, repo)
	if err != nil {
		spinner.Error(fmt.Sprintf("Failed to read skills from %s", repo))
		return 0, err
	}
	entries := make([]skill.SkillEntry, 0, len(installed))
	var missing []string
	for _, s := range installed {
		entry, ok := findSkillEntry(catalog, s.Name)
		if !ok {
			missing = append(missing, s.Name)
			continue
		}
		entries = append(entries, entry)
	}
	if len(missing) > 0 {
		spinner.Error(fmt.Sprintf("Cannot update %s", repo))
		return 0, fmt.Errorf("%s at %s no longer provides %s; uninstall them first to update",
			repo, displaySkillVersion(version), strings.Join(missing, ", "))
	}
	spinner.Success(sourceVersionLabel(verbUpdate, repo, version))

	return installSkillEntries(manifest, entries, repo, dir, version, verbUpdate)
}

func findSkillEntry(catalog *skill.RepoManifest, name string) (skill.SkillEntry, bool) {
	for _, entry := range catalog.Skills {
		if entry.Name == name {
			return entry, true
		}
	}
	return skill.SkillEntry{}, false
}

// installSkillEntry writes one skill from localDir into the agent targets
// and records it in the manifest under repo at version, which may be empty
// for sources without version identity. verb names the operation in output.
func installSkillEntry(manifest *skill.Manifest, entry *skill.SkillEntry, repo, localDir, version string, verb skillVerb) error {
	spinner := uicli.NewSpinner().
		WithStyle(uicli.SpinnerDots).
		WithColor(uicli.CyanColor).
		WithMessage(fmt.Sprintf("%s skill %q from %s...", verb.present, entry.Name, repo)).
		Start()

	var targets []string
	var err error
	if localDir != "" {
		targets, err = skill.InstallFromDir(entry.Name, localDir, entry.Path)
	} else {
		targets, err = skill.Install(entry.Name, repo, entry.Path)
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			spinner.Stop()
		} else {
			spinner.Error(fmt.Sprintf("Failed to %s skill %q", verb.base, entry.Name))
		}
		return fmt.Errorf("failed to %s skill %q: %w", verb.base, entry.Name, err)
	}

	if len(targets) == 0 {
		spinner.Stop()
		terminal.Warning("No skill directories were installed. Neither ~/.claude nor ~/.codex was found.")
		return nil
	}

	// The manifest identifies the source by repository alone; the resolved
	// version is recorded on the source so a floating query like @latest is
	// never persisted as part of its identity.
	sourceRepo, _ := skill.ParseSourceVersion(repo)
	if version != "" {
		manifest.SetSourceVersion(sourceRepo, version)
	}
	manifest.AddSkill(skill.InstalledSkill{
		Name:   entry.Name,
		Source: sourceRepo,
		Path:   entry.Path,
	})
	if err := manifest.Save(); err != nil {
		return fmt.Errorf("skill installed but failed to update manifest: %w", err)
	}

	spinner.Success(fmt.Sprintf("%s skill %q to %s", verb.past, entry.Name, strings.Join(targets, ", ")))
	return nil
}

type installCandidate struct {
	entry skill.SkillEntry
	label string
}

// selectInstallCandidates returns the repo skills that should be offered for
// installation. Already-installed skills are skipped unless force is set, in
// which case they are included and their label is marked "(reinstall)".
func selectInstallCandidates(repoSkills []skill.SkillEntry, manifest *skill.Manifest, force bool) []installCandidate {
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

// installFromRepo fetches skills from a repo and lets the user pick which to install.
// When force is true, skills that are already installed are kept in the list
// (instead of being filtered out) so they can be reinstalled and overwritten.
func installFromRepo(manifest *skill.Manifest, repo string, force bool) error {
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

	// Record the source so it appears in future interactive menus.
	manifest.AddSource(repo)
	if err := manifest.Save(); err != nil {
		return fmt.Errorf("failed to save skill source: %w", err)
	}

	candidates := selectInstallCandidates(repoManifest.Skills, manifest, force)
	if len(candidates) == 0 {
		terminal.Info("All skills from this repository are already installed. Use --force to reinstall them.")
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
	dir, version, cleanup, err := skill.PrepareRepoDir(repo)
	if err != nil {
		return err
	}
	defer cleanup()

	// Install each selected skill.
	fmt.Println()
	for _, idx := range selectedIdxs {
		entry := candidates[idx].entry
		if err := installSkillEntry(manifest, &entry, repo, dir, version, verbInstall); err != nil {
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
