package cmd

import (
	"errors"
	"fmt"
	"strings"

	uicli "github.com/alperdrsnn/clime"
	"github.com/git-hulk/clime/internal/prompt"
	"github.com/git-hulk/clime/internal/skill"
	"github.com/spf13/cobra"
)

func init() {
	skillsInstallCmd.Flags().BoolVarP(&skillsInstallForce, "force", "f", false,
		"also offer skills that are already installed so they can be reinstalled and overwritten")
	skillsCmd.AddCommand(skillsListCmd)
	skillsCmd.AddCommand(skillsInstallCmd)
	skillsCmd.AddCommand(skillsUpdateCmd)
	skillsCmd.AddCommand(skillsUninstallCmd)
	skillsCmd.AddCommand(skillsSyncCmd)
	skillsCmd.AddCommand(skillsPurgeCmd)
	rootCmd.AddCommand(skillsCmd)
}

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Manage versioned AI agent skills from Git repositories",
	Long: "Install skills from Git repositories into ~/.claude/skills and ~/.codex/skills for use with " +
		"Claude Code and Codex. Repositories are locked to a tag or commit in ~/.clime/skills.yaml and " +
		"cached as immutable snapshots under ~/.clime/cache.",
	RunE: skillsListCmd.RunE,
}

var (
	selectPrompt      = prompt.Select
	multiSelectPrompt = prompt.MultiSelect
	inputPrompt       = prompt.Input

	skillsInstallForce bool
)

const newRepoOption = "Enter a new repository..."

// shortVersion abbreviates a full commit SHA for display.
func shortVersion(v string) string {
	if skill.IsFullCommit(v) {
		return v[:12]
	}
	return v
}

var skillsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List repositories, locked versions, selected skills, and target state",
	RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := skill.LoadManifest()
		if err != nil {
			return fmt.Errorf("failed to load skills manifest: %w", err)
		}

		if len(manifest.Repos) == 0 {
			terminal.Warning("No skills installed.")
			terminal.Info("Install skills with: clime skills install <owner/repo>[@version]")
			return nil
		}

		total := len(manifest.SkillNames())
		fmt.Println()
		fmt.Printf("  %s %s\n\n",
			uicli.BoldColor.Sprint("Installed Skills"),
			uicli.DimColor.Sprintf("(%d skill(s) from %d repositor(ies))", total, len(manifest.Repos)),
		)

		headers := []string{"REPOSITORY", "VERSION", "SKILL", "TARGETS"}
		var rows [][]string
		for _, r := range manifest.Repos {
			for _, name := range r.Skills {
				targets := skill.InstalledTargets(name)
				state := strings.Join(targets, ", ")
				if state == "" {
					state = "not installed"
				}
				rows = append(rows, []string{r.Key, shortVersion(r.Version), name, state})
			}
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
	Use:   "install [repo[@version]]",
	Short: "Install skills from a Git repository at a locked version",
	Long: "Reads the repository's skill catalog and opens an interactive multi-select. Omitting the " +
		"version means latest: the highest stable SemVer tag, or the default branch's commit when the " +
		"repository has no SemVer tags. A branch or short commit is locked to the full commit SHA.",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := skill.LoadManifest()
		if err != nil {
			return fmt.Errorf("failed to load skills manifest: %w", err)
		}
		if len(args) > 0 {
			return installFromRepoArg(manifest, args[0], skillsInstallForce)
		}
		return runInteractiveSkillsInstall(manifest)
	},
}

// runInteractiveSkillsInstall lets the user pick a known repository, a plugin
// skill source, or type a new repository, then browse its catalog.
func runInteractiveSkillsInstall(manifest *skill.Manifest) error {
	var options []string
	for _, r := range manifest.Repos {
		options = append(options, r.Key)
	}
	options = append(options, pluginSkillsOption, newRepoOption)

	showSpacer := true
	for {
		if showSpacer {
			fmt.Println()
		} else {
			showSpacer = true
		}
		idx, err := selectPrompt(prompt.SelectConfig{
			Label:   "Select a skill source",
			Options: options,
		})
		if err != nil {
			if errors.Is(err, prompt.ErrBack) {
				showSpacer = false
				continue
			}
			return err
		}

		switch options[idx] {
		case pluginSkillsOption:
			err = installFromPluginSkills(manifest)
		case newRepoOption:
			var repo string
			repo, err = inputPrompt("Enter repository (owner/repo[@version])")
			if err != nil {
				return err
			}
			err = installFromRepoArg(manifest, repo, skillsInstallForce)
		default:
			// Browse an already-tracked repository at its locked version.
			r := manifest.Repos[idx]
			err = browseAndInstall(manifest, r.ID, r.Key, r.Version, skillsInstallForce)
		}
		if errors.Is(err, prompt.ErrBack) {
			showSpacer = false
			continue
		}
		return err
	}
}

// installFromRepoArg resolves a repo[@version] argument and installs selected
// skills from it.
func installFromRepoArg(manifest *skill.Manifest, arg string, force bool) error {
	repoArg, spec := skill.SplitRepoVersion(arg)
	id, err := skill.ParseRepo(repoArg)
	if err != nil {
		return err
	}

	version, err := resolveVersionWithSpinner(id, spec)
	if err != nil {
		return err
	}

	key := id.DisplayKey()
	if existing := manifest.FindRepo(id); existing != nil {
		key = existing.Key
	}
	return browseAndInstall(manifest, id, key, version, force)
}

// resolveVersionWithSpinner resolves a version spec against the remote.
func resolveVersionWithSpinner(id skill.RepoID, spec string) (string, error) {
	label := spec
	if label == "" {
		label = "latest"
	}
	spinner := uicli.NewSpinner().
		WithStyle(uicli.SpinnerDots).
		WithColor(uicli.CyanColor).
		WithMessage(fmt.Sprintf("Resolving %s@%s...", id.Canonical(), label)).
		Start()
	version, err := skill.ResolveVersion(id, spec)
	if err != nil {
		spinner.Error(fmt.Sprintf("Failed to resolve %s@%s", id.Canonical(), label))
		return "", err
	}
	spinner.Success(fmt.Sprintf("Resolved %s@%s to %s", id.Canonical(), label, shortVersion(version)))
	return version, nil
}

// fetchSnapshotWithSpinner ensures the snapshot is cached and reads its catalog.
func fetchSnapshotWithSpinner(id skill.RepoID, version string) (*skill.Catalog, error) {
	spinner := uicli.NewSpinner().
		WithStyle(uicli.SpinnerDots).
		WithColor(uicli.CyanColor).
		WithMessage(fmt.Sprintf("Fetching %s at %s...", id.Canonical(), shortVersion(version))).
		Start()
	snapDir, err := skill.EnsureSnapshot(id, version)
	if err != nil {
		spinner.Error(fmt.Sprintf("Failed to fetch %s", id.Canonical()))
		return nil, err
	}
	catalog, err := skill.ReadCatalog(snapDir)
	if err != nil {
		spinner.Error(fmt.Sprintf("No skill catalog in %s", id.Canonical()))
		return nil, fmt.Errorf("repository %s at %s: %w", id.Canonical(), shortVersion(version), err)
	}
	spinner.Success(fmt.Sprintf("Found %d skill(s) in %s at %s", len(catalog.Skills), id.Canonical(), shortVersion(version)))
	return catalog, nil
}

type installCandidate struct {
	name  string
	label string
}

// selectInstallCandidates returns the catalog skills that should be offered
// for installation. Skills already selected in the manifest are skipped
// unless force is set, in which case they are marked "(reinstall)". Skills
// claimed by a different repository are never offered because both agent
// targets install into the same <skills-root>/<skill-name> directory.
func selectInstallCandidates(catalog *skill.Catalog, manifest *skill.Manifest, id skill.RepoID, force bool) []installCandidate {
	var candidates []installCandidate
	for _, s := range catalog.Skills {
		owner := manifest.FindSkill(s.Name)
		if owner != nil && owner.ID.Canonical() != id.Canonical() {
			continue
		}
		if owner != nil && !force {
			continue
		}
		label := s.Name
		if s.Description != "" {
			label = fmt.Sprintf("%s — %s", s.Name, uicli.TruncateString(s.Description, 60))
		}
		if owner != nil {
			label += " (reinstall)"
		}
		candidates = append(candidates, installCandidate{name: s.Name, label: label})
	}
	return candidates
}

// browseAndInstall opens a multi-select over the repository catalog at the
// given locked version, updates the manifest, and reconciles it.
func browseAndInstall(manifest *skill.Manifest, id skill.RepoID, key, version string, force bool) error {
	catalog, err := fetchSnapshotWithSpinner(id, version)
	if err != nil {
		return err
	}
	if len(catalog.Skills) == 0 {
		return fmt.Errorf("repository %s has no skills defined", id.Canonical())
	}

	candidates := selectInstallCandidates(catalog, manifest, id, force)
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

	names := make([]string, 0, len(selectedIdxs))
	for _, idx := range selectedIdxs {
		names = append(names, candidates[idx].name)
	}
	return applySelection(manifest, id, key, version, names)
}

// applySelection records the selection in the manifest and reconciles it.
func applySelection(manifest *skill.Manifest, id skill.RepoID, key, version string, names []string) error {
	if r := manifest.FindRepo(id); r != nil {
		manifest.SetVersion(r, version)
		manifest.AddSkills(r, names)
	} else if _, err := manifest.AddRepo(key, version, names); err != nil {
		return err
	}

	targets, err := skill.Reconcile(manifest, nil, true)
	if err != nil {
		return err
	}
	reportApplied(targets, fmt.Sprintf("Installed %s from %s at %s", strings.Join(names, ", "), key, shortVersion(version)))
	return nil
}

func reportApplied(targets []string, message string) {
	if len(targets) == 0 {
		terminal.Warning("No skill directories were installed. Neither ~/.claude nor ~/.codex was found.")
		return
	}
	terminal.Successf("%s to %s", message, strings.Join(targets, ", "))
}

var skillsUpdateCmd = &cobra.Command{
	Use:   "update [repo[@version]]",
	Short: "Update one repository, or every repository to latest",
	Long: "Updates the named repository to the given version (latest when omitted), or updates every " +
		"repository to latest when no argument is given. Selected skills are preserved; an update that " +
		"would lose a selected skill fails without changing anything.",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := skill.LoadManifest()
		if err != nil {
			return fmt.Errorf("failed to load skills manifest: %w", err)
		}
		if len(manifest.Repos) == 0 {
			terminal.Warning("No skills installed.")
			return nil
		}

		var updating []*skill.RepoSpec
		spec := ""
		if len(args) > 0 {
			repoArg, verSpec := skill.SplitRepoVersion(args[0])
			id, err := skill.ParseRepo(repoArg)
			if err != nil {
				return err
			}
			r := manifest.FindRepo(id)
			if r == nil {
				return fmt.Errorf("repository %s is not in the skills manifest", id.Canonical())
			}
			updating = append(updating, r)
			spec = verSpec
		} else {
			updating = manifest.Repos
		}

		changed := false
		for _, r := range updating {
			version, err := resolveVersionWithSpinner(r.ID, spec)
			if err != nil {
				return err
			}
			if version != r.Version {
				manifest.SetVersion(r, version)
				changed = true
			} else {
				terminal.Infof("%s is already at %s.", r.Key, shortVersion(version))
			}
		}
		if !changed {
			return nil
		}

		targets, err := skill.Reconcile(manifest, nil, true)
		if err != nil {
			return err
		}
		reportApplied(targets, "Updated skills")
		return nil
	},
}

var skillsSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Reconcile agent targets with the versions locked in the manifest",
	Long: "Applies only the versions already locked in ~/.clime/skills.yaml. When every referenced " +
		"snapshot is cached, sync does not access the network.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := skill.LoadManifest()
		if err != nil {
			return fmt.Errorf("failed to load skills manifest: %w", err)
		}
		if len(manifest.Repos) == 0 {
			terminal.Warning("No skills installed.")
			return nil
		}

		// The manifest may have been edited by hand, so sync never
		// rewrites it: only agent targets are reconciled.
		targets, err := skill.Reconcile(manifest, nil, false)
		if err != nil {
			return err
		}
		reportApplied(targets, fmt.Sprintf("Synced %d skill(s)", len(manifest.SkillNames())))
		return nil
	},
}

var skillsPurgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Delete cache entries not referenced by the manifest",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := skill.LoadManifest()
		if err != nil {
			return fmt.Errorf("failed to load skills manifest: %w", err)
		}
		if err := manifest.Validate(); err != nil {
			return err
		}

		removed, err := skill.PurgeCache(manifest)
		if err != nil {
			return err
		}
		if len(removed) == 0 {
			terminal.Info("Cache is already clean.")
			return nil
		}
		for _, dir := range removed {
			terminal.Infof("Removed %s", dir)
		}
		terminal.Successf("Purged %d unreferenced cache entr(ies).", len(removed))
		return nil
	},
}

var skillsUninstallCmd = &cobra.Command{
	Use:   "uninstall [skill-name]",
	Short: "Uninstall a previously installed skill",
	Long: "Removes the skill from its repository's selection. Removing the final skill also removes " +
		"the repository entry from the manifest.",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := skill.LoadManifest()
		if err != nil {
			return fmt.Errorf("failed to load skills manifest: %w", err)
		}

		if len(args) == 0 {
			return interactiveUninstall(manifest)
		}
		return uninstallSkills(manifest, []string{args[0]})
	},
}

func uninstallSkills(manifest *skill.Manifest, names []string) error {
	for _, name := range names {
		if manifest.FindSkill(name) == nil {
			return fmt.Errorf("skill %q is not installed", name)
		}
	}
	for _, name := range names {
		manifest.RemoveSkill(name)
	}

	targets, err := skill.Reconcile(manifest, names, true)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		terminal.Warning("Neither ~/.claude nor ~/.codex was found.")
		return nil
	}
	terminal.Successf("Removed %s from %s", strings.Join(names, ", "), strings.Join(targets, ", "))
	return nil
}

func interactiveUninstall(manifest *skill.Manifest) error {
	names := manifest.SkillNames()
	if len(names) == 0 {
		terminal.Warning("No skills installed.")
		return nil
	}

	options := make([]string, len(names))
	for i, name := range names {
		r := manifest.FindSkill(name)
		options[i] = fmt.Sprintf("%s — %s@%s", name, r.Key, shortVersion(r.Version))
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

		selected := make([]string, len(selectedIdxs))
		for i, idx := range selectedIdxs {
			selected[i] = names[idx]
		}

		fmt.Println()
		return uninstallSkills(manifest, selected)
	}
}
